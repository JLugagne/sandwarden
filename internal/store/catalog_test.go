package store

import (
	"context"
	"testing"
)

func TestCatalogFingerprintTracksCatalogChanges(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	before, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	created, err := st.CreateSkillStore(ctx, SkillStore{Name: "acme", URL: "https://example.com/acme.git", Path: t.TempDir()})
	if err != nil {
		t.Fatalf("create skill store: %v", err)
	}
	afterCreate, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if afterCreate == before {
		t.Fatal("fingerprint must change when a store is registered")
	}
	if err := st.ReplaceSkillItems(ctx, created.ID, []SkillItem{{Kind: "skill", Name: "deploy", RelPath: "skills/deploy"}}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	afterItems, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if afterItems == afterCreate {
		t.Fatal("fingerprint must change when items are discovered")
	}
	if err := st.MarkSkillStoreSynced(ctx, created.ID, "2026-09-16T00:00:00Z", ""); err != nil {
		t.Fatalf("mark synced: %v", err)
	}
	afterSync, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if afterSync == afterItems {
		t.Fatal("fingerprint must change when a store is re-synced")
	}
	if err := st.DeleteSkillStore(ctx, created.ID); err != nil {
		t.Fatalf("delete store: %v", err)
	}
	afterDelete, err := st.CatalogFingerprint(ctx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if afterDelete == afterSync {
		t.Fatal("fingerprint must change when a store is deleted")
	}
}
