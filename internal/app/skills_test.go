package app

import (
	"context"
	"os"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// TestReconcileSkillsKeepsDirectMount reproduces the reported bug: a direct
// mount whose target lives under .agents/skills is torn down by the skills
// reconcile even though the sandbox still declares it, then re-mounted by the
// direct-mount sync on the next pass, flapping the directory.
func TestReconcileSkillsKeepsDirectMount(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	host := t.TempDir()
	target := sandboxSkillsDir + "/security-audit-v2"
	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	if _, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{
		Sandbox: "box",
		Mounts:  []fleet.MountRef{{HostPath: host, TargetPath: target}},
	}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	if _, err := a.Apply(ctx, "box"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if mounts := fake.mounts(t, "box"); !hasMount(mounts, host, target, false) {
		t.Fatalf("direct mount under %s was torn down by the skills reconcile: mounts = %v", sandboxSkillsDir, mounts)
	}
}

// TestReconcileSkillsRemovesStaleMount guards the other side of the fix: an
// undeclared mount under .agents/skills that no profile or direct mount wants
// is still an orphan and must be removed.
func TestReconcileSkillsRemovesStaleMount(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	if _, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{Sandbox: "box"}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	host := t.TempDir()
	target := sandboxSkillsDir + "/stale-skill"
	line := "box\x1f" + host + "\x1f" + target + "\x1f1\n"
	if err := os.WriteFile(fake.state, []byte(line), 0o644); err != nil {
		t.Fatalf("seed mount: %v", err)
	}

	if _, err := a.Apply(ctx, "box"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if mounts := fake.mounts(t, "box"); hasMount(mounts, host, target, true) {
		t.Fatalf("undeclared stale skill mount should have been removed: mounts = %v", mounts)
	}
}
