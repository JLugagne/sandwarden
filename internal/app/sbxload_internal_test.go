package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func newLoadStub(t *testing.T, failMount bool) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	fail := "0"
	if failMount {
		fail = "1"
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"case \"$1\" in\n" +
		"  inspect) printf '{\"workspace\":\"/w\",\"runtime_mounts\":[]}\\n' ;;\n" +
		"  mount) if [ " + fail + " = 1 ]; then echo boom; exit 1; fi ;;\n" +
		"  exec) exit 1 ;;\n" +
		"esac\n" +
		"exit 0\n"
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	return logPath
}

func seedMountedSandbox(t *testing.T, a *App) {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	app := fleet.SandboxApp{Sandbox: "box", Mounts: []fleet.MountRef{{HostPath: t.TempDir()}}}
	if _, err := a.Fleet.CreateSandbox("box", spec, app); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
}

func waitForCalls(t *testing.T, logPath, prefix string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countCalls(skillCalls(t, logPath), prefix) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d %q calls", want, prefix)
}

// TestApplyInspectsSandboxOncePerPass pins that one convergence pass spawns a
// single `sbx inspect`, however many sync steps need the runtime mounts.
func TestApplyInspectsSandboxOncePerPass(t *testing.T) {
	a, _ := seedLockTest(t)
	logPath := newLoadStub(t, false)

	if _, err := a.Apply(context.Background(), "box"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := countCalls(skillCalls(t, logPath), "inspect "); got != 1 {
		t.Fatalf("apply ran %d sbx inspect, want 1", got)
	}
}

// TestReconcileLoopBacksOffFailingSandbox pins that a sandbox whose pass keeps
// failing is not retried on every trigger.
func TestReconcileLoopBacksOffFailingSandbox(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, true)
	seedMountedSandbox(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.reconcileLoop(ctx)

	a.requestReconcile()
	waitForCalls(t, logPath, "mount ", 1)
	a.requestReconcile()
	time.Sleep(reconcileDebounce + time.Second)

	if got := countCalls(skillCalls(t, logPath), "mount "); got != 1 {
		t.Fatalf("failing mount retried %d times, want 1 before the backoff expires", got)
	}
}

// TestManualReconcileIgnoresBackoff pins that an explicit reconcile always
// retries, even while the background loop backs off.
func TestManualReconcileIgnoresBackoff(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, true)
	seedMountedSandbox(t, a)
	ctx := context.Background()

	_ = a.Reconcile(ctx)
	_ = a.Reconcile(ctx)

	if got := countCalls(skillCalls(t, logPath), "mount "); got != 2 {
		t.Fatalf("manual reconcile mounted %d times, want 2", got)
	}
}

// TestWatchStatsSkipsWithoutViewers pins that nobody watching means no probe
// exec inside the sandboxes.
func TestWatchStatsSkipsWithoutViewers(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	a.watchStats(ctx)

	if got := countCalls(skillCalls(t, logPath), "exec "); got != 0 {
		t.Fatalf("stats probed %d times without a viewer, want 0", got)
	}
}

// TestWatchStatsProbesWithViewer is the counterpart: a subscriber gets stats.
func TestWatchStatsProbesWithViewer(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, false)
	_, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	a.watchStats(ctx)

	if got := countCalls(skillCalls(t, logPath), "exec "); got != 1 {
		t.Fatalf("stats probed %d times with a viewer, want 1", got)
	}
}

// TestSampleStatsBacksOffFailingProbe pins that a failing probe is not
// re-run on the very next sweep.
func TestSampleStatsBacksOffFailingProbe(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, false)
	ctx := context.Background()

	a.sampleStats(ctx)
	a.sampleStats(ctx)

	if got := countCalls(skillCalls(t, logPath), "exec "); got != 1 {
		t.Fatalf("failing probe ran %d times, want 1", got)
	}
}

// TestPublishSkipsWithoutViewers pins that snapshots nobody receives are not
// built, since building a detail spawns `sbx inspect`.
func TestPublishSkipsWithoutViewers(t *testing.T) {
	a, _ := newTestApp(t, "box")
	logPath := newLoadStub(t, false)

	a.publishTopic(TopicSandbox("box"))

	if got := countCalls(skillCalls(t, logPath), "inspect "); got != 0 {
		t.Fatalf("publish ran %d sbx inspect without a viewer, want 0", got)
	}
}

// TestPortsEventDoesNotReconcile pins that port publications, which never
// change the configuration, do not trigger a convergence pass.
func TestPortsEventDoesNotReconcile(t *testing.T) {
	a, fake := newTestApp(t, "box")
	newLoadStub(t, false)
	fake.setEvents(`{"type":"sandbox.ports","action":"published","sandbox_name":"box"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.watchEvents(ctx)
	fake.waitEventsServed(t)
	time.Sleep(200 * time.Millisecond)

	if len(a.reconcileCh) != 0 {
		t.Fatal("a ports event requested a reconcile")
	}
}

// TestLifecycleEventReconciles is the counterpart for lifecycle events.
func TestLifecycleEventReconciles(t *testing.T) {
	a, fake := newTestApp(t, "box")
	newLoadStub(t, false)
	fake.setEvents(`{"type":"sandbox.lifecycle","action":"started","sandbox_name":"box"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.watchEvents(ctx)
	fake.waitEventsServed(t)
	time.Sleep(200 * time.Millisecond)

	if len(a.reconcileCh) != 1 {
		t.Fatal("a lifecycle event did not request a reconcile")
	}
}

func TestEventRetryDelayGrowsAndCaps(t *testing.T) {
	if got := eventRetryDelay(0); got != eventRetryBase {
		t.Fatalf("first retry waits %v, want %v", got, eventRetryBase)
	}
	if eventRetryDelay(1) <= eventRetryDelay(0) {
		t.Fatal("retry delay does not grow")
	}
	if got := eventRetryDelay(50); got != eventRetryMax {
		t.Fatalf("retry delay %v not capped at %v", got, eventRetryMax)
	}
}

// TestOnlyOneInstanceLeads pins that two processes sharing a fleet directory
// do not both run the background convergence loop.
func TestOnlyOneInstanceLeads(t *testing.T) {
	a, _ := newTestApp(t, "box")
	fl, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("open fleet: %v", err)
	}
	b := New(a.Sbx, a.Store, fl)

	if !a.tryLead() {
		t.Fatal("first instance could not lead")
	}
	if b.tryLead() {
		t.Fatal("second instance leads alongside the first")
	}
	a.stepDown()
	if !b.tryLead() {
		t.Fatal("second instance cannot take over once the leader steps down")
	}
	b.stepDown()
}
