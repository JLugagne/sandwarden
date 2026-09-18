package app

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ApplyReport summarises one convergence pass over a sandbox's files.
type ApplyReport struct {
	RulesApplied  int      `json:"rules_applied"`
	MountsApplied int      `json:"mounts_applied"`
	CachesApplied int      `json:"caches_applied"`
	SkillsApplied int      `json:"skills_applied"`
	SkillsRemoved int      `json:"skills_removed"`
	Warnings      []string `json:"warnings"`
	Errors        []string `json:"errors"`
}

// WriteTo prints a human summary for the CLI and job output.
func (r ApplyReport) Print(w io.Writer) {
	fmt.Fprintf(w, "apply: %d rule(s), %d mount(s), %d cache(s), %d skill(s) mounted, %d removed\n",
		r.RulesApplied, r.MountsApplied, r.CachesApplied, r.SkillsApplied, r.SkillsRemoved)
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	for _, err := range r.Errors {
		fmt.Fprintf(w, "error: %s\n", err)
	}
}

// lockWait bounds how long a mutation waits for a competing GUI or CLI
// process before giving up with fleet.ErrLocked.
var lockWait = 5 * time.Second

// lockFleet takes the cross-process fleet lock and refreshes the in-memory
// configuration from disk, so the read-modify-write that follows starts from
// the files another process may just have written. The caller must release the
// lock when the mutation completes.
func (a *App) lockFleet() (*fleet.Lock, error) {
	lock, err := fleet.AcquireLock(a.Fleet.Dir(), lockWait)
	if err != nil {
		return nil, err
	}
	if err := a.Fleet.Reload(); err != nil {
		_ = lock.Release()
		return nil, err
	}
	return lock, nil
}

// withFleetLock runs one complete mutation under the fleet lock.
func (a *App) withFleetLock(fn func() error) error {
	lock, err := a.lockFleet()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	return fn()
}

// Apply converges a running sandbox onto its files: profile rules, profile
// and direct mounts, caches and skill mounts. Errors are per-item and never
// abort the pass. Competing GUI and CLI processes are serialized: the loser
// reports fleet.ErrLocked instead of double-applying rules.
func (a *App) Apply(ctx context.Context, name string) (ApplyReport, error) {
	lock, err := a.lockFleet()
	if err != nil {
		return ApplyReport{Warnings: []string{}, Errors: []string{}}, err
	}
	defer func() { _ = lock.Release() }()
	return a.applyLocked(ctx, name)
}

// applyLocked is the convergence pass itself; the caller holds the fleet lock,
// so every helper it calls must stay lock-free.
func (a *App) applyLocked(ctx context.Context, name string) (ApplyReport, error) {
	report := ApplyReport{Warnings: []string{}, Errors: []string{}}
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return report, fmt.Errorf("no configuration found for sandbox %q", name)
	}
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return report, err
	}
	if !info.Running() {
		return report, fmt.Errorf("sandbox %q is not running; start it to apply its configuration", name)
	}
	for _, slug := range s.App.Profiles {
		if _, ok := a.Fleet.Profile(slug); !ok {
			report.Warnings = append(report.Warnings, fmt.Sprintf("profile %q is missing", slug))
		}
	}
	for _, p := range a.profilesForSandbox(s) {
		if err := a.applyProfileToTarget(ctx, p, name); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("profile %s: %v", p.Label(), err))
			continue
		}
		report.RulesApplied++
	}
	if applied, errs := a.syncProfileMounts(ctx, name, nil); applied > 0 || len(errs) > 0 {
		report.MountsApplied += applied
		report.Errors = append(report.Errors, errs...)
	}
	if applied, errs := a.syncDirectMounts(ctx, name); applied > 0 || len(errs) > 0 {
		report.MountsApplied += applied
		report.Errors = append(report.Errors, errs...)
	}
	if applied, errs := a.ReapplyCaches(ctx, name); applied > 0 || len(errs) > 0 {
		report.CachesApplied = applied
		report.Errors = append(report.Errors, errs...)
	}
	if result, err := a.ReconcileSkills(ctx, name); err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		report.SkillsApplied = result.Applied
		report.SkillsRemoved = result.Removed
		report.Errors = append(report.Errors, result.Errors...)
	}
	report.Warnings = append(report.Warnings, a.createDrift(ctx, s)...)
	if len(report.Errors) > 0 {
		a.Notify(TopicSandbox(name))
	}
	return report, nil
}

