package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ProfileView is a profile file plus its derived links.
type ProfileView struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Default     bool             `json:"default"`
	Global      bool             `json:"global"`
	Allow       []string         `json:"allow"`
	Deny        []string         `json:"deny"`
	Mounts      []fleet.MountRef `json:"mounts"`
	Caches      []string         `json:"caches"`
	Skills      []fleet.SkillRef `json:"skills"`
	Sandboxes   []string         `json:"sandboxes"`
}

// ListProfiles returns every profile with its derived sandbox list.
func (a *App) ListProfiles(ctx context.Context) ([]ProfileView, error) {
	profiles := a.Fleet.Profiles()
	out := make([]ProfileView, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, a.profileView(p))
	}
	return out, nil
}

// GetProfileView returns one profile by slug.
func (a *App) GetProfileView(ctx context.Context, slug string) (ProfileView, error) {
	p, ok := a.Fleet.Profile(slug)
	if !ok {
		return ProfileView{}, store.ErrNotFound
	}
	return a.profileView(p), nil
}

func (a *App) profileView(p *fleet.Profile) ProfileView {
	view := ProfileView{
		Slug:        p.Slug,
		Name:        p.Label(),
		Description: p.Spec.Description,
		Default:     p.App.Default,
		Global:      p.App.Global,
		Allow:       append([]string{}, p.Spec.NetworkAllow()...),
		Deny:        append([]string{}, p.Spec.NetworkDeny()...),
		Mounts:      append([]fleet.MountRef{}, p.App.Mounts...),
		Caches:      append([]string{}, p.App.Caches...),
		Skills:      append([]fleet.SkillRef{}, p.App.Skills...),
		Sandboxes:   []string{},
	}
	for _, s := range a.sandboxesReferencingProfile(p.Slug) {
		view.Sandboxes = append(view.Sandboxes, s.Name())
	}
	sort.Strings(view.Sandboxes)
	return view
}

// CreateProfile writes a new profile directory.
func (a *App) CreateProfile(ctx context.Context, name, description string, isDefault, isGlobal bool) (ProfileView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ProfileView{}, errors.New("profile name is required")
	}
	lock, err := a.lockFleet()
	if err != nil {
		return ProfileView{}, err
	}
	defer func() { _ = lock.Release() }()
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	spec.Description = strings.TrimSpace(description)
	created, err := a.Fleet.CreateProfile(name, spec, fleet.ProfileApp{Default: isDefault, Global: isGlobal})
	if err != nil {
		return ProfileView{}, err
	}
	if isDefault {
		a.clearOtherDefaults(created.Slug)
	}
	a.Notify(TopicProfiles)
	return a.profileView(created), nil
}

// UpdateProfile rewrites a profile's label, description and flags.
func (a *App) UpdateProfile(ctx context.Context, slug, name, description string, isDefault, isGlobal bool) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return store.ErrNotFound
		}
		label := strings.TrimSpace(name)
		if label == "" {
			return errors.New("profile name is required")
		}
		p.Spec.DisplayName = label
		p.Spec.Description = strings.TrimSpace(description)
		p.App.Default = isDefault
		p.App.Global = isGlobal
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		if isDefault {
			a.clearOtherDefaults(slug)
		}
		a.Notify(TopicProfiles)
		return nil
	})
}

// DeleteProfile removes a profile and everything it applied.
func (a *App) DeleteProfile(ctx context.Context, slug string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return store.ErrNotFound
		}
		for _, s := range a.sandboxesReferencingProfile(slug) {
			if err := a.unapplyProfileFromTarget(ctx, p, s.Name()); err != nil {
				return err
			}
			s.App.Profiles = removeString(s.App.Profiles, slug)
			if s.App.OptOuts != nil {
				_ = a.Fleet.SaveSandbox(s)
			}
			_, _ = a.syncProfileMounts(ctx, s.Name(), p.App.Mounts)
			a.ReapplyCaches(ctx, s.Name())
			_ = a.Fleet.SaveSandbox(s)
		}
		if err := a.Store.ClearAppliedForProfile(ctx, slug); err != nil {
			return err
		}
		if err := a.Fleet.DeleteProfile(slug); err != nil {
			return err
		}
		a.Notify(TopicProfiles)
		a.Notify(TopicSandboxes)
		return nil
	})
}

