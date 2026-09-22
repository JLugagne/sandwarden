package terminal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const testWorkspace = "/home/user/my work"

func TestBuildArgsForCLITerminals(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	snippet := shellCommand(testWorkspace, "sbx run --name box")
	line := shellLine(snippet)

	cases := []struct {
		id   string
		bin  string
		want []string
	}{
		{"gnome-terminal", "gnome-terminal", []string{"gnome-terminal", "--working-directory=" + testWorkspace, "--", "/bin/bash", "-l", "-c", snippet}},
		{"konsole", "konsole", []string{"konsole", "--workdir", testWorkspace, "-e", "/bin/bash", "-l", "-c", snippet}},
		{"ptyxis", "ptyxis", []string{"ptyxis", "--working-directory", testWorkspace, "--", "/bin/bash", "-l", "-c", snippet}},
		{"xfce4-terminal", "xfce4-terminal", []string{"xfce4-terminal", "--working-directory=" + testWorkspace, "--command", line}},
		{"tilix", "tilix", []string{"tilix", "--working-directory=" + testWorkspace, "-e", line}},
		{"terminator", "terminator", []string{"terminator", "--working-directory=" + testWorkspace, "-x", "/bin/bash", "-l", "-c", snippet}},
		{"mate-terminal", "mate-terminal", []string{"mate-terminal", "--working-directory=" + testWorkspace, "-x", "/bin/bash", "-l", "-c", snippet}},
		{"lxterminal", "lxterminal", []string{"lxterminal", "--working-directory=" + testWorkspace, "--command=" + line}},
		{"kitty", "kitty", []string{"kitty", "--directory", testWorkspace, "/bin/bash", "-l", "-c", snippet}},
		{"alacritty", "alacritty", []string{"alacritty", "--working-directory", testWorkspace, "-e", "/bin/bash", "-l", "-c", snippet}},
		{"wezterm", "wezterm", []string{"wezterm", "start", "--cwd", testWorkspace, "--", "/bin/bash", "-l", "-c", snippet}},
		{"ghostty", "ghostty", []string{"ghostty", "--window-save-state=never", "--working-directory=" + testWorkspace, "-e", "/bin/bash", "-l", "-c", snippet}},
		{"foot", "foot", []string{"foot", "--working-directory=" + testWorkspace, "/bin/bash", "-l", "-c", snippet}},
		{"xterm", "xterm", []string{"xterm", "-e", "/bin/bash", "-l", "-c", snippet}},
		{"uxterm", "uxterm", []string{"uxterm", "-e", "/bin/bash", "-l", "-c", snippet}},
		{"urxvt", "urxvt", []string{"urxvt", "-e", "/bin/bash", "-l", "-c", snippet}},
		{"st", "st", []string{"st", "-e", "/bin/bash", "-l", "-c", snippet}},
		{"guake", "guake", []string{"guake", "-e", line}},
		{"x-terminal-emulator", "x-terminal-emulator", []string{"x-terminal-emulator", "-e", "/bin/bash", "-l", "-c", snippet}},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			sp, ok := specFor(tc.id)
			if !ok {
				t.Fatalf("missing spec %q", tc.id)
			}
			got := buildArgs(sp, tc.bin, testWorkspace, "sbx run --name box")
			if !slices.Equal(got, tc.want) {
				t.Fatalf("argv:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestBuildArgsForAppleScriptTerminals(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")

	sp, ok := specFor("terminal")
	if !ok {
		t.Fatal("missing Terminal.app spec")
	}
	got := buildArgs(sp, "/usr/bin/osascript", testWorkspace, `sbx run --name "box"`)
	if len(got) != 3 || got[0] != "/usr/bin/osascript" || got[1] != "-e" {
		t.Fatalf("unexpected argv: %q", got)
	}
	if !strings.Contains(got[2], `do script "`) || !strings.Contains(got[2], `\"box\"`) {
		t.Fatalf("script not escaped: %s", got[2])
	}
	if !strings.Contains(got[2], "activate") {
		t.Fatalf("script does not activate the terminal: %s", got[2])
	}

	iterm, ok := specFor("iterm2")
	if !ok {
		t.Fatal("missing iTerm2 spec")
	}
	got = buildArgs(iterm, "/usr/bin/osascript", testWorkspace, "sbx shell box")
	if !strings.Contains(got[2], `create window with default profile command "`) {
		t.Fatalf("unexpected iTerm script: %s", got[2])
	}
}

func TestShellCommandKeepsTerminalOpen(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")

	got := shellCommand("/tmp/it's here", "sbx shell box")
	want := `cd '/tmp/it'\''s here' && sbx shell box; exec /bin/bash -l`
	if got != want {
		t.Fatalf("shellCommand:\n got %q\nwant %q", got, want)
	}
	if got := shellCommand("/srv/work", ""); got != "cd /srv/work; exec /bin/bash -l" {
		t.Fatalf("empty command: %q", got)
	}
	if got := shellCommand("", ""); got != "exec /bin/bash -l" {
		t.Fatalf("empty everything: %q", got)
	}
}

func TestShellPathFallsBack(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := shellPath(); got != "/bin/sh" {
		t.Fatalf("shellPath: got %q, want /bin/sh", got)
	}
}

func TestDetectResolvesPathAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	xterm := filepath.Join(dir, "xterm")
	if err := os.WriteFile(xterm, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake xterm: %v", err)
	}
	if err := os.Symlink(xterm, filepath.Join(dir, "x-terminal-emulator")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("PATH", dir)

	found := false
	entries := 0
	for _, term := range Detect() {
		if term.ID == "x-terminal-emulator" {
			t.Fatal("x-terminal-emulator should fold into the concrete terminal")
		}
		if term.Binary != xterm {
			continue
		}
		entries++
		if term.ID != "xterm" {
			t.Fatalf("unexpected id for %s: %q", xterm, term.ID)
		}
		found = true
	}
	if !found {
		t.Fatal("fake xterm was not detected")
	}
	if entries != 1 {
		t.Fatalf("expected one entry for the fake binary, got %d", entries)
	}
}

func TestLaunchRunsDetachedTerminal(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + captured + "\n"
	xterm := filepath.Join(dir, "xterm")
	if err := os.WriteFile(xterm, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xterm: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SHELL", "/bin/bash")

	if err := Launch("xterm", "/srv/work", "sbx shell box"); err != nil {
		t.Fatalf("launch: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var lines []string
	for {
		data, err := os.ReadFile(captured)
		if err == nil {
			lines = strings.Split(strings.TrimSpace(string(data)), "\n")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake terminal never ran: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	want := []string{"-e", "/bin/bash", "-l", "-c", "cd /srv/work && sbx shell box; exec /bin/bash -l"}
	if !slices.Equal(lines, want) {
		t.Fatalf("argv:\n got %q\nwant %q", lines, want)
	}
}

func TestLaunchUnknownTerminal(t *testing.T) {
	if err := Launch("nope", "/tmp", "true"); err == nil {
		t.Fatal("expected an error for an unknown terminal")
	}
}
