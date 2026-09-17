package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

func writeStats(t *testing.T, path string, memTotalKB, memAvailKB, busy, total uint64) {
	t.Helper()
	content := fmt.Sprintf(
		"cpus 4\nmem_total %d\nmem_available %d\ncpu %d 0 0 %d 0 0 0 0\n",
		memTotalKB, memAvailKB, busy, total-busy,
	)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write stats: %v", err)
	}
}

func TestStatsTrackerDerivesUsage(t *testing.T) {
	tracker := newStatsTracker()
	first := sbx.StatsSample{CPUs: 4, MemoryTotalKB: 1000, MemoryAvailKB: 400, CPUBusy: 100, CPUTotal: 400}

	usage := tracker.record("box", first, time.Now())
	if usage.CPUPercent != 0 {
		t.Fatalf("first sample cannot yield a CPU delta, got %v", usage.CPUPercent)
	}
	if usage.MemoryUsed != 600*1024 || usage.MemoryTotal != 1000*1024 {
		t.Fatalf("unexpected memory usage: %+v", usage)
	}

	second := sbx.StatsSample{CPUs: 4, MemoryTotalKB: 1000, MemoryAvailKB: 200, CPUBusy: 200, CPUTotal: 800}
	usage = tracker.record("box", second, time.Now())
	if usage.CPUPercent != 25 {
		t.Fatalf("expected 25%% CPU over the delta, got %v", usage.CPUPercent)
	}
	if usage.MemoryUsed != 800*1024 {
		t.Fatalf("expected refreshed memory, got %+v", usage)
	}

	// An idle delta must stay at zero rather than going negative.
	idle := sbx.StatsSample{CPUs: 4, MemoryTotalKB: 1000, MemoryAvailKB: 200, CPUBusy: 200, CPUTotal: 1200}
	if usage = tracker.record("box", idle, time.Now()); usage.CPUPercent != 0 {
		t.Fatalf("expected 0%% when idle, got %v", usage.CPUPercent)
	}

	tracker.retain(map[string]bool{})
	if _, ok := tracker.get("box"); ok {
		t.Fatal("expected the sample to be dropped once the sandbox is no longer running")
	}
}

func TestStatsTrackerApplyOnlyOnRunning(t *testing.T) {
	tracker := newStatsTracker()
	tracker.record("box", sbx.StatsSample{MemoryTotalKB: 1024, MemoryAvailKB: 512, CPUBusy: 10, CPUTotal: 40}, time.Now())

	running := SandboxSummary{Name: "box", Running: true}
	tracker.apply(&running)
	if running.MemoryTotal != 1024*1024 || running.MemoryUsed != 512*1024 {
		t.Fatalf("expected stats on the running summary, got %+v", running)
	}

	stopped := SandboxSummary{Name: "box"}
	tracker.apply(&stopped)
	if stopped.MemoryTotal != 0 || stopped.MemoryUsed != 0 || stopped.CPUPercent != 0 {
		t.Fatalf("stopped sandboxes must not carry stale stats, got %+v", stopped)
	}
}

func TestSampleStatsReadsRunningSandboxes(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	fake := newFakeSbx(t)

	writeStats(t, fake.stats, 4096, 1024, 100, 400)
	a.sampleStats(ctx)

	summaries, err := a.SandboxSummaries(ctx)
	if err != nil {
		t.Fatalf("summaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one sandbox, got %d", len(summaries))
	}
	if summaries[0].MemoryTotal != 4096*1024 || summaries[0].MemoryUsed != 3072*1024 {
		t.Fatalf("unexpected memory on the summary: %+v", summaries[0])
	}
	if summaries[0].CPUPercent != 0 {
		t.Fatalf("first probe has no delta yet, got %v", summaries[0].CPUPercent)
	}

	writeStats(t, fake.stats, 4096, 1024, 200, 800)
	a.sampleStats(ctx)

	summaries, _ = a.SandboxSummaries(ctx)
	if summaries[0].CPUPercent != 25 {
		t.Fatalf("expected 25%% after the second probe, got %+v", summaries[0])
	}
}

func TestSampleStatsIgnoresProbeFailures(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)

	// No stats file: the fake exec prints nothing, which is a parse error.
	a.sampleStats(ctx)

	summaries, err := a.SandboxSummaries(ctx)
	if err != nil {
		t.Fatalf("summaries: %v", err)
	}
	if summaries[0].MemoryTotal != 0 || summaries[0].CPUPercent != 0 {
		t.Fatalf("expected no stats after a failed probe, got %+v", summaries[0])
	}
}

// TestSampleStatsBoundsWedgedProbe pins that a sandbox whose sbx exec never
// returns cannot wedge the stats loop: the probe is cancelled after
// statsSampleTimeout and the sweep moves on instead of holding the sandbox in
// use forever.
func TestSampleStatsBoundsWedgedProbe(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	dir := t.TempDir()
	script := filepath.Join(dir, "sbx")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = exec ]; then sleep 30; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", script)

	restore := statsSampleTimeout
	statsSampleTimeout = 200 * time.Millisecond
	defer func() { statsSampleTimeout = restore }()

	done := make(chan struct{})
	go func() {
		a.sampleStats(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sampleStats blocked on a wedged sbx exec")
	}
}
