package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Package terminal detects the terminal emulators installed on the host and
// launches sandbox commands inside them.

// Terminal is one terminal emulator available on the host.
type Terminal struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Binary string `json:"binary"`
}

// macOS terminals are driven through their AppleScript dictionary; the single
// %s receives the escaped shell command.
const (
	terminalAppScript = `tell application id "com.apple.Terminal"
activate
do script "%s"
end tell`
	iTermScript = `tell application id "com.googlecode.iterm2"
activate
create window with default profile command "%s"
end tell`
)

// spec describes one terminal emulator. It drives both detection (apps, look)
// and the argv used to launch it (fixed, workdir, execFlag/execLine):
//
//   - apps: macOS bundles; when one exists the emulator is available and is
//     driven through osascript with script.
//   - look: executables to try, absolute paths (~ supported) or PATH names.
//   - fixed: extra leading arguments (wezterm: "start").
//   - workdir: flags receiving the working directory, %s is the directory.
//   - execFlag: separator before the shell argv (["-e"], ["--"], ["-x"]).
//   - execLine: flags receiving the whole "<shell> -l -c '<command>'" as a
//     single argument, %s is the argument.
type spec struct {
	id       string
	name     string
	apps     []string
	script   string
	look     []string
	fixed    []string
	workdir  []string
	execFlag []string
	execLine []string
}

// specs lists the classic terminal emulators of Linux desktops and macOS,
// most common first: the first available one becomes the default proposal.
var specs = []spec{
	{
		id: "terminal", name: "Terminal",
		apps:   []string{"/System/Applications/Utilities/Terminal.app", "/Applications/Utilities/Terminal.app"},
		script: terminalAppScript,
	},
	{
		id: "iterm2", name: "iTerm2",
		apps:   []string{"~/Applications/iTerm.app", "/Applications/iTerm.app"},
		script: iTermScript,
	},
	{
		id: "gnome-terminal", name: "GNOME Terminal",
		look:     []string{"gnome-terminal"},
		workdir:  []string{"--working-directory=%s"},
		execFlag: []string{"--"},
	},
	{
		id: "konsole", name: "Konsole",
		look:     []string{"konsole"},
		workdir:  []string{"--workdir", "%s"},
		execFlag: []string{"-e"},
	},
	{
		id: "ptyxis", name: "Ptyxis",
		look:     []string{"ptyxis"},
		workdir:  []string{"--working-directory", "%s"},
		execFlag: []string{"--"},
	},
	{
		id: "xfce4-terminal", name: "XFCE Terminal",
		look:     []string{"xfce4-terminal"},
		workdir:  []string{"--working-directory=%s"},
		execLine: []string{"--command", "%s"},
	},
	{
		id: "tilix", name: "Tilix",
		look:     []string{"tilix"},
		workdir:  []string{"--working-directory=%s"},
		execLine: []string{"-e", "%s"},
	},
	{
		id: "terminator", name: "Terminator",
		look:     []string{"terminator"},
		workdir:  []string{"--working-directory=%s"},
		execFlag: []string{"-x"},
	},
	{
		id: "mate-terminal", name: "MATE Terminal",
		look:     []string{"mate-terminal"},
		workdir:  []string{"--working-directory=%s"},
		execFlag: []string{"-x"},
	},
	{
		id: "lxterminal", name: "LXTerminal",
		look:     []string{"lxterminal"},
		workdir:  []string{"--working-directory=%s"},
		execLine: []string{"--command=%s"},
	},
	{
		id: "kitty", name: "kitty",
		look: []string{
			"/Applications/kitty.app/Contents/MacOS/kitty",
			"~/Applications/kitty.app/Contents/MacOS/kitty",
			"kitty",
		},
		workdir: []string{"--directory", "%s"},
	},
	{
		id: "alacritty", name: "Alacritty",
		look: []string{
			"/Applications/Alacritty.app/Contents/MacOS/alacritty",
			"~/Applications/Alacritty.app/Contents/MacOS/alacritty",
			"alacritty",
		},
		workdir:  []string{"--working-directory", "%s"},
		execFlag: []string{"-e"},
	},
	{
		id: "wezterm", name: "WezTerm",
		look: []string{
			"/Applications/WezTerm.app/Contents/MacOS/wezterm",
			"~/Applications/WezTerm.app/Contents/MacOS/wezterm",
			"wezterm",
		},
		fixed:    []string{"start"},
		workdir:  []string{"--cwd", "%s"},
		execFlag: []string{"--"},
	},
	{
		id: "ghostty", name: "Ghostty",
		look: []string{
			"/Applications/Ghostty.app/Contents/MacOS/ghostty",
			"~/Applications/Ghostty.app/Contents/MacOS/ghostty",
			"ghostty",
		},
		// A restored session would open a second, unrelated window.
		fixed:    []string{"--window-save-state=never"},
		workdir:  []string{"--working-directory=%s"},
		execFlag: []string{"-e"},
	},
	{
		id: "foot", name: "foot",
		look:    []string{"foot"},
		workdir: []string{"--working-directory=%s"},
	},
	{
		id: "xterm", name: "XTerm",
		look:     []string{"xterm"},
		execFlag: []string{"-e"},
	},
	{
		id: "uxterm", name: "UXTerm",
		look:     []string{"uxterm"},
		execFlag: []string{"-e"},
	},
	{
		id: "urxvt", name: "rxvt-unicode",
		look:     []string{"urxvt"},
		execFlag: []string{"-e"},
	},
	{
		id: "st", name: "st",
		look:     []string{"st"},
		execFlag: []string{"-e"},
	},
	{
		id: "guake", name: "Guake",
		look:     []string{"guake"},
		execLine: []string{"-e", "%s"},
	},
	{
		id: "x-terminal-emulator", name: "System terminal",
		look:     []string{"x-terminal-emulator"},
		execFlag: []string{"-e"},
	},
}

