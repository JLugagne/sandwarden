//go:build darwin

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type darwinPlatform struct{}

var current platform = darwinPlatform{}

func (darwinPlatform) name() string { return "launchd" }

func (darwinPlatform) unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", LaunchdLabel+".plist"), nil
}

func launchdDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func (darwinPlatform) install(ctx context.Context, run Runner, spec Spec) (Report, error) {
	path, err := darwinPlatform{}.unitPath()
	if err != nil {
		return Report{}, err
	}
	report := Report{Platform: darwinPlatform{}.name(), UnitPath: path}
	report.Previous, err = fileExists(path)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return report, fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(renderLaunchdPlist(spec)), 0o644); err != nil {
		return report, fmt.Errorf("write launchd agent: %w", err)
	}
	target := launchdDomain() + "/" + LaunchdLabel
	if report.Previous {
		if _, err := run(ctx, "launchctl", "bootout", target); err != nil {
			report.Warnings = append(report.Warnings, "launchctl bootout: "+firstLine(err.Error()))
		} else {
			report.Commands = append(report.Commands, "launchctl bootout "+target)
		}
	}
	steps := [][]string{
		{"bootstrap", launchdDomain(), path},
		{"kickstart", "-k", target},
	}
	for _, args := range steps {
		if _, err := run(ctx, "launchctl", args...); err != nil {
			return report, launchdFailed(err)
		}
		report.Commands = append(report.Commands, "launchctl "+strings.Join(args, " "))
	}
	report.Installed = true
	return report, nil
}

func (darwinPlatform) status(ctx context.Context, run Runner) (Status, error) {
	path, err := darwinPlatform{}.unitPath()
	if err != nil {
		return Status{}, err
	}
	st := Status{Platform: darwinPlatform{}.name(), UnitPath: path}
	st.Installed, err = fileExists(path)
	if err != nil {
		return st, err
	}
	if !st.Installed {
		return st, nil
	}
	out, err := run(ctx, "launchctl", "print", launchdDomain()+"/"+LaunchdLabel)
	if err != nil {
		st.Detail = "not loaded: " + firstLine(err.Error())
		return st, nil
	}
	st.Enabled = true
	st.Running = strings.Contains(out, "state = running")
	if st.Running {
		st.Detail = "loaded and running"
	} else {
		st.Detail = "loaded but not running"
	}
	return st, nil
}

func (darwinPlatform) uninstall(ctx context.Context, run Runner) (Report, error) {
	path, err := darwinPlatform{}.unitPath()
	if err != nil {
		return Report{}, err
	}
	report := Report{Platform: darwinPlatform{}.name(), UnitPath: path}
	report.Previous, err = fileExists(path)
	if err != nil {
		return report, err
	}
	if !report.Previous {
		return report, nil
	}
	target := launchdDomain() + "/" + LaunchdLabel
	if _, err := run(ctx, "launchctl", "bootout", target); err != nil {
		report.Warnings = append(report.Warnings, "launchctl bootout: "+firstLine(err.Error()))
	} else {
		report.Commands = append(report.Commands, "launchctl bootout "+target)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return report, fmt.Errorf("remove launchd agent: %w", err)
	}
	report.Removed = true
	return report, nil
}

func launchdFailed(err error) error {
	return fmt.Errorf("launchctl failed: %w\nthe watch service needs a logged-in GUI session (check `launchctl print gui/$UID`)", err)
}
