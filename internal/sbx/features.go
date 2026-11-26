package sbx

import (
	"errors"
	"fmt"
	"strings"
)

// Feature is one sandwarden feature that needs a minimum sbx version.
type Feature struct {
	ID         string
	Label      string
	MinVersion string
}

// Stable feature identifiers. They are part of the desktop compatibility DTO
// and the settings UI, so renaming one is a breaking change.
const (
	FeatureCreateSkills          = "create.skills"
	FeatureCreateKitArgs         = "create.kitArgs"
	FeatureCreateKitArgsFiles    = "create.kitArgsFiles"
	FeatureCreateEnvFiles        = "create.envFiles"
	FeatureStaticMCP             = "create.staticMcp"
	FeatureStaticMCPNone         = "create.staticMcp.none"
	FeatureCreateWorkspaces      = "create.workspaces"
	FeatureSbxenvWorkspaceTarget = "sbxenv.additionalWorkspaces.target"
	FeatureKitAddKitArg          = "kit.add.kitArg"
	FeatureSbxPort               = "sbx.port"
)

// featureTable lists every version-gated feature, oldest requirement first.
// The table order is the display order.
var featureTable = []Feature{
	{ID: FeatureCreateSkills, Label: "skills mode", MinVersion: "v0.43.0"},
	{ID: FeatureCreateKitArgs, Label: "kit arguments", MinVersion: "v0.43.0"},
	{ID: FeatureCreateKitArgsFiles, Label: "kit argument files", MinVersion: "v0.43.0"},
	{ID: FeatureCreateEnvFiles, Label: "environment files", MinVersion: "v0.43.0"},
	{ID: FeatureStaticMCP, Label: "static MCP servers", MinVersion: "v0.43.0"},
	{ID: FeatureKitAddKitArg, Label: "kit add --kit-arg", MinVersion: "v0.43.0"},
	{ID: FeatureStaticMCPNone, Label: "static MCP `none`", MinVersion: "v0.51.0"},
	{ID: FeatureCreateWorkspaces, Label: "workspace mount target", MinVersion: "v0.51.0"},
	{ID: FeatureSbxenvWorkspaceTarget, Label: "sbxenv additional workspace target", MinVersion: "v0.51.0"},
	{ID: FeatureSbxPort, Label: "sbx port ls/publish/unpublish", MinVersion: "v0.54.0"},
}

// Features returns the feature table. The slice is a copy: callers may sort or
// truncate it freely.
func Features() []Feature {
	out := make([]Feature, len(featureTable))
	copy(out, featureTable)
	return out
}

// featuresByID returns the table entries for ids, in table order, skipping
// blanks and unknown ids.
func featuresByID(ids ...string) []Feature {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	var out []Feature
	for _, f := range featureTable {
		if wanted[f.ID] {
			out = append(out, f)
		}
	}
	return out
}

// RequiredFeatures returns the features one `sbx create` invocation needs,
// deduplicated and in table order. An empty result means the create works on
// every sbx version.
func (o CreateOptions) RequiredFeatures() []Feature {
	var ids []string
	if strings.TrimSpace(o.Skills) != "" {
		ids = append(ids, FeatureCreateSkills)
	}
	if hasNonBlank(o.KitArgs) {
		ids = append(ids, FeatureCreateKitArgs)
	}
	if hasNonBlank(o.KitArgsFiles) {
		ids = append(ids, FeatureCreateKitArgsFiles)
	}
	if hasNonBlank(o.EnvFiles) {
		ids = append(ids, FeatureCreateEnvFiles)
	}
	if hasNonBlank(o.StaticMCP) {
		ids = append(ids, FeatureStaticMCP)
	}
	if staticMCPHasNone(o.StaticMCP) {
		ids = append(ids, FeatureStaticMCPNone)
	}
	if anyWorkspaceHasTarget(o.Workspaces) {
		ids = append(ids, FeatureCreateWorkspaces)
	}
	return featuresByID(ids...)
}

// KitAddFeatures returns the features `sbx kit add` needs for the given
// arguments. Passing kit arguments needs the --kit-arg flag.
func KitAddFeatures(kitArgs []string) []Feature {
	if !hasNonBlank(kitArgs) {
		return nil
	}
	return featuresByID(FeatureKitAddKitArg)
}

// Incompatible filters fs down to the features the installed version cannot
// run. An unknown version is compatible with everything.
func Incompatible(v Version, fs []Feature) []Feature {
	var out []Feature
	for _, f := range fs {
		if !v.AtLeast(f.MinVersion) {
			out = append(out, f)
		}
	}
	return out
}

// Messages renders one human sentence per incompatible feature, for the
// validate, doctor and sandbox-detail reports.
func Messages(v Version, fs []Feature) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, fmt.Sprintf("%s (%s) requires sbx %s or later; installed sbx is %s", f.Label, f.ID, f.MinVersion, v))
	}
	return out
}

// FormatIncompatible returns one error describing every feature of fs the
// installed version cannot run, or nil when all of them are supported. An
// unknown installed version is compatible with everything.
func FormatIncompatible(v Version, fs []Feature) error {
	bad := Incompatible(v, fs)
	if len(bad) == 0 {
		return nil
	}
	return errors.New(strings.Join(Messages(v, bad), "; "))
}

// hasNonBlank reports whether at least one entry is non-empty after trimming.
func hasNonBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// staticMCPHasNone reports whether the static MCP set carries the literal
// `none` value, which became a valid choice only in v0.51.0. Values may be
// comma-separated, so each entry is split.
func staticMCPHasNone(values []string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.TrimSpace(part) == "none" {
				return true
			}
		}
	}
	return false
}

// anyWorkspaceHasTarget reports whether a create workspace carries a custom
// mount target. The syntax is PATH[:TARGET][:ro]; a two-part PATH:ro value has
// no target.
func anyWorkspaceHasTarget(workspaces []string) bool {
	for _, workspace := range workspaces {
		if workspaceHasTarget(workspace) {
			return true
		}
	}
	return false
}

// workspaceHasTarget reports whether one workspace entry sets a container
// target. The :ro suffix is stripped first, so PATH:ro is a read-only path
// without a target.
func workspaceHasTarget(workspace string) bool {
	value := strings.TrimSpace(workspace)
	if value == "" {
		return false
	}
	if strings.HasSuffix(value, ":ro") {
		value = strings.TrimSpace(strings.TrimSuffix(value, ":ro"))
	}
	return strings.Contains(value, ":")
}