// Detect reports the terminal emulators available on this host, without
// duplicates: an emulator reachable through several names (x-terminal-emulator
// usually symlinks to the desktop terminal) is reported once under its
// specific name.
func Detect() []Terminal {
	out := make([]Terminal, 0, len(specs))
	seen := make(map[string]bool, len(specs))
	for _, sp := range specs {
		bin, ok := resolve(sp)
		if !ok {
			continue
		}
		key := sp.id
		if sp.script == "" {
			if resolved, err := filepath.EvalSymlinks(bin); err == nil {
				key = resolved
			} else {
				key = bin
			}
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Terminal{ID: sp.id, Name: sp.name, Binary: bin})
	}
	return out
}

// Launch opens dir in the chosen terminal and runs command there. The process
// is detached from the app so the terminal outlives it.
func Launch(id, dir, command string) error {
	sp, ok := specFor(id)
	if !ok {
		return fmt.Errorf("unknown terminal %q", id)
	}
	bin, ok := resolve(sp)
	if !ok {
		return fmt.Errorf("%s is not installed anymore", sp.name)
	}
	bin, argv := bundleLaunch(bin, buildArgs(sp, bin, dir, command))
	cmd := exec.Command(bin)
	cmd.Args = argv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", sp.name, err)
	}
	return cmd.Process.Release()
}

func specFor(id string) (spec, bool) {
	for _, sp := range specs {
		if sp.id == id {
			return sp, true
		}
	}
	return spec{}, false
}

// resolve returns the executable to run for sp: osascript for macOS script
// terminals, otherwise the first candidate that exists.
func resolve(sp spec) (string, bool) {
	if sp.script != "" {
		for _, app := range sp.apps {
			if path, ok := expand(app); ok && isDir(path) {
				return "/usr/bin/osascript", true
			}
		}
		return "", false
	}
	for _, candidate := range sp.look {
		if path, ok := expand(candidate); ok {
			if isExecutable(path) {
				return path, true
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, true
		}
	}
	return "", false
}

// buildArgs assembles the argv that opens dir and runs command in sp.
func buildArgs(sp spec, bin, dir, command string) []string {
	snippet := shellCommand(dir, command)
	if sp.script != "" {
		return []string{bin, "-e", fmt.Sprintf(sp.script, escapeScript(snippet))}
	}
	argv := append([]string{bin}, sp.fixed...)
	if dir != "" {
		for _, flag := range sp.workdir {
			argv = append(argv, strings.ReplaceAll(flag, "%s", dir))
		}
	}
	if len(sp.execLine) > 0 {
		line := shellLine(snippet)
		for _, flag := range sp.execLine {
			argv = append(argv, strings.ReplaceAll(flag, "%s", line))
		}
		return argv
	}
	argv = append(argv, sp.execFlag...)
	return append(argv, shellPath(), "-l", "-c", snippet)
}

// shellCommand is the snippet run inside the terminal: move to the workspace,
// run the command, then hand over to a login shell so a fast failure or a
// clean exit leaves the user in a usable terminal instead of a vanishing one.
func shellCommand(dir, command string) string {
	var parts []string
	if dir != "" {
		parts = append(parts, "cd "+quote(dir))
	}
	if command = strings.TrimSpace(command); command != "" {
		parts = append(parts, command)
	}
	if len(parts) == 0 {
		return "exec " + quote(shellPath()) + " -l"
	}
	return strings.Join(parts, " && ") + "; exec " + quote(shellPath()) + " -l"
}

// shellLine is the same command as one string, for terminals that take a
// single command argument instead of an argv tail.
func shellLine(snippet string) string {
	return shellPath() + " -l -c " + quote(snippet)
}

// shellPath is the login shell used inside the terminal.
func shellPath() string {
	if sh := strings.TrimSpace(os.Getenv("SHELL")); sh != "" {
		if path, err := exec.LookPath(sh); err == nil {
			return path
		}
	}
	return "/bin/sh"
}

// quote wraps s in single quotes unless it is already shell-safe.
func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`;&|()<>*?![]{}~#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeScript embeds s in an AppleScript string literal.
func escapeScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", " ")
}

// expand resolves ~/... to the user's home and reports whether p is absolute;
// relative candidates are left to the PATH lookup.
func expand(p string) (string, bool) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		return "", false
	}
	return p, true
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}
