//go:build darwin

package terminal

import (
	"slices"
	"testing"
)

func TestBundleLaunchGoesThroughOpen(t *testing.T) {
	bin := "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	got, argv := bundleLaunch(bin, []string{bin, "-e", "sh"})
	want := []string{"/usr/bin/open", "-na", "/Applications/Ghostty.app", "--args", "-e", "sh"}
	if got != openPath || !slices.Equal(argv, want) {
		t.Fatalf("got %s %q", got, argv)
	}
	if got, argv := bundleLaunch("/usr/bin/osascript", []string{"/usr/bin/osascript", "-e", "x"}); got != "/usr/bin/osascript" || len(argv) != 3 {
		t.Fatalf("plain binary must pass through, got %s %q", got, argv)
	}
}
