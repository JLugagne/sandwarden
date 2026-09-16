package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

// Start launches the background watchers. It returns immediately; they run until
// ctx is cancelled.
func (a *App) Start(ctx context.Context) {
	a.mu.Lock()
	a.rootCtx = ctx
	a.mu.Unlock()
	go a.watchEvents(ctx)
	go a.pollBlocked(ctx)
	go a.reconcileLoop(ctx)
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
	for ctx.Err() == nil {
		err := a.Sbx.StreamEvents(ctx, types, func(ev sbx.Event) error {
			if strings.HasPrefix(ev.Type, "policy.") {
				a.checkBlocked(ctx, false)
				a.Notify(TopicPolicyRules)
				if ev.SandboxName != "" {
					a.Notify(TopicSandbox(ev.SandboxName))
				}
				return nil
			}
			if strings.HasPrefix(ev.Type, "sandbox.") {
				a.Notify(TopicSandboxes)
				if ev.SandboxName != "" {
					a.Notify(TopicSandbox(ev.SandboxName))
				}
				a.requestReconcile()
			}
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		_ = err
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// reconcileLoop coalesces lifecycle bursts into single reconcile passes.
func (a *App) reconcileLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.reconcileCh:
			time.Sleep(500 * time.Millisecond)
			drain(a.reconcileCh)
			if ctx.Err() != nil {
				return
			}
			_ = a.Reconcile(ctx)
			a.Notify(TopicSandboxes)
		}
	}
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
