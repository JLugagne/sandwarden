package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/search"
	"github.com/JLugagne/sandwarden/internal/store"
)

// App coordinates the sandboxd client, the profile store, the event hub and
// background jobs.
type App struct {
	Sbx         *sbx.Client
	Store       *store.Store
	Hub         *Hub
	Jobs        *JobBroker
	notifier    *notifier
	stats       *statsTracker
	mu          sync.Mutex
	rootCtx     context.Context
	seenBlocked map[string]bool
	lastLogSig  string
	reconcileCh chan struct{}
	// searchMu guards the lazily built search index.
	searchMu sync.Mutex
	// searchIdx caches the BM25 index over the skill and kit catalogs.
	searchIdx *search.Index
	// searchFingerprint is the catalog digest the cached index was built from.
	searchFingerprint string
}

// New builds an App and its hub.
func New(client *sbx.Client, st *store.Store) *App {
	hub := NewHub()
	a := &App{
		Sbx:         client,
		Store:       st,
		Hub:         hub,
		Jobs:        NewJobBroker(hub),
		seenBlocked: make(map[string]bool),
		reconcileCh: make(chan struct{}, 1),
		stats:       newStatsTracker(),
	}
	a.notifier = newNotifier(a.publishTopic)
	return a
}

// DeleteSandbox unapplies every profile from the sandbox, removes it, then
// drops the local assignment rows.
// DeleteSandbox unapplies every profile from the sandbox, removes it, then
// drops the local assignment rows.
func (a *App) DeleteSandbox(ctx context.Context, name string, force bool) error {
	profiles, err := a.Store.ListProfilesForSandbox(ctx, name)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		if err := a.unapplyProfileFromTarget(ctx, p, name); err != nil {
			return err
		}
	}
	if err := a.Sbx.DeleteSandbox(ctx, name, force); err != nil {
		return err
	}
	if err := a.Store.DropSandboxAssignments(ctx, name); err != nil {
		return err
	}
	if err := a.Store.DropSandboxCaches(ctx, name); err != nil {
		return err
	}
	if err := a.Store.DropSandboxSkillItems(ctx, name); err != nil {
		return err
	}
	if err := a.Store.DropSandboxRunArgs(ctx, name); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicCaches)
	a.Notify(TopicSkills)
	return nil
}

// SetSandboxRunArgs stores the extra arguments appended after `--` to the
// sandbox's connect run command; an empty value clears it.
func (a *App) SetSandboxRunArgs(ctx context.Context, name, args string) error {
	if err := a.Store.SetRunArgs(ctx, name, args); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return nil
}

// StartSandbox starts a sandbox's VM.
// StartSandbox starts a sandbox's VM and re-attaches its desired cache mounts
// once it is up (bind mounts do not survive a restart).
func (a *App) StartSandbox(ctx context.Context, name string) error {
	if err := a.Sbx.StartSandbox(ctx, name); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	go a.reapplyCachesWhenReady(a.jobContext(), name)
	go a.reapplySkillsWhenReady(a.jobContext(), name)
	return nil
}

// StopSandbox stops a sandbox's VM.
func (a *App) StopSandbox(ctx context.Context, name string) error {
	if err := a.Sbx.StopSandbox(ctx, name); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return nil
}

// StartCreateJob runs `sbx create` as a background job and returns its id.
// StartCreateJob runs `sbx create` as a background job, optionally attaching
// the auto-attach caches to the new sandbox, and returns the job id.
func (a *App) StartCreateJob(opts sbx.CreateOptions, jobID string, attachCaches bool) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		var before map[string]bool
		if attachCaches {
			before = a.sandboxNameSet(ctx)
		}
		if err := a.Sbx.CreateSandbox(ctx, opts, w); err != nil {
			return err
		}
		a.Notify(TopicSandboxes)
		if !attachCaches {
			return nil
		}
		name := strings.TrimSpace(opts.Name)
		if name == "" {
			name = a.findNewSandbox(ctx, before)
		}
		if name == "" {
			fmt.Fprintln(w, "caches: could not identify the new sandbox; attach caches from its Caches tab")
			return nil
		}
		a.attachAutoCaches(ctx, name, w)
		return nil
	})
}

// StartExecJob runs a command in a sandbox as a background job.
func (a *App) StartExecJob(sandbox, command, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		return a.Sbx.Exec(ctx, sandbox, []string{"sh", "-lc", command}, w)
	})
}

// StartImportJob runs `sbx secret import` as a background job and refreshes the
// secret inventory once a real import succeeds.
func (a *App) StartImportJob(service string, all, dryRun, force bool, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		err := a.Sbx.ImportSecrets(ctx, service, all, dryRun, force, w)
		if err == nil && !dryRun {
			a.Notify(TopicSecrets)
		}
		return err
	})
}

// ApplyPolicy applies one or more daemon policy mutations and refreshes viewers.
func (a *App) ApplyPolicy(ctx context.Context, actions ...sbx.PolicyAction) ([]sbx.PolicyActionResult, error) {
	results, err := a.Sbx.ModifyPolicy(ctx, actions...)
	if err != nil {
		return nil, err
	}
	a.Notify(TopicTraffic)
	a.Notify(TopicPolicyRules)
	for _, action := range actions {
		if action.SandboxID != "" {
			a.Notify(TopicSandbox(action.SandboxID))
		}
	}
	var failures []error
	for _, res := range results {
		if res.Failed() {
			failures = append(failures, errors.New(res.Error))
		}
	}
	return results, errors.Join(failures...)
}

// SandboxTraffic returns the proxy host log filtered to one sandbox.
func (a *App) SandboxTraffic(ctx context.Context, name string) (sbx.PolicyLog, error) {
	log, err := a.Sbx.PolicyLog(ctx)
	if err != nil {
		return sbx.PolicyLog{}, err
	}
	var out sbx.PolicyLog
	for _, e := range log.BlockedHosts {
		if e.VMName == name {
			out.BlockedHosts = append(out.BlockedHosts, e)
		}
	}
	for _, e := range log.AllowedHosts {
		if e.VMName == name {
			out.AllowedHosts = append(out.AllowedHosts, e)
		}
	}
	return out, nil
}

func (a *App) jobContext() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rootCtx != nil {
		return a.rootCtx
	}
	return context.Background()
}

// AddMountAt bind-mounts a host path into a running sandbox at target; an
// empty target keeps the same path. Missing target directories are created.
func (a *App) AddMountAt(ctx context.Context, name, hostPath, target string, readOnly bool) error {
	hostPath = strings.TrimSpace(hostPath)
	if hostPath == "" {
		return errors.New("host path is required")
	}
	if !filepath.IsAbs(hostPath) {
		return errors.New("host path must be absolute")
	}
	target = strings.TrimSpace(target)
	if target != "" && !filepath.IsAbs(target) {
		return errors.New("target path must be absolute")
	}
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return err
	}
	if !info.Running() {
		return fmt.Errorf("sandbox %q is not running; start it before mounting", name)
	}
	if target != "" {
		_ = a.Sbx.MkdirAll(ctx, name, target)
	}
	if err := a.Sbx.MountFolderAt(ctx, name, hostPath, target, readOnly); err != nil {
		return err
	}
	a.Notify(TopicSandbox(name))
	return nil
}

// RemoveMountAt revokes a bind mount created with AddMountAt.
func (a *App) RemoveMountAt(ctx context.Context, name, hostPath, target string) error {
	if err := a.Sbx.UnmountFolderAt(ctx, name, hostPath, target); err != nil {
		return err
	}
	a.Notify(TopicSandbox(name))
	return nil
}
