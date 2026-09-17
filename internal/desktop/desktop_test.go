package desktop

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
)

type fakeDaemon struct {
	mu        sync.Mutex
	sandboxes []string
	actions   []sbx.PolicyAction
}

func (f *fakeDaemon) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandbox":
			out := make([]map[string]any, 0, len(f.sandboxes))
			for _, name := range f.sandboxes {
				out = append(out, map[string]any{
					"id": name, "name": name, "status": "running", "workspace": "/w",
				})
			}
			_ = json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/sandbox/"):
			name := strings.TrimPrefix(r.URL.Path, "/sandbox/")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": name, "name": name, "status": "running", "workspace": "/w",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/policy/network/log":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"blocked_hosts": []map[string]any{
					{"host": "evil.example", "vm_name": "box", "proxy_type": "http", "rule": "default-deny", "last_seen": "2026-01-01T00:00:00Z", "count_since": 3},
					{"host": "other.example", "vm_name": "other", "proxy_type": "http", "rule": "default-deny", "last_seen": "2026-01-01T00:00:00Z", "count_since": 1},
				},
				"allowed_hosts": []map[string]any{},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/policy/network/rules":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rules": []map[string]any{
					{"id": "r1", "decision": "allow", "resources": []string{"ok.example"}, "scope": "global", "editable": true},
				},
			})
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
	f.actions = append(f.actions, body.Actions...)
	results := make([]map[string]any, 0, len(body.Actions))
	for _, action := range body.Actions {
		results = append(results, map[string]any{
			"action":    action.Action,
			"resources": action.Resources,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (f *fakeDaemon) appliedActions() []sbx.PolicyAction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sbx.PolicyAction(nil), f.actions...)
}

// newTestDesktop wires the binding service against a fake daemon over a unix
// socket, mirroring the old HTTP handler tests.
func newTestDesktop(t *testing.T, sandboxes ...string) (*Desktop, *app.App, *fakeDaemon) {
	t.Helper()
	fake := &fakeDaemon{sandboxes: sandboxes}
	socket := filepath.Join(t.TempDir(), "sandboxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	daemon := &http.Server{Handler: fake.handler()}
	go func() { _ = daemon.Serve(listener) }()
	t.Cleanup(func() { _ = daemon.Close() })

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	fl, err := fleet.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open fleet: %v", err)
	}
	core := app.New(sbx.New(socket), st, fl)
	return New(core, context.Background(), "test"), core, fake
}

func TestListSandboxes(t *testing.T) {
	d, _, _ := newTestDesktop(t, "box")

	summaries, err := d.ListSandboxes()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Name != "box" {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
	if summaries[0].Connect.Run != "sbx run --name box" {
		t.Fatalf("unexpected connect command: %+v", summaries[0].Connect)
	}
}

func TestSandboxTrafficIsFiltered(t *testing.T) {
	d, _, _ := newTestDesktop(t, "box", "other")

	log, err := d.SandboxTraffic("box")
	if err != nil {
		t.Fatalf("traffic: %v", err)
	}
	if len(log.BlockedHosts) != 1 || log.BlockedHosts[0].VMName != "box" {
		t.Fatalf("expected only box entries, got %+v", log.BlockedHosts)
	}
}

func TestSandboxPolicyActionIsScoped(t *testing.T) {
	d, _, fake := newTestDesktop(t, "box")

	if _, err := d.SandboxPolicyAction("box", "allow", []string{"api.example.com"}); err != nil {
		t.Fatalf("policy action: %v", err)
	}
	actions := fake.appliedActions()
	if len(actions) != 1 || actions[0].SandboxID != "box" || actions[0].Resources[0] != "api.example.com" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

func TestCreateSandboxRunsJob(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-sbx")
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	content := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argsFile + "\nprintf 'created\\n'\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", script)

	d, _, _ := newTestDesktop(t, "box")

	jobID, err := d.CreateSandbox(CreateSandboxRequest{
		Agent:      "shell",
		Name:       "new-box",
		CPUs:       2,
		Memory:     "4g",
		JobID:      "job-1",
		Publish:    []string{"8080:80"},
		Env:        []string{"A=B"},
		Workspaces: []WorkspaceSpec{{Path: "/tmp/work", ReadOnly: true}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if jobID != "job-1" {
		t.Fatalf("job id: got %q", jobID)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		job, err := d.Job("job-1")
		if err != nil {
			t.Fatalf("job: %v", err)
		}
		if job.Status != "running" {
			if job.Status != "done" {
				t.Fatalf("job failed: %+v", job)
			}
			if !strings.Contains(job.Output, "created") {
				t.Fatalf("expected output to be captured, got %q", job.Output)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for job")
		}
		time.Sleep(20 * time.Millisecond)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.TrimSpace(string(args))
	for _, want := range []string{"create", "shell", "/tmp/work:ro", "--name new-box", "--cpus 2", "--memory 4g", "-p 8080:80", "-e A=B"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected args to contain %q, got %q", want, got)
		}
	}
}

func TestBridgeEmitsHubEnvelopes(t *testing.T) {
	d, core, _ := newTestDesktop(t, "box")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan app.Event, 4)
	go Bridge(ctx, d, func(event app.Event) { events <- event })

	deadline := time.Now().Add(3 * time.Second)
	for {
		core.Notify(app.TopicSandboxes)
		select {
		case event := <-events:
			if event.Topic != app.TopicSandboxes {
				t.Fatalf("unexpected topic: %q", event.Topic)
			}
			sums, ok := event.Data.([]app.SandboxSummary)
			if !ok || len(sums) != 1 || sums[0].Name != "box" {
				t.Fatalf("unexpected payload: %#v", event.Data)
			}
			return
		case <-time.After(50 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for hub event")
			}
		}
	}
}

func TestCachesCreateAttachDetach(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-sbx")
	content := "#!/bin/sh\ncase \"$1\" in inspect) printf '{\"runtime_mounts\":[]}' ;; esac\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", script)

	d, _, _ := newTestDesktop(t, "box")

	cache, err := d.CreateCache(CacheInput{
		Name: "go-mod", HostPath: "/tmp/cache/go", TargetPath: "/home/agent/go/pkg/mod", AutoAttach: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}

	if _, err := d.CreateCache(CacheInput{Name: "bad", HostPath: "relative", TargetPath: "/x", Enabled: true}); err == nil {
		t.Fatal("expected validation error for relative host path")
	}

	if err := d.AttachCache("box", cache.Slug); err != nil {
		t.Fatalf("attach: %v", err)
	}
	detail, err := d.SandboxDetail("box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Caches) != 1 || detail.Caches[0].Name != "go-mod" {
		t.Fatalf("unexpected detail caches: %+v", detail.Caches)
	}

	if err := d.DetachCache("box", cache.Slug); err != nil {
		t.Fatalf("detach: %v", err)
	}
}

func TestAssetMiddlewareServesSPAShell(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>shell</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}

	var gotPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	})
	handler := AssetMiddleware(assets)(next)

	cases := []struct {
		name     string
		path     string
		accept   string
		wantPath string
	}{
		{"deep link falls back to the shell", "/sandboxes/box", "text/html", "/"},
		{"static file is served as-is", "/assets/app.js", "*/*", "/assets/app.js"},
		{"runtime calls are never rewritten", "/wails/runtime", "text/html", "/wails/runtime"},
		{"root is served as-is", "/", "text/html", "/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPath = ""
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept", tc.accept)
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if gotPath != tc.wantPath {
				t.Fatalf("path: got %q, want %q", gotPath, tc.wantPath)
			}
		})
	}
}

func TestNotifierRejectsEmpty(t *testing.T) {
	notifier := NewNotifier()
	if err := notifier.Notify("  ", " "); err == nil {
		t.Fatal("expected an error for an empty notification")
	}
}
