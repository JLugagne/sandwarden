package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/goccy/go-yaml"
)

// SbxenvExport is one sandbox rendered as an sbxenv.yaml document plus the
// sandbox fields the format cannot express.
type SbxenvExport struct {
	Slug        string
	Name        string
	Document    []byte
	Unsupported []string
}

// sbxenvSchemaVersion is the only schemaVersion `sbx env` accepts.
const sbxenvSchemaVersion = 1

// sbxenvDocument is the subset of the sbxenv.yaml schema sandwarden fills.
// The field order is the document order, so the output is deterministic.
type sbxenvDocument struct {
	SchemaVersion int              `yaml:"schemaVersion"`
	Name          string           `yaml:"name,omitempty"`
	Agent         string           `yaml:"agent"`
	Kits          []string         `yaml:"kits,omitempty"`
	Workspace     *sbxenvWorkspace `yaml:"workspace,omitempty"`
	Env           yaml.MapSlice    `yaml:"env,omitempty"`
	Ports         []sbxenvPort     `yaml:"ports,omitempty"`
}

// sbxenvWorkspace is the single read-write workspace sbxenv mounts, cloned
// into the sandbox when Clone is set.
type sbxenvWorkspace struct {
	Path  string `yaml:"path"`
	Clone bool   `yaml:"clone,omitempty"`
}

// sbxenvPort is one published port. An omitted host asks sbxenv for an
// ephemeral one, as `sbx create -p SANDBOX_PORT` does.
type sbxenvPort struct {
	Sandbox  int    `yaml:"sandbox"`
	Host     int    `yaml:"host,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`
}

// sbxenvProtocols are the protocols `sbx env` accepts in a port binding.
var sbxenvProtocols = map[string]bool{
	"tcp": true, "tcp4": true, "tcp6": true,
	"udp": true, "udp4": true, "udp6": true,
}

// ExportSbxenv renders a sandbox's spec and sidecar as an sbxenv.yaml
// document. The sandbox is resolved by canonical name first, then by slug.
// The document is built from the files alone: neither the daemon nor
// `sbx env` is needed. Every field sbxenv cannot express is listed in
// Unsupported instead of being dropped silently.
func (a *App) ExportSbxenv(name string) (SbxenvExport, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SbxenvExport{}, errors.New("sandbox name is required")
	}
	s, ok := a.Fleet.SandboxByName(name)
	if !ok {
		s, ok = a.Fleet.Sandbox(name)
	}
	if !ok {
		return SbxenvExport{}, fmt.Errorf("sandbox %q not found in the config directory", name)
	}
	return renderSbxenv(s)
}

// renderSbxenv maps one sandbox onto the sbxenv schema.
func renderSbxenv(s *fleet.Sandbox) (SbxenvExport, error) {
	agent := strings.TrimSpace(s.Spec.Agent())
	if agent == "" {
		return SbxenvExport{}, fmt.Errorf("sandbox %q has no requires.agent; sbxenv.yaml requires an agent", s.Label())
	}
	var report []string
	doc := sbxenvDocument{SchemaVersion: sbxenvSchemaVersion, Agent: agent}
	doc.Name = sbxenvName(s, &report)
	if c := s.App.Create; c != nil {
		doc.Kits = cleanStrings(c.Kits)
		for i, kit := range doc.Kits {
			if sbxenvRelativeSource(kit) {
				report = append(report, fmt.Sprintf("create.kits[%d] (%s): sbxenv resolves a relative kit source against the sbxenv.yaml file", i, kit))
			}
		}
		doc.Workspace = sbxenvWorkspaceFrom(s, &report)
		doc.Ports = sbxenvPortsFrom(c.Publish, &report)
		sbxenvCreateReport(c, &report)
	}
	doc.Env = sbxenvEnv(s, &report)
	sbxenvSpecReport(s, doc.Name, &report)
	sbxenvSidecarReport(s, &report)
	document, err := yaml.Marshal(doc)
	if err != nil {
		return SbxenvExport{}, fmt.Errorf("encode sbxenv.yaml for %q: %w", s.Label(), err)
	}
	return SbxenvExport{Slug: s.Slug, Name: s.Name(), Document: document, Unsupported: report}, nil
}

