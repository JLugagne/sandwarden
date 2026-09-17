package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/search"
	"github.com/JLugagne/sandwarden/internal/store"
)

func TestSearchFindsDiscoveredItemsAndTracksCatalogChanges(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	checkout := filepath.Join(dataHome, "sandwarden", "skill-stores", "acme")
	skillDir := filepath.Join(checkout, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Roll out kubernetes workloads."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if _, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreSkills, Name: "acme", URL: "https://example.com/acme.git"}); err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := a.Store.ReplaceSkillItems(ctx, "acme", []store.SkillItem{{Store: "acme", Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"}}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	if err := a.Store.SetStoreSync(ctx, "skill", "acme", "2026-09-16T00:00:00Z", ""); err != nil {
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
	if err := a.Store.ReplaceSkillItems(ctx, "acme", []store.SkillItem{
		{Store: "acme", Kind: "skill", Name: "deploy", Description: "ship things", RelPath: "skills/deploy"},
		{Store: "acme", Kind: "skill", Name: "rollback", Description: "revert releases", RelPath: "skills/rollback"},
	}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	if err := a.Store.SetStoreSync(ctx, "skill", "acme", "2026-09-16T00:00:01Z", ""); err != nil {
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

func TestSearchFindsStoppedSandboxByDisplayName(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	spec := fleet.NewMixin("")
	spec.DisplayName = "Frontend Box"
	spec.Description = "UI work sandbox"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	if _, err := a.Fleet.CreateSandbox("frontend-box", spec, fleet.SandboxApp{}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	results, err := a.Search(ctx, "Frontend")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Kind != search.KindSandbox {
		t.Fatalf("a stopped sandbox must still be searchable: %+v", results)
	}
	if results[0].Name != "frontend-box" || results[0].DisplayName != "Frontend Box" || results[0].Agent != "claude" {
		t.Fatalf("unexpected sandbox hit: %+v", results[0])
	}
}

func TestSearchFindsProfilesAndCaches(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	profile, err := a.Fleet.CreateProfile("Net Allow", fleet.NewMixin(""), fleet.ProfileApp{})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profile.Spec.Description = "corporate egress allowlist"
	profile.Spec.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{
		Allow: []string{"api.github.com"},
		Deny:  []string{"telemetry.example.com"},
	}}
	if err := a.Fleet.SaveProfile(profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	cache, err := a.Fleet.CreateCache("go-mod", fleet.CacheApp{HostPath: "~/.cache/go-mod"})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	cache.App.Name = "Go module cache"
	cache.App.Description = "shared downloads"
	cache.App.TargetPath = "/home/agent/go/pkg/mod"
	if err := a.Fleet.SaveCache(cache); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	spec := fleet.NewMixin("")
	spec.DisplayName = "Frontend Box"
	if _, err := a.Fleet.CreateSandbox("frontend-box", spec, fleet.SandboxApp{
		Profiles: []string{profile.Slug},
		Caches:   []string{cache.Slug},
	}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	checks := []struct {
		query string
		kind  search.Kind
		name  string
		slug  string
	}{
		{"corporate", search.KindProfile, "Net Allow", "net-allow"},
		{"api.github.com", search.KindProfile, "Net Allow", "net-allow"},
		{"telemetry.example.com", search.KindProfile, "Net Allow", "net-allow"},
		{"module", search.KindCache, "Go module cache", "go-mod"},
		{"pkg", search.KindCache, "Go module cache", "go-mod"},
		{"Frontend", search.KindSandbox, "frontend-box", "frontend-box"},
	}
	for _, check := range checks {
		results, err := a.Search(ctx, check.query)
		if err != nil {
			t.Fatalf("search %q: %v", check.query, err)
		}
		if len(results) == 0 || results[0].Kind != check.kind || results[0].Name != check.name || results[0].Slug != check.slug {
			t.Fatalf("query %q must rank the %s first: %+v", check.query, check.kind, results)
		}
	}
}

func TestSearchDropsStaleProfileHitsAfterRenameAndReload(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)

	profile, err := a.Fleet.CreateProfile("Old Relic", fleet.NewMixin(""), fleet.ProfileApp{})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	results, err := a.Search(ctx, "Relic")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Kind != search.KindProfile || results[0].Slug != profile.Slug || results[0].Name != "Old Relic" {
		t.Fatalf("profile must be searchable by label: %+v", results)
	}

	profile.Spec.DisplayName = "Egress Allowlist"
	if err := a.Fleet.SaveProfile(profile); err != nil {
		t.Fatalf("rename profile: %v", err)
	}
	if err := a.Fleet.Reload(); err != nil {
		t.Fatalf("reload fleet: %v", err)
	}

	results, err = a.Search(ctx, "Relic")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("renamed profile must not leave stale hits: %+v", results)
	}
	results, err = a.Search(ctx, "Egress")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Kind != search.KindProfile || results[0].Slug != profile.Slug || results[0].Name != "Egress Allowlist" {
		t.Fatalf("renamed profile must be searchable: %+v", results)
	}
}
