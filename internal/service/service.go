// Package service installs, inspects and removes the optional per-user
// service that runs `sandwarden watch` in the background. On Linux it manages
// a systemd --user unit, on macOS a launchd agent. It never uses root and
// never writes outside the user's config or home directory.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Name identifies the service on both platforms: the systemd unit name on
// Linux and the launchd label on macOS.
const Name = "sandwarden-watch"

// LaunchdLabel is the launchd job label used on macOS.
const LaunchdLabel = "com.sandwarden.watch"

// Spec is the command the installed service runs.
type Spec struct {
	Executable string
	Args       []string
}

// Status is the observed state of the user service.
type Status struct {
	Platform  string
	UnitPath  string
	Installed bool
	Enabled   bool
	Running   bool
	Detail    string
}

// Report describes what install or uninstall changed.
type Report struct {
	Platform  string
	UnitPath  string
	Previous  bool
	Removed   bool
	Installed bool
	Commands  []string
	Warnings  []string
}

// Runner executes one external command and returns its combined output.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Manager installs, inspects and removes the per-user service.
type Manager struct {
	run Runner
}

// New returns a Manager; a nil runner executes real commands.
func New(run Runner) *Manager {
	if run == nil {
		run = execRunner
	}
	return &Manager{run: run}
}

// Platform names the service manager in use ("systemd --user" or "launchd").
func (m *Manager) Platform() string { return current.name() }

// Install writes the service definition, loads it and starts it.
func (m *Manager) Install(ctx context.Context, spec Spec) (Report, error) {
	return current.install(ctx, m.run, spec)
}

// Status reports whether the service is installed, enabled and running.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	return current.status(ctx, m.run)
}

// Uninstall stops, disables and removes the service.
func (m *Manager) Uninstall(ctx context.Context) (Report, error) {
	return current.uninstall(ctx, m.run)
}

type platform interface {
	name() string
	unitPath() (string, error)
	install(ctx context.Context, run Runner, spec Spec) (Report, error)
	status(ctx context.Context, run Runner) (Status, error)
	uninstall(ctx context.Context, run Runner) (Report, error)
}

func execRunner(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	text := string(out)
	if err == nil {
		return text, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return text, fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	if message := strings.ReplaceAll(strings.TrimSpace(text), "\n", "; "); message != "" {
		return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
	}
	return text, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
}

func unsupportedError(goos string) error {
	return fmt.Errorf("sandwarden setup is not supported on %s: the watch service needs systemd --user (Linux) or launchd (macOS)", goos)
}
