package app

import (
	"context"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/store"
)

func TestCreateKitStoreRejectsSSHAuthOverHTTP(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	_, err := a.CreateKitStore(ctx, store.KitStore{
		Name: "bad",
		URL:  "https://gitlab.example.com/ai/sbx-kits.git",
		Auth: "ssh",
	})
	if err == nil || !strings.Contains(err.Error(), "ssh") {
		t.Fatalf("expected an ssh/url mismatch error, got %v", err)
	}

	stores, err := a.Store.ListKitStores(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stores) != 0 {
		t.Fatalf("expected no kit store to be persisted, got %+v", stores)
	}
}

func TestCreateSkillStoreRejectsSSHAuthOverHTTP(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	_, err := a.CreateSkillStore(ctx, store.SkillStore{
		Name: "bad",
		URL:  "https://gitlab.example.com/ai/sbx-skills.git",
		Auth: "ssh",
	})
	if err == nil || !strings.Contains(err.Error(), "ssh") {
		t.Fatalf("expected an ssh/url mismatch error, got %v", err)
	}

	stores, err := a.Store.ListSkillStores(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stores) != 0 {
		t.Fatalf("expected no skill store to be persisted, got %+v", stores)
	}
}