// AddRuleToProfile appends an allow or deny pattern to a profile and
// converges every sandbox it covers.
func (a *App) AddRuleToProfile(ctx context.Context, slug, decision, pattern string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return store.ErrNotFound
		}
		decision = strings.ToLower(strings.TrimSpace(decision))
		pattern = strings.TrimSpace(pattern)
		if decision != "allow" && decision != "deny" {
			return fmt.Errorf("unknown rule decision %q", decision)
		}
		if pattern == "" {
			return errors.New("rule pattern is required")
		}
		network := ensureNetwork(&p.Spec)
		if decision == "allow" {
			if !slices.Contains(network.Allow, pattern) {
				network.Allow = append(network.Allow, pattern)
			}
		} else if !slices.Contains(network.Deny, pattern) {
			network.Deny = append(network.Deny, pattern)
		}
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		a.convergeProfileRules(ctx, p)
		a.Notify(TopicProfiles)
		return nil
	})
}

// RemoveRuleFromProfile drops a pattern from a profile and from the daemon.
func (a *App) RemoveRuleFromProfile(ctx context.Context, slug, decision, pattern string) error {
	return a.withFleetLock(func() error {
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return store.ErrNotFound
		}
		decision = strings.ToLower(strings.TrimSpace(decision))
		pattern = strings.TrimSpace(pattern)
		if network := ensureNetwork(&p.Spec); network != nil {
			if decision == "deny" {
				network.Deny = removeString(network.Deny, pattern)
			} else {
				network.Allow = removeString(network.Allow, pattern)
			}
		}
		if err := a.Fleet.SaveProfile(p); err != nil {
			return err
		}
		a.convergeProfileRules(ctx, p)
		a.Notify(TopicProfiles)
		return nil
	})
}

// ApplyProfile assigns a profile to a sandbox and compiles its rules.
func (a *App) ApplyProfile(ctx context.Context, name, slug string) error {
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		if _, err := a.Sbx.InspectSandbox(ctx, name); err != nil {
			return err
		}
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return fmt.Errorf("profile %q not found", slug)
		}
		if !slices.Contains(s.App.Profiles, slug) {
			s.App.Profiles = append(s.App.Profiles, slug)
		}
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if err := a.applyProfileToTarget(ctx, p, name); err != nil {
			return err
		}
		a.reconcileSandboxSkills(ctx, name)
		a.ReapplyCaches(ctx, name)
		a.syncProfileMounts(ctx, name, nil)
		a.Notify(TopicProfiles)
		a.Notify(TopicSandboxes)
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// UnapplyProfile removes a profile from a sandbox and its daemon rules.
func (a *App) UnapplyProfile(ctx context.Context, name, slug string) error {
	return a.withFleetLock(func() error {
		s, err := a.ensureSandboxConfig(ctx, name)
		if err != nil {
			return err
		}
		p, ok := a.Fleet.Profile(slug)
		if !ok {
			return fmt.Errorf("profile %q not found", slug)
		}
		s.App.Profiles = removeString(s.App.Profiles, slug)
		if err := a.Fleet.SaveSandbox(s); err != nil {
			return err
		}
		if err := a.unapplyProfileFromTarget(ctx, p, name); err != nil {
			return err
		}
		a.reconcileSandboxSkills(ctx, name)
		a.ReapplyCaches(ctx, name)
		a.syncProfileMounts(ctx, name, p.App.Mounts)
		a.Notify(TopicProfiles)
		a.Notify(TopicSandboxes)
		a.Notify(TopicSandbox(name))
		return nil
	})
}

// Reconcile converges every sandbox onto its files and the global profiles,
// and prunes ledger rows whose sandbox or profile is gone.
func (a *App) Reconcile(ctx context.Context) error {
	return a.reconcile(ctx, false)
}

