package sbx

import (
	"encoding/json"
	"time"
)

// Sandbox is the daemon's description of a sandbox (GET /sandbox,
// GET /sandbox/{name}).
type Sandbox struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Agent                *string           `json:"agent,omitempty"`
	Status               string            `json:"status"`
	Workspace            string            `json:"workspace"`
	Profile              *string           `json:"profile,omitempty"`
	CreatedAt            *time.Time        `json:"created_at,omitempty"`
	StoppedAt            *time.Time        `json:"stopped_at,omitempty"`
	Ports                []PublishedPort   `json:"ports,omitempty"`
	AdditionalWorkspaces []WorkspaceMount  `json:"additional_workspaces,omitempty"`
	MountPolicyDenied    *bool             `json:"mount_policy_denied,omitempty"`
	SourceRepoDir        *string           `json:"source_repo_dir,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
}

// Running reports whether the sandbox's last-known status is running.
func (s Sandbox) Running() bool { return s.Status == "running" }

// AgentName returns the agent string, or "" when unset.
func (s Sandbox) AgentName() string {
	if s.Agent == nil {
		return ""
	}
	return *s.Agent
}

// WorkspaceMount is a host directory bind-mounted into a sandbox.
type WorkspaceMount struct {
	Dir      string `json:"dir"`
	ReadOnly *bool  `json:"read_only,omitempty"`
}

// IsReadOnly reports the mount's read-only intent (absent means read-write).
func (m WorkspaceMount) IsReadOnly() bool { return m.ReadOnly != nil && *m.ReadOnly }

// PublishedPort is one host <-> sandbox port mapping.
type PublishedPort struct {
	HostIP      string `json:"host_ip"`
	HostPort    int    `json:"host_port"`
	Protocol    string `json:"protocol"`
	SandboxPort int    `json:"sandbox_port"`
}

// PolicyRule is one rule from GET /policy/network/rules.
type PolicyRule struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	PolicyID     string   `json:"policy_id"`
	Scope        string   `json:"scope"`
	AppliesTo    string   `json:"applies_to"`
	SandboxID    string   `json:"sandbox_id,omitempty"`
	ResourceType string   `json:"resource_type"`
	Decision     string   `json:"decision"`
	Resources    []string `json:"resources"`
	Origin       string   `json:"origin"`
	Status       string   `json:"status"`
	Editable     bool     `json:"editable"`
}

// PolicyLog is the GET /policy/network/log response.
type PolicyLog struct {
	BlockedHosts []LogEntry `json:"blocked_hosts"`
	AllowedHosts []LogEntry `json:"allowed_hosts"`
}

// LogEntry is one allowed/blocked host record from the proxy.
type LogEntry struct {
	Host       string `json:"host"`
	VMName     string `json:"vm_name"`
	ProxyType  string `json:"proxy_type"`
	Rule       string `json:"rule"`
	LastSeen   string `json:"last_seen"`
	Since      string `json:"since"`
	CountSince int    `json:"count_since"`
}

// Event is one record from the daemon's GET /events stream. Unknown fields are
// preserved in Raw for callers that need the full payload.
type Event struct {
	Action    string `json:"action"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	// SandboxName is the sandbox the event concerns, when the daemon provides it.
	SandboxName string          `json:"sandbox_name,omitempty"`
	Raw         json.RawMessage `json:"-"`
}

// MountInfo is a host path bind-mounted into a sandbox at runtime
// (`sbx inspect --json` → runtime_mounts).
type MountInfo struct {
	HostPath string `json:"host_path"`
	Target   string `json:"container_target,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// InspectDetail is the subset of `sbx inspect --json` the UI needs. It carries
// runtime mounts, which have no REST equivalent.
type InspectDetail struct {
	Workspace     string      `json:"workspace"`
	RuntimeMounts []MountInfo `json:"runtime_mounts"`
	Mounts        []MountInfo `json:"mounts"`
	Kits          []string    `json:"kits"`
	Image         string      `json:"image"`
	ImageDigest   string      `json:"image_digest"`
}

// UnmarshalJSON accepts both the container_target field and the target alias
// used by some sbx builds.
func (m *MountInfo) UnmarshalJSON(data []byte) error {
	var raw struct {
		HostPath        string `json:"host_path"`
		ContainerTarget string `json:"container_target"`
		Target          string `json:"target"`
		ReadOnly        bool   `json:"read_only"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.HostPath = raw.HostPath
	m.Target = raw.ContainerTarget
	if m.Target == "" {
		m.Target = raw.Target
	}
	m.ReadOnly = raw.ReadOnly
	return nil
}
