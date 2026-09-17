package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newSkillSourceRepo builds a local repository holding one skill, so the
// checkout path can be exercised offline.
func newSkillSourceRepo(t *testing.T, name, description string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	skillDir := filepath.Join(dir, "skills", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	meta := "---\ndescription: " + description + "\n---\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(meta), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := worktree.Commit("seed", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestStartRepopulatesCatalogsFromFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	src := newSkillSourceRepo(t, "pdf", "Fill in PDF forms.")
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreSkills, Name: "acme", URL: "file://" + src})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	a.Start(ctx)
	waitForTopic(t, events, TopicSkills)

	items, err := a.Store.ListSkillItems(ctx, reg.Slug)
	if err != nil {
		t.Fatalf("list skill items: %v", err)
	}
	if len(items) != 1 || items[0].Name != "pdf" || items[0].Description != "Fill in PDF forms." {
		t.Fatalf("catalog not repopulated: %+v", items)
	}
	states, err := a.Store.StoreStates(ctx, string(fleet.StoreSkills))
	if err != nil {
		t.Fatalf("store states: %v", err)
	}
	if states[reg.Slug].SyncedAt == "" || states[reg.Slug].Error != "" {
		t.Fatalf("sync state not recorded: %+v", states[reg.Slug])
	}

	results, err := a.Search(ctx, "fill")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].Name != "pdf" {
		t.Fatalf("search not repopulated: %+v", results)
	}
}

// newKitStub installs a fake sbx CLI answering `kit inspect --json` so the kit
// checkout path can be exercised without sandboxd.
func newKitStub(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"kit\" ] && [ \"$2\" = \"inspect\" ]; then\n" +
		"  printf '%s\\n' '{\"schemaVersion\":\"v1\",\"kind\":\"mixin\",\"name\":\"go-lint\",\"version\":\"1.0.0\",\"displayName\":\"Go lint\",\"description\":\"Lint Go code\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write kit stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
}

// newKitSourceRepo builds a local repository holding one kit spec.
func newKitSourceRepo(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	kitDir := filepath.Join(dir, name)
	if err := os.MkdirAll(kitDir, 0o755); err != nil {
		t.Fatalf("mkdir kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kitDir, "spec.yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := worktree.Commit("seed", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestStartRepopulatesKitsFromFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	newKitStub(t)

	src := newKitSourceRepo(t, "go-lint")
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreKits, Name: "kitbox", URL: "file://" + src})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	a.Start(ctx)
	waitForTopic(t, events, TopicKits)

	items, err := a.Store.ListKitItems(ctx, reg.Slug)
	if err != nil {
		t.Fatalf("list kit items: %v", err)
	}
	if len(items) != 1 || items[0].Name != "go-lint" || items[0].DisplayName != "Go lint" {
		t.Fatalf("kit catalog not repopulated: %+v", items)
	}
	states, err := a.Store.StoreStates(ctx, string(fleet.StoreKits))
	if err != nil {
		t.Fatalf("store states: %v", err)
	}
	if states[reg.Slug].SyncedAt == "" || states[reg.Slug].Error != "" {
		t.Fatalf("kit sync state not recorded: %+v", states[reg.Slug])
	}
}

func TestStartRepopulationKeepsWorkingWhenStoreFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreSkills, Name: "offline", URL: "file:///nonexistent/sandwarden-store"})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	a.Start(ctx)
	waitForTopic(t, events, TopicSkills)

	states, err := a.Store.StoreStates(ctx, string(fleet.StoreSkills))
	if err != nil {
		t.Fatalf("store states: %v", err)
	}
	if states[reg.Slug].Error == "" || states[reg.Slug].SyncedAt != "" {
		t.Fatalf("offline store error not surfaced: %+v", states[reg.Slug])
	}
	views, err := a.ListSkillStores(ctx)
	if err != nil {
		t.Fatalf("app must keep working after a failed store: %v", err)
	}
	if len(views) != 1 || views[0].Error == "" {
		t.Fatalf("store view must carry the error: %+v", views)
	}
}

func TestRepopulateSkipsHealthyStores(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	src := newSkillSourceRepo(t, "pdf", "Fill in PDF forms.")
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreSkills, Name: "acme", URL: "file://" + src})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	a.repopulateCatalogs(ctx)

	// A healthy store must be skipped: it would fail to re-clone now.
	if err := os.RemoveAll(src); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	a.repopulateCatalogs(ctx)

	states, err := a.Store.StoreStates(ctx, string(fleet.StoreSkills))
	if err != nil {
		t.Fatalf("store states: %v", err)
	}
	if states[reg.Slug].SyncedAt == "" {
		t.Fatalf("first sweep did not sync the store: %+v", states[reg.Slug])
	}
	if states[reg.Slug].Error != "" {
		t.Fatalf("healthy store must be skipped, got error %q", states[reg.Slug].Error)
	}
	if _, err := os.Stat(storeCheckoutPath(fleet.StoreSkills, reg.Slug)); err != nil {
		t.Fatalf("healthy checkout must be kept: %v", err)
	}
}

func TestRepopulateReclonesMissingCheckout(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	src := newSkillSourceRepo(t, "pdf", "Fill in PDF forms.")
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreSkills, Name: "acme", URL: "file://" + src})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	a.repopulateCatalogs(ctx)

	checkout := storeCheckoutPath(fleet.StoreSkills, reg.Slug)
	if err := os.RemoveAll(checkout); err != nil {
		t.Fatalf("remove checkout: %v", err)
	}
	a.repopulateCatalogs(ctx)

	if _, err := os.Stat(filepath.Join(checkout, "skills", "pdf", "SKILL.md")); err != nil {
		t.Fatalf("sweep did not restore the checkout: %v", err)
	}
	items, err := a.Store.ListSkillItems(ctx, reg.Slug)
	if err != nil || len(items) != 1 {
		t.Fatalf("catalog after restore = %+v, %v", items, err)
	}
}
