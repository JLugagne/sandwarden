package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/store"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newKitRepo builds a local git repository holding two kit directories.
func newKitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeKitSpec(t, filepath.Join(dir, "code-server"), "code-server")
	writeKitSpec(t, filepath.Join(dir, "amp"), "amp")
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := worktree.Commit("kits", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return dir
}

func writeKitSpec(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	spec := "schemaVersion: \"2\"\nkind: mixin\nname: " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

// newKitStub installs a fake sbx CLI that answers `kit inspect --json` with a
// spec derived from the kit directory name and `kit validate` with VALID. A
// directory named broken fails inspection.
func newKitStub(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := `#!/bin/sh
if [ -n "$SBX_STUB_LOG" ]; then
  printf '%s\n' "$*" >> "$SBX_STUB_LOG"
fi
if [ "$1" = settings ] && [ -n "$SBX_STUB_SETTINGS" ]; then
  if [ "$2" = get ]; then
    cat "$SBX_STUB_SETTINGS"
    exit 0
  fi
  if [ "$2" = set ]; then
    printf '%s' "$4" > "$SBX_STUB_SETTINGS"
    exit 0
  fi
fi
if [ "$1" = kit ] && [ "$2" = inspect ]; then
  name=$(basename "$4")
  if [ "$name" = broken ]; then
    echo "manifest: schemaVersion is required" >&2
    exit 1
  fi
  printf '{"schemaVersion":"2","kind":"mixin","name":"%s","displayName":"%s kit","description":"desc %s","requires":{"agent":"claude"}}\n' "$name" "$name" "$name"
  exit 0
fi
if [ "$1" = kit ] && [ "$2" = validate ]; then
  printf 'VALID: %s (directory)\n' "$4"
  exit 0
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
}

func TestSyncKitStoreDiscoversKitsAndRefs(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	newKitStub(t)
	src := newKitRepo(t)

	created, err := a.Store.CreateKitStore(ctx, store.KitStore{
		Name: "contrib",
		URL:  "file://" + src,
		Path: filepath.Join(t.TempDir(), "checkout"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	synced, err := a.syncKitStore(ctx, created)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if synced.Error != "" {
		t.Fatalf("unexpected sync error: %s", synced.Error)
	}
	if synced.SyncedAt == "" {
		t.Fatal("expected synced_at to be recorded")
	}

	views, err := a.ListKitItems(ctx, created.ID)
	if err != nil {
		t.Fatalf("list kits: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 kits, got %+v", views)
	}
	byName := make(map[string]KitItemView, len(views))
	for _, view := range views {
		byName[view.Name] = view
	}
	amp, ok := byName["amp"]
	if !ok {
		t.Fatalf("missing amp kit in %+v", byName)
	}
	if amp.Kind != "mixin" || amp.RequiresAgent != "claude" || amp.DisplayName != "amp kit" {
		t.Fatalf("unexpected kit metadata: %+v", amp)
	}
	if want := "git+file://" + src + "#dir=amp"; amp.Ref != want {
		t.Fatalf("want ref %q, got %q", want, amp.Ref)
	}
	if amp.Spec.SchemaVersion != "2" || amp.Spec.Name != "amp" {
		t.Fatalf("expected the parsed spec, got %+v", amp.Spec)
	}

	validation, err := a.KitValidate(ctx, amp.ID)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !validation.OK || !strings.Contains(validation.Output, "VALID") {
		t.Fatalf("unexpected validation: %+v", validation)
	}
}

func TestSyncKitStoreSkipsInvalidArtifacts(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	newKitStub(t)
	src := newKitRepo(t)
	writeKitSpec(t, filepath.Join(src, "broken"), "broken")
	commitAll(t, src)

	created, err := a.Store.CreateKitStore(ctx, store.KitStore{
		Name: "contrib",
		URL:  "file://" + src,
		Path: filepath.Join(t.TempDir(), "checkout"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	synced, err := a.syncKitStore(ctx, created)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.Contains(synced.Error, "1 kit(s) skipped") {
		t.Fatalf("expected a skip warning, got %q", synced.Error)
	}

	views, err := a.ListKitItems(ctx, created.ID)
	if err != nil {
		t.Fatalf("list kits: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected the two valid kits, got %+v", views)
	}
	for _, view := range views {
		if view.Name == "broken" {
			t.Fatalf("broken kit must not be cataloged: %+v", view)
		}
	}
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := worktree.Commit("more kits", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestKitSourcePrefix(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://github.com/Acme/kit-repo.git", "github.com/Acme/kit-repo"},
		{"git+https://github.com/Acme/kit-repo.git", "github.com/Acme/kit-repo"},
		{"https://user:pass@github.com/Acme/kit-repo.git", "github.com/Acme/kit-repo"},
		{"ssh://git@github.com:22/Acme/kit-repo.git", "github.com/Acme/kit-repo"},
		{"git@github.com:Acme/kit-repo.git", "github.com/Acme/kit-repo"},
		{"https://gitlab.example.com/group/sub/repo", "gitlab.example.com/group/sub/repo"},
		{"file:///srv/kits", ""},
		{"/srv/kits", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := kitSourcePrefix(tc.raw); got != tc.want {
			t.Fatalf("kitSourcePrefix(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestKitSourceAllowed(t *testing.T) {
	cases := []struct {
		entry  string
		source string
		want   bool
	}{
		{"*", "github.com/Acme/kit-repo", true},
		{"github.com/", "github.com/Acme/kit-repo", true},
		{"github.com/Acme", "github.com/Acme/kit-repo", true},
		{"github.com/Acme/kit-repo", "github.com/Acme/kit-repo", true},
		{"docker.io/", "github.com/Acme/kit-repo", false},
		{"github.com/Acme-other", "github.com/Acme/kit-repo", false},
		{"", "github.com/Acme/kit-repo", false},
	}
	for _, tc := range cases {
		if got := kitSourceAllowed(tc.entry, tc.source); got != tc.want {
			t.Fatalf("kitSourceAllowed(%q, %q) = %v, want %v", tc.entry, tc.source, got, tc.want)
		}
	}
}

func TestAllowKitSourceMergesAndPersists(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	newKitStub(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`["docker.io/"]`), 0o644); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("SBX_STUB_SETTINGS", settingsPath)
	t.Setenv("SBX_STUB_LOG", logPath)

	if err := a.allowKitSource(ctx, "git+https://github.com/Acme/kit-repo.git"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if got, want := string(raw), `["docker.io/","github.com/Acme/kit-repo"]`; got != want {
		t.Fatalf("want %s, got %s", want, got)
	}

	if err := a.allowKitSource(ctx, "https://github.com/Acme/kit-repo"); err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if sets := countCalls(skillCalls(t, logPath), "settings set "); sets != 1 {
		t.Fatalf("expected a single settings set, got %d (%v)", sets, skillCalls(t, logPath))
	}
}

func TestAllowKitSourceSkipsLocalAndWildcard(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		settings string
		url      string
	}{
		{"local source", `["docker.io/"]`, "file:///srv/kits"},
		{"wildcard", `["*"]`, "https://github.com/Acme/kit-repo.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := newTestApp(t)
			newKitStub(t)
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(settingsPath, []byte(tc.settings), 0o644); err != nil {
				t.Fatalf("seed settings: %v", err)
			}
			logPath := filepath.Join(t.TempDir(), "calls.log")
			t.Setenv("SBX_STUB_SETTINGS", settingsPath)
			t.Setenv("SBX_STUB_LOG", logPath)

			if err := a.allowKitSource(ctx, tc.url); err != nil {
				t.Fatalf("allow: %v", err)
			}
			raw, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("read settings: %v", err)
			}
			if string(raw) != tc.settings {
				t.Fatalf("settings changed: %s", raw)
			}
			if sets := countCalls(skillCalls(t, logPath), "settings set "); sets != 0 {
				t.Fatalf("unexpected settings write (%d) in %v", sets, skillCalls(t, logPath))
			}
		})
	}
}
