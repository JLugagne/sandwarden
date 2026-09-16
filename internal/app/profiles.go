package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// ProfileView is a profile with its rules and the sandboxes it is assigned to.
type ProfileView struct {
	store.Profile
	Rules     []store.Rule      `json:"rules"`
	Items     []store.SkillItem `json:"items"`
	Sandboxes []string          `json:"sandboxes"`
}

// ListProfiles returns every profile with rules and assignments loaded.
func (a *App) ListProfiles(ctx context.Context) ([]ProfileView, error) {
	profiles, err := a.Store.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	rulesByProfile, err := a.Store.ListAllRules(ctx)
	if err != nil {
		return nil, err
	}
	itemsByProfile, err := a.Store.AllProfileSkillItems(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProfileView, 0, len(profiles))
	for _, p := range profiles {
		view := ProfileView{Profile: p, Rules: rulesByProfile[p.ID], Items: itemsByProfile[p.ID]}
		sandboxes, err := a.Store.SandboxesForProfile(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		view.Sandboxes = sandboxes
		out = append(out, view)
	}
	return out, nil
}

// GetProfileView returns one profile with rules and assignments loaded.
func (a *App) GetProfileView(ctx context.Context, id int64) (ProfileView, error) {
	p, err := a.Store.GetProfile(ctx, id)
	if err != nil {
		return ProfileView{}, err
	}
	rules, err := a.Store.ListRules(ctx, id)
	if err != nil {
		return ProfileView{}, err
	}
	items, err := a.Store.ListProfileSkillItems(ctx, id)
	if err != nil {
		return ProfileView{}, err
	}
	sandboxes, err := a.Store.SandboxesForProfile(ctx, id)
	if err != nil {
		return ProfileView{}, err
	}
	return ProfileView{Profile: p, Rules: rules, Items: items, Sandboxes: sandboxes}, nil
}

// CreateProfile creates a profile and pushes it globally when flagged global.
func (a *App) CreateProfile(ctx context.Context, name, description string, isDefault, isGlobal bool) (store.Profile, error) {
	p, err := a.Store.CreateProfile(ctx, name, description, isDefault, isGlobal)
	if err != nil {
		return store.Profile{}, err
	}
	if p.IsGlobal {
		if err := a.applyProfileToTarget(ctx, p, ""); err != nil {
			return store.Profile{}, err
		}
	}
	a.Notify(TopicProfiles)
	return p, nil
}

// UpdateProfile rewrites a profile, moving its compiled rules to match any
// change in global/default scope.
func (a *App) UpdateProfile(ctx context.Context, id int64, name, description string, isDefault, isGlobal bool) error {
	old, err := a.Store.GetProfile(ctx, id)
	if err != nil {
		return err
	}
	targets, err := a.targetsForProfile(ctx, old)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := a.unapplyProfileFromTarget(ctx, old, target); err != nil {
			return err
		}
	}
	if err := a.Store.UpdateProfile(ctx, id, name, description, isDefault, isGlobal); err != nil {
		return err
	}
	updated, err := a.Store.GetProfile(ctx, id)
	if err != nil {
		return err
	}
	if err := a.reapplyProfile(ctx, updated); err != nil {
		return err
	}
	a.reconcileAllSkills(ctx)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	return nil
}

// DeleteProfile unapplies a profile everywhere, then deletes it.
func (a *App) DeleteProfile(ctx context.Context, id int64) error {
	old, err := a.Store.GetProfile(ctx, id)
	if err != nil {
		return err
	}
	targets, err := a.targetsForProfile(ctx, old)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := a.unapplyProfileFromTarget(ctx, old, target); err != nil {
			return err
		}
	}
	if err := a.Store.DeleteProfile(ctx, id); err != nil {
		return err
	}
	for _, target := range targets {
		if target != "" {
			a.reconcileSandboxSkills(ctx, target)
		}
	}
	a.Notify(TopicProfiles)
	a.Notify(TopicSkills)
	a.Notify(TopicSandboxes)
	return nil
}

