package cli

import (
	"path/filepath"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestApplyWithoutArgumentsConvergesEveryConfig(t *testing.T) {
	env := newCLIEnv(t, "alpha", "beta")
	fl := env.fleet(t)
	profile := mustCreateProfile(t, fl, "dev", []string{"example.com"}, fleet.ProfileApp{})
	mustCreateSandbox(t, fl, "alpha", fleet.SandboxApp{Profiles: []string{profile.Slug}})
	mustCreateSandbox(t, fl, "beta", fleet.SandboxApp{Profiles: []string{profile.Slug}})

	stdout, _, err := env.execute(t, "apply")
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, stdout)
	}
	mustContain(t, stdout, "alpha:")
	mustContain(t, stdout, "beta:")

	scopes := map[string]bool{}
	for _, action := range env.daemon.appliedActions() {
		if action.Action == "allow" && len(action.Resources) == 1 && action.Resources[0] == "example.com" {
			scopes[action.SandboxID] = true
		}
	}
	if !scopes["alpha"] || !scopes["beta"] {
		t.Errorf("expected rules for alpha and beta, got %v", scopes)
	}
}

func TestApplyFailsWhenASandboxReportsErrors(t *testing.T) {
	env := newCLIEnv(t, "good", "broken")
	fl := env.fleet(t)
	profile := mustCreateProfile(t, fl, "dev", []string{"example.com"}, fleet.ProfileApp{})
	mustCreateSandbox(t, fl, "good", fleet.SandboxApp{Profiles: []string{profile.Slug}})
	mustCreateSandbox(t, fl, "broken", fleet.SandboxApp{
		Mounts: []fleet.MountRef{{HostPath: filepath.Join(t.TempDir(), "broken"), TargetPath: "/data"}},
	})

	stdout, _, err := env.execute(t, "apply")
	if err == nil {
		t.Fatalf("expected apply to fail\n%s", stdout)
	}
	mustContain(t, err.Error(), "1 sandbox(es) reported errors")
	mustContain(t, stdout, "good:")
	mustContain(t, stdout, "broken:")
	mustContain(t, stdout, "error:")

	scopes := map[string]bool{}
	for _, action := range env.daemon.appliedActions() {
		scopes[action.SandboxID] = true
	}
	if !scopes["good"] {
		t.Errorf("expected good to be converged before the failure, got %v", scopes)
	}
}
