//go:build darwin

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

func darwinSpec() Spec {
	return Spec{
		Executable: "/Applications/Sandwarden.app/Contents/MacOS/sandwarden",
		Args:       []string{"watch"},
	}
}

func newDarwinTestManager(t *testing.T) (*Manager, *fakeRunner, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	run := newFakeRunner()
	path := filepath.Join(home, "Library", "LaunchAgents", LaunchdLabel+".plist")
	return New(run.run), run, path
}

func launchdTarget() string { return launchdDomain() + "/" + LaunchdLabel }

func writeAgent(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("previous plist\n"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
}

func TestDarwinInstallWritesPlistAndLoadsIt(t *testing.T) {
	manager, run, plistPath := newDarwinTestManager(t)
	report, err := manager.Install(context.Background(), darwinSpec())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	raw, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if string(raw) != renderLaunchdPlist(darwinSpec()) {
		t.Fatalf("unexpected plist content:\n%s", raw)
	}
	want := []string{
		"launchctl bootstrap " + launchdDomain() + " " + plistPath,
		"launchctl kickstart -k " + launchdTarget(),
	}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	if report.Previous || !report.Installed {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestDarwinInstallReplacesPreviousAgent(t *testing.T) {
	manager, run, plistPath := newDarwinTestManager(t)
	writeAgent(t, plistPath)
	report, err := manager.Install(context.Background(), darwinSpec())
	if err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if !report.Previous {
		t.Fatal("reinstall did not report the previous install")
	}
	want := []string{
		"launchctl bootout " + launchdTarget(),
		"launchctl bootstrap " + launchdDomain() + " " + plistPath,
		"launchctl kickstart -k " + launchdTarget(),
	}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestDarwinStatus(t *testing.T) {
	manager, run, plistPath := newDarwinTestManager(t)
	st, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Installed || len(run.calls) != 0 {
		t.Fatalf("status without a plist: %+v, calls %v", st, run.sequence())
	}
	writeAgent(t, plistPath)
	run.outputs["launchctl print "+launchdTarget()] = "state = running\npid = 42\n"
	st, err = manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Installed || !st.Enabled || !st.Running || !strings.Contains(st.Detail, "running") {
		t.Fatalf("unexpected status: %+v", st)
	}
	run.outputs["launchctl print "+launchdTarget()] = "state = exited\n"
	st, err = manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Enabled || st.Running {
		t.Fatalf("unexpected status: %+v", st)
	}
	run.fail["launchctl print "+launchdTarget()] = errors.New("Could not find service")
	st, err = manager.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Enabled || st.Running || !strings.Contains(st.Detail, "not loaded") {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestDarwinUninstall(t *testing.T) {
	manager, run, plistPath := newDarwinTestManager(t)
	report, err := manager.Uninstall(context.Background())
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if report.Previous || len(run.calls) != 0 {
		t.Fatalf("unexpected report %+v, calls %v", report, run.sequence())
	}
	writeAgent(t, plistPath)
	report, err = manager.Uninstall(context.Background())
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	want := []string{"launchctl bootout " + launchdTarget()}
	if got := run.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Fatalf("plist still present (err=%v)", err)
	}
	if !report.Previous || !report.Removed {
		t.Fatalf("unexpected report: %+v", report)
	}
}
