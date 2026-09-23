package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/search"
	"github.com/JLugagne/sandwarden/internal/store"
)

// deleteSandboxTimeout bounds one daemon delete so a sandboxd stuck on a
// wedged sandbox cannot leave the GUI waiting forever.
var deleteSandboxTimeout = 2 * time.Minute

// App coordinates the sandboxd client, the filesystem fleet, the derived
// store index, the event hub and background jobs.
type App struct {
	Sbx         *sbx.Client
	Store       *store.Store
	Fleet       *fleet.Fleet
	Hub         *Hub
	Jobs        *JobBroker
	notifier    *notifier
	stats       *statsTracker
	mu          sync.Mutex
	rootCtx     context.Context
	seenBlocked map[string]bool
	lastLogSig  string
	reconcileCh chan struct{}
	// lastState tracks the daemon status seen per sandbox so a stopped →
	// running transition converges the sidecar exactly once.
	lastState map[string]string
	// stopping tracks sandboxes with an in-flight stop. Background convergence
	// and the stats sampler must not touch them: `sbx exec` starts a stopped
	// sandbox, and the daemon keeps reporting a stopping sandbox as running until
	// the stop completes, so any probe would boot it right back up.
	stopping     map[string]bool
	leader       *fleet.Lock
	applyBackoff *backoff
	// searchMu guards the lazily built search index.
	searchMu sync.Mutex
	// searchIdx caches the BM25 index over the skill and kit catalogs.
	searchIdx *search.Index
	// searchFingerprint is the catalog digest the cached index was built from.
	searchFingerprint string
}

// New wires the application services together.
func New(client *sbx.Client, st *store.Store, fl *fleet.Fleet) *App {
	a := &App{
		Sbx:         client,
		Store:       st,
		Fleet:       fl,
		Hub:         NewHub(),
		Jobs:        NewJobBroker(nil),
		seenBlocked: map[string]bool{},
		lastState:   map[string]string{},
		stopping:    map[string]bool{},
		reconcileCh: make(chan struct{}, 1),
	}
	a.Jobs = NewJobBroker(a.Hub)
	a.notifier = newNotifier(a.publishTopic)
	a.stats = newStatsTracker()
	a.applyBackoff = newBackoff(reconcileRetryBase, reconcileRetryMax)
	return a
}

// DeleteSandbox unapplies every profile from the sandbox, removes it from the
// daemon, and drops the config directory when the caller asked to purge it.
func (a *App) DeleteSandbox(ctx context.Context, name string, force, purgeConfig bool) error {
	ctx, cancel := context.WithTimeout(ctx, deleteSandboxTimeout)
	defer cancel()

	var current *fleet.Sandbox
	if s, ok := a.Fleet.SandboxByName(name); ok {
		current = s
		for _, slug := range s.App.Profiles {
			if p, found := a.Fleet.Profile(slug); found {
				if err := a.unapplyProfileFromTarget(ctx, p, name); err != nil {
					return err
				}
			}
		}
	}
	if err := a.Sbx.DeleteSandbox(ctx, name, force); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("sandboxd did not answer the delete of %q within %s; it may be stuck, restart it with `sbx daemon restart`: %w", name, deleteSandboxTimeout, err)
		}
		return err
	}
	if err := a.Store.ClearAppliedForSandbox(ctx, name); err != nil {
		return err
	}
	if purgeConfig && current != nil {
		if err := a.Fleet.DeleteSandbox(current.Slug); err != nil {
			return err
		}
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicCaches)
	a.Notify(TopicSkills)
	return nil
}

// SetSandboxRunArgs stores the custom arguments appended after -- on connect.
func (a *App) SetSandboxRunArgs(ctx context.Context, name, args string) error {
	s, err := a.ensureSandboxConfig(ctx, name)
	if err != nil {
		return err
	}
	s.App.RunArgs = strings.TrimSpace(args)
	if err := a.Fleet.SaveSandbox(s); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return nil
}

// StartSandbox starts a sandbox's VM and converges its sidecar once it is up:
// profile rules, profile and direct mounts, caches and skill mounts do not
// survive a restart on their own.
func (a *App) StartSandbox(ctx context.Context, name string) error {
	if err := a.Sbx.StartSandbox(ctx, name); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	go a.applyWhenReady(a.jobContext(), name)
	return nil
}

// StopSandbox stops a sandbox's VM. The sandbox is flagged as stopping for the
// duration so background convergence and the stats sampler do not exec into it:
// `sbx exec` would start it again, and the daemon reports a stopping sandbox as
// running until the stop completes.
func (a *App) StopSandbox(ctx context.Context, name string) error {
	a.markStopping(name)
	defer a.clearStopping(name)
	if err := a.Sbx.StopSandbox(ctx, name); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return nil
}

