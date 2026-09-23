package sbx_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

func installStub(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\nSTUB_DIR='" + dir + "'\n" + body
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
	return dir
}

// TestCLICallsAreBounded pins that concurrent callers cannot fan out into an
// unbounded number of sbx processes hitting the daemon at once.
func TestCLICallsAreBounded(t *testing.T) {
	dir := installStub(t, `mkdir "$STUB_DIR/run.$$"
sleep 0.3
ls "$STUB_DIR" | grep -c '^run\.' >> "$STUB_DIR/peak"
rmdir "$STUB_DIR/run.$$"
`)
	client := sbx.New(filepath.Join(t.TempDir(), "none.sock"))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.MkdirAll(context.Background(), "box", "/tmp/x")
		}()
	}
	wg.Wait()

	raw, err := os.ReadFile(filepath.Join(dir, "peak"))
	if err != nil {
		t.Fatalf("read peak: %v", err)
	}
	peak := 0
	for _, line := range strings.Fields(string(raw)) {
		n, _ := strconv.Atoi(line)
		peak = max(peak, n)
	}
	if peak > sbx.MaxConcurrentCLI {
		t.Fatalf("%d sbx processes ran at once, want at most %d", peak, sbx.MaxConcurrentCLI)
	}
}

const trapScript = `trap 'echo term > "$STUB_DIR/marker"; exit 0' TERM
sleep 5 &
wait
`

// TestCancelledCLIIsTerminatedGracefully pins that a cancelled sbx call gets
// SIGTERM, letting the CLI close its daemon session, instead of SIGKILL.
func TestCancelledCLIIsTerminatedGracefully(t *testing.T) {
	dir := installStub(t, trapScript)
	client := sbx.New(filepath.Join(t.TempDir(), "none.sock"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = client.MkdirAll(ctx, "box", "/tmp/x")

	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Fatal("sbx was killed without a chance to handle SIGTERM")
	}
}

// TestCancelledStatsProbeIsTerminatedGracefully covers the exec path.
func TestCancelledStatsProbeIsTerminatedGracefully(t *testing.T) {
	dir := installStub(t, trapScript)
	client := sbx.New(filepath.Join(t.TempDir(), "none.sock"))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, _ = client.Stats(ctx, "box")

	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Fatal("sbx exec was killed without a chance to handle SIGTERM")
	}
}

// TestInspectCacheReusesDetailUntilMutation pins the pass-scoped inspect cache:
// repeated reads share one `sbx inspect`, a mount invalidates it.
func TestInspectCacheReusesDetailUntilMutation(t *testing.T) {
	dir := installStub(t, `printf '%s\n' "$1" >> "$STUB_DIR/calls"
if [ "$1" = inspect ]; then printf '{"runtime_mounts":[]}\n'; fi
exit 0
`)
	client := sbx.New(filepath.Join(t.TempDir(), "none.sock"))
	ctx := sbx.WithInspectCache(context.Background())

	for range 3 {
		if _, err := client.Mounts(ctx, "box"); err != nil {
			t.Fatalf("mounts: %v", err)
		}
	}
	if err := client.MountFolder(ctx, "box", "/h", false); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if _, err := client.Mounts(ctx, "box"); err != nil {
		t.Fatalf("mounts: %v", err)
	}
	if _, err := client.Mounts(context.Background(), "box"); err != nil {
		t.Fatalf("mounts: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if got := strings.Count(string(raw), "inspect\n"); got != 3 {
		t.Fatalf("ran %d sbx inspect, want 3 (cached, after mutation, uncached ctx)", got)
	}
}
