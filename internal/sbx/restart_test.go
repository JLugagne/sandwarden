package sbx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// restartStderr is the exact sbx output when a command refuses to run because
// the upgraded CLI cannot drive the old sandboxd non-interactively.
const restartStderr = `Client Version:  v0.54.1 3df896d5784200e8dfe6dca28ebd7102fe064169
Server Version:  v0.43.0 79805a6e3c6667520dc2da4f6bdeddae9b700969
warning: Docker Sandboxes has been updated and needs to restart; all running sandboxes will be stopped
error: ensure daemon: cannot prompt for restart: stdin is not a terminal; run the command in an interactive terminal to confirm the restart
  try: sbx daemon restart
`

// newRestartSbx installs a fake sbx that fails every command with stderr and a
// non-zero exit, as the real CLI does when a restart is pending.
func newRestartSbx(t *testing.T, stderr string) *Client {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\ncat <<'EOF'\n" + stderr + "EOF\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv(envBinary, bin)
	return New("/nonexistent.sock")
}

func TestRunCLIReturnsDaemonRestartRequired(t *testing.T) {
	client := newRestartSbx(t, restartStderr)

	_, err := client.KitInspect(context.Background(), "example")
	if !errors.Is(err, ErrDaemonRestartRequired) {
		t.Fatalf("err = %v, want ErrDaemonRestartRequired", err)
	}
	var restartErr *DaemonRestartError
	if !errors.As(err, &restartErr) {
		t.Fatalf("err = %T, want *DaemonRestartError", err)
	}
	if got := restartErr.ClientVersion.String(); got != "v0.54.1" {
		t.Fatalf("client version = %q, want v0.54.1", got)
	}
	if got := restartErr.ServerVersion.String(); got != "v0.43.0" {
		t.Fatalf("server version = %q, want v0.43.0", got)
	}
	want := "sbx was updated (CLI v0.54.1, daemon v0.43.0) and sandboxd must restart; run `sbx daemon restart` (this stops running sandboxes)"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRunCLIDaemonRestartRequiredWithoutVersions(t *testing.T) {
	stderr := "warning: Docker Sandboxes has been updated and needs to restart; all running sandboxes will be stopped\n" +
		"error: ensure daemon: cannot prompt for restart: stdin is not a terminal\n"
	client := newRestartSbx(t, stderr)

	_, err := client.runCLI(context.Background(), nil, nil, "secret", "ls")
	if !errors.Is(err, ErrDaemonRestartRequired) {
		t.Fatalf("err = %v, want ErrDaemonRestartRequired", err)
	}
	if !strings.Contains(err.Error(), "sbx was updated and sandboxd must restart") {
		t.Fatalf("error = %q, want the generic restart message", err.Error())
	}
}

func TestRunCLILeavesOtherFailuresUntyped(t *testing.T) {
	client := newRestartSbx(t, "error: sandbox not found\n")

	_, err := client.runCLI(context.Background(), nil, nil, "inspect", "missing")
	if err == nil {
		t.Fatal("runCLI should fail")
	}
	if errors.Is(err, ErrDaemonRestartRequired) {
		t.Fatalf("unrelated failure was typed as a restart error: %v", err)
	}
}

func TestDaemonRestartNotice(t *testing.T) {
	for _, output := range []string{
		"warning: Docker Sandboxes has been updated and needs to restart; all running sandboxes will be stopped",
		"error: ensure daemon: cannot prompt for restart: stdin is not a terminal",
	} {
		if !daemonRestartNotice(output) {
			t.Fatalf("matcher missed %q", output)
		}
	}
	if daemonRestartNotice("error: sandbox not found") {
		t.Fatal("matcher matched an unrelated failure")
	}
}