// markStopping records that a stop is in flight for name.
func (a *App) markStopping(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopping == nil {
		a.stopping = map[string]bool{}
	}
	a.stopping[name] = true
}

// clearStopping forgets an in-flight stop.
func (a *App) clearStopping(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.stopping, name)
}

// isStopping reports whether a stop is in flight for name.
func (a *App) isStopping(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopping[name]
}

// CreateRequest is everything needed to create a sandbox and record its
// configuration directory.
type CreateRequest struct {
	Opts         sbx.CreateOptions
	Profiles     []string
	Caches       []string
	Skills       []fleet.SkillRef
	Mounts       []fleet.MountRef
	RunArgs      string
	AttachCaches bool
}

// StartCreateJob runs `sbx create` as a background job, writes the new
// sandbox's config directory from the request, then converges it.
// StartCreateJob runs `sbx create` as a background job, writes the new
// sandbox's config directory from the request, then converges it.
func (a *App) StartCreateJob(req CreateRequest, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		return a.CreateSandbox(ctx, req, w)
	})
}

// StartExecJob runs a command in a sandbox as a background job.
func (a *App) StartExecJob(sandbox, command, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		return a.Sbx.Exec(ctx, sandbox, []string{"sh", "-lc", command}, w)
	})
}

// StartImportJob imports host secrets through the sbx CLI as a job.
func (a *App) StartImportJob(service string, all, dryRun, force bool, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		err := a.Sbx.ImportSecrets(ctx, service, all, dryRun, force, w)
		if err == nil && !dryRun {
			a.Notify(TopicSecrets)
		}
		return err
	})
}

// ApplyPolicy runs ad-hoc policy mutations, untracked by the ledger.
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

// SandboxTraffic returns the proxy log entries of one sandbox.
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

// AddMountAt declares a bind mount in the sandbox sidecar and attaches it
// immediately when the sandbox runs.
func (a *App) AddMountAt(ctx context.Context, name, hostPath, target string, readOnly bool) error {
	hostPath = fleet.ExpandHome(hostPath)
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
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		mount := fleet.MountRef{HostPath: hostPath, TargetPath: target, ReadOnly: readOnly}
		replaced := false
		for i, m := range s.App.Mounts {
			if m.HostPath == mount.HostPath && m.TargetPath == mount.TargetPath {
				s.App.Mounts[i] = mount
				replaced = true
				break
			}
		}
		if !replaced {
			s.App.Mounts = append(s.App.Mounts, mount)
		}
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			live, err := a.Sbx.Mounts(ctx, name)
			if err != nil {
				return err
			}
			current, err := a.releaseDriftedMount(ctx, name, live, hostPath, target, readOnly)
			if err != nil {
				return err
			}
			if !current {
				if err := a.mountRef(ctx, name, mount); err != nil {
					return err
				}
			}
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// RemoveMountAt removes a declared bind mount and detaches it if attached.
func (a *App) RemoveMountAt(ctx context.Context, name, hostPath, target string) error {
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		kept := s.App.Mounts[:0]
		for _, m := range s.App.Mounts {
			if m.HostPath == hostPath && m.TargetPath == target {
				continue
			}
			kept = append(kept, m)
		}
		s.App.Mounts = kept
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			if err := a.Sbx.UnmountFolderAt(ctx, name, hostPath, target); err != nil {
				return err
			}
		}
		a.Notify(TopicSandbox(name))
		return nil
	})
}

func (a *App) jobContext() context.Context {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rootCtx != nil {
		return a.rootCtx
	}
	return context.Background()
}

func (a *App) sandboxNameSet(ctx context.Context) map[string]bool {
	set := make(map[string]bool)
	if sandboxes, err := a.Sbx.ListSandboxes(ctx); err == nil {
		for _, s := range sandboxes {
			set[s.Name] = true
		}
	}
	return set
}

func (a *App) findNewSandbox(ctx context.Context, before map[string]bool) string {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return ""
	}
	newest := ""
	var newestAt time.Time
	for _, s := range sandboxes {
		if before[s.Name] {
			continue
		}
		if s.CreatedAt == nil {
			if newest == "" {
				newest = s.Name
			}
			continue
		}
		if newest == "" || s.CreatedAt.After(newestAt) {
			newestAt = *s.CreatedAt
			newest = s.Name
		}
	}
	return newest
}
