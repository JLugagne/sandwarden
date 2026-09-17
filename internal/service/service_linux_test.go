//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func linuxSpec() Spec {
	return Spec{
		Executable: "/usr/local/bin/sandwarden",
		Args:       []string{"watch", "--config", "/home/me/.config/sandwarden"},
	}
}

func newTestManager(t *testing.T) (*Manager, *fakeRunner, string) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	run := newFakeRunner()
	return New(run.run), run, filepath.Join(configHome, "systemd", "user", systemdUnitName)
}

func writeUnit(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("previous unit\n"), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}
}

func TestLinuxInstallWritesUnitAndEnablesIt(t *testing.T) {
	manager, run, unitPath := newTestManager(t)
	report, err := manager.Install(context.Background(), linuxSpec())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	raw, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("unit not written: %v", err)
	}
	if string(raw) != renderSystemdUnit(linuxSpec()) {
		t.Fatalf("unexpected unit content:\n%s", raw)
	}
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + systemdUnitName,
	}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	if report.Previous {
		t.Fatal("fresh install reported a previous install")
	}
	if !report.Installed {
		t.Fatal("report does not mark the service installed")
	}
	if report.UnitPath != unitPath || report.Platform != "systemd --user" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestLinuxInstallReinstallsAndRestarts(t *testing.T) {
	manager, run, unitPath := newTestManager(t)
	writeUnit(t, unitPath)
	report, err := manager.Install(context.Background(), linuxSpec())
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if !report.Previous {
		t.Fatal("reinstall did not report the previous install")
	}
	want := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + systemdUnitName,
		"systemctl --user restart " + systemdUnitName,
	}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestLinuxInstallFailsClearlyWithoutSystemd(t *testing.T) {
	manager, run, _ := newTestManager(t)
	run.fail["systemctl --user daemon-reload"] = errors.New("Failed to connect to bus: No such file or directory")
	_, err := manager.Install(context.Background(), linuxSpec())
	if err == nil {
		t.Fatal("expected an error without a systemd user session")
	}
	for _, want := range []string{"systemd user session", "Failed to connect to bus"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestLinuxStatusNotInstalled(t *testing.T) {
	manager, run, _ := newTestManager(t)
	st, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Installed || st.Enabled || st.Running {
		t.Fatalf("unexpected status: %+v", st)
	}
	if len(run.calls) != 0 {
		t.Fatalf("status ran commands for a missing unit: %v", run.sequence())
	}
}

func TestLinuxStatusParsesSystemctl(t *testing.T) {
	tests := []struct {
		name       string
		enabled    string
		enabledErr error
		active     string
		activeErr  error
		enabledNow bool
		runningNow bool
		detail     string
	}{
		{name: "enabled and active", enabled: "enabled\n", active: "active\n", enabledNow: true, runningNow: true, detail: "enabled and running"},
		{name: "disabled and inactive", enabled: "disabled\n", enabledErr: errors.New("exit status 1"), active: "inactive\n", activeErr: errors.New("exit status 1"), detail: "disabled and not running"},
		{name: "enabled but failed", enabled: "enabled\n", active: "failed\n", activeErr: errors.New("exit status 3"), enabledNow: true, detail: "enabled but not running"},
		{name: "running but disabled", enabled: "disabled\n", enabledErr: errors.New("exit status 1"), active: "active\n", runningNow: true, detail: "running but not enabled at login"},
		{name: "systemctl unavailable", enabledErr: errors.New("systemctl not found in PATH"), activeErr: errors.New("systemctl not found in PATH"), detail: "state unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, run, unitPath := newTestManager(t)
			writeUnit(t, unitPath)
			run.outputs["systemctl --user is-enabled "+systemdUnitName] = tt.enabled
			run.fail["systemctl --user is-enabled "+systemdUnitName] = tt.enabledErr
			run.outputs["systemctl --user is-active "+systemdUnitName] = tt.active
			run.fail["systemctl --user is-active "+systemdUnitName] = tt.activeErr
			st, err := manager.Status(context.Background())
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if !st.Installed {
				t.Fatal("unit file present but status says not installed")
			}
			if st.Enabled != tt.enabledNow || st.Running != tt.runningNow {
				t.Fatalf("status = %+v, want enabled=%v running=%v", st, tt.enabledNow, tt.runningNow)
			}
			if !strings.Contains(st.Detail, tt.detail) {
				t.Fatalf("detail = %q, want it to contain %q", st.Detail, tt.detail)
			}
		})
	}
}

func TestLinuxUninstallStopsAndRemoves(t *testing.T) {
	manager, run, unitPath := newTestManager(t)
	writeUnit(t, unitPath)
	report, err := manager.Uninstall(context.Background())
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	want := []string{
		"systemctl --user disable --now " + systemdUnitName,
		"systemctl --user daemon-reload",
	}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file still present (err=%v)", err)
	}
	if !report.Previous || !report.Removed {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestLinuxUninstallWithoutInstallIsNoop(t *testing.T) {
	manager, run, _ := newTestManager(t)
	report, err := manager.Uninstall(context.Background())
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(run.calls) != 0 {
		t.Fatalf("uninstall ran commands with no unit installed: %v", run.sequence())
	}
	if report.Previous || report.Removed {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestLinuxUninstallRemovesFileWhenSystemctlUnavailable(t *testing.T) {
	manager, run, unitPath := newTestManager(t)
	writeUnit(t, unitPath)
	run.fail["systemctl --user disable --now "+systemdUnitName] = errors.New("systemctl not found in PATH")
	run.fail["systemctl --user daemon-reload"] = errors.New("systemctl not found in PATH")
	report, err := manager.Uninstall(context.Background())
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file still present (err=%v)", err)
	}
	if len(report.Warnings) != 2 {
		t.Fatalf("warnings = %v, want two entries", report.Warnings)
	}
	if !report.Removed {
		t.Fatalf("report does not mark the file removed: %+v", report)
	}
}
