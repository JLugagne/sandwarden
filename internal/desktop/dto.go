package desktop

import (
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// CreateSandboxRequest mirrors the create form payload.
type CreateSandboxRequest struct {
	Agent        string            `json:"agent"`
	Workspaces   []WorkspaceSpec   `json:"workspaces"`
	Name         string            `json:"name"`
	CPUs         int               `json:"cpus"`
	Memory       string            `json:"memory"`
	Profile      string            `json:"profile"`
	Template     string            `json:"template"`
	Kits         []string          `json:"kits"`
	Profiles     []string          `json:"profiles"`
	Caches       []string          `json:"caches"`
	Skills       []SkillRefRequest `json:"skills"`
	RunArgs      string            `json:"run_args"`
	Mounts       []MountRequest    `json:"mounts"`
	Publish      []string          `json:"publish"`
	Env          []string          `json:"env"`
	DenyNetwork  []string          `json:"deny_network"`
	Clone        bool              `json:"clone"`
	AttachCaches bool              `json:"attach_caches"`
	JobID        string            `json:"job_id"`
}

// WorkspaceSpec is one workspace entry of the create form.
type WorkspaceSpec struct {
	Path     string `json:"path"`
	ReadOnly bool   `json:"read_only"`
}

// MountRequest is the add-mount payload.
type MountRequest struct {
	Path     string `json:"path"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ExecRequest is the one-shot command payload.
type ExecRequest struct {
	Command string `json:"command"`
	JobID   string `json:"job_id"`
}

// ProfileRequest is the profile create/update payload.
type ProfileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	IsGlobal    bool   `json:"is_global"`
}

// RuleRequest is the profile rule payload.
type RuleRequest struct {
	Decision string `json:"decision"`
	Pattern  string `json:"pattern"`
}

// RulesRequest carries several patterns sharing one allow/deny decision.
type RulesRequest struct {
	Decision string   `json:"decision"`
	Patterns []string `json:"patterns"`
}

// ServiceSecretRequest is the service secret payload.
type ServiceSecretRequest struct {
	Service   string `json:"service"`
	Scope     string `json:"scope"`
	Value     string `json:"value"`
	Ref       string `json:"ref"`
	Command   string `json:"command"`
	Refresh   string `json:"refresh"`
	Overwrite bool   `json:"overwrite"`
}

// RegistrySecretRequest is the registry secret payload.
type RegistrySecretRequest struct {
	Host      string `json:"host"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Scope     string `json:"scope"`
	Overwrite bool   `json:"overwrite"`
}

// CustomSecretRequest is the custom (placeholder) secret payload.
type CustomSecretRequest struct {
	Hosts       []string `json:"hosts"`
	Env         string   `json:"env"`
	Value       string   `json:"value"`
	Ref         string   `json:"ref"`
	Command     string   `json:"command"`
	Placeholder string   `json:"placeholder"`
	Refresh     string   `json:"refresh"`
	Scope       string   `json:"scope"`
	Overwrite   bool     `json:"overwrite"`
}

// ImportSecretsRequest is the bulk import payload.
type ImportSecretsRequest struct {
	Service string `json:"service"`
	All     bool   `json:"all"`
	DryRun  bool   `json:"dry_run"`
	Force   bool   `json:"force"`
	JobID   string `json:"job_id"`
}

// KitStoreRequest is the kit repository create/update payload.
type KitStoreRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	Auth        string `json:"auth"`
}

// CacheInput is the shared-cache create/update payload.
type CacheInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HostPath    string `json:"host_path"`
	TargetPath  string `json:"target_path"`
	ReadOnly    bool   `json:"read_only"`
	AutoAttach  bool   `json:"auto_attach"`
	Enabled     bool   `json:"enabled"`
}

// PolicyActionRequest is the policy action payload.
type PolicyActionRequest struct {
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
	SandboxID string   `json:"sandbox_id"`
	ID        string   `json:"id"`
}

// Health reports the daemon socket and CLI in use.
type Health struct {
	OK            bool   `json:"ok"`
	Socket        string `json:"socket"`
	SbxBinary     string `json:"sbx_binary"`
	DaemonRunning bool   `json:"daemon_running"`
	DaemonStatus  string `json:"daemon_status"`
}

// ReapplyResult reports the outcome of re-applying a sandbox's caches.
type ReapplyResult struct {
	Applied int      `json:"applied"`
	Errors  []string `json:"errors"`
}

// SkillStoreRequest mirrors the skill store form payload.
type SkillStoreRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	Auth        string `json:"auth"`
}

// ProfileMountRequest is the default profile mount payload.
type ProfileMountRequest struct {
	HostPath   string `json:"host_path"`
	TargetPath string `json:"target_path"`
	ReadOnly   bool   `json:"read_only"`
}

// SkillRefRequest identifies one catalog item by store slug, kind and name.
type SkillRefRequest struct {
	Store string `json:"store"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}

// SkillRefToFleet maps the binding payload onto the fleet reference.
func (r SkillRefRequest) SkillRefToFleet() fleet.SkillRef {
	return fleet.SkillRef{
		Store: strings.TrimSpace(r.Store),
		Kind:  strings.TrimSpace(r.Kind),
		Name:  strings.TrimSpace(r.Name),
	}
}

// ImportSandboxesRequest is the bulk import payload: an empty names list
// imports every daemon sandbox.
type ImportSandboxesRequest struct {
	Names []string `json:"names"`
}

// CompleteSandboxRequest records the create parameters an imported sandbox
// could not recover from the daemon.
type CompleteSandboxRequest struct {
	CPUs   int      `json:"cpus"`
	Memory string   `json:"memory"`
	Env    []string `json:"env"`
}
