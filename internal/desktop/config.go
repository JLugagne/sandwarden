package desktop

import (
	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
)

// GetConfig returns the global sandwarden configuration (terminal launcher
// and desktop notifications preferences).
func (d *Desktop) GetConfig() (fleet.AppConfig, error) {
	return d.app.GetConfig(d.root)
}

// SetConfig writes the global configuration file.
func (d *Desktop) SetConfig(cfg fleet.AppConfig) error {
	return d.app.SetConfig(d.root, cfg)
}

// FleetDir returns the configuration directory that holds every file.
func (d *Desktop) FleetDir() string {
	return d.app.FleetDir()
}

// SandboxConfigDir returns the config directory of a sandbox.
func (d *Desktop) SandboxConfigDir(slug string) (string, error) {
	return d.app.SandboxConfigDir(slug)
}

// ProfileConfigDir returns the config directory of a profile.
func (d *Desktop) ProfileConfigDir(slug string) (string, error) {
	return d.app.ProfileConfigDir(slug)
}

// ReloadFleet re-reads every configuration file from disk and returns the
// per-file errors, if any.
func (d *Desktop) ReloadFleet() ([]string, error) {
	return d.app.ReloadFleet(d.root)
}

// ConfigStaleness reports the loaded configuration files that changed on disk
// since the last load; the UI polls it on window focus and on a short interval.
func (d *Desktop) ConfigStaleness() (fleet.Staleness, error) {
	return d.app.ConfigStaleness(d.root)
}

// ReadSandboxConfig returns the raw spec.yaml and sandwarden.yaml of a
// sandbox for the read-only viewer. A broken file is reported in its own
// ConfigFile entry, so the viewer still shows the raw text and the parse error.
func (d *Desktop) ReadSandboxConfig(name string) ([]app.ConfigFile, error) {
	return d.app.ReadSandboxConfig(d.root, name)
}

// ReadProfileConfig returns the raw spec.yaml and sandwarden.yaml of a
// profile for the read-only viewer.
func (d *Desktop) ReadProfileConfig(slug string) ([]app.ConfigFile, error) {
	return d.app.ReadProfileConfig(d.root, slug)
}

// AgentsFile returns the shared AGENTS.md mounted into every sandbox,
// creating it from the built-in template on first use.
func (d *Desktop) AgentsFile() (app.AgentsDoc, error) {
	return d.app.AgentsFile(d.root)
}

// SaveAgentsFile replaces the shared AGENTS.md; running sandboxes see the new
// content immediately.
func (d *Desktop) SaveAgentsFile(content string) (app.AgentsDoc, error) {
	return d.app.SaveAgentsFile(d.root, content)
}

// ResetAgentsFile restores the built-in AGENTS.md template.
func (d *Desktop) ResetAgentsFile() (app.AgentsDoc, error) {
	return d.app.ResetAgentsFile(d.root)
}
