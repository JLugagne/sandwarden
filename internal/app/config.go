package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/goccy/go-yaml"
)

// GetConfig returns the global sandwarden configuration.
func (a *App) GetConfig(ctx context.Context) (fleet.AppConfig, error) {
	return a.Fleet.Config(), nil
}

// SetConfig writes the global configuration (terminal launcher and desktop
// notifications preferences) under the fleet lock.
func (a *App) SetConfig(ctx context.Context, cfg fleet.AppConfig) error {
	return a.withFleetLock(func() error { return a.Fleet.SetConfig(cfg) })
}

// ReloadFleet re-reads every configuration file from disk and returns the
// per-file errors, if any.
func (a *App) ReloadFleet(ctx context.Context) ([]string, error) {
	if err := a.Fleet.Reload(); err != nil {
		return nil, err
	}
	var errs []string
	for _, err := range a.Fleet.Errors() {
		errs = append(errs, err.Error())
	}
	a.Notify(TopicSandboxes)
	a.Notify(TopicProfiles)
	a.Notify(TopicCaches)
	a.Notify(TopicKits)
	a.Notify(TopicSkills)
	return errs, nil
}

// ConfigStaleness reports the loaded configuration files that changed on disk
// since the last load. The check only stats the files: it never re-reads their
// content and never touches the daemon, so the GUI can poll it cheaply.
func (a *App) ConfigStaleness(ctx context.Context) (fleet.Staleness, error) {
	return a.Fleet.CheckStaleness(), nil
}

// FleetDir is the configuration directory shown in the UI.
func (a *App) FleetDir() string { return a.Fleet.Dir() }

// SandboxConfigDir returns the config directory of a sandbox.
func (a *App) SandboxConfigDir(slug string) (string, error) {
	s, ok := a.Fleet.Sandbox(slug)
	if !ok {
		return "", fmt.Errorf("sandbox %q not found", slug)
	}
	return s.Dir, nil
}

// ProfileConfigDir returns the config directory of a profile.
func (a *App) ProfileConfigDir(slug string) (string, error) {
	p, ok := a.Fleet.Profile(slug)
	if !ok {
		return "", fmt.Errorf("profile %q not found", slug)
	}
	return p.Dir, nil
}

const (
	specFileName       = "spec.yaml"
	sidecarFileName    = "sandwarden.yaml"
	sandboxesConfigDir = "sandboxes"
	profilesConfigDir  = "profiles"

	// maxConfigFileBytes caps how much of a configuration file the read-only
	// viewer loads; larger files are reported instead of held in memory.
	maxConfigFileBytes = 256 << 10
)

// ConfigFile is one configuration file of a sandbox or profile, as shown by
// the read-only viewer. Error is set when the file is missing, too large or
// invalid; Content still carries the raw text when it could be read.
type ConfigFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// ReadSandboxConfig returns the raw spec.yaml and sandwarden.yaml of a
// sandbox. The files are resolved through the fleet, never from the caller's
// string. A missing, oversized or invalid file is reported in its own entry
// instead of failing the call, so a hand-edited sidecar stays inspectable.
func (a *App) ReadSandboxConfig(ctx context.Context, name string) ([]ConfigFile, error) {
	dir, err := a.sandboxConfigDir(name)
	if err != nil {
		return nil, err
	}
	return readConfigFiles(dir, sandboxConfigError), nil
}

// ReadProfileConfig returns the raw spec.yaml and sandwarden.yaml of a profile.
func (a *App) ReadProfileConfig(ctx context.Context, slug string) ([]ConfigFile, error) {
	dir, err := a.profileConfigDir(slug)
	if err != nil {
		return nil, err
	}
	return readConfigFiles(dir, profileConfigError), nil
}

// sandboxConfigDir resolves a sandbox's config directory from its canonical
// name or its directory slug. When the files on disk no longer parse, the
// fleet cannot index the sandbox but still records the failing paths, which
// keeps the directory reachable for the viewer.
func (a *App) sandboxConfigDir(selector string) (string, error) {
	name := strings.TrimSpace(selector)
	if name == "" {
		return "", errors.New("sandbox name is required")
	}
	if s, ok := a.Fleet.Sandbox(name); ok {
		return s.Dir, nil
	}
	if s, ok := a.Fleet.SandboxByName(name); ok {
		return s.Dir, nil
	}
	if dir := brokenConfigDir(a.Fleet, name, sandboxesConfigDir); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("sandbox %q not found", name)
}

// profileConfigDir resolves a profile's config directory from its slug,
// including profiles the fleet could not index because a file is broken.
func (a *App) profileConfigDir(selector string) (string, error) {
	slug := strings.TrimSpace(selector)
	if slug == "" {
		return "", errors.New("profile slug is required")
	}
	if p, ok := a.Fleet.Profile(slug); ok {
		return p.Dir, nil
	}
	if dir := brokenConfigDir(a.Fleet, slug, profilesConfigDir); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("profile %q not found", slug)
}

// brokenConfigDir finds the directory of an entity the fleet could not index
// because one of its files failed to parse. Only the directory base name is
// compared, so the selector is never joined into a path.
func brokenConfigDir(f *fleet.Fleet, selector, parent string) string {
	for _, loadErr := range f.Errors() {
		dir := filepath.Dir(loadErr.Path)
		if filepath.Base(dir) != selector || filepath.Base(filepath.Dir(dir)) != parent {
			continue
		}
		return dir
	}
	return ""
}

// readConfigFiles loads both files of a config directory. The validate
// function checks one file against the entity's strict schema.
func readConfigFiles(dir string, validate func(name string, content []byte) error) []ConfigFile {
	files := make([]ConfigFile, 0, 2)
	for _, name := range []string{specFileName, sidecarFileName} {
		file := ConfigFile{Name: name, Path: filepath.Join(dir, name)}
		content, err := readConfigFile(file.Path)
		if err != nil {
			file.Error = err.Error()
			files = append(files, file)
			continue
		}
		file.Content = string(content)
		if err := validate(name, content); err != nil {
			file.Error = err.Error()
		}
		files = append(files, file)
	}
	return files
}

// readConfigFile reads one configuration file, capped at maxConfigFileBytes.
func readConfigFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("file not found")
		}
		return nil, err
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, maxConfigFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxConfigFileBytes {
		return nil, fmt.Errorf("file is larger than the %d KiB viewer limit", maxConfigFileBytes>>10)
	}
	return content, nil
}

func sandboxConfigError(name string, content []byte) error {
	switch name {
	case specFileName:
		var spec fleet.Spec
		return strictYAML(content, &spec)
	case sidecarFileName:
		var app fleet.SandboxApp
		return strictYAML(content, &app)
	}
	return nil
}

func profileConfigError(name string, content []byte) error {
	switch name {
	case specFileName:
		var spec fleet.Spec
		return strictYAML(content, &spec)
	case sidecarFileName:
		var app fleet.ProfileApp
		return strictYAML(content, &app)
	}
	return nil
}

// strictYAML decodes content the way the fleet loader does: unknown fields are
// errors, so the viewer surfaces the same problems Apply reports.
func strictYAML(content []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(content), yaml.DisallowUnknownField())
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("file is empty")
		}
		return err
	}
	return nil
}
