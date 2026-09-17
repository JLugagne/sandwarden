package fleet

import "strings"

// SandboxApp is the sandwarden sidecar of a sandbox directory: everything the
// kit spec cannot express, kept next to spec.yaml.
type SandboxApp struct {
	// Sandbox is the canonical sandboxd name; the directory slug may differ.
	Sandbox  string         `yaml:"sandbox" json:"sandbox"`
	Create   *SandboxCreate `yaml:"create,omitempty" json:"create,omitempty"`
	Profiles []string       `yaml:"profiles,omitempty" json:"profiles"`
	Caches   []string       `yaml:"caches,omitempty" json:"caches"`
	Skills   []SkillRef     `yaml:"skills,omitempty" json:"skills"`
	Mounts   []MountRef     `yaml:"mounts,omitempty" json:"mounts"`
	RunArgs  string         `yaml:"runArgs,omitempty" json:"run_args"`
	OptOuts  *SandboxOptOut `yaml:"optOuts,omitempty" json:"opt_outs,omitempty"`
}

// SandboxCreate records the parameters a sandbox was created with so it can
// be recreated identically from its files.
type SandboxCreate struct {
	CPUs          int      `yaml:"cpus,omitempty" json:"cpus"`
	Memory        string   `yaml:"memory,omitempty" json:"memory"`
	Workspaces    []string `yaml:"workspaces,omitempty" json:"workspaces"`
	Clone         bool     `yaml:"clone,omitempty" json:"clone"`
	Template      string   `yaml:"template,omitempty" json:"template"`
	DaemonProfile string   `yaml:"daemonProfile,omitempty" json:"daemon_profile"`
	Publish       []string `yaml:"publish,omitempty" json:"publish"`
	Kits          []string `yaml:"kits,omitempty" json:"kits"`
	// Incomplete marks a sandbox imported without all create parameters.
	Incomplete bool `yaml:"incomplete,omitempty" json:"incomplete"`
}

// SandboxOptOut records the inherited items a sandbox deliberately detached.
type SandboxOptOut struct {
	Mounts []string `yaml:"mounts,omitempty" json:"mounts"`
	Caches []string `yaml:"caches,omitempty" json:"caches"`
}

// ProfileApp is the sandwarden sidecar of a profile directory: the items a
// profile carries beyond its kit network rules.
type ProfileApp struct {
	Default bool       `yaml:"default,omitempty" json:"default"`
	Global  bool       `yaml:"global,omitempty" json:"global"`
	Mounts  []MountRef `yaml:"mounts,omitempty" json:"mounts"`
	Caches  []string   `yaml:"caches,omitempty" json:"caches"`
	Skills  []SkillRef `yaml:"skills,omitempty" json:"skills"`
}

// CacheApp is a shared cache definition.
type CacheApp struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description"`
	HostPath    string `yaml:"hostPath" json:"host_path"`
	TargetPath  string `yaml:"targetPath,omitempty" json:"target_path"`
	ReadOnly    bool   `yaml:"readOnly,omitempty" json:"read_only"`
	AutoAttach  *bool  `yaml:"autoAttach,omitempty" json:"auto_attach"`
	Enabled     *bool  `yaml:"enabled,omitempty" json:"enabled"`
}

// AutoAttaches reports whether new sandboxes receive the cache by default.
func (c CacheApp) AutoAttaches() bool {
	return c.AutoAttach == nil || *c.AutoAttach
}

// IsEnabled reports whether the cache may be mounted at all.
func (c CacheApp) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// SkillRef points at one catalog item by store slug, kind and name.
type SkillRef struct {
	Store string `yaml:"store" json:"store"`
	Kind  string `yaml:"kind" json:"kind"`
	Name  string `yaml:"name" json:"name"`
}

// MountRef is a host bind mount: empty TargetPath keeps the host path.
type MountRef struct {
	HostPath   string `yaml:"hostPath" json:"host_path"`
	TargetPath string `yaml:"targetPath,omitempty" json:"target_path"`
	ReadOnly   bool   `yaml:"readOnly,omitempty" json:"read_only"`
}

// EffectiveTarget is the container path a mount lands on.
func (m MountRef) EffectiveTarget() string {
	if strings.TrimSpace(m.TargetPath) == "" {
		return m.HostPath
	}
	return m.TargetPath
}

// Key identifies a mount by host and target path.
func (m MountRef) Key() string {
	return m.HostPath + ":" + m.TargetPath
}

// StoreKind separates the two git store registries.
type StoreKind string

const (
	// StoreSkills is the skill and command store registry.
	StoreSkills StoreKind = "skill"
	// StoreKits is the kit repository registry.
	StoreKits StoreKind = "kit"
)

// dir is the on-disk registry directory for the kind.
func (k StoreKind) dir() string {
	if k == StoreKits {
		return "kits"
	}
	return "skills"
}

// StoreReg is a git store registration; the catalog itself is rediscovered
// from the checkout.
type StoreReg struct {
	Kind        StoreKind `yaml:"-" json:"kind"`
	Slug        string    `yaml:"-" json:"slug"`
	Name        string    `yaml:"name" json:"name"`
	Description string    `yaml:"description,omitempty" json:"description"`
	URL         string    `yaml:"url" json:"url"`
	Ref         string    `yaml:"ref,omitempty" json:"ref"`
	Auth        string    `yaml:"auth,omitempty" json:"auth"`
}

// TerminalPrefs is the global terminal-launcher preference.
type TerminalPrefs struct {
	Enabled *[]string `yaml:"enabled" json:"enabled"`
	Default string    `yaml:"default,omitempty" json:"default"`
}

// AppConfig is the global sandwarden configuration file.
type AppConfig struct {
	Terminals     TerminalPrefs `yaml:"terminals,omitempty" json:"terminals"`
	Notifications bool          `yaml:"notifications" json:"notifications"`
}

// EffectiveTarget is the container path a cache binds to.
func (c CacheApp) EffectiveTarget() string {
	if strings.TrimSpace(c.TargetPath) == "" {
		return c.HostPath
	}
	return c.TargetPath
}
