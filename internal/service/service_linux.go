//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// systemdUnitName is both the unit filename and the systemctl argument.
const systemdUnitName = Name + ".service"

type linuxPlatform struct{}

var current platform = linuxPlatform{}

func (linuxPlatform) name() string { return "systemd --user" }

func (linuxPlatform) unitPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(dir, "systemd", "user", systemdUnitName), nil
}

func (linuxPlatform) install(ctx context.Context, run Runner, spec Spec) (Report, error) {
	path, err := linuxPlatform{}.unitPath()
	if err != nil {
		return Report{}, err
	}
	report := Report{Platform: linuxPlatform{}.name(), UnitPath: path}
	report.Previous, err = fileExists(path)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return report, fmt.Errorf("create systemd user unit directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderSystemdUnit(spec)), 0o644); err != nil {
		return report, fmt.Errorf("write systemd unit: %w", err)
	}
	steps := [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "--now", systemdUnitName},
	}
	if report.Previous {
		steps = append(steps, []string{"--user", "restart", systemdUnitName})
	}
	for _, args := range steps {
		if _, err := run(ctx, "systemctl", args...); err != nil {
			return report, systemdUnavailable(err)
		}
		report.Commands = append(report.Commands, "systemctl "+strings.Join(args, " "))
	}
	report.Installed = true
	return report, nil
}

func (linuxPlatform) status(ctx context.Context, run Runner) (Status, error) {
	path, err := linuxPlatform{}.unitPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{Platform: linuxPlatform{}.name(), UnitPath: path}
	st.Installed, err = fileExists(path)
	if err != nil {
		return st, err
	}
	if !st.Installed {
		return st, nil
	}
	enabledOut, enabledErr := run(ctx, "systemctl", "--user", "is-enabled", systemdUnitName)
	activeOut, activeErr := run(ctx, "systemctl", "--user", "is-active", systemdUnitName)
	enabled, enabledKnown := parseEnabled(enabledOut)
	running, runningKnown := parseActive(activeOut)
	if !enabledKnown && !runningKnown {
		stateErr := enabledErr
		if stateErr == nil {
			stateErr = activeErr
		}
		if stateErr == nil {
			st.Detail = "state unknown: unexpected systemctl output"
		} else {
			st.Detail = "state unknown: " + firstLine(stateErr.Error())
		}
		return st, nil
	}
	st.Enabled = enabled
	st.Running = running
	switch {
	case st.Enabled && st.Running:
		st.Detail = "enabled and running"
	case !st.Enabled && !st.Running:
		st.Detail = "disabled and not running"
	case !st.Enabled:
		st.Detail = "running but not enabled at login"
	default:
		st.Detail = "enabled but not running"
	}
	return st, nil
}

// parseEnabled reads `systemctl --user is-enabled` output. is-enabled reports
// a plain "disabled" and exits 1, so the caller must not treat every error as
// a broken systemd session.
func parseEnabled(out string) (bool, bool) {
	switch strings.TrimSpace(out) {
	case "enabled", "enabled-runtime", "alias":
		return true, true
	case "disabled", "static", "indirect", "generated", "transient", "masked":
		return false, true
	default:
		return false, false
	}
}

// parseActive reads `systemctl --user is-active` output.
func parseActive(out string) (bool, bool) {
	switch strings.TrimSpace(out) {
	case "active", "activating", "reloading":
		return true, true
	case "inactive", "deactivating", "failed":
		return false, true
	default:
		return false, false
	}
}

func (linuxPlatform) uninstall(ctx context.Context, run Runner) (Report, error) {
	path, err := linuxPlatform{}.unitPath()
	if err != nil {
		return Report{}, err
	}
	report := Report{Platform: linuxPlatform{}.name(), UnitPath: path}
	report.Previous, err = fileExists(path)
	if err != nil {
		return report, err
	}
	if !report.Previous {
		return report, nil
	}
	if _, err := run(ctx, "systemctl", "--user", "disable", "--now", systemdUnitName); err != nil {
		report.Warnings = append(report.Warnings, "systemctl --user disable --now: "+firstLine(err.Error()))
	} else {
		report.Commands = append(report.Commands, "systemctl --user disable --now "+systemdUnitName)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return report, fmt.Errorf("remove systemd unit: %w", err)
	}
	report.Removed = true
	if _, err := run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		report.Warnings = append(report.Warnings, "systemctl --user daemon-reload: "+firstLine(err.Error()))
	} else {
		report.Commands = append(report.Commands, "systemctl --user daemon-reload")
	}
	return report, nil
}

func systemdUnavailable(err error) error {
	return fmt.Errorf("systemctl failed: %w\nthe watch service needs a running systemd user session (check `systemctl --user status`; over SSH, `loginctl enable-linger $USER` keeps one available)", err)
}
