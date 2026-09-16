package sbx

import (
	"context"
	"testing"
)

func TestDaemonStatusParsesCLIOutput(t *testing.T) {
	newStubSbx(t, "Status: stopped\nSocket: /tmp/sandboxd.sock\nLogs: /tmp/daemon.log\n")
	status, err := New("/nonexistent.sock").DaemonStatus(context.Background())
	if err != nil {
		t.Fatalf("daemon status: %v", err)
	}
	if status.Running {
		t.Fatal("expected a stopped daemon")
	}
	if status.Socket != "/tmp/sandboxd.sock" || status.Logs != "/tmp/daemon.log" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestDaemonStatusRunning(t *testing.T) {
	newStubSbx(t, "Status: running\nSocket: /tmp/sandboxd.sock\n")
	status, err := New("/nonexistent.sock").DaemonStatus(context.Background())
	if err != nil {
		t.Fatalf("daemon status: %v", err)
	}
	if !status.Running {
		t.Fatalf("expected a running daemon: %+v", status)
	}
}

func TestStartDaemonRunsCLI(t *testing.T) {
	logPath := newStubSbx(t, "")
	if err := New("/nonexistent.sock").StartDaemon(context.Background()); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	calls := stubCalls(t, logPath)
	if len(calls) != 1 || calls[0] != "daemon start" {
		t.Fatalf("expected one 'daemon start' call, got %v", calls)
	}
}
