package sbx

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

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

func newUnixServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	socket := filepath.Join(shortTempDir(t), "sandboxd.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socket
}

func TestListSandboxes(t *testing.T) {
	socket := newUnixServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sandbox" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"1","name":"box","status":"running","workspace":"/w","agent":"claude","additional_workspaces":[{"dir":"/data","read_only":true}],"ports":[{"host_ip":"127.0.0.1","host_port":8080,"protocol":"tcp","sandbox_port":80}]}]`))
	})

	client := New(socket)
	got, err := client.ListSandboxes(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 sandbox, got %d", len(got))
	}
	sb := got[0]
	if sb.Name != "box" || !sb.Running() || sb.AgentName() != "claude" {
		t.Fatalf("unexpected sandbox: %+v", sb)
	}
	if len(sb.AdditionalWorkspaces) != 1 || !sb.AdditionalWorkspaces[0].IsReadOnly() {
		t.Fatalf("unexpected workspaces: %+v", sb.AdditionalWorkspaces)
	}
	if len(sb.Ports) != 1 || sb.Ports[0].HostPort != 8080 {
		t.Fatalf("unexpected ports: %+v", sb.Ports)
	}
}

func TestInspectNotFound(t *testing.T) {
	socket := newUnixServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"sandbox \"x\" not found"}`))
	})

	_, err := New(socket).InspectSandbox(context.Background(), "x")
	if !IsNotFound(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message == "" {
		t.Fatalf("expected APIError with message, got %#v", err)
	}
}

func TestModifyPolicy(t *testing.T) {
	var body struct {
		Actions []PolicyAction `json:"actions"`
	}
	socket := newUnixServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/policy/network/rules" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"results":[{"action":"allow","resources":["example.com"],"created":[{"resource":"example.com","rule_id":"r1"}]}]}`))
	})

	results, err := New(socket).ModifyPolicy(context.Background(), PolicyAction{Action: "allow", Resources: []string{"example.com"}, SandboxID: "box"})
	if err != nil {
		t.Fatalf("modify: %v", err)
	}
	if len(body.Actions) != 1 || body.Actions[0].SandboxID != "box" {
		t.Fatalf("unexpected request body: %+v", body)
	}
	if len(results) != 1 || results[0].Failed() || results[0].Created[0].RuleID != "r1" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestStreamEvents(t *testing.T) {
	socket := newUnixServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query()["type"]; len(got) != 1 || got[0] != EventTypeLifecycle {
			t.Errorf("unexpected type filter: %v", got)
		}
		_, _ = w.Write([]byte(`{"action":"created","id":"e1","type":"sandbox.lifecycle.created"}` + "\n"))
		_, _ = w.Write([]byte(`{"action":"complete","id":"e2","type":"sync"}` + "\n"))
	})

	var types []string
	err := New(socket).StreamEvents(context.Background(), []string{EventTypeLifecycle}, func(ev Event) error {
		types = append(types, ev.Type)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(types) != 2 || types[0] != "sandbox.lifecycle.created" || types[1] != "sync" {
		t.Fatalf("unexpected events: %v", types)
	}
}

func TestSocketPathEnvOverride(t *testing.T) {
	t.Setenv(envSocketPath, "/custom/socket.sock")
	if got := SocketPath(); got != "/custom/socket.sock" {
		t.Fatalf("expected env override, got %q", got)
	}
}