func (a *App) reconcile(ctx context.Context, honourBackoff bool) error {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return err
	}
	exists := make(map[string]bool, len(sandboxes))
	for _, s := range sandboxes {
		exists[s.Name] = true
	}
	for _, s := range sandboxes {
		if a.isStopping(s.Name) {
			continue
		}
		cfg, hasConfig := a.Fleet.SandboxByName(s.Name)
		if hasConfig && s.Running() {
			if honourBackoff && !a.applyBackoff.ready(s.Name, time.Now()) {
				continue
			}
			report, err := a.Apply(ctx, s.Name)
			switch {
			case errors.Is(err, fleet.ErrLocked):
			case err != nil || len(report.Errors) > 0:
				a.applyBackoff.fail(s.Name, time.Now())
			default:
				a.applyBackoff.succeed(s.Name)
			}
			if err != nil {
				return err
			}
			continue
		}
		var profiles []*fleet.Profile
		if hasConfig {
			profiles = a.profilesForSandbox(cfg)
		} else {
			for _, p := range a.Fleet.Profiles() {
				if p.App.Global {
					profiles = append(profiles, p)
				}
			}
		}
		for _, p := range profiles {
			if err := a.applyProfileRules(ctx, p, s.Name); err != nil {
				return err
			}
		}
	}
	a.applyBackoff.retain(exists)
	return a.pruneLedger(ctx, exists)
}

