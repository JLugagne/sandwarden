//go:build !darwin

package sbx

import (
	"path/filepath"
	"testing"
)

func TestSocketPathDefaultsToXDGState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(envSocketPath, "")
	want := filepath.Join(home, ".local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock")
	if got := SocketPath(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
