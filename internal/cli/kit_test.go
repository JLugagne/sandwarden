package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// kitSbxStub installs a fake sbx CLI that logs every invocation, answers
// `inspect ... --json`, and refuses `kit add` with the stub's refusal.
func kitSbxStub(t *testing.T, refusal string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_KIT_SBX_LOG"
if [ "$1" = "inspect" ]; then
  printf '{"workspace":"/w","runtime_mounts":[],"kits":[]}\n'
  exit 0
fi
if [ "$1" = "kit" ] && [ "$2" = "add" ]; then
  if [ -n "$FAKE_KIT_SBX_REFUSAL" ]; then
    printf '%s\n' "$FAKE_KIT_SBX_REFUSAL" >&2
    exit 1
  fi
  printf 'kit add: container recreated\n'
  exit 0
fi
exit 0
`
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	t.Setenv("FAKE_KIT_SBX_LOG", logPath)
	t.Setenv("FAKE_KIT_SBX_REFUSAL", refusal)
	return logPath
}

func TestKitAddAttachesAndPersists(t *testing.T) {
	env := newCLIEnv(t, "alpha")
	env.sbxLog = kitSbxStub(t, "")
	mustCreateSandbox(t, env.fleet(t), "alpha", fleet.SandboxApp{
		Sandbox: "alpha",
		Create:  &fleet.SandboxCreate{Clone: true, Kits: []string{"base"}},
	})

	stdout, _, err := env.execute(t, "kit", "add", "alpha", "./mcp-postgres")
	if err != nil {
		t.Fatalf("kit add: %v", err)
	}
	mustContain(t, stdout, "attached kit ./mcp-postgres to sandbox alpha")
	env.requireSbxCall(t, "kit add alpha ./mcp-postgres")

	fresh, err := fleet.Open(env.opts.ConfigDir)
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	s, ok := fresh.SandboxByName("alpha")
	if !ok {
		t.Fatal("sandbox alpha missing after kit add")
	}
	if s.App.Create == nil || len(s.App.Create.Kits) != 2 || s.App.Create.Kits[1] != "./mcp-postgres" {
		t.Fatalf("create.kits = %+v, want base plus ./mcp-postgres", s.App.Create)
	}
	if !s.App.Create.Clone {
		t.Fatal("the clone flag was lost")
	}
}

func TestKitAddSurfacesRefusalVerbatim(t *testing.T) {
	env := newCLIEnv(t, "alpha")
	refusal := "ERROR: sandbox 'alpha' was created before the kit-add recreate feature shipped"
	env.sbxLog = kitSbxStub(t, refusal)
	mustCreateSandbox(t, env.fleet(t), "alpha", fleet.SandboxApp{
		Sandbox: "alpha",
		Create:  &fleet.SandboxCreate{Kits: []string{"base"}},
	})

	_, _, err := env.execute(t, "kit", "add", "alpha", "./mcp-postgres")
	if err == nil || err.Error() != refusal {
		t.Fatalf("err = %v, want the refusal verbatim: %q", err, refusal)
	}
	fresh, err := fleet.Open(env.opts.ConfigDir)
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	s, ok := fresh.SandboxByName("alpha")
	if !ok {
		t.Fatal("sandbox alpha missing after the refusal")
	}
	if len(s.App.Create.Kits) != 1 || s.App.Create.Kits[0] != "base" {
		t.Fatalf("create.kits = %v, want the original [base]", s.App.Create.Kits)
	}
}

func TestKitAddRejectsEmptyRef(t *testing.T) {
	env := newCLIEnv(t, "alpha")
	env.sbxLog = kitSbxStub(t, "")
	mustCreateSandbox(t, env.fleet(t), "alpha", fleet.SandboxApp{
		Sandbox: "alpha",
		Create:  &fleet.SandboxCreate{},
	})

	_, _, err := env.execute(t, "kit", "add", "alpha", "")
	if err == nil || !strings.Contains(err.Error(), "kit reference is required") {
		t.Fatalf("err = %v, want an empty-reference error", err)
	}
	env.requireNoSbxCall(t, "kit add")
}
