package cli

import (
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestValidateReportsIncompatibleFeatures(t *testing.T) {
	env := newCLIEnv(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.43.0 abcdef")
	mustCreateSandbox(t, env.fleet(t), "box", fleet.SandboxApp{
		Sandbox: "box",
		Create:  &fleet.SandboxCreate{Workspaces: []string{"/host/project:/srv/project"}},
	})

	stdout, _, err := env.execute(t, "validate", "box")
	if err == nil {
		t.Fatalf("validate exited zero despite an incompatible feature:\n%s", stdout)
	}
	mustContain(t, stdout, "incompatible: workspace mount target (create.workspaces) requires sbx v0.51.0 or later; installed sbx is v0.43.0")
}

func TestValidateAcceptsUnknownSbxVersion(t *testing.T) {
	env := newCLIEnv(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: dev-abc")
	mustCreateSandbox(t, env.fleet(t), "box", fleet.SandboxApp{
		Sandbox: "box",
		Create:  &fleet.SandboxCreate{Workspaces: []string{"/host/project:/srv/project"}},
	})

	stdout, _, err := env.execute(t, "validate", "box")
	if err != nil {
		t.Fatalf("validate failed for an unknown version: %v\n%s", err, stdout)
	}
	if strings.Contains(stdout, "incompatible:") {
		t.Fatalf("unknown version must not report incompatibilities:\n%s", stdout)
	}
}
