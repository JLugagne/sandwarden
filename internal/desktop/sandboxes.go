package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ListSandboxes returns live sandbox summaries with cached profile names.
func (d *Desktop) ListSandboxes() ([]app.SandboxSummary, error) {
	return d.app.SandboxSummaries(d.root)
}

// SandboxDetail returns the detail payload the sandbox page renders.
func (d *Desktop) SandboxDetail(name string) (app.SandboxDetail, error) {
	return d.app.SandboxDetail(d.root, name)
}

// CreateSandbox validates the request and starts `sbx create` as a job,
// returning the job id whose output streams over hub events.
func (d *Desktop) CreateSandbox(req CreateSandboxRequest) (string, error) {
	opts := sbx.CreateOptions{
		Agent:       strings.TrimSpace(req.Agent),
		Name:        strings.TrimSpace(req.Name),
		CPUs:        req.CPUs,
		Memory:      strings.TrimSpace(req.Memory),
		Profile:     strings.TrimSpace(req.Profile),
		Template:    strings.TrimSpace(req.Template),
		Publish:     cleanStrings(req.Publish),
		Env:         cleanStrings(req.Env),
		DenyNetwork: cleanStrings(req.DenyNetwork),
		Clone:       req.Clone,
	}
	for _, workspace := range req.Workspaces {
		path := strings.TrimSpace(workspace.Path)
		if path == "" {
			continue
		}
		if workspace.ReadOnly {
			path += ":ro"
		}
		opts.Workspaces = append(opts.Workspaces, path)
	}
	if _, err := opts.Args(); err != nil {
		return "", err
	}
	return d.app.StartCreateJob(opts, req.JobID, req.AttachCaches), nil
}

// DeleteSandbox removes a sandbox, optionally forcing past the daemon's checks.
func (d *Desktop) DeleteSandbox(name string, force bool) error {
	return d.app.DeleteSandbox(d.root, name, force)
}

// StartSandbox boots a stopped sandbox.
func (d *Desktop) StartSandbox(name string) error {
	return d.app.StartSandbox(d.root, name)
}

// StopSandbox halts a running sandbox.
func (d *Desktop) StopSandbox(name string) error {
	return d.app.StopSandbox(d.root, name)
}

// AddMount bind-mounts a host folder into the sandbox at runtime.
func (d *Desktop) AddMount(name string, req MountRequest) error {
	return d.app.AddMountAt(d.root, name, req.Path, req.Target, req.ReadOnly)
}

// RemoveMount detaches a runtime mount.
func (d *Desktop) RemoveMount(name, path, target string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	return d.app.RemoveMountAt(d.root, name, path, target)
}

// AssignProfile applies a profile to a sandbox.
func (d *Desktop) AssignProfile(name string, profileID int64) error {
	if profileID == 0 {
		return errors.New("profile_id is required")
	}
	return d.app.ApplyProfile(d.root, name, profileID)
}

// UnassignProfile removes a profile from a sandbox.
func (d *Desktop) UnassignProfile(name string, profileID int64) error {
	return d.app.UnapplyProfile(d.root, name, profileID)
}

// Exec starts a one-shot command in the sandbox, returning the job id whose
// output streams over hub events.
func (d *Desktop) Exec(name string, req ExecRequest) (string, error) {
	if strings.TrimSpace(req.Command) == "" {
		return "", errors.New("command is required")
	}
	return d.app.StartExecJob(name, req.Command, req.JobID), nil
}
