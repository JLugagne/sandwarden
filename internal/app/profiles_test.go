package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

type fakeDaemon struct {
	mu        sync.Mutex
	actions   []sbx.PolicyAction
	nextRule  int
	sandboxes []string
	scopes    map[string]string
}

func (f *fakeDaemon) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandbox":
			var out []map[string]string
			for _, name := range f.sandboxes {
				out = append(out, map[string]string{"id": name, "name": name, "status": "running", "workspace": "/w"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": name, "name": name, "status": "running", "workspace": "/w"})
		case r.Method == http.MethodPost && r.URL.Path == "/policy/network/rules":
			f.handlePolicy(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func (f *fakeDaemon) handlePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Actions []sbx.PolicyAction `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	results := make([]map[string]any, 0, len(body.Actions))
	for _, action := range body.Actions {
		f.actions = append(f.actions, action)
		if action.Action == "remove-id" {
			results = append(results, f.removeRule(action))
			continue
		}
		created := make([]map[string]string, 0, len(action.Resources))
		for _, resource := range action.Resources {
			f.nextRule++
			id := fmt.Sprintf("r%d", f.nextRule)
			f.scopes[id] = action.SandboxID
			created = append(created, map[string]string{"resource": resource, "rule_id": id})
		}
		results = append(results, map[string]any{
			"action":     action.Action,
			"resources":  action.Resources,
			"sandbox_id": action.SandboxID,
			"created":    created,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (f *fakeDaemon) appliedActions() []sbx.PolicyAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sbx.PolicyAction(nil), f.actions...)
}

func newTestApp(t *testing.T, sandboxes ...string) (*App, *fakeDaemon) {
	t.Helper()
	fake := &fakeDaemon{sandboxes: sandboxes, scopes: map[string]string{}}
	socket := filepath.Join(t.TempDir(), "sandboxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: fake.handler()}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	return New(sbx.New(socket), st), fake
}

func TestApplyProfileCompilesScopedRules(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	p, err := a.CreateProfile(ctx, "dev", "", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := a.AddRuleToProfile(ctx, p.ID, "allow", "example.com"); err != nil {
		t.Fatalf("add allow: %v", err)
	}
	if _, err := a.AddRuleToProfile(ctx, p.ID, "deny", "evil.com"); err != nil {
		t.Fatalf("add deny: %v", err)
	}
	if !p.IsGlobal {
		// no global apply expected yet
	}

	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var allow, deny *sbx.PolicyAction
	for i := range fake.appliedActions() {
		act := fake.appliedActions()[i]
		switch act.Action {
		case "allow":
			allow = &act
		case "deny":
			deny = &act
		}
	}
	if allow == nil || allow.SandboxID != "box" || allow.Resources[0] != "example.com" {
		t.Fatalf("unexpected allow action: %+v", allow)
	}
	if deny == nil || deny.SandboxID != "box" || deny.Resources[0] != "evil.com" {
		t.Fatalf("unexpected deny action: %+v", deny)
	}

	ledger, err := a.Store.ListAppliedRules(ctx, p.ID, "box")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if len(ledger) != 2 {
		t.Fatalf("expected 2 ledger rows, got %d", len(ledger))
	}
}

func TestApplyProfileIsConvergent(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	_, _ = a.AddRuleToProfile(ctx, p.ID, "allow", "example.com")
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	before := len(fake.appliedActions())

	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if got := len(fake.appliedActions()); got != before {
		t.Fatalf("re-apply should issue no new mutations, went from %d to %d", before, got)
	}
}

func TestUnapplyProfileRemovesRules(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	_, _ = a.AddRuleToProfile(ctx, p.ID, "allow", "example.com")
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := a.UnapplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("unapply: %v", err)
	}

	var removed int
	for _, act := range fake.appliedActions() {
		if act.Action == "remove-id" {
			removed++
		}
	}
	if removed != 1 {
		t.Fatalf("expected 1 remove-id action, got %d", removed)
	}
	ledger, _ := a.Store.ListAppliedRules(ctx, p.ID, "box")
	if len(ledger) != 0 {
		t.Fatalf("expected empty ledger, got %d", len(ledger))
	}
	profiles, _ := a.Store.ListProfilesForSandbox(ctx, "box")
	if len(profiles) != 0 {
		t.Fatalf("expected assignment removed, got %d", len(profiles))
	}
}

func TestReconcileAppliesGlobalAndDefault(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	glob, _ := a.CreateProfile(ctx, "global", "", false, true)
	_, _ = a.AddRuleToProfile(ctx, glob.ID, "allow", "global.com")

	def, _ := a.CreateProfile(ctx, "default", "", true, false)
	_, _ = a.AddRuleToProfile(ctx, def.ID, "allow", "default.com")

	// Reset recorded actions from the global profile's immediate apply.
	fake.mu.Lock()
	fake.actions = nil
	fake.mu.Unlock()

	if err := a.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var sawDefault bool
	for _, act := range fake.appliedActions() {
		if act.Action == "allow" && act.SandboxID == "box" && len(act.Resources) > 0 && act.Resources[0] == "default.com" {
			sawDefault = true
		}
	}
	globalLedger, err := a.Store.ListAppliedRules(ctx, glob.ID, "")
	if err != nil {
		t.Fatalf("global ledger: %v", err)
	}
	if len(globalLedger) != 1 || globalLedger[0].Pattern != "global.com" {
		t.Fatalf("expected global rule in ledger, got %+v", globalLedger)
	}
	if !sawDefault {
		t.Fatal("expected default profile applied to box")
	}

	profiles, _ := a.Store.ListProfilesForSandbox(ctx, "box")
	if len(profiles) != 1 || profiles[0].ID != def.ID {
		t.Fatalf("expected default profile assigned to box, got %+v", profiles)
	}
}

func TestUnapplyDefaultOptsOut(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	def, _ := a.CreateProfile(ctx, "default", "", true, false)
	if err := a.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if err := a.UnapplyProfile(ctx, "box", def.ID); err != nil {
		t.Fatalf("unapply: %v", err)
	}
	if err := a.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	profiles, _ := a.Store.ListProfilesForSandbox(ctx, "box")
	if len(profiles) != 0 {
		t.Fatalf("opted-out sandbox should not get the default profile back, got %+v", profiles)
	}
}

// removeRule mirrors sandboxd: a sandbox-scoped rule can only be removed when
// the action carries the same sandbox scope.
func (f *fakeDaemon) removeRule(action sbx.PolicyAction) map[string]any {
	scope, ok := f.scopes[action.ID]
	if !ok {
		return map[string]any{"action": action.Action, "id": action.ID, "sandbox_id": action.SandboxID,
			"error": fmt.Sprintf("remove-id: rule %q not found", action.ID)}
	}
	if scope != action.SandboxID {
		return map[string]any{"action": action.Action, "id": action.ID, "sandbox_id": action.SandboxID,
			"error": fmt.Sprintf("rule %s exists in sandbox %q, not in the global policy", action.ID, scope)}
	}
	delete(f.scopes, action.ID)
	return map[string]any{"action": action.Action, "id": action.ID, "sandbox_id": action.SandboxID}
}

func (f *fakeDaemon) ruleExists(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.scopes[id]
	return ok
}

func TestRemovePolicyRuleRequiresSandboxScope(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	results, err := a.ApplyPolicy(ctx, sbx.PolicyAction{Action: "allow", Resources: []string{"perdu.com:80"}, SandboxID: "box"})
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	ruleID := results[0].Created[0].RuleID

	if _, err := a.ApplyPolicy(ctx, sbx.PolicyAction{Action: "remove-id", ID: ruleID}); err == nil {
		t.Fatal("expected removing a sandbox-scoped rule without its scope to fail")
	}
	if !fake.ruleExists(ruleID) {
		t.Fatal("rule should still exist after the unscoped removal attempt")
	}

	if _, err := a.ApplyPolicy(ctx, sbx.PolicyAction{Action: "remove-id", ID: ruleID, SandboxID: "box"}); err != nil {
		t.Fatalf("scoped remove: %v", err)
	}
	if fake.ruleExists(ruleID) {
		t.Fatal("rule should be gone after the scoped removal")
	}
}

func TestUnapplyProfileRemovesScopedRulesFromDaemon(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	_, _ = a.AddRuleToProfile(ctx, p.ID, "allow", "example.com")
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	ledger, err := a.Store.ListAppliedRules(ctx, p.ID, "box")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if len(ledger) != 1 {
		t.Fatalf("expected 1 ledger row, got %d", len(ledger))
	}

	if err := a.UnapplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("unapply: %v", err)
	}
	if fake.ruleExists(ledger[0].RuleID) {
		t.Fatal("daemon rule should be removed when the profile is unapplied")
	}
}

func TestRemoveRuleFromProfilePrunesDaemonRule(t *testing.T) {
	ctx := context.Background()
	a, fake := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	rule, _ := a.AddRuleToProfile(ctx, p.ID, "allow", "example.com")
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	ledger, err := a.Store.ListAppliedRules(ctx, p.ID, "box")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if len(ledger) != 1 {
		t.Fatalf("expected 1 ledger row, got %d", len(ledger))
	}

	if err := a.RemoveRuleFromProfile(ctx, p.ID, rule.ID); err != nil {
		t.Fatalf("remove rule: %v", err)
	}
	if fake.ruleExists(ledger[0].RuleID) {
		t.Fatal("daemon rule should be removed when the profile rule is removed")
	}
}

func waitForTopic(t *testing.T, events <-chan Event, topic Topic) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Topic == topic {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for topic %q", topic)
		}
	}
}

func TestApplyProfileNotifiesSandboxDetail(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	p, err := a.CreateProfile(ctx, "dev", "", false, false)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	waitForTopic(t, events, TopicSandbox("box"))
}

func TestUnapplyProfileNotifiesSandboxDetail(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	p, _ := a.CreateProfile(ctx, "dev", "", false, false)
	if err := a.ApplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()
	if err := a.UnapplyProfile(ctx, "box", p.ID); err != nil {
		t.Fatalf("unapply: %v", err)
	}
	waitForTopic(t, events, TopicSandbox("box"))
}
