package app

import (
	"context"
	"testing"
	"time"
)

// TestStopSandboxNotUndoneByStatsSampler pins the bug where stopping a sandbox
// did not stick: the daemon keeps reporting the sandbox as running for the
// whole stop, so the periodic stats sampler exec'd into it. Because `sbx exec`
// starts a stopped sandbox, that probe booted it right back up and the GUI's
// success notification was a lie.
func TestStopSandboxNotUndoneByStatsSampler(t *testing.T) {
	ctx := context.Background()
	a, daemon := newTestApp(t, "box")
	logPath := newSkillsStub(t)

	daemon.mu.Lock()
	daemon.stopDelay = 300 * time.Millisecond
	daemon.stopEntered = make(chan string, 1)
	daemon.mu.Unlock()

	stopErr := make(chan error, 1)
	go func() { stopErr <- a.StopSandbox(ctx, "box") }()

	select {
	case <-daemon.stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("stop request never reached the daemon")
	}

	// The stop is in flight: the daemon still reports the sandbox as running.
	a.sampleStats(ctx)

	if err := <-stopErr; err != nil {
		t.Fatalf("stop: %v", err)
	}
	if calls := skillCalls(t, logPath); countCalls(calls, "exec") != 0 {
		t.Fatalf("the stats sampler exec'd into a sandbox being stopped, which boots it: %v", calls)
	}
}
