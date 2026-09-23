package app

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// Start launches the background watchers. It returns immediately; they run until
// ctx is cancelled.
func (a *App) Start(ctx context.Context) {
	a.mu.Lock()
	a.rootCtx = ctx
	if a.lastState == nil {
		a.lastState = map[string]string{}
	}
	a.mu.Unlock()
	a.primeStates(ctx)
	go a.repopulateCatalogs(ctx)
	go a.watchEvents(ctx)
	go a.pollBlocked(ctx)
	go a.reconcileLoop(ctx)
	go a.reconcileTicker(ctx)
	go a.watchStats(ctx)
}

// Traffic returns the proxy's current allowed/blocked host log.
func (a *App) Traffic(ctx context.Context) (sbx.PolicyLog, error) {
	return a.Sbx.PolicyLog(ctx)
}

func (a *App) pollBlocked(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	a.checkBlocked(ctx, true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkBlocked(ctx, false)
		}
	}
}

// checkBlocked fetches the policy log and publishes a blocked event for each
// newly seen denied host. When seed is true it records the current hosts
// without notifying, so startup does not replay old blocks.
func (a *App) checkBlocked(ctx context.Context, seed bool) {
	log, err := a.Sbx.PolicyLog(ctx)
	if err != nil {
		return
	}
	sig := trafficSignature(log)
	a.mu.Lock()
	changed := sig != a.lastLogSig
	a.lastLogSig = sig
	var notifications []Event
	for _, e := range log.BlockedHosts {
		key := e.VMName + "\x00" + e.Host
		if a.seenBlocked[key] {
			continue
		}
		a.seenBlocked[key] = true
		if seed {
			continue
		}
		notifications = append(notifications, EventFor(TopicBlocked, BlockedEvent{
			Host:       e.Host,
			Sandbox:    e.VMName,
			ProxyType:  e.ProxyType,
			Rule:       e.Rule,
			CountSince: e.CountSince,
			At:         time.Now().UTC().Format(time.RFC3339),
		}))
	}
	a.mu.Unlock()

	for _, ev := range notifications {
		a.Hub.Publish(ev)
	}
	if changed {
		a.Notify(TopicTraffic)
	}
}

func (a *App) watchEvents(ctx context.Context) {
	types := []string{sbx.EventTypeLifecycle, sbx.EventTypePorts, sbx.EventTypePolicy}
	failures := 0
	for ctx.Err() == nil {
		received := false
		err := a.Sbx.StreamEvents(ctx, types, func(ev sbx.Event) error {
			received = true
			a.handleDaemonEvent(ctx, ev)
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		if err == nil || received {
			failures = 0
		} else {
			failures++
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(eventRetryDelay(failures)):
		}
	}
}

func (a *App) handleDaemonEvent(ctx context.Context, ev sbx.Event) {
	if strings.HasPrefix(ev.Type, "policy.") {
		a.checkBlocked(ctx, false)
		a.Notify(TopicPolicyRules)
		if ev.SandboxName != "" {
			a.Notify(TopicSandbox(ev.SandboxName))
		}
		return
	}
	if !strings.HasPrefix(ev.Type, "sandbox.") {
		return
	}
	a.Notify(TopicSandboxes)
	if ev.SandboxName != "" {
		a.Notify(TopicSandbox(ev.SandboxName))
	}
	if ev.Type == sbx.EventTypePorts || strings.HasPrefix(ev.Type, sbx.EventTypePorts+".") {
		return
	}
	if ev.SandboxName != "" && a.stateTransitioned(ctx, ev.SandboxName) && a.tryLead() {
		go a.applyWhenReady(a.jobContext(), ev.SandboxName)
	}
	a.requestReconcile()
}

const (
	eventRetryBase = 2 * time.Second
	eventRetryMax  = 30 * time.Second
)

func eventRetryDelay(failures int) time.Duration {
	if failures >= 5 {
		return eventRetryMax
	}
	return min(eventRetryBase<<failures, eventRetryMax)
}

var reconcileDebounce = 2 * time.Second

// reconcileLoop coalesces lifecycle bursts into single reconcile passes.
func (a *App) reconcileLoop(ctx context.Context) {
	defer a.stepDown()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.reconcileCh:
			time.Sleep(reconcileDebounce)
			drain(a.reconcileCh)
			if ctx.Err() != nil {
				return
			}
			if !a.tryLead() {
				continue
			}
			_ = a.reconcile(ctx, true)
			a.Notify(TopicSandboxes)
		}
	}
}

