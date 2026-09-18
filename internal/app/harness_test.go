package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

type fakeDaemon struct {
	mu            sync.Mutex
	actions       []sbx.PolicyAction
	nextRule      int
	sandboxes     []string
	scopes        map[string]string
	existingRules map[string]string
	deleteDelay   time.Duration
	// statuses overrides the reported status per sandbox; absent means running.
	statuses map[string]string
	// stopDelay stalls a stop request so tests can observe the in-flight window.
	stopDelay time.Duration
	// stopEntered receives the name of each sandbox whose stop request arrived.
	stopEntered chan string
}

func (f *fakeDaemon) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandbox":
			var out []map[string]string
			for _, name := range f.sandboxes {
				out = append(out, map[string]string{"id": name, "name": name, "status": f.statusOf(name), "workspace": "/w"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": name, "name": name, "status": f.statusOf(name), "workspace": "/w"})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			f.handleDelete(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop"):
			f.handleStop(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			f.handleStart(w, r)
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
		existing := make([]map[string]string, 0)
		for _, resource := range action.Resources {
			if id, ok := f.existingRules[action.Action+"\x00"+resource]; ok {
				existing = append(existing, map[string]string{"resource": resource, "rule_id": id})
				continue
			}
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
			"existing":   existing,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

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

func (f *fakeDaemon) appliedActions() []sbx.PolicyAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sbx.PolicyAction(nil), f.actions...)
}

func (f *fakeDaemon) ruleExists(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.scopes[id]
	return ok
}

func (f *fakeDaemon) setExistingRules(rules map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.existingRules = rules
}

// newTestApp starts a fake sandboxd socket and a fresh config directory.
func newTestApp(t *testing.T, sandboxes ...string) (*App, *fakeDaemon) {
	t.Helper()
	fake := &fakeDaemon{sandboxes: sandboxes, statuses: map[string]string{}, scopes: map[string]string{}, existingRules: map[string]string{}}
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

	fl, err := fleet.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open fleet: %v", err)
	}
	return New(sbx.New(socket), st, fl), fake
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

// newSkillsStub installs a fake sbx CLI that keeps a runtime mount table in a
// state file: `inspect` renders it as JSON, `mount` and `umount` mutate it, and
// every call is appended to a log. It returns the log path.
func newSkillsStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	statePath := filepath.Join(dir, "mounts")
	if err := os.WriteFile(statePath, nil, 0o644); err != nil {
		t.Fatalf("state file: %v", err)
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$SBX_STUB_LOG"
if [ "$1" = inspect ]; then
  printf '{"workspace":"/w","runtime_mounts":['
  first=1
  while IFS='|' read -r h t; do
    if [ -z "$h" ]; then continue; fi
    if [ "$first" = 0 ]; then printf ','; fi
    first=0
    printf '{"host_path":"%s","container_target":"%s","read_only":true}' "$h" "$t"
  done < "$SBX_STUB_STATE"
  printf ']}\n'
  exit 0
fi
if [ "$1" = mount ]; then
  spec="$3"
  case "$spec" in
    *:ro) spec="${spec%:ro}" ;;
    *:rw) spec="${spec%:rw}" ;;
  esac
  host="${spec%%:*}"
  rest="${spec#*:}"
  if [ "$rest" = "$spec" ]; then target="$host"; else target="$rest"; fi
  printf '%s|%s\n' "$host" "$target" >> "$SBX_STUB_STATE"
  exit 0
fi
if [ "$1" = umount ]; then
  spec="$3"
  host="${spec%%:*}"
  rest="${spec#*:}"
  if [ "$rest" = "$spec" ]; then target="$host"; else target="$rest"; fi
  grep -vF "$host|$target" "$SBX_STUB_STATE" > "$SBX_STUB_STATE.tmp" 2>/dev/null
  mv "$SBX_STUB_STATE.tmp" "$SBX_STUB_STATE" 2>/dev/null
  exit 0
fi
exit 0
`
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	t.Setenv("SBX_STUB_LOG", logPath)
	t.Setenv("SBX_STUB_STATE", statePath)
	return logPath
}

func skillCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

func countCalls(calls []string, prefix string) int {
	n := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

// handleDelete removes a sandbox from the fake daemon, optionally after a
// delay so tests can exercise client-side timeouts.
func (f *fakeDaemon) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
	f.mu.Lock()
	delay := f.deleteDelay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	f.mu.Lock()
	for i, s := range f.sandboxes {
		if s == name {
			f.sandboxes = append(f.sandboxes[:i], f.sandboxes[i+1:]...)
			break
		}
	}
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "status": "deleted"})
}

// setDeleteDelay makes the fake daemon stall delete requests.
func (f *fakeDaemon) setDeleteDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteDelay = d
}

// hasSandbox reports whether the fake daemon still lists a sandbox.
func (f *fakeDaemon) hasSandbox(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sandboxes {
		if s == name {
			return true
		}
	}
	return false
}

// statusOf returns the overridden status of a sandbox, defaulting to running.
func (f *fakeDaemon) statusOf(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status, ok := f.statuses[name]; ok {
		return status
	}
	return "running"
}

// handleStop models the real daemon: the sandbox keeps reporting running for
// the whole stop, then flips to stopped. The optional delay widens that window
// so tests can exercise operations that run while a stop is in flight.
func (f *fakeDaemon) handleStop(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/sandbox/"), "/stop")
	f.mu.Lock()
	entered := f.stopEntered
	delay := f.stopDelay
	f.mu.Unlock()
	if entered != nil {
		select {
		case entered <- name:
		default:
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	f.mu.Lock()
	f.statuses[name] = "stopped"
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "status": "stopped"})
}

// handleStart clears the stopped override, mirroring a real boot.
func (f *fakeDaemon) handleStart(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/sandbox/"), "/start")
	f.mu.Lock()
	f.statuses[name] = "running"
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "status": "running"})
}