// AddRuleToProfile adds a pattern to a profile and re-pushes it to every target.
func (a *App) AddRuleToProfile(ctx context.Context, profileID int64, decision, pattern string) (store.Rule, error) {
	rule, err := a.Store.AddRule(ctx, profileID, decision, pattern)
	if err != nil {
		return store.Rule{}, err
	}
	p, err := a.Store.GetProfile(ctx, profileID)
	if err != nil {
		return store.Rule{}, err
	}
	if err := a.reapplyProfile(ctx, p); err != nil {
		return store.Rule{}, err
	}
	a.Notify(TopicProfiles)
	return rule, nil
}

// RemoveRuleFromProfile removes a rule and reconciles every target.
func (a *App) RemoveRuleFromProfile(ctx context.Context, profileID, ruleID int64) error {
	if err := a.Store.RemoveRule(ctx, ruleID); err != nil {
		return err
	}
	p, err := a.Store.GetProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if err := a.reapplyProfile(ctx, p); err != nil {
		return err
	}
	a.Notify(TopicProfiles)
	return nil
}

// ApplyProfile assigns a profile to a sandbox and compiles its rules into
// sandbox-scoped sbx rules.
func (a *App) ApplyProfile(ctx context.Context, sandbox string, profileID int64) error {
	if _, err := a.Sbx.InspectSandbox(ctx, sandbox); err != nil {
		return err
	}
	p, err := a.Store.GetProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if err := a.Store.AssignProfile(ctx, sandbox, profileID); err != nil {
		return err
	}
	if p.IsDefault {
		if err := a.Store.ClearOptOut(ctx, sandbox); err != nil {
			return err
		}
	}
	if err := a.applyProfileToTarget(ctx, p, sandbox); err != nil {
		return err
	}
	a.reconcileSandboxSkills(ctx, sandbox)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// UnapplyProfile removes a profile's rules from a sandbox and drops the
// assignment. Unassigning the default profile records an opt-out so reconcile
// will not re-add it.
func (a *App) UnapplyProfile(ctx context.Context, sandbox string, profileID int64) error {
	p, err := a.Store.GetProfile(ctx, profileID)
	if err != nil {
		return err
	}
	if err := a.unapplyProfileFromTarget(ctx, p, sandbox); err != nil {
		return err
	}
	if err := a.Store.UnassignProfile(ctx, sandbox, profileID); err != nil {
		return err
	}
	if p.IsDefault {
		if err := a.Store.OptOutDefault(ctx, sandbox); err != nil {
			return err
		}
	}
	a.reconcileSandboxSkills(ctx, sandbox)
	a.Notify(TopicProfiles)
	a.Notify(TopicSandboxes)
	a.Notify(TopicSandbox(sandbox))
	return nil
}

// Reconcile re-applies global and assigned profiles, prunes assignments for
// sandboxes that no longer exist, applies the default profile to sandboxes that
// never opted out, and converges the skill mounts of every running sandbox.
func (a *App) Reconcile(ctx context.Context) error {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return err
	}
	exists := make(map[string]bool, len(sandboxes))
	for _, s := range sandboxes {
		exists[s.Name] = true
	}

	globals, err := a.Store.GlobalProfiles(ctx)
	if err != nil {
		return err
	}
	for _, p := range globals {
		if err := a.applyProfileToTarget(ctx, p, ""); err != nil {
			return err
		}
	}

	assignments, err := a.Store.AllAssignments(ctx)
	if err != nil {
		return err
	}
	for name, ids := range assignments {
		if !exists[name] {
			if err := a.Store.DropSandboxAssignments(ctx, name); err != nil {
				return err
			}
			continue
		}
		for _, id := range ids {
			p, err := a.Store.GetProfile(ctx, id)
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := a.applyProfileToTarget(ctx, p, name); err != nil {
				return err
			}
		}
	}

	optedOut, err := a.Store.OptedOutSandboxes(ctx)
	if err != nil {
		return err
	}
	if def, err := a.Store.DefaultProfile(ctx); err == nil {
		for _, s := range sandboxes {
			if optedOut[s.Name] || containsID(assignments[s.Name], def.ID) {
				continue
			}
			if err := a.Store.AssignProfile(ctx, s.Name, def.ID); err != nil {
				return err
			}
			if err := a.applyProfileToTarget(ctx, def, s.Name); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	attachments, err := a.Store.AllSandboxSkillItems(ctx)
	if err != nil {
		return err
	}
	for name := range attachments {
		if !exists[name] {
			if err := a.Store.DropSandboxSkillItems(ctx, name); err != nil {
				return err
			}
		}
	}
	selections, err := a.Store.AllProfileSkillItems(ctx)
	if err != nil {
		return err
	}
	if len(attachments) > 0 || len(selections) > 0 {
		for _, s := range sandboxes {
			if s.Running() {
				a.reconcileSandboxSkills(ctx, s.Name)
			}
		}
	}
	return nil
}

func (a *App) targetsForProfile(ctx context.Context, p store.Profile) ([]string, error) {
	var targets []string
	if p.IsGlobal {
		targets = append(targets, "")
	}
	sandboxes, err := a.Store.SandboxesForProfile(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return append(targets, sandboxes...), nil
}

func (a *App) reapplyProfile(ctx context.Context, p store.Profile) error {
	targets, err := a.targetsForProfile(ctx, p)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := a.applyProfileToTarget(ctx, p, target); err != nil {
			return err
		}
	}
	return nil
}

// applyProfileToTarget converges sbx rules for one profile/target pair onto the
// profile's current rules: it removes ledger rules that are no longer wanted and
// creates any that are missing. target "" means global.
func (a *App) applyProfileToTarget(ctx context.Context, p store.Profile, target string) error {
	rules, err := a.Store.ListRules(ctx, p.ID)
	if err != nil {
		return err
	}
	applied, err := a.Store.ListAppliedRules(ctx, p.ID, target)
	if err != nil {
		return err
	}

	desired := make(map[string]store.Rule, len(rules))
	for _, r := range rules {
		desired[ruleKey(r.Decision, r.Pattern)] = r
	}

	have := make(map[string]bool, len(rules))
	for _, ar := range applied {
		if _, ok := desired[ruleKey(ar.Decision, ar.Pattern)]; ok {
			have[ruleKey(ar.Decision, ar.Pattern)] = true
			continue
		}
		if err := a.removeAppliedRule(ctx, ar); err != nil {
			return fmt.Errorf("remove stale rule %q: %w", ar.Pattern, err)
		}
		if err := a.Store.DeleteAppliedRule(ctx, ar.ID); err != nil {
			return err
		}
	}

	var allowMissing, denyMissing []string
	for _, r := range rules {
		if have[ruleKey(r.Decision, r.Pattern)] {
			continue
		}
		if r.Decision == "deny" {
			denyMissing = append(denyMissing, r.Pattern)
		} else {
			allowMissing = append(allowMissing, r.Pattern)
		}
	}

	groups := []struct {
		action   string
		patterns []string
	}{{"allow", allowMissing}, {"deny", denyMissing}}
	for _, g := range groups {
		if len(g.patterns) == 0 {
			continue
		}
		action := sbx.PolicyAction{Action: g.action, Resources: g.patterns}
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
			refs := append(append([]sbx.PolicyRuleRef{}, res.Created...), res.Existing...)
			for _, ref := range refs {
				if err := a.Store.RecordAppliedRule(ctx, p.ID, target, ref.RuleID, ref.Resource, g.action); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// unapplyProfileFromTarget removes every ledger rule for a profile/target pair.
func (a *App) unapplyProfileFromTarget(ctx context.Context, p store.Profile, target string) error {
	applied, err := a.Store.ListAppliedRules(ctx, p.ID, target)
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
	return a.Store.ClearAppliedForTarget(ctx, p.ID, target)
}

func ruleKey(decision, pattern string) string { return decision + "\x00" + pattern }

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// removeAppliedRule deletes one ledger-recorded sbx rule, scoping the removal
// to the sandbox the rule was created in. A rule that is already gone counts
// as removed.
func (a *App) removeAppliedRule(ctx context.Context, ar store.AppliedRule) error {
	results, err := a.Sbx.ModifyPolicy(ctx, sbx.PolicyAction{Action: "remove-id", ID: ar.RuleID, SandboxID: ar.SandboxName})
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