// sbxenvName returns the canonical sandbox name when sbxenv accepts it, and
// reports the name otherwise.
func sbxenvName(s *fleet.Sandbox, report *[]string) string {
	name := strings.TrimSpace(s.Name())
	if sbxenvNameOK(name) {
		return name
	}
	*report = append(*report, fmt.Sprintf("name %q cannot be expressed in sbxenv.yaml (sbxenv names match ^[a-zA-Z0-9][a-zA-Z0-9.-]+$, need at least two characters and cannot be \"default\")", name))
	return ""
}

// sbxenvNameOK mirrors the name rule `sbx env` enforces.
func sbxenvNameOK(name string) bool {
	if len(name) < 2 || name == "default" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case (c == '.' || c == '-') && i > 0:
		default:
			return false
		}
	}
	return true
}

// sbxenvWorkspaceFrom maps create.workspaces onto the single read-write
// workspace sbxenv supports and carries create.clone over as workspace.clone.
func sbxenvWorkspaceFrom(s *fleet.Sandbox, report *[]string) *sbxenvWorkspace {
	c := s.App.Create
	if c == nil {
		return nil
	}
	var workspace *sbxenvWorkspace
	for i, raw := range c.Workspaces {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		path, readOnly := sbxenvSplitWorkspace(raw)
		if readOnly {
			sbxenvUnsupported(report, fmt.Sprintf("create.workspaces[%d] (%s)", i, raw), "sbxenv mounts one read-write workspace")
			continue
		}
		if workspace != nil {
			sbxenvUnsupported(report, fmt.Sprintf("create.workspaces[%d] (%s)", i, raw), fmt.Sprintf("sbxenv mounts one workspace, %s is used", workspace.Path))
			continue
		}
		if path != "" && !filepath.IsAbs(path) {
			*report = append(*report, fmt.Sprintf("create.workspaces[%d] (%s): sbxenv resolves a relative workspace path against the sbxenv.yaml file", i, path))
		}
		workspace = &sbxenvWorkspace{Path: path, Clone: c.Clone}
	}
	if c.Clone && workspace == nil {
		sbxenvUnsupported(report, "create.clone", "workspace.clone needs a workspace path")
	}
	return workspace
}

// sbxenvSplitWorkspace separates a workspace path from its read-only marker.
func sbxenvSplitWorkspace(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if strings.HasSuffix(value, ":ro") {
		return strings.TrimSpace(strings.TrimSuffix(value, ":ro")), true
	}
	return value, false
}

