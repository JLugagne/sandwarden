package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// TestDeleteSandboxClearsDaemonConfigAndLedger pins the whole delete flow:
// the daemon removal, the ledger cleanup and the config purge.
func TestDeleteSandboxClearsDaemonConfigAndLedger(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")
	if _, err := a.Fleet.CreateSandbox("box", fleet.NewMixin(""), fleet.SandboxApp{Sandbox: "box"}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := a.Store.RecordAppliedRule(ctx, "web-dev", "box", "r1", "api.example.com", "allow"); err != nil {
		t.Fatalf("record rule: %v", err)
	}

	if err := a.DeleteSandbox(ctx, "box", true, true); err != nil {
		t.Fatalf("DeleteSandbox: %v", err)
	}
	if fake.hasSandbox("box") {
		t.Fatal("daemon still lists the sandbox")
	}
	if _, ok := a.Fleet.SandboxByName("box"); ok {
		t.Fatal("config directory survived the purge")
	}
	rules, err := a.Store.ListAppliedRules(ctx, "web-dev", "box")
	if err != nil || len(rules) != 0 {
		t.Fatalf("ledger not cleared: %+v, %v", rules, err)
	}
}

// TestDeleteSandboxBoundsDaemonHang pins that a sandboxd that never answers a
// delete cannot leave the GUI spinning forever.
func TestDeleteSandboxBoundsDaemonHang(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")
	fake.setDeleteDelay(2 * time.Second)

	restore := deleteSandboxTimeout
	deleteSandboxTimeout = 200 * time.Millisecond
	defer func() { deleteSandboxTimeout = restore }()

	start := time.Now()
	err := a.DeleteSandbox(ctx, "box", true, false)
	if err == nil {
		t.Fatal("expected a bounded delete to fail")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("DeleteSandbox blocked for %s", elapsed)
	}
	if !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("error = %v, want the daemon-timeout explanation", err)
	}
}
