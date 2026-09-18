package sbx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newTemplateSaveSbx writes a fake sbx binary that records every invocation
// and refuses `template save` with refusal when it is non-empty.
func newTemplateSaveSbx(t *testing.T, refusal string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_SBX_LOG"
if [ "$1" = "template" ] && [ "$2" = "save" ] && [ -n "$FAKE_SBX_REFUSAL" ]; then
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

func TestSaveTemplateArgs(t *testing.T) {
	logPath := newTemplateSaveSbx(t, "")
	if _, err := New("").SaveTemplate(context.Background(), "box", "myimage:v1", nil); err != nil {
		t.Fatalf("save template: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := string(raw), "template save box myimage:v1\n"; got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestSaveTemplateSurfacesRefusalVerbatim(t *testing.T) {
	refusal := "ERROR: sandbox 'box' not found"
	newTemplateSaveSbx(t, refusal)
	_, err := New("").SaveTemplate(context.Background(), "box", "myimage:v1", nil)
	if err == nil || err.Error() != refusal {
		t.Fatalf("err = %v, want the refusal verbatim: %q", err, refusal)
	}
}

func TestSaveTemplateValidatesInput(t *testing.T) {
	newTemplateSaveSbx(t, "")
	if _, err := New("").SaveTemplate(context.Background(), "  ", "myimage:v1", nil); err == nil {
		t.Fatal("expected an error for an empty sandbox name")
	}
	if _, err := New("").SaveTemplate(context.Background(), "box", "  ", nil); err == nil {
		t.Fatal("expected an error for an empty tag")
	}
}
