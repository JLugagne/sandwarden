package store

import (
	"context"
	"errors"
	"testing"
)

func TestSkillStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	registered, err := st.CreateSkillStore(ctx, SkillStore{
		Name:        "anthropics",
		Description: "official plugins",
		URL:         "https://github.com/anthropics/skills",
		Path:        "/data/stores/anthropics",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if registered.ID == 0 || registered.CreatedAt == "" {
		t.Fatalf("unexpected record: %+v", registered)
	}

	if _, err := st.CreateSkillStore(ctx, SkillStore{Name: " ", URL: "https://x", Path: "/p"}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := st.CreateSkillStore(ctx, SkillStore{Name: "nourl", Path: "/p"}); err == nil {
		t.Fatal("expected error for empty url")
	}
	if _, err := st.CreateSkillStore(ctx, SkillStore{Name: "nopath", URL: "https://x"}); err == nil {
		t.Fatal("expected error for empty path")
	}

	if err := st.UpdateSkillStore(ctx, SkillStore{ID: registered.ID, Name: "plugins", URL: "https://github.com/anthropics/plugins", Ref: "v2", Auth: "ssh"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetSkillStore(ctx, registered.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "plugins" || got.Ref != "v2" || got.URL != "https://github.com/anthropics/plugins" || got.Auth != "ssh" {
		t.Fatalf("update not applied: %+v", got)
	}
	if got.Path != "/data/stores/anthropics" {
		t.Fatalf("checkout path should stay stable, got %q", got.Path)
	}

	if err := st.MarkSkillStoreSynced(ctx, registered.ID, "2026-01-02T03:04:05Z", "clone failed"); err != nil {
		t.Fatalf("mark synced: %v", err)
	}
	got, _ = st.GetSkillStore(ctx, registered.ID)
	if got.SyncedAt != "2026-01-02T03:04:05Z" || got.Error != "clone failed" {
		t.Fatalf("sync state not applied: %+v", got)
	}

	list, err := st.ListSkillStores(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("unexpected list: %+v (%v)", list, err)
	}

	if err := st.DeleteSkillStore(ctx, registered.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetSkillStore(ctx, registered.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestReplaceSkillItemsKeepsIDsAndCascades(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	registered, err := st.CreateSkillStore(ctx, SkillStore{Name: "store", URL: "https://example.com/repo", Path: "/data/store"})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := st.ReplaceSkillItems(ctx, registered.ID, []SkillItem{
		{Kind: "skill", Name: "alpha", RelPath: "skills/alpha", Description: "first"},
		{Kind: "command", Name: "beta", RelPath: "commands/beta.md"},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	items, err := st.ListSkillItems(ctx, registered.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("unexpected items: %+v (%v)", items, err)
	}
	alpha := skillItemNamed(t, items, "skill", "alpha")
	if alpha.StoreName != "store" || alpha.Description != "first" {
		t.Fatalf("unexpected alpha: %+v", alpha)
	}

	profile, _ := st.CreateProfile(ctx, "dev", "", false, false)
	if err := st.AddProfileSkillItem(ctx, profile.ID, alpha.ID); err != nil {
		t.Fatalf("select in profile: %v", err)
	}
	if err := st.AddProfileSkillItem(ctx, profile.ID, alpha.ID); err != nil {
		t.Fatalf("re-select should be idempotent: %v", err)
	}
	if err := st.AddSandboxSkillItem(ctx, "box", alpha.ID); err != nil {
		t.Fatalf("attach to sandbox: %v", err)
	}

	// Refresh: alpha updated, beta dropped, gamma added.
	if err := st.ReplaceSkillItems(ctx, registered.ID, []SkillItem{
		{Kind: "skill", Name: "alpha", RelPath: "skills/alpha", Description: "updated"},
		{Kind: "skill", Name: "gamma", RelPath: "skills/gamma"},
	}); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	items, _ = st.ListSkillItems(ctx, registered.ID)
	if len(items) != 2 {
		t.Fatalf("expected 2 items after refresh, got %+v", items)
	}
	var refreshed SkillItem
	for _, item := range items {
		if item.Name == "alpha" {
			refreshed = item
		}
		if item.Name == "beta" {
			t.Fatalf("beta should be removed: %+v", item)
		}
	}
	if refreshed.ID != alpha.ID || refreshed.Description != "updated" {
		t.Fatalf("alpha should keep its id and take the new description: %+v", refreshed)
	}

	selected, err := st.ListProfileSkillItems(ctx, profile.ID)
	if err != nil || len(selected) != 1 || selected[0].ID != alpha.ID {
		t.Fatalf("profile selection should survive a refresh: %+v (%v)", selected, err)
	}
	attached, err := st.ListSandboxSkillItems(ctx, "box")
	if err != nil || len(attached) != 1 || attached[0].ID != alpha.ID {
		t.Fatalf("sandbox attachment should survive a refresh: %+v (%v)", attached, err)
	}

	if err := st.DeleteSkillStore(ctx, registered.ID); err != nil {
		t.Fatalf("delete store: %v", err)
	}
	if all, _ := st.ListAllSkillItems(ctx); len(all) != 0 {
		t.Fatalf("items should cascade on store delete: %+v", all)
	}
	if selected, _ := st.ListProfileSkillItems(ctx, profile.ID); len(selected) != 0 {
		t.Fatalf("profile selections should cascade: %+v", selected)
	}
	if attached, _ := st.ListSandboxSkillItems(ctx, "box"); len(attached) != 0 {
		t.Fatalf("sandbox attachments should cascade: %+v", attached)
	}
}

func TestAllProfileAndSandboxSkillItems(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	registered, _ := st.CreateSkillStore(ctx, SkillStore{Name: "store", URL: "https://example.com/repo", Path: "/data/store"})
	if err := st.ReplaceSkillItems(ctx, registered.ID, []SkillItem{
		{Kind: "skill", Name: "alpha", RelPath: "skills/alpha"},
		{Kind: "command", Name: "beta", RelPath: "commands/beta.md"},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	items, _ := st.ListSkillItems(ctx, registered.ID)
	first := skillItemNamed(t, items, "skill", "alpha")
	second := skillItemNamed(t, items, "command", "beta")

	p1, _ := st.CreateProfile(ctx, "p1", "", false, false)
	p2, _ := st.CreateProfile(ctx, "p2", "", false, false)
	_ = st.AddProfileSkillItem(ctx, p1.ID, first.ID)
	_ = st.AddProfileSkillItem(ctx, p2.ID, first.ID)
	_ = st.AddProfileSkillItem(ctx, p2.ID, second.ID)
	_ = st.AddSandboxSkillItem(ctx, "box-a", second.ID)

	byProfile, err := st.AllProfileSkillItems(ctx)
	if err != nil || len(byProfile[p1.ID]) != 1 || len(byProfile[p2.ID]) != 2 {
		t.Fatalf("unexpected profile items: %+v (%v)", byProfile, err)
	}
	bySandbox, err := st.AllSandboxSkillItems(ctx)
	if err != nil || len(bySandbox["box-a"]) != 1 || bySandbox["box-a"][0].Name != "beta" {
		t.Fatalf("unexpected sandbox items: %+v (%v)", bySandbox, err)
	}

	if err := st.RemoveProfileSkillItem(ctx, p2.ID, first.ID); err != nil {
		t.Fatalf("remove profile item: %v", err)
	}
	byProfile, _ = st.AllProfileSkillItems(ctx)
	if len(byProfile[p2.ID]) != 1 {
		t.Fatalf("expected 1 item after removal, got %+v", byProfile[p2.ID])
	}

	if err := st.DropSandboxSkillItems(ctx, "box-a"); err != nil {
		t.Fatalf("drop sandbox items: %v", err)
	}
	if attached, _ := st.ListSandboxSkillItems(ctx, "box-a"); len(attached) != 0 {
		t.Fatalf("expected no attachments after drop, got %+v", attached)
	}
}

// skillItemNamed returns one catalog item by (kind, name).
func skillItemNamed(t *testing.T, items []SkillItem, kind, name string) SkillItem {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			return item
		}
	}
	t.Fatalf("item %s/%s not found in %+v", kind, name, items)
	return SkillItem{}
}
