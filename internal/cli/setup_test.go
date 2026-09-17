package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeSystemctlScript is a stand-in for systemctl: it logs every invocation
// and answers the status questions from environment variables, so setup tests
// run in containers without a systemd session.
const fakeSystemctlScript = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_SYSTEMCTL_LOG"
if [ -n "$FAKE_SYSTEMCTL_FAIL" ]; then
  echo "Failed to connect to bus: No such file or directory" >&2
  exit 1
fi
case "$*" in
  *" is-enabled "*)
    printf '%s\n' "${FAKE_SYSTEMCTL_ENABLED:-enabled}"
    [ "${FAKE_SYSTEMCTL_ENABLED:-enabled}" = "disabled" ] && exit 1
    ;;
  *" is-active "*)
    printf '%s\n' "${FAKE_SYSTEMCTL_ACTIVE:-active}"
    [ "${FAKE_SYSTEMCTL_ACTIVE:-active}" = "inactive" ] && exit 1
    ;;
esac
exit 0
`

// newFakeSystemctl puts a fake systemctl first on PATH and returns its log.
func newFakeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "systemctl.log")
	bin := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(bin, []byte(fakeSystemctlScript), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_SYSTEMCTL_LOG", logPath)
	return logPath
}

func systemctlCalls(t *testing.T, logPath string) []string {
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

func executeCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	opts := Options{}
	root := New(&opts, func(*Options) error { return errors.New("unexpected GUI launch") })
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func currentExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

func requireSystemctlCall(t *testing.T, calls []string, want string) {
	t.Helper()
	for _, call := range calls {
		if call == want {
			return
		}
	}
	t.Errorf("missing systemctl call %q; got %q", want, calls)
}

func TestSetupInstallStatusUninstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake systemctl flow is Linux-specific")
	}
	logPath := newFakeSystemctl(t)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	unitPath := filepath.Join(configHome, "systemd", "user", "sandwarden-watch.service")

	stdout, stderr, err := executeCLI(t, "setup", "--config", "/tmp/fleet", "--socket", "/tmp/sandboxd.sock")
	if err != nil {
		t.Fatalf("setup: %v (stderr %s)", err, stderr)
	}
	mustContain(t, stdout, "Installed the sandwarden watch service (systemd --user)")
	mustContain(t, stdout, "unit: "+unitPath)
	mustContain(t, stdout, "exec: "+currentExecutable(t)+" watch --config /tmp/fleet --socket /tmp/sandboxd.sock")
	raw, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	mustContain(t, string(raw), "ExecStart="+currentExecutable(t)+" watch --config /tmp/fleet --socket /tmp/sandboxd.sock")
	mustContain(t, string(raw), "Restart=on-failure")
	mustContain(t, string(raw), "WantedBy=default.target")
	calls := systemctlCalls(t, logPath)
	requireSystemctlCall(t, calls, "--user daemon-reload")
	requireSystemctlCall(t, calls, "--user enable --now sandwarden-watch.service")

	stdout, stderr, err = executeCLI(t, "setup", "status")
	if err != nil {
		t.Fatalf("setup status: %v (stderr %s)", err, stderr)
	}
	for _, want := range []string{"installed: yes", "enabled:   yes", "running:   yes"} {
		mustContain(t, stdout, want)
	}

	stdout, stderr, err = executeCLI(t, "setup", "uninstall")
	if err != nil {
		t.Fatalf("setup uninstall: %v (stderr %s)", err, stderr)
	}
	mustContain(t, stdout, "Removed the sandwarden watch service (systemd --user)")
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file still present after uninstall (err=%v)", err)
	}
	calls = systemctlCalls(t, logPath)
	requireSystemctlCall(t, calls, "--user disable --now sandwarden-watch.service")

	stdout, stderr, err = executeCLI(t, "setup", "status")
	if err != nil {
		t.Fatalf("setup status after uninstall: %v (stderr %s)", err, stderr)
	}
	mustContain(t, stdout, "installed: no")
}

func TestSetupReinstallReportsPreviousInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake systemctl flow is Linux-specific")
	}
	logPath := newFakeSystemctl(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, stderr, err := executeCLI(t, "setup"); err != nil {
		t.Fatalf("first setup: %v (stderr %s)", err, stderr)
	}
	stdout, stderr, err := executeCLI(t, "setup")
	if err != nil {
		t.Fatalf("second setup: %v (stderr %s)", err, stderr)
	}
	mustContain(t, stdout, "Reinstalled the sandwarden watch service")
	requireSystemctlCall(t, systemctlCalls(t, logPath), "--user restart sandwarden-watch.service")
}

func TestSetupFailsWithoutSystemd(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake systemctl flow is Linux-specific")
	}
	newFakeSystemctl(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FAKE_SYSTEMCTL_FAIL", "1")
	_, _, err := executeCLI(t, "setup")
	if err == nil {
		t.Fatal("expected setup to fail without a systemd user session")
	}
	for _, want := range []string{"systemd user session", "Failed to connect to bus"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestSetupUninstallWithoutInstall(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the fake systemctl flow is Linux-specific")
	}
	newFakeSystemctl(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stdout, stderr, err := executeCLI(t, "setup", "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v (stderr %s)", err, stderr)
	}
	mustContain(t, stdout, "is not installed")
}

func TestWatchArgsIncludeOnlyProvidedFlags(t *testing.T) {
	got := watchArgs(&Options{ConfigDir: " /tmp/fleet ", Socket: "/tmp/sandboxd.sock"})
	want := []string{"watch", "--config", "/tmp/fleet", "--socket", "/tmp/sandboxd.sock"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("watchArgs = %v, want %v", got, want)
	}
	if got := watchArgs(&Options{}); len(got) != 1 || got[0] != "watch" {
		t.Fatalf("watchArgs = %v, want just watch", got)
	}
}
