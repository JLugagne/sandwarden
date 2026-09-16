package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/store"
)

// newSkillsStub installs a fake sbx CLI that keeps a runtime mount table in a
// state file: `inspect` renders it as JSON, `mount` and `umount` mutate it, and
// every call is appended to a log. It returns the log path.
func newSkillsStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	statePath := filepath.Join(dir, "mounts")
	if err := os.WriteFile(statePath, nil, 0o644); err != nil {
		t.Fatalf("state file: %v", err)
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$SBX_STUB_LOG"
if [ "$1" = inspect ]; then
  printf '{"workspace":"/w","runtime_mounts":['
  first=1
  while IFS='|' read -r h t; do
    if [ -z "$h" ]; then continue; fi
    if [ "$first" = 0 ]; then printf ','; fi
    first=0
    printf '{"host_path":"%s","container_target":"%s","read_only":true}' "$h" "$t"
  done < "$SBX_STUB_STATE"
  printf ']}\n'
  exit 0
fi
if [ "$1" = mount ]; then
  spec="$3"
  case "$spec" in
    *:ro) spec="${spec%:ro}" ;;
    *:rw) spec="${spec%:rw}" ;;
  esac
  host="${spec%%:*}"
  rest="${spec#*:}"
  if [ "$rest" = "$spec" ]; then target="$host"; else target="$rest"; fi
  printf '%s|%s\n' "$host" "$target" >> "$SBX_STUB_STATE"
  exit 0
fi
if [ "$1" = umount ]; then
  spec="$3"
  host="${spec%%:*}"
  rest="${spec#*:}"
  if [ "$rest" = "$spec" ]; then target="$host"; else target="$rest"; fi
  grep -vF "$host|$target" "$SBX_STUB_STATE" > "$SBX_STUB_STATE.tmp" 2>/dev/null
  mv "$SBX_STUB_STATE.tmp" "$SBX_STUB_STATE" 2>/dev/null
  exit 0