// pruneLedger removes daemon rules and rows whose sandbox disappeared or no
// longer references the profile.
func (a *App) pruneLedger(ctx context.Context, exists map[string]bool) error {
	return a.withFleetLock(func() error {
		rows, err := a.Store.ListAllAppliedRules(ctx)
		if err != nil {
			return err
		}
		desired := map[string]map[string]bool{}
		for _, s := range a.Fleet.Sandboxes() {
			set := map[string]bool{}
			for _, p := range a.profilesForSandbox(s) {
				set[p.Slug] = true
			}
			desired[s.Name()] = set
		}
		for _, row := range rows {
			if exists[row.Sandbox] && desired[row.Sandbox][row.Profile] {
				continue
			}
			if err := a.removeAppliedRule(ctx, row); err != nil {
				return err
			}
			if err := a.Store.DeleteAppliedRule(ctx, row.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

// applyProfileToTarget compiles a profile's allow/deny lists into
// sandbox-scoped daemon rules, tracked by the ledger. The caller holds the
// fleet lock so a competing process cannot interleave its own apply.
func (a *App) applyProfileToTarget(ctx context.Context, p *fleet.Profile, target string) error {
	desired := map[string]bool{}
	for _, pattern := range p.Spec.NetworkAllow() {
		desired[ruleKey("allow", pattern)] = true
	}
	for _, pattern := range p.Spec.NetworkDeny() {
		desired[ruleKey("deny", pattern)] = true
	}
	applied, err := a.Store.ListAppliedRules(ctx, p.Slug, target)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, ar := range applied {
		key := ruleKey(ar.Decision, ar.Pattern)
		if desired[key] {
			have[key] = true
			continue
		}
		if err := a.removeAppliedRule(ctx, ar); err != nil {
			return fmt.Errorf("remove stale rule %q: %w", ar.Pattern, err)
		}
		if err := a.Store.DeleteAppliedRule(ctx, ar.ID); err != nil {
			return err
		}
	}
	missing := map[string][]string{}
	for _, pattern := range p.Spec.NetworkAllow() {
		if !have[ruleKey("allow", pattern)] {
			missing["allow"] = append(missing["allow"], pattern)
		}
	}
	for _, pattern := range p.Spec.NetworkDeny() {
		if !have[ruleKey("deny", pattern)] {
			missing["deny"] = append(missing["deny"], pattern)
		}
	}
	for _, decision := range []string{"allow", "deny"} {
		patterns := missing[decision]
		if len(patterns) == 0 {
			continue
		}
		action := sbx.PolicyAction{Action: decision, Resources: patterns}
		if target != "" {
			action.SandboxID = target
		}
		results, err := a.Sbx.ModifyPolicy(ctx, action)
		if err != nil {
			return err
		}
		for _, res := range results {
			if res.Failed() {
				return errors.New(res.Error)
			}
			for _, ref := range res.Created {
				if err := a.Store.RecordAppliedRule(ctx, p.Slug, target, ref.RuleID, ref.Resource, decision); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// unapplyProfileFromTarget removes every ledger rule of a profile on a
// sandbox. The caller holds the fleet lock.
func (a *App) unapplyProfileFromTarget(ctx context.Context, p *fleet.Profile, target string) error {
	applied, err := a.Store.ListAppliedRules(ctx, p.Slug, target)
	if err != nil {
		return err
	}
	for _, ar := range applied {
		if err := a.removeAppliedRule(ctx, ar); err != nil {
			return fmt.Errorf("remove rule %q: %w", ar.Pattern, err)
		}
		if err := a.Store.DeleteAppliedRule(ctx, ar.ID); err != nil {
			return err
		}
	}
	return a.Store.ClearAppliedForTarget(ctx, p.Slug, target)
}

func (a *App) removeAppliedRule(ctx context.Context, ar store.AppliedRule) error {
	results, err := a.Sbx.ModifyPolicy(ctx, sbx.PolicyAction{Action: "remove-id", ID: ar.RuleID, SandboxID: ar.Sandbox})
	if err != nil {
		if sbx.IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, res := range results {
		if res.Failed() && !res.NotFound() {
			return errors.New(res.Error)
		}
	}
	return nil
}

// applyProfileRules converges one profile onto one target as a complete ledger
// mutation, for callers that do not already hold the fleet lock. Apply holds
// the lock for its whole pass and calls applyProfileToTarget directly.
func (a *App) applyProfileRules(ctx context.Context, p *fleet.Profile, target string) error {
	return a.withFleetLock(func() error { return a.applyProfileToTarget(ctx, p, target) })
}

// convergeProfileRules re-applies a profile everywhere it is active. The
// caller holds the fleet lock.
func (a *App) convergeProfileRules(ctx context.Context, p *fleet.Profile) {
	if p.App.Global {
		if sandboxes, err := a.Sbx.ListSandboxes(ctx); err == nil {
			for _, s := range sandboxes {
				_ = a.applyProfileToTarget(ctx, p, s.Name)
			}
		}
		return
	}
	for _, s := range a.sandboxesReferencingProfile(p.Slug) {
		_ = a.applyProfileToTarget(ctx, p, s.Name())
	}
}

func (a *App) clearOtherDefaults(keepSlug string) {
	for _, p := range a.Fleet.Profiles() {
		if p.Slug == keepSlug || !p.App.Default {
			continue
		}
		p.App.Default = false
		_ = a.Fleet.SaveProfile(p)
	}
}

func ensureNetwork(spec *fleet.Spec) *fleet.SpecNetwork {
	if spec.Permissions == nil {
		spec.Permissions = &fleet.SpecPermission{}
	}
	if spec.Permissions.Network == nil {
		spec.Permissions.Network = &fleet.SpecNetwork{}
	}
	return spec.Permissions.Network
}

func ruleKey(decision, pattern string) string { return decision + "\x00" + pattern }

// ProfileRef is a profile as referenced by one sandbox.
type ProfileRef struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Global  bool   `json:"global"`
}

// profileRefs resolves the profiles active on a sandbox.
func (a *App) profileRefs(cfg *fleet.Sandbox) []ProfileRef {
	var profiles []*fleet.Profile
	if cfg != nil {
		profiles = a.profilesForSandbox(cfg)
	} else {
		for _, p := range a.Fleet.Profiles() {
			if p.App.Global {
				profiles = append(profiles, p)
			}
		}
	}
	out := make([]ProfileRef, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, ProfileRef{Slug: p.Slug, Name: p.Label(), Default: p.App.Default, Global: p.App.Global})
	}
	return out
}

// profileNames is the label list shown on sandbox summaries.
func (a *App) profileNames(cfg *fleet.Sandbox) []string {
	refs := a.profileRefs(cfg)
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

// runArgsOf returns the connect arguments declared by a sandbox config.
func runArgsOf(cfg *fleet.Sandbox) string {
	if cfg == nil {
		return ""
	}
	return cfg.App.RunArgs
}
