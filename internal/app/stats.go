package app

import (
	"context"
	"sync"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

// statsInterval is how often running sandboxes are sampled. The probe execs
// inside each sandbox, so keep it well above the daemon's own polling.
const statsInterval = 5 * time.Second

// statsSampleTimeout bounds one resource probe. A sandbox whose sbx exec
// wedges (its SSH layer hangs, the agent is unresponsive) must not keep the
// probe child alive forever or mark the sandbox in use indefinitely.
var statsSampleTimeout = 8 * time.Second

// SandboxStats is the last sampled resource usage of a running sandbox.
type SandboxStats struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  int64     `json:"memory_used_bytes"`
	MemoryTotal int64     `json:"memory_total_bytes"`
	SampledAt   time.Time `json:"sampled_at"`
}

// statsTracker keeps the previous CPU counters and the derived usage per
// sandbox. CPU usage is a delta, so the first sample only seeds the counters
// while memory is available immediately.
type statsTracker struct {
	mu    sync.Mutex
	prev  map[string]sbx.StatsSample
	usage map[string]SandboxStats
}

func newStatsTracker() *statsTracker {
	return &statsTracker{prev: map[string]sbx.StatsSample{}, usage: map[string]SandboxStats{}}
}

// record folds one sample in and returns the derived usage.
func (t *statsTracker) record(sandbox string, sample sbx.StatsSample, at time.Time) SandboxStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	used := (sample.MemoryTotalKB - sample.MemoryAvailKB) * 1024
	if used < 0 {
		used = 0
	}
	usage := SandboxStats{
		MemoryUsed:  used,
		MemoryTotal: sample.MemoryTotalKB * 1024,
		SampledAt:   at,
	}
	if prev, ok := t.prev[sandbox]; ok && prev.CPUTotal > 0 {
		busy := float64(sample.CPUBusy - prev.CPUBusy)
		total := float64(sample.CPUTotal - prev.CPUTotal)
		if total > 0 && busy >= 0 {
			percent := busy / total * 100
			if percent > 100 {
				percent = 100
			}
			usage.CPUPercent = percent
		}
	}
	t.prev[sandbox] = sample
	t.usage[sandbox] = usage
	return usage
}

// get returns the last usage recorded for a sandbox.
func (t *statsTracker) get(sandbox string) (SandboxStats, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	usage, ok := t.usage[sandbox]
	return usage, ok
}

// retain forgets every sandbox that is no longer being sampled.
func (t *statsTracker) retain(seen map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for name := range t.prev {
		if !seen[name] {
			delete(t.prev, name)
			delete(t.usage, name)
		}
	}
}

// apply decorates a running summary with the last sample.
func (t *statsTracker) apply(sum *SandboxSummary) {
	if !sum.Running {
		return
	}
	usage, ok := t.get(sum.Name)
	if !ok {
		return
	}
	sum.CPUPercent = usage.CPUPercent
	sum.MemoryUsed = usage.MemoryUsed
	sum.MemoryTotal = usage.MemoryTotal
}

// sampleStats refreshes the cached usage of every running sandbox and pushes
// the list topic when at least one probe landed.
func (a *App) sampleStats(ctx context.Context) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return
	}
	seen := make(map[string]bool, len(sandboxes))
	sampled := false
	for _, sandbox := range sandboxes {
		if !sandbox.Running() {
			continue
		}
		// A stop in flight still reports the sandbox as running, and the probe
		// below execs into it: `sbx exec` starts a stopped sandbox, so probing
		// a stopping one would boot it right back up.
		if a.isStopping(sandbox.Name) {
			continue
		}
		seen[sandbox.Name] = true
		probeCtx, cancel := context.WithTimeout(ctx, statsSampleTimeout)
		sample, err := a.Sbx.Stats(probeCtx, sandbox.Name)
		cancel()
		if err != nil {
			continue
		}
		a.stats.record(sandbox.Name, sample, time.Now())
		sampled = true
	}
	a.stats.retain(seen)
	if sampled {
		a.Notify(TopicSandboxes)
	}
}

// watchStats samples running sandboxes on a timer until ctx is cancelled.
func (a *App) watchStats(ctx context.Context) {
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()
	a.sampleStats(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sampleStats(ctx)
		}
	}
}
