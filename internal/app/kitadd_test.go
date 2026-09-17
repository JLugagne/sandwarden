package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// kitSbxScript is a fake sbx CLI for the attach flow: it logs every
// invocation, appends the reference of a successful `kit add` to a state file,
// answers `inspect ... --json` from that file, and refuses the add with an
// arbitrary message when FAKE_KIT_SBX_REFUSAL is set.
const kitSbxScript = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_KIT_SBX_LOG"
state="$FAKE_KIT_SBX_STATE"
if [ "$1" = "kit" ] && [ "$2" = "add" ]; then
  if [ -n "$FAKE_KIT_SBX_REFUSAL" ]; then
    printf '%s\n' "$FAKE_KIT_SBX_REFUSAL" >&2
    exit 1
  fi
  printf '%s\n' "$4" >> "$state"
  printf 'kit add: container recreated for %s\n' "$3"
  exit 0
fi
if [ "$1" = "inspect" ]; then
  kits=""
  if [ -f "$state" ]; then
    while IFS= read -r ref; do
      if [ -n "$ref" ]; then
        kits="${kits:+$kits,}\"$ref\""
      fi
    done < "$state"
  fi
  printf '{"workspace":"/w","runtime_mounts":[],"kits":[%s]}\n' "$kits"
  exit 0
fi
exit 0
`

// newKitAddApp builds an app around a running fake sandbox "alpha" whose
// sidecar records the "base" kit — the fake sbx reports the same live kit —
// plus the fake sbx CLI. It returns the sbx invocation log path.
func newKitAddApp(t *testing.T) (*App, string) {
	t.Helper()
	a, _ := newTestApp(t, "alpha")
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	statePath := filepath.Join(dir, "kits")
	if err := os.WriteFile(statePath, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("seed sbx kits: %v", err)
	}
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(kitSbxScript), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	t.Setenv("FAKE_KIT_SBX_LOG", logPath)
	t.Setenv("FAKE_KIT_SBX_STATE", statePath)
	t.Setenv("FAKE_KIT_SBX_REFUSAL", "")

	spec := fleet.NewMixin("")
	spec.DisplayName = "alpha"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	app := fleet.SandboxApp{
		Sandbox: "alpha",
		Create:  &fleet.SandboxCreate{Clone: true, Kits: []string{"base"}},
	}
	if _, err := a.Fleet.CreateSandbox("alpha", spec, app); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return a, logPath
}

// sandboxKits re-reads the sidecar from disk so assertions see what a later
// recreate would load.
func sandboxKits(t *testing.T, a *App, name string) []string {
	t.Helper()
	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	s, ok := fresh.SandboxByName(name)
	if !ok {
		t.Fatalf("sandbox %s missing after reopen", name)
	}
	if s.App.Create == nil {
		return nil
	}
	return s.App.Create.Kits
}

func TestAttachKitRunsSbxAddAndPersistsReference(t *testing.T) {
	ctx := context.Background()
	a, logPath := newKitAddApp(t)

	var out strings.Builder
	result, err := a.AttachKit(ctx, "alpha", "./mcp-postgres", &out)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if result.Sandbox != "alpha" || result.Ref != "./mcp-postgres" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Report.Errors) != 0 {
		t.Fatalf("apply reported errors: %v", result.Report.Errors)
	}
	if !strings.Contains(out.String(), "kit add: container recreated for alpha") {
		t.Fatalf("sbx output was not streamed: %q", out.String())
	}
	calls := skillCalls(t, logPath)
	if !slices.Contains(calls, "kit add alpha ./mcp-postgres") {
		t.Fatalf("sbx did not receive `kit add alpha ./mcp-postgres`: %v", calls)
	}
	if n := countCalls(calls, "kit add "); n != 1 {
		t.Fatalf("kit add ran %d times, want once: %v", n, calls)
	}
	if got, want := sandboxKits(t, a, "alpha"), []string{"base", "./mcp-postgres"}; !slices.Equal(got, want) {
		t.Fatalf("create.kits = %v, want %v", got, want)
	}
	if !slices.Contains(calls, "inspect alpha --json") {
		t.Fatalf("the sidecar was not re-applied: %v", calls)
	}
	for _, prefix := range []string{"create ", "rm "} {
		if n := countCalls(calls, prefix); n != 0 {
			t.Fatalf("attach must not run `sbx %s` on the sandbox: %v", strings.TrimSpace(prefix), calls)
		}
	}
}

func TestAttachKitReferenceSurvivesRecreate(t *testing.T) {
	ctx := context.Background()
	a, _ := newKitAddApp(t)
	if _, err := a.AttachKit(ctx, "alpha", "./mcp-postgres", nil); err != nil {
		t.Fatalf("attach: %v", err)
	}
	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	s, ok := fresh.SandboxByName("alpha")
	if !ok {
		t.Fatal("sandbox alpha missing after reopen")
	}
	opts := a.CreateOptionsFromConfig(s)
	if !opts.Clone {
		t.Fatal("recreate options lost the clone flag")
	}
	if !slices.Contains(opts.Kits, "./mcp-postgres") {
		t.Fatalf("recreate options lost the attached kit: %+v", opts.Kits)
	}
}

func TestAttachKitRefusalLeavesSidecarUnchanged(t *testing.T) {
	ctx := context.Background()
	a, logPath := newKitAddApp(t)
	refusal := "ERROR: sandbox 'alpha' was created before the kit-add recreate feature shipped"
	t.Setenv("FAKE_KIT_SBX_REFUSAL", refusal)

	_, err := a.AttachKit(ctx, "alpha", "./mcp-postgres", nil)
	if err == nil || err.Error() != refusal {
		t.Fatalf("err = %v, want the refusal verbatim: %q", err, refusal)
	}
	if got, want := sandboxKits(t, a, "alpha"), []string{"base"}; !slices.Equal(got, want) {
		t.Fatalf("create.kits = %v, want the original %v", got, want)
	}
	calls := skillCalls(t, logPath)
	if !slices.Contains(calls, "kit add alpha ./mcp-postgres") {
		t.Fatalf("the refusal was never attempted: %v", calls)
	}
	if n := countCalls(calls, "mount "); n != 0 {
		t.Fatalf("the sidecar was re-applied after a refusal: %v", calls)
	}
}

func TestAttachKitRejectsRecordedKit(t *testing.T) {
	a, logPath := newKitAddApp(t)
	_, err := a.AttachKit(context.Background(), "alpha", "base", nil)
	if err == nil || !strings.Contains(err.Error(), "already attached") {
		t.Fatalf("err = %v, want an already-attached error", err)
	}
	if calls := skillCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("sbx was called for a duplicate attach: %v", calls)
	}
}

func TestAttachKitValidatesBeforeCallingSbx(t *testing.T) {
	a, logPath := newKitAddApp(t)
	if _, err := a.AttachKit(context.Background(), "alpha", "   ", nil); err == nil {
		t.Fatal("expected an error for an empty kit reference")
	}
	if _, err := a.AttachKit(context.Background(), "ghost", "./kit", nil); err == nil {
		t.Fatal("expected an error for a sandbox without a config")
	}
	if calls := skillCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("sbx was called before validation: %v", calls)
	}
}
