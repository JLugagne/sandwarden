package sbx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStubSbx installs a fake sbx on PATH that records its argv to a log file and
// prints stdout. It returns the log path.
func newStubSbx(t *testing.T, stdout string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	outPath := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(outPath, []byte(stdout), 0o644); err != nil {
		t.Fatalf("write stub output: %v", err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SBX_STUB_LOG\"\ncat \"$SBX_STUB_OUT\" 2>/dev/null\nexit 0\n"
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("SBX_STUB_LOG", logPath)
	t.Setenv("SBX_STUB_OUT", outPath)
	return logPath
}

func stubCalls(t *testing.T, logPath string) []string {
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

func TestMountFolderArgs(t *testing.T) {
	logPath := newStubSbx(t, "")
	client := New("/nonexistent.sock")

	if err := client.MountFolder(context.Background(), "box", "/data/extra", true); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if err := client.MountFolder(context.Background(), "box", "/data/rw", false); err != nil {
		t.Fatalf("mount rw: %v", err)
	}
	if err := client.UnmountFolder(context.Background(), "box", "/data/extra", true); err != nil {
		t.Fatalf("umount: %v", err)
	}

	calls := stubCalls(t, logPath)
	want := []string{"mount box /data/extra::ro", "mount box /data/rw", "umount box /data/extra"}
	if len(calls) != len(want) {
		t.Fatalf("expected %d calls, got %v", len(want), calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call %d: want %q, got %q", i, want[i], calls[i])
		}
	}
}

func TestMountsParsesInspect(t *testing.T) {
	fixture := `{"workspace":"/w","runtime_mounts":[{"host_path":"/data/one","read_only":true},{"host_path":"/data/two"}]}`
	newStubSbx(t, fixture)

	mounts, err := New("/nonexistent.sock").Mounts(context.Background(), "box")
	if err != nil {
		t.Fatalf("mounts: %v", err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %+v", mounts)
	}
	if mounts[0].HostPath != "/data/one" || !mounts[0].ReadOnly {
		t.Fatalf("unexpected first mount: %+v", mounts[0])
	}
	if mounts[1].HostPath != "/data/two" || mounts[1].ReadOnly {
		t.Fatalf("unexpected second mount: %+v", mounts[1])
	}
}

func TestMountFolderSurfacesCLIError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\necho 'sandbox \"box\" is not running' 1>&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(envBinary, bin)

	err := New("/nonexistent.sock").MountFolder(context.Background(), "box", "/data", false)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected CLI message in error, got %v", err)
	}
}

func TestUnmountFolderBreaksStaleDeadlock(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	mark := filepath.Join(dir, "mounted")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"$SBX_STUB_LOG\"\n" +
		"if [ \"$1\" = umount ] && [ ! -f \"$SBX_STUB_MARK\" ]; then echo 'not bound' 1>&2; exit 1; fi\n" +
		"if [ \"$1\" = mount ]; then : > \"$SBX_STUB_MARK\"; fi\n" +
		"exit 0\n"
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("SBX_STUB_LOG", logPath)
	t.Setenv("SBX_STUB_MARK", mark)

	// The socket path does not exist, so the REST fallback fails and the
	// re-mount fallback must recover.
	err := New(filepath.Join(dir, "missing.sock")).UnmountFolder(context.Background(), "box", "/data/ro", true)
	if err != nil {
		t.Fatalf("expected the deadlock to be broken, got %v", err)
	}

	want := []string{"umount box /data/ro", "mount box /data/ro::ro", "umount box /data/ro"}
	got := stubCalls(t, logPath)
	if len(got) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestMountsAcceptsSchemaVariants(t *testing.T) {
	fixture := `{"workspace":"/w","mounts":[{"host_path":"/data/one","target":"/ctr/one","read_only":true}]}`
	newStubSbx(t, fixture)

	mounts, err := New("/nonexistent.sock").Mounts(context.Background(), "box")
	if err != nil {
		t.Fatalf("mounts: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %+v", mounts)
	}
	if mounts[0].HostPath != "/data/one" || mounts[0].Target != "/ctr/one" || !mounts[0].ReadOnly {
		t.Fatalf("unexpected mount: %+v", mounts[0])
	}
}