fi
exit 0
`
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	t.Setenv("SBX_STUB_LOG", logPath)
	t.Setenv("SBX_STUB_STATE", statePath)
	return logPath
}

func skillCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

func countCalls(calls []string, prefix string) int {
	n := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

func writeSkillFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// registerTestStore registers a store record pointing at a fresh directory and
// returns its path.
func registerTestStore(t *testing.T, a *App, name string) string {
	t.Helper()
	checkout := filepath.Join(t.TempDir(), "checkout")
	if _, err := a.Store.CreateSkillStore(context.Background(), store.SkillStore{
		Name: name,
		URL:  "https://example.com/" + name,
		Path: checkout,
	}); err != nil {
		t.Fatalf("create store: %v", err)
	}
	return checkout
}

func TestReconcileSkillsLifecycle(t *testing.T) {
	ctx := context.Background()
	logPath := newSkillsStub(t)
	a, _ := newTestApp(t, "box")

	checkout := registerTestStore(t, a, "official")
	writeSkillFile(t, filepath.Join(checkout, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	writeSkillFile(t, filepath.Join(checkout, "commands", "deploy.md"), "Deploy\n")
	stored, err := a.Store.ListSkillStores(ctx)
	if err != nil || len(stored) != 1 {
		t.Fatalf("unexpected stores: %+v (%v)", stored, err)
	}
	if err := a.Store.ReplaceSkillItems(ctx, stored[0].ID, []store.SkillItem{
		{Kind: "skill", Name: "alpha", RelPath: "skills/alpha"},
		{Kind: "command", Name: "deploy", RelPath: "commands/deploy.md"},
	}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	items, _ := a.Store.ListSkillItems(ctx, stored[0].ID)
	alpha := skillItemNamed(t, items, "skill", "alpha")
	deploy := skillItemNamed(t, items, "command", "deploy")

	profile, err := a.CreateProfile(ctx, "dev", "", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", profile.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if err := a.AddSkillItemToProfile(ctx, profile.ID, alpha.ID); err != nil {
		t.Fatalf("select skill: %v", err)
	}

	alphaHost := filepath.Join(checkout, "skills", "alpha")
	wantMount := "mount box " + alphaHost + ":/home/agent/.agents/skills/alpha:ro"
	if got := countCalls(skillCalls(t, logPath), wantMount); got != 1 {
		t.Fatalf("expected one %q call, got %d in %v", wantMount, got, skillCalls(t, logPath))
	}
	if got := countCalls(skillCalls(t, logPath), "exec box mkdir -p /home/agent/.agents/skills/alpha"); got != 1 {
		t.Fatalf("expected the target directory to be prepared, got %d", got)
	}

	if err := a.AttachSkillItem(ctx, "box", deploy.ID); err != nil {
		t.Fatalf("attach command: %v", err)
	}
	deployHost := filepath.Join(checkout, "commands", "deploy.md")
	wantCommandMount := "mount box " + deployHost + ":/home/agent/.agents/commands/deploy.md:ro"
	if got := countCalls(skillCalls(t, logPath), wantCommandMount); got != 1 {
		t.Fatalf("expected one %q call, got %d in %v", wantCommandMount, got, skillCalls(t, logPath))
	}

	before := skillCalls(t, logPath)
	if _, err := a.ReconcileSkills(ctx, "box"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	after := skillCalls(t, logPath)
	if countCalls(after, "mount box") != countCalls(before, "mount box") || countCalls(after, "umount box") != countCalls(before, "umount box") {
		t.Fatalf("reconcile should be a no-op, went from %v to %v", before, after)
	}

	if err := a.DetachSkillItem(ctx, "box", deploy.ID); err != nil {
		t.Fatalf("detach command: %v", err)
	}
	wantCommandUmount := "umount box " + deployHost + ":/home/agent/.agents/commands/deploy.md"
	if got := countCalls(skillCalls(t, logPath), wantCommandUmount); got != 1 {
		t.Fatalf("expected one %q call, got %d in %v", wantCommandUmount, got, skillCalls(t, logPath))
	}

	if err := a.RemoveSkillItemFromProfile(ctx, profile.ID, alpha.ID); err != nil {
		t.Fatalf("deselect skill: %v", err)
	}
	wantAlphaUmount := "umount box " + alphaHost + ":/home/agent/.agents/skills/alpha"
	if got := countCalls(skillCalls(t, logPath), wantAlphaUmount); got != 1 {
		t.Fatalf("expected one %q call, got %d in %v", wantAlphaUmount, got, skillCalls(t, logPath))
	}
}

func TestReconcileSkillsUnmountsStaleMounts(t *testing.T) {
	ctx := context.Background()
	logPath := newSkillsStub(t)
	a, _ := newTestApp(t, "box")

	stale := "/old/skills/beta"
	if err := os.WriteFile(os.Getenv("SBX_STUB_STATE"), []byte(stale+"|/home/agent/.agents/skills/beta\n"), 0o644); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	result, err := a.ReconcileSkills(ctx, "box")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Removed != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	want := "umount box " + stale + ":/home/agent/.agents/skills/beta"
	if got := countCalls(skillCalls(t, logPath), want); got != 1 {
		t.Fatalf("expected one %q call, got %v", want, skillCalls(t, logPath))
	}
}

func TestReconcileSkillsReportsConflictsAndMissing(t *testing.T) {
	ctx := context.Background()
	newSkillsStub(t)
	a, _ := newTestApp(t, "box")

	first := registerTestStore(t, a, "first")
	second := registerTestStore(t, a, "second")
	writeSkillFile(t, filepath.Join(first, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	writeSkillFile(t, filepath.Join(second, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	stores, _ := a.Store.ListSkillStores(ctx)
	for _, registered := range stores {
		catalog := []store.SkillItem{{Kind: "skill", Name: "alpha", RelPath: "skills/alpha"}}
		if registered.Name == "first" {
			catalog = append(catalog, store.SkillItem{Kind: "skill", Name: "ghost", RelPath: "skills/ghost"})
		}
		if err := a.Store.ReplaceSkillItems(ctx, registered.ID, catalog); err != nil {
			t.Fatalf("replace items: %v", err)
		}
	}

	profile, _ := a.CreateProfile(ctx, "dev", "", false, false)
	if err := a.ApplyProfile(ctx, "box", profile.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	items, _ := a.Store.ListAllSkillItems(ctx)
	for _, item := range items {
		if err := a.AddSkillItemToProfile(ctx, profile.ID, item.ID); err != nil {
			t.Fatalf("select %s: %v", item.Name, err)
		}
	}

	result, err := a.ReconcileSkills(ctx, "box")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	joined := strings.Join(result.Errors, "\n")
	if !strings.Contains(joined, "already provided by store") {
		t.Fatalf("expected a target conflict in %v", result.Errors)
	}
	if !strings.Contains(joined, "ghost") {
		t.Fatalf("expected a missing source in %v", result.Errors)
	}
	if result.Applied > 1 {
		t.Fatalf("only the winning item should mount, applied %d", result.Applied)
	}

	detail, err := a.SandboxDetail(ctx, "box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	var conflicts int
	for _, skill := range detail.Skills {
		if skill.Conflict != "" {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("expected one conflict in the detail projection, got %+v", detail.Skills)
	}
}

func TestSandboxDetailProjectsSkillSources(t *testing.T) {
	ctx := context.Background()
	newSkillsStub(t)
	a, _ := newTestApp(t, "box")

	checkout := registerTestStore(t, a, "official")
	writeSkillFile(t, filepath.Join(checkout, "skills", "alpha", "SKILL.md"), "# Alpha\n")
	stores, _ := a.Store.ListSkillStores(ctx)
	if err := a.Store.ReplaceSkillItems(ctx, stores[0].ID, []store.SkillItem{
		{Kind: "skill", Name: "alpha", RelPath: "skills/alpha"},
	}); err != nil {
		t.Fatalf("replace items: %v", err)
	}
	items, _ := a.Store.ListSkillItems(ctx, stores[0].ID)

	profile, _ := a.CreateProfile(ctx, "dev", "", false, false)
	if err := a.ApplyProfile(ctx, "box", profile.ID); err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if err := a.AddSkillItemToProfile(ctx, profile.ID, items[0].ID); err != nil {
		t.Fatalf("select: %v", err)
	}
	if err := a.AttachSkillItem(ctx, "box", items[0].ID); err != nil {
		t.Fatalf("attach: %v", err)
	}

	detail, err := a.SandboxDetail(ctx, "box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Skills) != 1 {
		t.Fatalf("expected 1 projected skill, got %+v", detail.Skills)
	}
	skill := detail.Skills[0]
	if skill.Target != "/home/agent/.agents/skills/alpha" {
		t.Fatalf("unexpected target: %+v", skill)
	}
	if len(skill.Sources) != 2 || skill.Sources[0] != "dev" || skill.Sources[1] != "sandbox" {
		t.Fatalf("expected both sources, got %+v", skill.Sources)
	}
	if !skill.Mounted {
		t.Fatalf("expected the skill to be mounted: %+v", skill)
	}
}

// skillItemNamed returns one catalog item by (kind, name).
func skillItemNamed(t *testing.T, items []store.SkillItem, kind, name string) store.SkillItem {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			return item
		}
	}
	t.Fatalf("item %s/%s not found in %+v", kind, name, items)
	return store.SkillItem{}
}
