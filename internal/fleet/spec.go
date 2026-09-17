package fleet

import "strings"

// Spec is the subset of the sbx kit schema v2 sandwarden reads and writes.
// Decoding is strict (unknown fields are errors), mirroring sbx's own parser;
// `sbx kit validate` remains the reference validator. Credentials and other
// opaque blocks are carried through untouched.
type Spec struct {
	SchemaVersion string          `yaml:"schemaVersion" json:"schemaVersion"`
	Kind          string          `yaml:"kind" json:"kind"`
	Name          string          `yaml:"name" json:"name"`
	DisplayName   string          `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Description   string          `yaml:"description,omitempty" json:"description,omitempty"`
	SourceURL     string          `yaml:"sourceURL,omitempty" json:"sourceURL,omitempty"`
	Requires      *SpecRequires   `yaml:"requires,omitempty" json:"requires,omitempty"`
	Sandbox       *SpecSandbox    `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	Environment   *SpecEnv        `yaml:"environment,omitempty" json:"environment,omitempty"`
	Ports         []SpecPort      `yaml:"ports,omitempty" json:"ports,omitempty"`
	Permissions   *SpecPermission `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Setup         *SpecSetup      `yaml:"setup,omitempty" json:"setup,omitempty"`
	Credentials   []any           `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	Arguments     map[string]any  `yaml:"arguments,omitempty" json:"arguments,omitempty"`
}

// SpecRequires names the base agent a mixin attaches to.
type SpecRequires struct {
	Agent string `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// SpecSandbox carries the fields of a kind: sandbox kit; sandwarden only
// round-trips them.
type SpecSandbox struct {
	Image      string        `yaml:"image,omitempty" json:"image,omitempty"`
	Entrypoint []string      `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	Command    *SpecCommands `yaml:"command,omitempty" json:"command,omitempty"`
	Resources  *SpecResource `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// SpecCommands is a kit's default and interactive command lines.
type SpecCommands struct {
	Default     CommandLine `yaml:"default,omitempty" json:"default,omitempty"`
	Interactive CommandLine `yaml:"interactive,omitempty" json:"interactive,omitempty"`
}

// SpecResource is a sandbox kit's cpu and memory request.
type SpecResource struct {
	CPU    float64 `yaml:"cpu,omitempty" json:"cpu,omitempty"`
	Memory string  `yaml:"memory,omitempty" json:"memory,omitempty"`
}

// SpecEnv declares environment variables injected into the sandbox.
type SpecEnv struct {
	Variables map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// SpecPort is a container port a kit opens.
type SpecPort struct {
	Container int    `yaml:"container" json:"container"`
	Protocol  string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	Name      string `yaml:"name,omitempty" json:"name,omitempty"`
}

// SpecPermission is the kit's permission block.
type SpecPermission struct {
	Network *SpecNetwork `yaml:"network,omitempty" json:"network,omitempty"`
}

// SpecNetwork is a kit's network allow and deny lists.
type SpecNetwork struct {
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty"`
}

// SpecSetup is a kit's install and startup hooks and injected files.
type SpecSetup struct {
	Install []SpecCommand `yaml:"install,omitempty" json:"install,omitempty"`
	Startup []SpecCommand `yaml:"startup,omitempty" json:"startup,omitempty"`
	Files   []SpecFile    `yaml:"files,omitempty" json:"files,omitempty"`
}

// SpecCommand is one setup command; the command may be a string or an argv
// array, as sbx accepts both.
type SpecCommand struct {
	Command     CommandLine `yaml:"command" json:"command"`
	User        string      `yaml:"user,omitempty" json:"user,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
}

// SpecFile is a file a kit injects into the sandbox.
type SpecFile struct {
	Path        string `yaml:"path" json:"path"`
	Mode        string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// CommandLine decodes either a scalar command line or an argv array, joining
// an array with spaces exactly as sbx does.
type CommandLine string

// UnmarshalYAML accepts a string or a list of strings.
func (c *CommandLine) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*c = CommandLine(s)
		return nil
	}
	var parts []string
	if err := unmarshal(&parts); err != nil {
		return err
	}
	*c = CommandLine(strings.Join(parts, " "))
	return nil
}

// NewMixin returns an empty schema v2 mixin spec with the given name.
func NewMixin(name string) Spec {
	return Spec{SchemaVersion: "2", Kind: "mixin", Name: name}
}

// NetworkAllow returns the spec's allow list, or nil.
func (s Spec) NetworkAllow() []string {
	if s.Permissions == nil || s.Permissions.Network == nil {
		return nil
	}
	return s.Permissions.Network.Allow
}

// NetworkDeny returns the spec's deny list, or nil.
func (s Spec) NetworkDeny() []string {
	if s.Permissions == nil || s.Permissions.Network == nil {
		return nil
	}
	return s.Permissions.Network.Deny
}

// Agent returns the base agent a mixin requires, or "".
func (s Spec) Agent() string {
	if s.Requires == nil {
		return ""
	}
	return s.Requires.Agent
}

// Env returns the spec's environment variables, or nil.
func (s Spec) Env() map[string]string {
	if s.Environment == nil {
		return nil
	}
	return s.Environment.Variables
}
