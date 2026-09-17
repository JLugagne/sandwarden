package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSkillCatalogConverges(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	items := []SkillItem{
		{Store: "alpha", Kind: "skill", Name: "pdf", Description: "read pdf", RelPath: "skills/pdf"},
		{Store: "alpha", Kind: "command", Name: "review", RelPath: "commands/review.md"},
	}
	if err := st.ReplaceSkillItems(ctx, "alpha", items); err != nil {
		t.Fatalf("ReplaceSkillItems: %v", err)
	}
	got, err := st.ListSkillItems(ctx, "alpha")
	if err != nil || len(got) != 2 {
		t.Fatalf("ListSkillItems = %v, %v", got, err)
	}
	if err := st.ReplaceSkillItems(ctx, "alpha", items[:1]); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListSkillItems(ctx, "alpha")
	if len(got) != 1 || got[0].Name != "pdf" {
		t.Fatalf("convergence failed: %+v", got)
	}
	if err := st.ReplaceSkillItems(ctx, "alpha", []SkillItem{{Store: "alpha", Kind: "skill", Name: "pdf", Description: "updated"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListSkillItems(ctx, "alpha")
	if got[0].Description != "updated" {
		t.Fatalf("upsert failed: %+v", got[0])
	}
}

func TestKitCatalogAndFingerprint(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	before, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kits := []KitItem{
		{Store: "kits", Kind: "mixin", Name: "go-lint", DisplayName: "Go lint", RelPath: "go-lint", Spec: `{"name":"go-lint"}`},
	}
	if err := st.ReplaceKitItems(ctx, "kits", kits); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetKitItem(ctx, "kits", "go-lint")
	if err != nil || got.DisplayName != "Go lint" {
		t.Fatalf("GetKitItem = %+v, %v", got, err)
	}
	after, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("fingerprint must change when the catalog changes")
	}
	if _, err := st.GetKitItem(ctx, "kits", "nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLedgerAndStoreState(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	if err := st.RecordAppliedRule(ctx, "web-dev", "my-vm", "rule-1", "api.github.com", "allow"); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordAppliedRule(ctx, "web-dev", "my-vm", "rule-2", "api.github.com", "deny"); err != nil {
		t.Fatal(err)
	}
	rules, err := st.ListAppliedRules(ctx, "web-dev", "my-vm")
	if err != nil || len(rules) != 2 {
		t.Fatalf("ListAppliedRules = %+v, %v", rules, err)
	}
	if err := st.DeleteAppliedRule(ctx, rules[0].ID); err != nil {
		t.Fatal(err)
	}
	rules, _ = st.ListAppliedRules(ctx, "web-dev", "my-vm")
	if len(rules) != 1 {
		t.Fatalf("delete failed: %+v", rules)
	}
	if err := st.ClearAppliedForTarget(ctx, "web-dev", "my-vm"); err != nil {
		t.Fatal(err)
	}
	rules, _ = st.ListAppliedRules(ctx, "web-dev", "my-vm")
	if len(rules) != 0 {
		t.Fatalf("clear failed: %+v", rules)
	}
	if err := st.SetStoreSync(ctx, "kit", "kits", "2026-01-01T00:00:00Z", "boom"); err != nil {
		t.Fatal(err)
	}
	states, err := st.StoreStates(ctx, "kit")
	if err != nil || states["kits"].SyncedAt == "" || states["kits"].Error != "boom" {
		t.Fatalf("StoreStates = %+v, %v", states, err)
	}
}

func TestOpenDropsLegacySchemaDespiteForeignKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	legacy := []string{
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE profile_rules (id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE)`,
		`CREATE TABLE cache_mounts (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`,
		`CREATE TABLE profile_caches (profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE, cache_id INTEGER NOT NULL REFERENCES cache_mounts(id) ON DELETE CASCADE)`,
		`CREATE TABLE skill_items (id INTEGER PRIMARY KEY AUTOINCREMENT, store_id INTEGER NOT NULL)`,
		`INSERT INTO profiles (name) VALUES ('legacy')`,
		`INSERT INTO cache_mounts (name) VALUES ('go-mod')`,
		`INSERT INTO profile_caches (profile_id, cache_id) VALUES (1, 1)`,
	}
	for _, stmt := range legacy {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy schema (%s): %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a legacy database must succeed: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.ReplaceSkillItems(ctx, "acme", []SkillItem{{Store: "acme", Kind: "skill", Name: "deploy", RelPath: "skills/deploy"}}); err != nil {
		t.Fatalf("new schema unusable after legacy drop: %v", err)
	}
	items, err := st.ListSkillItems(ctx, "acme")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListSkillItems = %+v, %v", items, err)
	}
}

// TestOpenResetsLegacyAppliedRules pins the pre-fleet ledger migration: an
// applied_rules table using profile_id/sandbox_name cannot be queried by the
// current code, so Open must replace it with the new shape.
func TestOpenResetsLegacyAppliedRules(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-ledger.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	legacy := []string{
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE)`,
		`CREATE TABLE applied_rules (id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL, sandbox_name TEXT NOT NULL DEFAULT '', rule_id TEXT NOT NULL, pattern TEXT NOT NULL, decision TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`INSERT INTO profiles (name) VALUES ('web-dev')`,
		`INSERT INTO applied_rules (profile_id, sandbox_name, rule_id, pattern, decision, created_at) VALUES (1, 'box', 'r1', 'api.example.com', 'allow', '2026-01-01T00:00:00Z')`,
	}
	for _, stmt := range legacy {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy schema (%s): %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open on a legacy ledger must succeed: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.RecordAppliedRule(ctx, "web-dev", "box", "r2", "api.example.com", "allow"); err != nil {
		t.Fatalf("record on the reset ledger: %v", err)
	}
	rules, err := st.ListAppliedRules(ctx, "web-dev", "box")
	if err != nil || len(rules) != 1 {
		t.Fatalf("ListAppliedRules = %+v, %v", rules, err)
	}
	if err := st.ClearAppliedForSandbox(ctx, "box"); err != nil {
		t.Fatalf("ClearAppliedForSandbox: %v", err)
	}
	if rules, _ = st.ListAppliedRules(ctx, "web-dev", "box"); len(rules) != 0 {
		t.Fatalf("legacy rows survived the reset: %+v", rules)
	}
}

// TestOpenKeepsCurrentLedger guards the other side of the migration: unlike
// the rediscoverable catalogs, a ledger in the current shape must survive a
// reopen, because the applied rules cannot be rebuilt from files.
func TestOpenKeepsCurrentLedger(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.RecordAppliedRule(ctx, "web-dev", "box", "r1", "api.example.com", "allow"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()
	rules, err := again.ListAppliedRules(ctx, "web-dev", "box")
	if err != nil || len(rules) != 1 {
		t.Fatalf("the ledger must survive a reopen, got %+v, %v", rules, err)
	}
}
