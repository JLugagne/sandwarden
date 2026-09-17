package sbx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newKitSbx writes a fake sbx binary that records every invocation and refuses
// `kit add` with refusal when it is non-empty.
func newKitSbx(t *testing.T, refusal string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_SBX_LOG"
if [ "$1" = "kit" ] && [ "$2" = "add" ] && [ -n "$FAKE_SBX_REFUSAL" ]; then
  printf '%s\n' "$FAKE_SBX_REFUSAL" >&2
  exit 1
fi
exit 0
`
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("FAKE_SBX_LOG", logPath)
	t.Setenv("FAKE_SBX_REFUSAL", refusal)
	return logPath
}

func TestKitAddArgs(t *testing.T) {
	logPath := newKitSbx(t, "")
	if _, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", nil); err != nil {
		t.Fatalf("kit add: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := string(raw), "kit add box ./mcp-postgres\n"; got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestKitAddSurfacesRefusalVerbatim(t *testing.T) {
	refusal := "ERROR: sandbox 'box' was created before the kit-add recreate feature shipped and cannot be recreated safely"
	newKitSbx(t, refusal)
	_, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", nil)
	if err == nil || err.Error() != refusal {
		t.Fatalf("err = %v, want the refusal verbatim: %q", err, refusal)
	}
}

func TestKitAddValidatesInput(t *testing.T) {
	newKitSbx(t, "")
	if _, err := New("").KitAdd(context.Background(), "  ", "./mcp-postgres", nil); err == nil {
		t.Fatal("expected an error for an empty sandbox name")
	}
	if _, err := New("").KitAdd(context.Background(), "box", "  ", nil); err == nil {
		t.Fatal("expected an error for an empty kit reference")
	}
}