// applyWhenReady waits for a freshly started sandbox and converges it,
// retrying while the VM comes up.
func (a *App) applyWhenReady(ctx context.Context, name string) {
	for attempt := 0; attempt < 20; attempt++ {
		if ctx.Err() != nil {
			return
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			if _, err := a.Apply(ctx, name); err == nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// profilesForSandbox resolves the profiles a sandbox references plus every
// global profile, deduplicated.
func (a *App) profilesForSandbox(s *fleet.Sandbox) []*fleet.Profile {
	seen := map[string]bool{}
	out := []*fleet.Profile{}
	for _, slug := range s.App.Profiles {
		if seen[slug] {
			continue
		}
		if p, ok := a.Fleet.Profile(slug); ok {
			seen[slug] = true
			out = append(out, p)
		}
	}
	for _, p := range a.Fleet.Profiles() {
		if seen[p.Slug] || !p.App.Global {
			continue
		}
		seen[p.Slug] = true
		out = append(out, p)
	}
	return out
}

// upsertSandboxConfig creates or replaces the config directory of a sandbox.
func (a *App) upsertSandboxConfig(name string, spec fleet.Spec, app fleet.SandboxApp) (*fleet.Sandbox, error) {
	if existing, ok := a.Fleet.SandboxByName(name); ok {
		existing.Spec = spec
		existing.Spec.DisplayName = name
		existing.App = app
		if err := a.Fleet.SaveSandbox(existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	return a.Fleet.CreateSandbox(name, spec, app)
}

// recordCreatedSandbox writes the config directory of a freshly created
// sandbox from the create request.
func (a *App) recordCreatedSandbox(name string, req CreateRequest) (*fleet.Sandbox, error) {
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	if agent := strings.TrimSpace(req.Opts.Agent); agent != "" {
		spec.Requires = &fleet.SpecRequires{Agent: agent}
	}
	env := map[string]string{}
	for _, kv := range req.Opts.Env {
		key, value, found := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !found {
			value = ""
		}
		env[key] = value
	}
	if len(env) > 0 {
		spec.Environment = &fleet.SpecEnv{Variables: env}
	}
	if deny := cleanStrings(req.Opts.DenyNetwork); len(deny) > 0 {
		spec.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{Deny: deny}}
	}
	sidecar := fleet.SandboxApp{
		Sandbox:  name,
		Create:   &fleet.SandboxCreate{CPUs: req.Opts.CPUs, Memory: req.Opts.Memory, Workspaces: cleanStrings(req.Opts.Workspaces), Clone: req.Opts.Clone, Template: req.Opts.Template, DaemonProfile: req.Opts.Profile, Publish: cleanStrings(req.Opts.Publish), Kits: a.resolveKitRefs(cleanStrings(req.Opts.Kits))},
		Profiles: cleanStrings(req.Profiles),
		Caches:   cleanStrings(req.Caches),
		Skills:   req.Skills,
		Mounts:   req.Mounts,
		RunArgs:  strings.TrimSpace(req.RunArgs),
	}
	if req.AttachCaches {
		for _, c := range a.Fleet.Caches() {
			if c.App.AutoAttaches() && c.App.IsEnabled() && !slices.Contains(sidecar.Caches, c.Slug) {
				sidecar.Caches = append(sidecar.Caches, c.Slug)
			}
		}
	}
	if p := a.defaultProfileSlug(); p != "" && !slices.Contains(sidecar.Profiles, p) {
		sidecar.Profiles = append(sidecar.Profiles, p)
	}
	return a.upsertSandboxConfig(name, spec, sidecar)
}

// CreateOptionsFromConfig rebuilds `sbx create` options from a sandbox's
// files, for recreate and export.
func (a *App) CreateOptionsFromConfig(s *fleet.Sandbox) sbx.CreateOptions {
	opts := sbx.CreateOptions{Name: s.Name(), Agent: s.Spec.Agent()}
	if c := s.App.Create; c != nil {
		opts.CPUs = c.CPUs
		opts.Memory = c.Memory
		opts.Workspaces = append([]string(nil), c.Workspaces...)
		opts.Clone = c.Clone
		opts.Template = c.Template
		opts.Profile = c.DaemonProfile
		opts.Publish = append([]string(nil), c.Publish...)
		opts.Kits = a.resolveKitRefs(c.Kits)
	}
	for key, value := range s.Spec.Env() {
		if value == "" {
			opts.Env = append(opts.Env, key)
			continue
		}
		opts.Env = append(opts.Env, key+"="+value)
	}
	sort.Strings(opts.Env)
	opts.DenyNetwork = append([]string(nil), s.Spec.NetworkDeny()...)
	return opts
}

// RecreateSandbox deletes a sandbox from the daemon (keeping its files) and
// creates it again from the config directory, then converges it.
func (a *App) RecreateSandbox(ctx context.Context, name string, w io.Writer) error {
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		return fmt.Errorf("no configuration found for sandbox %q", name)
	}
	if sandboxIncomplete(s) {
		fmt.Fprintln(w, incompleteRecreateWarning(name))
	}
	if _, err := a.Sbx.InspectSandbox(ctx, name); err == nil {
		fmt.Fprintf(w, "removing sandbox %s (its config is kept)\n", name)
		if err := a.Sbx.DeleteSandbox(ctx, name, true); err != nil {
			return err
		}
	}
	opts := a.CreateOptionsFromConfig(s)
	fmt.Fprintf(w, "creating sandbox %s from %s\n", name, s.Dir)
	if err := a.Sbx.CreateSandbox(ctx, opts, w); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	report, err := a.Apply(ctx, name)
	if err != nil {
		return err
	}
	report.Print(w)
	return nil
}

// createDrift lists the create-time fields that no longer match the daemon;
// changing them needs a recreate. CPU, memory and env are not observable
// through `sbx inspect`, so they are only reported when the file was saved
// from a create request.
func (a *App) createDrift(ctx context.Context, s *fleet.Sandbox) []string {
	info, err := a.Sbx.InspectSandbox(ctx, s.Name())
	if err != nil {
		return nil
	}
	var drift []string
	if want := strings.TrimSpace(s.Spec.Agent()); want != "" && info.AgentName() != want {
		drift = append(drift, fmt.Sprintf("agent: daemon has %q, file wants %q", info.AgentName(), want))
	}
	if c := s.App.Create; c != nil && len(c.Workspaces) > 0 {
		live := map[string]bool{}
		if strings.TrimSpace(info.Workspace) != "" {
			live[info.Workspace] = true
		}
		for _, w := range info.AdditionalWorkspaces {
			live[w.Dir] = true
		}
		for _, want := range c.Workspaces {
			if !live[strings.TrimSuffix(want, ":ro")] {
				drift = append(drift, fmt.Sprintf("workspace %q is not mounted", want))
			}
		}
	}
	if c := s.App.Create; c != nil && len(c.Kits) > 0 {
		if inspected, err := a.Sbx.InspectDetail(ctx, s.Name()); err == nil {
			have := map[string]bool{}
			for _, k := range inspected.Kits {
				have[k] = true
			}
			for _, want := range c.Kits {
				if !have[want] {
					drift = append(drift, fmt.Sprintf("kit %q is not applied", want))
				}
			}
		}
	}
	if len(drift) > 0 {
		for i := range drift {
			drift[i] = "recreate required — " + drift[i]
		}
	}
	return drift
}

// ValidateSandbox runs `sbx kit validate` over a sandbox's config directory.
func (a *App) ValidateSandbox(ctx context.Context, slug string) (KitValidation, error) {
	s, ok := a.Fleet.Sandbox(slug)
	if !ok {
		return KitValidation{}, fmt.Errorf("sandbox %q not found", slug)
	}
	output, err := a.Sbx.KitValidate(ctx, s.Dir)
	if err != nil {
		return KitValidation{OK: false, Output: err.Error()}, nil
	}
	return KitValidation{OK: true, Output: output}, nil
}

// ValidateProfile runs `sbx kit validate` over a profile's config directory.
func (a *App) ValidateProfile(ctx context.Context, slug string) (KitValidation, error) {
	p, ok := a.Fleet.Profile(slug)
	if !ok {
		return KitValidation{}, fmt.Errorf("profile %q not found", slug)
	}
	output, err := a.Sbx.KitValidate(ctx, p.Dir)
	if err != nil {
		return KitValidation{OK: false, Output: err.Error()}, nil
	}
	return KitValidation{OK: true, Output: output}, nil
}

func (a *App) defaultProfileSlug() string {
	for _, p := range a.Fleet.Profiles() {
		if p.App.Default {
			return p.Slug
		}
	}
	return ""
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// StartApplyJob converges a sandbox onto its files as a background job.
func (a *App) StartApplyJob(name, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		report, err := a.Apply(ctx, name)
		if err != nil {
			return err
		}
		report.Print(w)
		if len(report.Errors) > 0 {
			return fmt.Errorf("%d error(s) during apply", len(report.Errors))
		}
		return nil
	})
}

// StartRecreateJob deletes and recreates a sandbox from its files as a
// background job.
func (a *App) StartRecreateJob(name, jobID string) string {
	return a.Jobs.Start(a.jobContext(), jobID, func(ctx context.Context, w io.Writer) error {
		return a.RecreateSandbox(ctx, name, w)
	})
}

// CreateSandbox runs `sbx create` synchronously, writes the new sandbox's
// config directory and converges it.
func (a *App) CreateSandbox(ctx context.Context, req CreateRequest, w io.Writer) error {
	if err := a.checkCreateEnvSecrets(req); err != nil {
		return err
	}
	before := a.sandboxNameSet(ctx)
	if err := a.Sbx.CreateSandbox(ctx, req.Opts, w); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	name := strings.TrimSpace(req.Opts.Name)
	if name == "" {
		name = a.findNewSandbox(ctx, before)
	}
	if name == "" {
		fmt.Fprintln(w, "config: could not identify the new sandbox; its settings were not saved")
		return nil
	}
	if err := a.withFleetLock(func() error {
		_, err := a.recordCreatedSandbox(name, req)
		return err
	}); err != nil {
		fmt.Fprintf(w, "config: %v\n", err)
		return nil
	}
	fmt.Fprintf(w, "config: saved %s\n", a.sandboxConfigPath(name))
	report, err := a.Apply(ctx, name)
	if err != nil {
		fmt.Fprintf(w, "apply: %v\n", err)
		return nil
	}
	report.Print(w)
	return nil
}

// StartAndConverge starts a sandbox and converges it before returning.
func (a *App) StartAndConverge(ctx context.Context, name string) (ApplyReport, error) {
	if err := a.Sbx.StartSandbox(ctx, name); err != nil {
		return ApplyReport{}, err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return a.waitAndApply(ctx, name)
}

// waitAndApply waits for a sandbox to run and converges it, retrying while
// the VM comes up.
func (a *App) waitAndApply(ctx context.Context, name string) (ApplyReport, error) {
	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		if ctx.Err() != nil {
			return ApplyReport{}, ctx.Err()
		}
		if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil && info.Running() {
			report, err := a.Apply(ctx, name)
			if err == nil {
				return report, nil
			}
			lastErr = err
		} else if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ApplyReport{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("sandbox %q did not become ready", name)
	}
	return ApplyReport{}, lastErr
}

// ensureSandboxConfig returns a sandbox config, creating a minimal one marked
// incomplete from the daemon state when the sandbox was created outside
// sandwarden. Creating a missing config writes a sidecar, so locked entry
// points call it while holding the fleet lock.
func (a *App) ensureSandboxConfig(ctx context.Context, name string) (*fleet.Sandbox, error) {
	if s, ok := a.Fleet.SandboxByName(name); ok {
		return s, nil
	}
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	create := &fleet.SandboxCreate{Incomplete: true}
	if info, err := a.Sbx.InspectSandbox(ctx, name); err == nil {
		if agent := strings.TrimSpace(info.AgentName()); agent != "" {
			spec.Requires = &fleet.SpecRequires{Agent: agent}
		}
		if ws := strings.TrimSpace(info.Workspace); ws != "" {
			create.Workspaces = append(create.Workspaces, ws)
		}
		for _, w := range info.AdditionalWorkspaces {
			if dir := strings.TrimSpace(w.Dir); dir != "" {
				create.Workspaces = append(create.Workspaces, dir)
			}
		}
	}
	if detail, err := a.Sbx.InspectDetail(ctx, name); err == nil {
		create.Kits = append(create.Kits, a.resolveKitRefs(detail.Kits)...)
		if strings.TrimSpace(spec.Agent()) == "" && strings.TrimSpace(detail.Image) != "" {
			create.Template = detail.Image
		}
	}
	return a.Fleet.CreateSandbox(name, spec, fleet.SandboxApp{Sandbox: name, Create: create})
}

// sandboxIncomplete reports whether a config directory was adopted from the
// daemon without its original create parameters.
func sandboxIncomplete(s *fleet.Sandbox) bool {
	return s != nil && s.App.Create != nil && s.App.Create.Incomplete
}

// incompleteRecreateWarning explains the gap a recreate cannot close for a
// config imported without its create parameters.
func incompleteRecreateWarning(name string) string {
	return fmt.Sprintf("warning: sandbox %s has an incomplete config: its original CPU, memory and environment settings were not recoverable, so recreate will use defaults and will not restore them", name)
}

// CompleteConfigInput carries the create parameters a user records to complete
// an imported config; they cannot be read back from the daemon.
type CompleteConfigInput struct {
	CPUs   int
	Memory string
	Env    []string
}

// CompleteSandboxConfig records the create parameters of an incomplete config
// and clears its incomplete marker; a missing config directory is created
// first. Env entries are KEY=VALUE pairs and replace the recorded environment.
func (a *App) CompleteSandboxConfig(ctx context.Context, name string, input CompleteConfigInput) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if input.CPUs < 0 {
		return fmt.Errorf("cpus cannot be negative")
	}
	if err := a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		if s.App.Create == nil {
			s.App.Create = &fleet.SandboxCreate{}
		}
		s.App.Create.CPUs = input.CPUs
		s.App.Create.Memory = strings.TrimSpace(input.Memory)
		s.App.Create.Incomplete = false
		env := map[string]string{}
		for _, kv := range input.Env {
			key, value, _ := strings.Cut(kv, "=")
			if key = strings.TrimSpace(key); key != "" {
				env[key] = value
			}
		}
		if len(env) > 0 {
			s.Spec.Environment = &fleet.SpecEnv{Variables: env}
		} else {
			s.Spec.Environment = nil
		}
		return a.Fleet.SaveSandbox(s)
	}); err != nil {
		return err
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(name))
	return nil
}
