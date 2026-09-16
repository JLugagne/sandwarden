package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestProfileLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, err := st.CreateProfile(ctx, "dev", "dev endpoints", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if p.ID == 0 || p.Name != "dev" || p.IsDefault {
		t.Fatalf("unexpected profile: %+v", p)
	}

	if _, err := st.CreateProfile(ctx, "  ", "", false, false); err == nil {
		t.Fatal("expected error for empty name")
	}

	if err := st.UpdateProfile(ctx, p.ID, "dev2", "renamed", true, true); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "dev2" || !got.IsDefault || !got.IsGlobal {
		t.Fatalf("update not applied: %+v", got)
	}

	if err := st.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetProfile(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSingleDefaultProfile(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first, err := st.CreateProfile(ctx, "first", "", true, false)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := st.CreateProfile(ctx, "second", "", true, false)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	def, err := st.DefaultProfile(ctx)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	if def.ID != second.ID {
		t.Fatalf("expected second (%d) to be default, got %d", second.ID, def.ID)
	}
	reloaded, err := st.GetProfile(ctx, first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if reloaded.IsDefault {
		t.Fatal("first profile should no longer be default")
	}
}

func TestAddRuleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, err := st.CreateProfile(ctx, "p", "", false, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	r1, err := st.AddRule(ctx, p.ID, "allow", "example.com")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	r2, err := st.AddRule(ctx, p.ID, "ALLOW", " example.com ")
	if err != nil {
		t.Fatalf("add again: %v", err)
	}
	if r1.ID != r2.ID {
		t.Fatalf("expected same rule id, got %d and %d", r1.ID, r2.ID)
	}
	rules, err := st.ListRules(ctx, p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if _, err := st.AddRule(ctx, p.ID, "bogus", "x.com"); err == nil {
		t.Fatal("expected invalid decision error")
	}
}

func TestAssignmentsManyToMany(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p1, _ := st.CreateProfile(ctx, "p1", "", false, false)
	p2, _ := st.CreateProfile(ctx, "p2", "", false, false)

	if err := st.AssignProfile(ctx, "sbx-a", p1.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := st.AssignProfile(ctx, "sbx-a", p2.ID); err != nil {
		t.Fatalf("assign2: %v", err)
	}
	if err := st.AssignProfile(ctx, "sbx-b", p1.ID); err != nil {
		t.Fatalf("assign3: %v", err)
	}
	if err := st.AssignProfile(ctx, "sbx-a", p1.ID); err != nil {
		t.Fatalf("re-assign should be idempotent: %v", err)
	}

	profiles, err := st.ListProfilesForSandbox(ctx, "sbx-a")
	if err != nil {
		t.Fatalf("list for sandbox: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles for sbx-a, got %d", len(profiles))
	}

	sandboxes, err := st.SandboxesForProfile(ctx, p1.ID)
	if err != nil {
		t.Fatalf("sandboxes for profile: %v", err)
	}
	if len(sandboxes) != 2 {
		t.Fatalf("expected p1 on 2 sandboxes, got %d", len(sandboxes))
	}

	if err := st.UnassignProfile(ctx, "sbx-a", p2.ID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	profiles, _ = st.ListProfilesForSandbox(ctx, "sbx-a")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile after unassign, got %d", len(profiles))
	}
}

func TestAppliedRuleLedger(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, _ := st.CreateProfile(ctx, "p", "", false, false)
	if err := st.RecordAppliedRule(ctx, p.ID, "sbx-a", "rule-1", "example.com", "allow"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.RecordAppliedRule(ctx, p.ID, "", "rule-2", "global.com", "deny"); err != nil {
		t.Fatalf("record global: %v", err)
	}

	rows, err := st.ListAppliedRules(ctx, p.ID, "sbx-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].RuleID != "rule-1" {
		t.Fatalf("unexpected rows: %+v", rows)
	}

	owned, err := st.OwnedRuleIDs(ctx)
	if err != nil {
		t.Fatalf("owned: %v", err)
	}
	if !owned["rule-1"] || !owned["rule-2"] || len(owned) != 2 {
		t.Fatalf("unexpected owned set: %+v", owned)
	}

	if err := st.ClearAppliedForTarget(ctx, p.ID, "sbx-a"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rows, _ = st.ListAppliedRules(ctx, p.ID, "sbx-a")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after clear, got %d", len(rows))
	}
}

func TestDefaultOptOut(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.OptOutDefault(ctx, "sbx-a"); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	opted, err := st.OptedOutSandboxes(ctx)
	if err != nil {
		t.Fatalf("opted out: %v", err)
	}
	if !opted["sbx-a"] {
		t.Fatal("sbx-a should be opted out")
	}
	if err := st.ClearOptOut(ctx, "sbx-a"); err != nil {
		t.Fatalf("clear opt out: %v", err)
	}
	opted, _ = st.OptedOutSandboxes(ctx)
	if opted["sbx-a"] {
		t.Fatal("sbx-a should no longer be opted out")
	}
}

func TestDeleteProfileCascades(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	p, _ := st.CreateProfile(ctx, "p", "", false, false)
	if _, err := st.AddRule(ctx, p.ID, "allow", "example.com"); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	if err := st.AssignProfile(ctx, "sbx-a", p.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := st.RecordAppliedRule(ctx, p.ID, "sbx-a", "rule-1", "example.com", "allow"); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := st.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, _ := st.ListRules(ctx, p.ID)
	if len(rules) != 0 {
		t.Fatalf("expected rules cascaded, got %d", len(rules))
	}
	sandboxes, _ := st.SandboxesForProfile(ctx, p.ID)
	if len(sandboxes) != 0 {
		t.Fatalf("expected assignments cascaded, got %d", len(sandboxes))
	}
	owned, _ := st.OwnedRuleIDs(ctx)
	if len(owned) != 0 {
		t.Fatalf("expected ledger cascaded, got %+v", owned)
	}
}

func TestCacheMountLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	c, err := st.CreateCacheMount(ctx, CacheMount{Name: "go-mod", HostPath: "/host/go/pkg/mod", TargetPath: "/home/agent/go/pkg/mod", AutoAttach: true, Enabled: true})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	if c.ID == 0 || !c.AutoAttach || !c.Enabled || c.ReadOnly {
		t.Fatalf("unexpected cache: %+v", c)
	}

	if _, err := st.CreateCacheMount(ctx, CacheMount{Name: "  ", HostPath: "/h", TargetPath: "/t"}); err == nil {
		t.Fatal("expected error for empty name")
	}

	if err := st.UpdateCacheMount(ctx, CacheMount{ID: c.ID, Name: "go-mod", HostPath: "/host/new", TargetPath: "/home/agent/go/pkg/mod", Enabled: true}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetCacheMount(ctx, c.ID)
	if err != nil || got.HostPath != "/host/new" || got.AutoAttach {
		t.Fatalf("update not applied: %+v (%v)", got, err)
	}

	auto, err := st.AutoAttachCaches(ctx)
	if err != nil || len(auto) != 0 {
		t.Fatalf("auto attach should be empty after update: %+v (%v)", auto, err)
	}

	if err := st.DeleteCacheMount(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetCacheMount(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCacheAssignmentsCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	c, err := st.CreateCacheMount(ctx, CacheMount{Name: "npm", HostPath: "/host/npm", TargetPath: "/home/agent/.npm", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.AssignCache(ctx, "box", c.ID); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := st.AssignCache(ctx, "box", c.ID); err != nil {
		t.Fatalf("re-assign should be idempotent: %v", err)
	}

	list, err := st.ListCachesForSandbox(ctx, "box")
	if err != nil || len(list) != 1 || list[0].Name != "npm" {
		t.Fatalf("unexpected list: %+v (%v)", list, err)
	}

	assignments, err := st.AllCacheAssignments(ctx)
	if err != nil || len(assignments[c.ID]) != 1 || assignments[c.ID][0] != "box" {
		t.Fatalf("unexpected assignments: %+v (%v)", assignments, err)
	}

	if err := st.DropSandboxCaches(ctx, "box"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	list, _ = st.ListCachesForSandbox(ctx, "box")
	if len(list) != 0 {
		t.Fatalf("expected no caches after drop, got %+v", list)
	}

	if err := st.AssignCache(ctx, "box", c.ID); err != nil {
		t.Fatalf("re-assign: %v", err)
	}
	if err := st.DeleteCacheMount(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	assignments, _ = st.AllCacheAssignments(ctx)
	if len(assignments) != 0 {
		t.Fatalf("assignments should cascade on cache delete, got %+v", assignments)
	}
}
