package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/sandwarden/internal/store"
)

func TestSearchFindsDiscoveredItemsAndTracksCatalogChanges(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	checkout := t.TempDir()
	skillDir := filepath.Join(checkout, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Roll out kubernetes workloads."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	registered, err := a.Store.CreateSkillStore(ctx, store.SkillStore{Name: "acme", URL: "https://example.com/acme.git", Path: checkout})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := a.Store.ReplaceSkillItems(ctx, registered.ID, []store.SkillItem{{Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"}}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	if err := a.Store.MarkSkillStoreSynced(ctx, registered.ID, "2026-09-16T00:00:00Z", ""); err != nil {
		t.Fatalf("mark synced: %v", err)
	}

	results, err := a.Search(ctx, "kubernetes")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "deploy" {
		t.Fatalf("search missed the discovered skill: %+v", results)
	}

	// A catalog change after the first search must invalidate the cached index.
	if err := a.Store.ReplaceSkillItems(ctx, registered.ID, []store.SkillItem{
		{Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"},
		{Kind: "skill", Name: "rollback", Description: "revert releases", RelPath: "skills/rollback"},
	}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	if err := a.Store.MarkSkillStoreSynced(ctx, registered.ID, "2026-09-16T00:00:01Z", ""); err != nil {
		t.Fatalf("mark synced: %v", err)
	}
	results, err = a.Search(ctx, "revert")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "rollback" {
		t.Fatalf("cached index must rebuild after a catalog change: %+v", results)
	}
}

func TestSearchIgnoresShortQueries(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	results, err := a.Search(ctx, "a")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if results != nil {
		t.Fatalf("short query must return nothing: %+v", results)
	}
}
