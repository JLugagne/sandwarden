package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func seedBareSandbox(t *testing.T, a *App) {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	if _, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{Sandbox: "box"}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
}

// TestConcurrentMountEditsBothSurvive pins that direct mount edits are
// serialized: each one rewrites the whole sidecar, so unlocked edits drop
// each other's mounts.
func TestConcurrentMountEditsBothSurvive(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)
	seedBareSandbox(t, a)
	restore := lockWait
	lockWait = time.Minute
	defer func() { lockWait = restore }()
	const edits = 12

	root := t.TempDir()
	start := make(chan struct{})
	errs := make([]error, edits)
	var wg sync.WaitGroup
	for i := range edits {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = a.AddMountAt(ctx, "box", filepath.Join(root, string(rune('a'+i))), "", false)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
	}
	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	s, ok := fresh.SandboxByName("box")
	if !ok {
		t.Fatal("sandbox disappeared")
	}
	if got := len(s.App.Mounts); got != edits {
		t.Fatalf("sidecar kept %d of %d concurrent mounts", got, edits)
	}
}

// TestApplyFixesReadOnlyDrift pins that a mount whose declared read-only flag
// differs from the live bind is re-bound with the declared mode.
func TestApplyFixesReadOnlyDrift(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)
	host := t.TempDir()
	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	app := fleet.SandboxApp{Sandbox: "box", Mounts: []fleet.MountRef{{HostPath: host, ReadOnly: true}}}
	if _, err := a.Fleet.CreateSandbox("box", spec, app); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := os.WriteFile(fake.state, []byte("box\x1f"+host+"\x1f\x1f0\n"), 0o644); err != nil {
		t.Fatalf("seed live mount: %v", err)
	}

	if _, err := a.Apply(ctx, "box"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var binds []fakeMount
	for _, m := range fake.mounts(t, "box") {
		if m.host == host {
			binds = append(binds, m)
		}
	}
	if len(binds) != 1 || !binds[0].readOn {
		t.Fatalf("live binds of %s: %+v, want one read-only bind", host, binds)
	}
}
