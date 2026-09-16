package store

import (
	"context"
	"errors"
	"testing"
)

func TestKitStoreLifecycleAndCatalog(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	registered, err := st.CreateKitStore(ctx, KitStore{
		Name:        "contrib",
		Description: "community kits",
		URL:         "https://github.com/docker/sbx-kits-contrib",
		Path:        "/data/kits/contrib",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if registered.ID == 0 || registered.CreatedAt == "" {
		t.Fatalf("unexpected record: %+v", registered)
	}

	if _, err := st.CreateKitStore(ctx, KitStore{Name: " ", URL: "https://x", Path: "/p"}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if _, err := st.CreateKitStore(ctx, KitStore{Name: "nourl", Path: "/p"}); err == nil {
		t.Fatal("expected error for empty url")
	}
	if _, err := st.CreateKitStore(ctx, KitStore{Name: "nopath", URL: "https://x"}); err == nil {
		t.Fatal("expected error for empty path")
	}

	if err := st.UpdateKitStore(ctx, KitStore{ID: registered.ID, Name: "sbx-kits", URL: "https://github.com/docker/sbx-kits-contrib", Ref: "v1", Auth: "ssh"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetKitStore(ctx, registered.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "sbx-kits" || got.Ref != "v1" || got.Auth != "ssh" {
		t.Fatalf("unexpected updated row: %+v", got)
	}

	if err := st.ReplaceKitItems(ctx, registered.ID, []KitItem{
		{Kind: "mixin", Name: "amp", DisplayName: "Amp", Description: "d", Version: "1.0.0", Image: "docker.io/sbx/amp-image:latest", RequiresAgent: "claude", RelPath: "amp", Spec: `{"name":"amp"}`},
		{Kind: "sandbox", Name: "kiro", RelPath: "kiro"},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	items, err := st.ListKitItems(ctx, registered.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %+v", items)
	}
	if items[0].Name != "amp" || items[0].StoreName != "sbx-kits" || items[0].RequiresAgent != "claude" || items[0].Spec != `{"name":"amp"}` {
		t.Fatalf("unexpected first item: %+v", items[0])
	}

	item, err := st.GetKitItem(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if item.RelPath != "amp" {
		t.Fatalf("unexpected item: %+v", item)
	}

	if err := st.ReplaceKitItems(ctx, registered.ID, []KitItem{{Kind: "mixin", Name: "amp", RelPath: "amp"}}); err != nil {
		t.Fatalf("converge: %v", err)
	}
	items, err = st.ListKitItems(ctx, registered.ID)
	if err != nil {
		t.Fatalf("list after converge: %v", err)
	}
	if len(items) != 1 || items[0].Name != "amp" || items[0].ID != item.ID {
		t.Fatalf("expected amp to survive with its id, got %+v", items)
	}

	if err := st.DeleteKitStore(ctx, registered.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetKitStore(ctx, registered.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := st.ListKitItems(ctx, registered.ID); err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	all, err := st.ListAllKitItems(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("cascade must remove items, got %+v", all)
	}
}
