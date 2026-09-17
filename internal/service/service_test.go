package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}

// fakeRunner records commands and answers from scripted tables, so no test
// ever reaches the real systemd.
type fakeRunner struct {
	calls   []call
	outputs map[string]string
	fail    map[string]error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{}, fail: map[string]error{}}
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	key := name + " " + strings.Join(args, " ")
	return f.outputs[key], f.fail[key]
}

func (f *fakeRunner) sequence() []string {
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.name+" "+strings.Join(c.args, " "))
	}
	return out
}

func TestRenderSystemdUnit(t *testing.T) {
	spec := Spec{
		Executable: "/usr/local/bin/sandwarden",
		Args:       []string{"watch", "--config", "/home/me/.config/sandwarden"},
	}
	want := `[Unit]
Description=sandwarden convergence loop (sandwarden watch)
Documentation=https://github.com/JLugagne/sandwarden/blob/main/docs/service.md

[Service]
Type=simple
ExecStart=/usr/local/bin/sandwarden watch --config /home/me/.config/sandwarden
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`
	if got := renderSystemdUnit(spec); got != want {
		t.Fatalf("systemd unit mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderSystemdUnitEscapesArguments(t *testing.T) {
	spec := Spec{
		Executable: "/opt/sand warden/bin/sandwarden",
		Args:       []string{"watch", "--config", `/home/me/100% config`},
	}
	got := renderSystemdUnit(spec)
	for _, want := range []string{
		`ExecStart="/opt/sand warden/bin/sandwarden" watch --config "/home/me/100%% config"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExecStart missing %q:\n%s", want, got)
		}
	}
}

func TestRenderLaunchdPlist(t *testing.T) {
	spec := Spec{
		Executable: "/Applications/Sandwarden.app/Contents/MacOS/sandwarden",
		Args:       []string{"watch", "--config", "/Users/me/Library/Application Support/sandwarden"},
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.sandwarden.watch</string>
	<key>ProgramArguments</key>
	<array>
		<string>/Applications/Sandwarden.app/Contents/MacOS/sandwarden</string>
		<string>watch</string>
		<string>--config</string>
		<string>/Users/me/Library/Application Support/sandwarden</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`
	if got := renderLaunchdPlist(spec); got != want {
		t.Fatalf("launchd plist mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderLaunchdPlistEscapesXML(t *testing.T) {
	got := renderLaunchdPlist(Spec{Executable: `/Users/a&b/bin/sandwarden<1>`})
	if !strings.Contains(got, `/Users/a&amp;b/bin/sandwarden&lt;1&gt;`) {
		t.Fatalf("XML not escaped:\n%s", got)
	}
}

func TestExecRunnerReportsMissingBinary(t *testing.T) {
	_, err := execRunner(context.Background(), "sandwarden-no-such-binary-xyz")
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecRunnerUsesPathAndReportsOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-systemctl")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\necho boom >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := execRunner(context.Background(), "fake-systemctl", "--user", "is-active", "x")
	if err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
	if !strings.Contains(out, "--user is-active x") {
		t.Fatalf("combined output not returned: %q", out)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error does not carry the command output: %v", err)
	}
}

func TestUnsupportedErrorNamesPlatform(t *testing.T) {
	err := unsupportedError("windows")
	for _, want := range []string{"windows", "systemd", "launchd"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}
