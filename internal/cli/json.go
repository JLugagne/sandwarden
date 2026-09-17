package cli

import (
	"encoding/json"
	"io"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/fleet"
)

// jsonSchemaVersion pins the --json contract documented in docs/cli.md.
// It changes whenever a documented key is renamed, removed or reshaped.
const jsonSchemaVersion = 1

// jsonConfigError is one per-file fleet load error.
type jsonConfigError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// jsonSandbox is the stable, file-side projection shared by ls and status.
type jsonSandbox struct {
	Name       string   `json:"name"`
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Running    bool     `json:"running"`
	ConfigSlug string   `json:"configSlug"`
	ConfigPath string   `json:"configPath"`
	Profiles   []string `json:"profiles"`
	Caches     int      `json:"caches"`
	Skills     int      `json:"skills"`
	Mounts     int      `json:"mounts"`
	Incomplete bool     `json:"incomplete"`
}

// jsonLsOutput is the ls --json envelope.
type jsonLsOutput struct {
	SchemaVersion int               `json:"schemaVersion"`
	Sandboxes     []jsonSandbox     `json:"sandboxes"`
	ConfigErrors  []jsonConfigError `json:"configErrors"`
}

// jsonStatusOutput is the status --json envelope.
type jsonStatusOutput struct {
	SchemaVersion int                 `json:"schemaVersion"`
	ConfigDir     string              `json:"configDir"`
	ConfigErrors  []jsonConfigError   `json:"configErrors"`
	Sandboxes     []jsonStatusSandbox `json:"sandboxes"`
}

// jsonStatusSandbox adds the daemon detail and the per-name errors to jsonSandbox.
type jsonStatusSandbox struct {
	jsonSandbox
	Errors []string           `json:"errors"`
	Detail *app.SandboxDetail `json:"detail,omitempty"`
}

// jsonApplyOutput is the apply --json envelope.
type jsonApplyOutput struct {
	SchemaVersion int               `json:"schemaVersion"`
	Reports       []jsonApplyReport `json:"reports"`
}

// jsonApplyReport is one sandbox's convergence result.
type jsonApplyReport struct {
	Name   string           `json:"name"`
	Report *jsonApplyResult `json:"report,omitempty"`
	Error  string           `json:"error,omitempty"`
}

// jsonApplyResult mirrors app.ApplyReport with arrays that are never null.
type jsonApplyResult struct {
	RulesApplied  int      `json:"rules_applied"`
	MountsApplied int      `json:"mounts_applied"`
	CachesApplied int      `json:"caches_applied"`
	SkillsApplied int      `json:"skills_applied"`
	SkillsRemoved int      `json:"skills_removed"`
	Warnings      []string `json:"warnings"`
	Errors        []string `json:"errors"`
}

// jsonStartOutput is the start --json envelope.
type jsonStartOutput struct {
	SchemaVersion int               `json:"schemaVersion"`
	Sandboxes     []jsonStartResult `json:"sandboxes"`
}

// jsonStartResult is one sandbox's start result.
type jsonStartResult struct {
	Name    string           `json:"name"`
	Started bool             `json:"started"`
	Report  *jsonApplyResult `json:"report,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// writeJSON writes one compact JSON document followed by a newline.
func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// jsonConfigErrors converts the per-file load errors; the result is never nil.
func jsonConfigErrors(core *app.App) []jsonConfigError {
	out := []jsonConfigError{}
	for _, fileErr := range core.Fleet.Errors() {
		out = append(out, jsonConfigError{Path: fileErr.Path, Error: fileErr.Err.Error()})
	}
	return out
}

// jsonApplyResultOf converts a convergence report; slices are never nil.
func jsonApplyResultOf(report app.ApplyReport) *jsonApplyResult {
	return &jsonApplyResult{
		RulesApplied:  report.RulesApplied,
		MountsApplied: report.MountsApplied,
		CachesApplied: report.CachesApplied,
		SkillsApplied: report.SkillsApplied,
		SkillsRemoved: report.SkillsRemoved,
		Warnings:      append([]string{}, report.Warnings...),
		Errors:        append([]string{}, report.Errors...),
	}
}

// jsonSandboxFromConfig fills the file-side fields of a sandbox. cfg is the
// Fleet.SandboxByName result; nil when the sandbox has no config directory.
func jsonSandboxFromConfig(name string, cfg *fleet.Sandbox) jsonSandbox {
	out := jsonSandbox{Name: name, Profiles: []string{}}
	if cfg == nil {
		return out
	}
	out.ConfigSlug = cfg.Slug
	out.ConfigPath = cfg.Dir
	out.Profiles = append(out.Profiles, cfg.App.Profiles...)
	out.Caches = len(cfg.App.Caches)
	out.Skills = len(cfg.App.Skills)
	out.Mounts = len(cfg.App.Mounts)
	out.Incomplete = cfg.App.Create != nil && cfg.App.Create.Incomplete
	return out
}

// jsonImportResult is one sandbox's import outcome.
type jsonImportResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Incomplete bool   `json:"incomplete"`
	Error      string `json:"error,omitempty"`
}

// jsonImportOutput is the import --json envelope.
type jsonImportOutput struct {
	SchemaVersion int                `json:"schemaVersion"`
	Created       int                `json:"created"`
	Configured    int                `json:"alreadyConfigured"`
	Failed        int                `json:"failed"`
	Results       []jsonImportResult `json:"results"`
}

// jsonImportReport converts an import report; the results are never nil.
func jsonImportReport(report app.ImportReport) jsonImportOutput {
	out := jsonImportOutput{
		SchemaVersion: jsonSchemaVersion,
		Created:       report.Created,
		Configured:    report.Configured,
		Failed:        report.Failed,
		Results:       make([]jsonImportResult, 0, len(report.Results)),
	}
	for _, res := range report.Results {
		out.Results = append(out.Results, jsonImportResult{
			Name:       res.Name,
			Status:     res.Status,
			Incomplete: res.Incomplete,
			Error:      res.Error,
		})
	}
	return out
}
