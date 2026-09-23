package app

import (
	"context"
	"slices"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestAddRulesToProfileInOnePass(t *testing.T) {
	ctx := context.Background()
	a, _ := seedLockTest(t)

	err := a.AddRulesToProfile(ctx, "web-dev", "allow", []string{"a.example.com", " b.example.com ", "a.example.com", "telemetry.example.com", ""})
	if err != nil {
		t.Fatalf("add rules: %v", err)
	}

	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	p, _ := fresh.Profile("web-dev")
	allow := p.Spec.NetworkAllow()
	for _, host := range []string{"api.github.com", "a.example.com", "b.example.com", "telemetry.example.com"} {
		if !slices.Contains(allow, host) {
			t.Fatalf("allow list %v misses %s", allow, host)
		}
	}
	if len(allow) != 4 {
		t.Fatalf("allow list %v has duplicates", allow)
	}
	if deny := p.Spec.Permissions.Network.Deny; slices.Contains(deny, "telemetry.example.com") {
		t.Fatalf("host still denied after being allowed: %v", deny)
	}
}

func TestAddRulesToProfileRejectsEmptyBatch(t *testing.T) {
	a, _ := seedLockTest(t)
	if err := a.AddRulesToProfile(context.Background(), "web-dev", "allow", []string{" ", ""}); err == nil {
		t.Fatal("expected an error for an empty batch")
	}
}
