package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

// ListSandboxes returns live sandbox summaries with their config profiles.
func (d *Desktop) ListSandboxes() ([]app.SandboxSummary, error) {
	return d.app.SandboxSummaries(d.root)
}

// SandboxDetail returns the detail payload the sandbox page renders.
func (d *Desktop) SandboxDetail(name string) (app.SandboxDetail, error) {
	return d.app.SandboxDetail(d.root, name)
}

// CreateSandbox validates the request and starts `sbx create` as a job,
// writing the new sandbox's config directory and converging it.
func (d *Desktop) CreateSandbox(req CreateSandboxRequest) (string, error) {
	opts := sbx.CreateOptions{
		Agent:       strings.TrimSpace(req.Agent),
		Name:        strings.TrimSpace(req.Name),
		CPUs:        req.CPUs,
		Memory:      strings.TrimSpace(req.Memory),
		Profile:     strings.TrimSpace(req.Profile),
		Template:    strings.TrimSpace(req.Template),
		Kits:        cleanStrings(req.Kits),
		Publish:     cleanStrings(req.Publish),
		Env:         cleanStrings(req.Env),
		DenyNetwork: cleanStrings(req.DenyNetwork),
		Clone:       req.Clone,
	}
	for _, workspace := range req.Workspaces {
		path := fleet.ExpandHome(workspace.Path)
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
	create := app.CreateRequest{
		Opts:         opts,
		Profiles:     cleanStrings(req.Profiles),
		Caches:       cleanStrings(req.Caches),
		RunArgs:      strings.TrimSpace(req.RunArgs),
		AttachCaches: req.AttachCaches,
	}
	for _, skill := range req.Skills {
		create.Skills = append(create.Skills, skill.SkillRefToFleet())
	}
	for _, mount := range req.Mounts {
		if strings.TrimSpace(mount.Path) == "" {
			continue
		}
		create.Mounts = append(create.Mounts, fleet.MountRef{HostPath: mount.Path, TargetPath: mount.Target, ReadOnly: mount.ReadOnly})
	}
	return d.app.StartCreateJob(create, req.JobID), nil
}

// DeleteSandbox removes a sandbox, optionally purging its config directory.
func (d *Desktop) DeleteSandbox(name string, force, purgeConfig bool) error {
	return d.app.DeleteSandbox(d.root, name, force, purgeConfig)
}

// StartSandbox boots a stopped sandbox and converges its configuration.
func (d *Desktop) StartSandbox(name string) error {
	return d.app.StartSandbox(d.root, name)
}

// StopSandbox halts a running sandbox.
func (d *Desktop) StopSandbox(name string) error {
	return d.app.StopSandbox(d.root, name)
}

// ApplySandbox converges a running sandbox onto its files as a job.
func (d *Desktop) ApplySandbox(name string, jobID string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("name is required")
	}
	return d.app.StartApplyJob(name, jobID), nil
}

// RecreateSandbox deletes and recreates a sandbox from its files as a job.
func (d *Desktop) RecreateSandbox(name string, jobID string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("name is required")
	}
	return d.app.StartRecreateJob(name, jobID), nil
}

// ValidateSandbox runs `sbx kit validate` over a sandbox's config directory.
func (d *Desktop) ValidateSandbox(slug string) (app.KitValidation, error) {
	return d.app.ValidateSandbox(d.root, slug)
}

// ValidateProfile runs `sbx kit validate` over a profile's config directory.
func (d *Desktop) ValidateProfile(slug string) (app.KitValidation, error) {
	return d.app.ValidateProfile(d.root, slug)
}

// AddMount declares a bind mount in the sandbox config and attaches it.
func (d *Desktop) AddMount(name string, req MountRequest) error {
	return d.app.AddMountAt(d.root, name, req.Path, req.Target, req.ReadOnly)
}

// RemoveMount removes a declared bind mount and detaches it.
func (d *Desktop) RemoveMount(name, path, target string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	return d.app.RemoveMountAt(d.root, name, path, target)
}

// AssignProfile applies a profile to a sandbox.
func (d *Desktop) AssignProfile(name, profileSlug string) error {
	if strings.TrimSpace(profileSlug) == "" {
		return errors.New("profile slug is required")
	}
	return d.app.ApplyProfile(d.root, name, profileSlug)
}

// UnassignProfile removes a profile from a sandbox.
func (d *Desktop) UnassignProfile(name, profileSlug string) error {
	if strings.TrimSpace(profileSlug) == "" {
		return errors.New("profile slug is required")
	}
	return d.app.UnapplyProfile(d.root, name, profileSlug)
}

// SetSandboxRunArgs stores the custom arguments appended after `--` to the
// sandbox's connect run command; an empty value clears it.
func (d *Desktop) SetSandboxRunArgs(name, args string) error {
	return d.app.SetSandboxRunArgs(d.root, name, args)
}

// Exec starts a one-shot command in the sandbox, returning the job id whose
// output streams over hub events.
func (d *Desktop) Exec(name string, req ExecRequest) (string, error) {
	if strings.TrimSpace(req.Command) == "" {
		return "", errors.New("command is required")
	}
	return d.app.StartExecJob(name, req.Command, req.JobID), nil
}

// ImportSandboxes adopts the named daemon sandboxes, or every daemon sandbox
// when the request names none, into the config directory. Adopted configs
// missing create parameters are marked incomplete.
func (d *Desktop) ImportSandboxes(req ImportSandboxesRequest) (app.ImportReport, error) {
	return d.app.ImportSandboxes(d.root, cleanStrings(req.Names))
}

// CompleteSandboxConfig records the create parameters an imported sandbox was
// missing and clears its incomplete marker.
func (d *Desktop) CompleteSandboxConfig(name string, req CompleteSandboxRequest) error {
	return d.app.CompleteSandboxConfig(d.root, name, app.CompleteConfigInput{
		CPUs:   req.CPUs,
		Memory: strings.TrimSpace(req.Memory),
		Env:    cleanStrings(req.Env),
	})
}