func (a *App) tryLead() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.leader != nil {
		return true
	}
	lock, err := fleet.AcquireLeaderLock(a.Fleet.Dir())
	if err != nil {
		return false
	}
	a.leader = lock
	return true
}

func (a *App) stepDown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = a.leader.Release()
	a.leader = nil
}

// trafficSignature fingerprints the proxy log so unchanged polls do not push
// redundant snapshots to every viewer.
func trafficSignature(log sbx.PolicyLog) string {
	var b strings.Builder
	for _, entries := range [][]sbx.LogEntry{log.BlockedHosts, log.AllowedHosts} {
		for _, e := range entries {
			b.WriteString(e.VMName)
			b.WriteByte(0)
			b.WriteString(e.Host)
			b.WriteByte(0)
			b.WriteString(e.LastSeen)
			b.WriteByte(0)
			b.WriteString(strconv.Itoa(e.CountSince))
			b.WriteByte(1)
		}
	}
	return b.String()
}

func (a *App) requestReconcile() {
	select {
	case a.reconcileCh <- struct{}{}:
	default:
	}
}

func drain(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// primeStates records the daemon status of every sandbox so the first event
// after startup does not look like a stopped → running transition.
func (a *App) primeStates(ctx context.Context) {
	sandboxes, err := a.Sbx.ListSandboxes(ctx)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastState == nil {
		a.lastState = map[string]string{}
	}
	for _, s := range sandboxes {
		a.lastState[s.Name] = s.Status
	}
}

// stateTransitioned records a sandbox's daemon state and reports a
// stopped → running transition, which needs its sidecar converged.
func (a *App) stateTransitioned(ctx context.Context, name string) bool {
	info, err := a.Sbx.InspectSandbox(ctx, name)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastState == nil {
		a.lastState = map[string]string{}
	}
	previous, seen := a.lastState[name]
	a.lastState[name] = info.Status
	return seen && previous != "running" && info.Status == "running"
}

// reconcileTicker is the slow backstop that converges external changes even
// when no daemon event fires.
func (a *App) reconcileTicker(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.requestReconcile()
		}
	}
}

// catalogSweepConcurrency bounds how many store checkouts run at once during
// the startup catalog sweep.
const catalogSweepConcurrency = 4

// repopulateCatalogs re-runs checkout and discovery for every registered store
// whose catalog has no rows or whose checkout directory disappeared, so a
// fresh or reset index heals itself without a manual refresh per store.
// Failures are recorded on the store's sync state, exactly like a manual
// Refresh, and never abort the sweep or startup.
func (a *App) repopulateCatalogs(ctx context.Context) {
	type storeSyncer struct {
		kind fleet.StoreKind
		sync func(context.Context, *fleet.StoreReg) (*fleet.StoreReg, error)
	}
	syncers := []storeSyncer{
		{kind: fleet.StoreSkills, sync: a.syncSkillStore},
		{kind: fleet.StoreKits, sync: a.syncKitStore},
	}
	sem := make(chan struct{}, catalogSweepConcurrency)
	var wg sync.WaitGroup
	stale := false
	for _, syncer := range syncers {
		counts, err := a.Store.CatalogSizes(ctx, string(syncer.kind))
		if err != nil {
			continue
		}
		for _, reg := range a.Fleet.Stores(syncer.kind) {
			if !catalogStale(syncer.kind, reg, counts[reg.Slug]) {
				continue
			}
			stale = true
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				_, _ = syncer.sync(ctx, reg)
			}()
		}
	}
	wg.Wait()
	if !stale {
		return
	}
	a.Notify(TopicSkills)
	a.Notify(TopicKits)
	a.requestReconcile()
}

// catalogStale reports whether a store needs a checkout and discovery pass:
// its catalog has no rows or its checkout directory is gone.
func catalogStale(kind fleet.StoreKind, reg *fleet.StoreReg, rows int) bool {
	if rows == 0 {
		return true
	}
	info, err := os.Stat(storeCheckoutPath(kind, reg.Slug))
	return err != nil || !info.IsDir()
}
