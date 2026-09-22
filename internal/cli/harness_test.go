package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

// fakeDaemon is a sandboxd stand-in served over a unix socket: it tracks
// sandbox state, records every request and applies policy actions in memory.
type fakeDaemon struct {
	mu        sync.Mutex
	socket    string
	sandboxes map[string]bool
	requests  []string
	actions   []sbx.PolicyAction
	nextRule  int
	scopes    map[string]string
}

// shortTempDir keeps unix socket paths inside the 104-byte limit macOS enforces.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newFakeDaemon(t *testing.T, running ...string) *fakeDaemon {
	t.Helper()
	f := &fakeDaemon{sandboxes: map[string]bool{}, scopes: map[string]string{}}
	for _, name := range running {
		f.sandboxes[name] = true
	}
	f.socket = filepath.Join(shortTempDir(t), "sandboxd.sock")
	listener, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: f.handler()}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return f
}

func (f *fakeDaemon) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandbox":
			f.mu.Lock()
			out := make([]map[string]string, 0, len(f.sandboxes))
			for name, running := range f.sandboxes {
				status := "stopped"
				if running {
					status = "running"
				}
				out = append(out, map[string]string{"id": name, "name": name, "status": status, "workspace": "/w"})
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodPost && r.URL.Path == "/policy/network/rules":
			f.handlePolicy(w, r)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			f.mu.Lock()
			_, ok := f.sandboxes[name]
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "sandbox " + name + " not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": name, "name": name, "status": "running", "workspace": "/w"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/sandbox/"), "/start")
			f.mu.Lock()
			f.sandboxes[name] = true
			f.mu.Unlock()
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			f.mu.Lock()
			delete(f.sandboxes, name)
			f.mu.Unlock()
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

func (f *fakeDaemon) called(method, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if req == method+" "+path {
			return true
		}
	}
	return false
}

func (f *fakeDaemon) appliedActions() []sbx.PolicyAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sbx.PolicyAction(nil), f.actions...)
}

// newFakeSbx installs a fake sbx CLI that logs every invocation and keeps a
// runtime mount table in a state file: `inspect` renders it as JSON, `mount`
// and `umount` mutate it, and creating a mount whose host path contains
// "broken" fails. It returns the log path.
func newFakeSbx(t *testing.T) string {
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
    *broken*) exit 1 ;;
  esac
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

// cliEnv wires a fake daemon, a fake sbx and private fleet/index directories.
type cliEnv struct {
	opts   Options
	daemon *fakeDaemon
	sbxLog string
}

func newCLIEnv(t *testing.T, running ...string) *cliEnv {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg-config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "xdg-state"))
	daemon := newFakeDaemon(t, running...)
	return &cliEnv{
		opts: Options{
			Socket:    daemon.socket,
			DB:        filepath.Join(t.TempDir(), "index.db"),
			ConfigDir: filepath.Join(t.TempDir(), "fleet"),
		},
		daemon: daemon,
		sbxLog: newFakeSbx(t),
	}
}

// execute runs one CLI invocation against the environment's daemon and paths.
func (e *cliEnv) execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := Options{}
	root := New(&opts, func(*Options) error { return errors.New("unexpected GUI launch") })
	opts.Socket, opts.DB, opts.ConfigDir = e.opts.Socket, e.opts.DB, e.opts.ConfigDir
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func (e *cliEnv) fleet(t *testing.T) *fleet.Fleet {
	t.Helper()
	fl, err := fleet.Open(e.opts.ConfigDir)
	if err != nil {
		t.Fatalf("open fleet: %v", err)
	}
	return fl
}

func (e *cliEnv) sbxCalls(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(e.sbxLog)
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

func (e *cliEnv) requireSbxCall(t *testing.T, want string) {
	t.Helper()
	for _, call := range e.sbxCalls(t) {
		if call == want {
			return
		}
	}
	t.Errorf("missing sbx call %q; got %q", want, e.sbxCalls(t))
}

func (e *cliEnv) requireNoSbxCall(t *testing.T, prefix string) {
	t.Helper()
	for _, call := range e.sbxCalls(t) {
		if strings.HasPrefix(call, prefix) {
			t.Errorf("unexpected sbx call %q", call)
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output does not contain %q:\n%s", needle, haystack)
	}
}

func mustCreateSandbox(t *testing.T, fl *fleet.Fleet, name string, app fleet.SandboxApp) *fleet.Sandbox {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	s, err := fl.CreateSandbox(name, spec, app)
	if err != nil {
		t.Fatalf("create sandbox %s: %v", name, err)
	}
	return s
}

func mustCreateProfile(t *testing.T, fl *fleet.Fleet, label string, allow []string, app fleet.ProfileApp) *fleet.Profile {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{Allow: allow}}
	p, err := fl.CreateProfile(label, spec, app)
	if err != nil {
		t.Fatalf("create profile %s: %v", label, err)
	}
	return p
}

// seedSkillStore registers one skill store, checks a skill out on disk and
// indexes it in the store so sandboxes can reference it. It returns the
// reference and the host path the sandbox should bind.
func seedSkillStore(t *testing.T, e *cliEnv, fl *fleet.Fleet, storeName, skillName string) (fleet.SkillRef, string) {
	t.Helper()
	reg, err := fl.CreateStore(fleet.StoreReg{
		Kind: fleet.StoreSkills,
		Name: storeName,
		URL:  "https://example.invalid/skills.git",
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	host := filepath.Join(os.Getenv("XDG_DATA_HOME"), "sandwarden", "skill-stores", reg.Slug, skillName)
	if err := os.MkdirAll(host, 0o755); err != nil {
		t.Fatalf("skill checkout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(host, "SKILL.md"), []byte("# "+skillName+"\n"), 0o644); err != nil {
		t.Fatalf("skill file: %v", err)
	}
	items := []store.SkillItem{{Store: reg.Slug, Kind: "skill", Name: skillName, RelPath: skillName}}
	previous := openApp
	openApp = func(ctx context.Context, opts *Options) (*app.App, func(), error) {
		core, closeApp, err := previous(ctx, opts)
		if err != nil {
			return nil, nil, err
		}
		if err := core.Store.ReplaceSkillItems(ctx, reg.Slug, items); err != nil {
			closeApp()
			return nil, nil, err
		}
		return core, closeApp, nil
	}
	t.Cleanup(func() { openApp = previous })
	return fleet.SkillRef{Store: reg.Slug, Kind: "skill", Name: skillName}, host
}