// sbxenvEnv maps environment.variables onto the env mapping, in key order. A
// bare key inherits its value from the host environment at create time, which
// sbxenv cannot express.
func sbxenvEnv(s *fleet.Sandbox, report *[]string) yaml.MapSlice {
	vars := s.Spec.Env()
	if len(vars) == 0 {
		return nil
	}
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make(yaml.MapSlice, 0, len(keys))
	for _, key := range keys {
		if vars[key] == "" {
			sbxenvUnsupported(report, fmt.Sprintf("environment.variables[%s]", key), "a bare key takes its value from the host environment")
			continue
		}
		env = append(env, yaml.MapItem{Key: key, Value: vars[key]})
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

// sbxenvPortsFrom maps create.publish onto sbxenv port bindings.
func sbxenvPortsFrom(publish []string, report *[]string) []sbxenvPort {
	var ports []sbxenvPort
	for i, raw := range publish {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		port, err := sbxenvParsePort(raw)
		if err != nil {
			sbxenvUnsupported(report, fmt.Sprintf("create.publish[%d] (%s)", i, raw), err.Error())
			continue
		}
		ports = append(ports, port)
	}
	return ports
}

// sbxenvParsePort reads one `[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]`
// value, the syntax `sbx create -p` accepts.
func sbxenvParsePort(raw string) (sbxenvPort, error) {
	spec := strings.TrimSpace(raw)
	protocol := ""
	if slash := strings.LastIndex(spec, "/"); slash >= 0 {
		protocol = strings.ToLower(strings.TrimSpace(spec[slash+1:]))
		spec = strings.TrimSpace(spec[:slash])
		if !sbxenvProtocols[protocol] {
			return sbxenvPort{}, fmt.Errorf("unsupported protocol %q", protocol)
		}
	}
	parts := strings.Split(spec, ":")
	if len(parts) > 2 {
		return sbxenvPort{}, errors.New("a host IP binding cannot be expressed")
	}
	var host, sandbox int
	var err error
	switch len(parts) {
	case 1:
		sandbox, err = sbxenvPortNumber(parts[0], "sandbox")
	case 2:
		host, err = sbxenvPortNumber(parts[0], "host")
		if err == nil {
			sandbox, err = sbxenvPortNumber(parts[1], "sandbox")
		}
	default:
		return sbxenvPort{}, errors.New("empty port")
	}
	if err != nil {
		return sbxenvPort{}, err
	}
	return sbxenvPort{Sandbox: sandbox, Host: host, Protocol: protocol}, nil
}

// sbxenvPortNumber parses one port and enforces sbxenv's range.
func sbxenvPortNumber(raw, kind string) (int, error) {
	value := strings.TrimSpace(raw)
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s port %q", kind, value)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("%s port %d out of range (1-65535)", kind, n)
	}
	return n, nil
}

// sbxenvCreateReport lists the recorded create parameters sbxenv cannot hold.
func sbxenvCreateReport(c *fleet.SandboxCreate, report *[]string) {
	if c.CPUs > 0 {
		sbxenvUnsupported(report, "create.cpus", strconv.Itoa(c.CPUs))
	}
	if c.Memory != "" {
		sbxenvUnsupported(report, "create.memory", c.Memory)
	}
	if c.Template != "" {
		sbxenvUnsupported(report, "create.template", c.Template)
	}
	if c.DaemonProfile != "" {
		sbxenvUnsupported(report, "create.daemonProfile", c.DaemonProfile)
	}
	if c.Incomplete {
		*report = append(*report, "create is marked incomplete; the export may be missing create parameters")
	}
}

// sbxenvSpecReport lists the spec fields sbxenv cannot hold.
func sbxenvSpecReport(s *fleet.Sandbox, exportedName string, report *[]string) {
	spec := s.Spec
	if d := strings.TrimSpace(spec.DisplayName); d != "" && d != exportedName && d != strings.TrimSpace(s.Name()) {
		sbxenvUnsupported(report, "displayName", d)
	}
	if spec.Description != "" {
		sbxenvUnsupported(report, "description", spec.Description)
	}
	if spec.SourceURL != "" {
		sbxenvUnsupported(report, "sourceURL", spec.SourceURL)
	}
	if box := spec.Sandbox; box != nil {
		if box.Image != "" {
			sbxenvUnsupported(report, "spec.sandbox.image", box.Image)
		}
		if len(box.Entrypoint) > 0 {
			sbxenvUnsupported(report, "spec.sandbox.entrypoint", strings.Join(box.Entrypoint, " "))
		}
		if box.Command != nil && (box.Command.Default != "" || box.Command.Interactive != "") {
			sbxenvUnsupported(report, "spec.sandbox.command", "")
		}
		if box.Resources != nil {
			if box.Resources.CPU != 0 {
				sbxenvUnsupported(report, "spec.sandbox.resources.cpu", strconv.FormatFloat(box.Resources.CPU, 'g', -1, 64))
			}
			if box.Resources.Memory != "" {
				sbxenvUnsupported(report, "spec.sandbox.resources.memory", box.Resources.Memory)
			}
		}
	}
	if len(spec.Ports) > 0 {
		values := make([]string, 0, len(spec.Ports))
		for _, port := range spec.Ports {
			value := strconv.Itoa(port.Container)
			if port.Protocol != "" {
				value += "/" + port.Protocol
			}
			values = append(values, value)
		}
		sbxenvUnsupported(report, "spec.ports", strings.Join(values, ", "))
	}
	if allow := cleanStrings(spec.NetworkAllow()); len(allow) > 0 {
		sbxenvUnsupported(report, "permissions.network.allow", strings.Join(allow, ", "))
	}
	if deny := cleanStrings(spec.NetworkDeny()); len(deny) > 0 {
		sbxenvUnsupported(report, "permissions.network.deny", strings.Join(deny, ", "))
	}
	if setup := spec.Setup; setup != nil {
		if n := len(setup.Install); n > 0 {
			sbxenvUnsupported(report, "spec.setup.install", sbxenvEntryCount(n))
		}
		if n := len(setup.Startup); n > 0 {
			sbxenvUnsupported(report, "spec.setup.startup", sbxenvEntryCount(n))
		}
		if n := len(setup.Files); n > 0 {
			sbxenvUnsupported(report, "spec.setup.files", sbxenvEntryCount(n))
		}
	}
	if n := len(spec.Credentials); n > 0 {
		sbxenvUnsupported(report, "spec.credentials", sbxenvEntryCount(n))
	}
	if len(spec.Arguments) > 0 {
		keys := make([]string, 0, len(spec.Arguments))
		for key := range spec.Arguments {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sbxenvUnsupported(report, "spec.arguments", strings.Join(keys, ", "))
	}
}

// sbxenvSidecarReport lists the sidecar fields sbxenv cannot hold.
func sbxenvSidecarReport(s *fleet.Sandbox, report *[]string) {
	if values := cleanStrings(s.App.Profiles); len(values) > 0 {
		sbxenvUnsupported(report, "profiles", strings.Join(values, ", "))
	}
	if values := cleanStrings(s.App.Caches); len(values) > 0 {
		sbxenvUnsupported(report, "caches", strings.Join(values, ", "))
	}
	if len(s.App.Skills) > 0 {
		values := make([]string, 0, len(s.App.Skills))
		for _, skill := range s.App.Skills {
			values = append(values, skill.Store+":"+skill.Kind+":"+skill.Name)
		}
		sbxenvUnsupported(report, "skills", strings.Join(values, ", "))
	}
	if len(s.App.Mounts) > 0 {
		values := make([]string, 0, len(s.App.Mounts))
		for _, mount := range s.App.Mounts {
			values = append(values, mount.Key())
		}
		sbxenvUnsupported(report, "mounts", strings.Join(values, ", "))
	}
	if args := strings.TrimSpace(s.App.RunArgs); args != "" {
		sbxenvUnsupported(report, "runArgs", args)
	}
	if optOuts := s.App.OptOuts; optOuts != nil {
		if values := cleanStrings(optOuts.Mounts); len(values) > 0 {
			sbxenvUnsupported(report, "optOuts.mounts", strings.Join(values, ", "))
		}
		if values := cleanStrings(optOuts.Caches); len(values) > 0 {
			sbxenvUnsupported(report, "optOuts.caches", strings.Join(values, ", "))
		}
	}
}

// sbxenvUnsupported appends one report line.
func sbxenvUnsupported(report *[]string, field, detail string) {
	line := field + " cannot be expressed in sbxenv.yaml"
	if detail != "" {
		line += " (" + detail + ")"
	}
	*report = append(*report, line)
}

// sbxenvEntryCount describes a list length.
func sbxenvEntryCount(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return strconv.Itoa(n) + " entries"
}

// sbxenvRelativeSource reports whether sbxenv would resolve a kit source
// against the directory of the sbxenv.yaml file.
func sbxenvRelativeSource(source string) bool {
	return strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") ||
		source == "." || source == ".." || strings.HasSuffix(source, ".zip")
}
