package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/goccy/go-yaml"
)

// SbxenvExport is one sandbox rendered as an sbxenv.yaml document plus the
// sandbox fields the format cannot express.
type SbxenvExport struct {
	Slug        string
	Name        string
	Document    []byte
	Unsupported []string
	// additionalTargets are the custom mount targets the document carries in
	// additionalWorkspaces. They are reported when the installed sbx predates
	// v0.51.0.
	additionalTargets []string
}

// sbxenvSchemaVersion is the only schemaVersion `sbx env` accepts.
const sbxenvSchemaVersion = 1

// sbxenvDocument is the subset of the sbxenv.yaml schema sandwarden fills.
// The field order is the document order, so the output is deterministic.
type sbxenvDocument struct {
	SchemaVersion        int                         `yaml:"schemaVersion"`
	Name                 string                      `yaml:"name,omitempty"`
	Agent                string                      `yaml:"agent"`
	Kits                 []any                       `yaml:"kits,omitempty"`
	Workspace            *sbxenvWorkspace            `yaml:"workspace,omitempty"`
	AdditionalWorkspaces []sbxenvAdditionalWorkspace `yaml:"additionalWorkspaces,omitempty"`
	Env                  yaml.MapSlice               `yaml:"env,omitempty"`
	Ports                []sbxenvPort                `yaml:"ports,omitempty"`
}

// sbxenvKitEntry is the mapping form of a kits: entry, carrying the arguments
// that kit declares. A kit without arguments is emitted as a bare string.
type sbxenvKitEntry struct {
	Source string        `yaml:"source"`
	Args   yaml.MapSlice `yaml:"args,omitempty"`
}

// sbxenvKitArgValue is one resolved argument of a kit. scoped records whether
// it names its kit, because a scoped value wins over a bare one for the same
// key regardless of declaration order.
type sbxenvKitArgValue struct {
	value  string
	scoped bool
}

// sbxenvAdditionalWorkspace is one extra workspace. The primary workspace is
// the single workspace: block; additionalWorkspaces carries the rest.
type sbxenvAdditionalWorkspace struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target,omitempty"`
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
	result, err := renderSbxenv(s)
	if err != nil {
		return SbxenvExport{}, err
	}
	// The compatibility annotation needs a live client; export stays usable
	// from tests that build an App around the fleet alone.
	if a.Sbx != nil {
		v, _ := a.Sbx.Version(context.Background())
		annotateSbxenvCompatibility(&result, v)
	}
	return result, nil
}

// annotateSbxenvCompatibility reports the additional-workspace mount targets
// the installed sbx cannot honor. The targets stay in the document: the export
// is informational, not a silent downgrade.
func annotateSbxenvCompatibility(result *SbxenvExport, v sbx.Version) {
	if result == nil || len(result.additionalTargets) == 0 || v.AtLeast("v0.51.0") {
		return
	}
	result.Unsupported = append(result.Unsupported, fmt.Sprintf(
		"additionalWorkspaces[].target %s requires sbx v0.51.0 or later; installed sbx is %s (the target is kept in the document)",
		strings.Join(result.additionalTargets, ", "), v,
	))
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
		doc.Kits = sbxenvKits(c, &report)
		doc.Workspace, doc.AdditionalWorkspaces = sbxenvWorkspacesFrom(s, &report)
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
	var targets []string
	for _, workspace := range doc.AdditionalWorkspaces {
		if target := strings.TrimSpace(workspace.Target); target != "" {
			targets = append(targets, target)
		}
	}
	return SbxenvExport{Slug: s.Slug, Name: s.Name(), Document: document, Unsupported: report, additionalTargets: targets}, nil
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

// sbxenvWorkspacesFrom maps create.workspaces onto the single read-write
// workspace sbxenv supports, carrying create.clone over as workspace.clone,
// and returns the remaining writable entries as additionalWorkspaces (with
// their custom mount target when one is set). Read-only entries are reported
// and dropped.
func sbxenvWorkspacesFrom(s *fleet.Sandbox, report *[]string) (*sbxenvWorkspace, []sbxenvAdditionalWorkspace) {
	c := s.App.Create
	if c == nil {
		return nil, nil
	}
	type entry struct {
		index    int
		path     string
		target   string
		readOnly bool
	}
	var writable []entry
	for i, raw := range c.Workspaces {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed := entry{index: i}
		parsed.path, parsed.target, parsed.readOnly = sbxenvSplitWorkspace(raw)
		if parsed.readOnly {
			sbxenvUnsupported(report, fmt.Sprintf("create.workspaces[%d] (%s)", i, raw), "sbxenv mounts one read-write workspace")
			continue
		}
		if parsed.path != "" && !filepath.IsAbs(parsed.path) {
			*report = append(*report, fmt.Sprintf("create.workspaces[%d] (%s): sbxenv resolves a relative workspace path against the sbxenv.yaml file", i, parsed.path))
		}
		writable = append(writable, parsed)
	}
	if len(writable) == 0 {
		if c.Clone {
			sbxenvUnsupported(report, "create.clone", "workspace.clone needs a workspace path")
		}
		return nil, nil
	}
	// `sbx create` mounts the first workspace as the primary one, so the
	// document's workspace block must mirror writable[0] even when it carries a
	// custom target. A different choice would move clone and the primary mount.
	primary := writable[0]
	if primary.target != "" {
		sbxenvUnsupported(report, fmt.Sprintf("create.workspaces[%d].target", primary.index), primary.target)
	}
	workspace := &sbxenvWorkspace{Path: primary.path, Clone: c.Clone}
	var additional []sbxenvAdditionalWorkspace
	for i, item := range writable {
		if i == 0 {
			continue
		}
		additional = append(additional, sbxenvAdditionalWorkspace{Path: item.path, Target: item.target})
	}
	return workspace, additional
}

// sbxenvSplitWorkspace separates a PATH[:TARGET][:ro] workspace entry into its
// host path, optional container target and read-only marker.
func sbxenvSplitWorkspace(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	readOnly := false
	if strings.HasSuffix(value, ":ro") {
		readOnly = true
		value = strings.TrimSpace(strings.TrimSuffix(value, ":ro"))
	}
	path, target := value, ""
	if idx := strings.Index(value, ":"); idx >= 0 {
		path = strings.TrimSpace(value[:idx])
		target = strings.TrimSpace(value[idx+1:])
	}
	return path, target, readOnly
}

// sbxenvKits maps create.kits and create.kitArgs onto the sbxenv kits list. A
// kit without arguments stays a bare reference; one with arguments becomes a
// mapping carrying its args mapping. A bare name=value kit argument applies to
// every kit, while kit.name=value selects the kit whose reference (or last path
// segment) matches kit.
func sbxenvKits(c *fleet.SandboxCreate, report *[]string) []any {
	refs := cleanStrings(c.Kits)
	entries := make([]sbxenvKitEntry, len(refs))
	argsByIndex := make([]map[string]sbxenvKitArgValue, len(refs))
	for i := range argsByIndex {
		argsByIndex[i] = map[string]sbxenvKitArgValue{}
	}
	for i, ref := range refs {
		entries[i] = sbxenvKitEntry{Source: ref}
		if sbxenvRelativeSource(ref) {
			*report = append(*report, fmt.Sprintf("create.kits[%d] (%s): sbxenv resolves a relative kit source against the sbxenv.yaml file", i, ref))
		}
	}
	// setArgs merges one argument into a kit, letting a scoped value win over a
	// bare one and a later value win over an earlier one in the same scope. A
	// duplicate collapses to one key; the report names the key but never the
	// value, which may look like a secret.
	setArg := func(argIndex, kitIndex int, name, value string, scoped bool) {
		current, exists := argsByIndex[kitIndex][name]
		if exists {
			*report = append(*report, fmt.Sprintf("create.kitArgs[%d]: argument %s for kit %s overrides an earlier value", argIndex, name, refs[kitIndex]))
			if current.scoped && !scoped {
				return
			}
		}
		argsByIndex[kitIndex][name] = sbxenvKitArgValue{value: value, scoped: scoped}
	}
	for i, raw := range c.KitArgs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		left, value, ok := strings.Cut(raw, "=")
		if !ok {
			sbxenvUnsupported(report, fmt.Sprintf("create.kitArgs[%d] (%s)", i, raw), "a kit argument must be name=value")
			continue
		}
		selector, name, scoped := sbxenvSplitKitArg(strings.TrimSpace(left))
		if name == "" {
			sbxenvUnsupported(report, fmt.Sprintf("create.kitArgs[%d] (%s)", i, raw), "the argument name is empty")
			continue
		}
		if !scoped {
			if len(refs) == 0 {
				sbxenvUnsupported(report, fmt.Sprintf("create.kitArgs[%d] (%s)", i, raw), "no kit to apply the argument to")
				continue
			}
			for j := range entries {
				setArg(i, j, name, value, false)
			}
			continue
		}
		index := sbxenvMatchKit(refs, selector)
		if index < 0 {
			sbxenvUnsupported(report, fmt.Sprintf("create.kitArgs[%d] (%s)", i, raw), fmt.Sprintf("no kit matching %q", selector))
			continue
		}
		setArg(i, index, name, value, true)
	}
	out := make([]any, len(entries))
	for i := range entries {
		if len(argsByIndex[i]) == 0 {
			out[i] = entries[i].Source
			continue
		}
		items := make(yaml.MapSlice, 0, len(argsByIndex[i]))
		for name, arg := range argsByIndex[i] {
			items = append(items, yaml.MapItem{Key: name, Value: arg.value})
		}
		entries[i].Args = sortMapSlice(items)
		out[i] = entries[i]
	}
	return out
}

// sbxenvSplitKitArg separates a scoped kit.name argument from a bare name.
func sbxenvSplitKitArg(left string) (selector, name string, scoped bool) {
	if idx := strings.Index(left, "."); idx > 0 {
		return left[:idx], left[idx+1:], true
	}
	return "", left, false
}

// sbxenvMatchKit returns the index of the kit reference matching selector,
// either by its full reference or by its derived name.
func sbxenvMatchKit(refs []string, selector string) int {
	for i, ref := range refs {
		if ref == selector {
			return i
		}
	}
	for i, ref := range refs {
		if sbxenvKitName(ref) == selector {
			return i
		}
	}
	return -1
}

// sbxenvKitName derives the name sbx uses to scope a kit argument: the last
// path or URL segment, without a .git/.zip suffix or an OCI tag.
func sbxenvKitName(ref string) string {
	value := strings.TrimSpace(ref)
	if strings.HasPrefix(value, "git+") {
		value = strings.TrimPrefix(value, "git+")
		target, fragment, _ := strings.Cut(value, "#")
		for _, param := range strings.Split(fragment, "&") {
			if dir, ok := strings.CutPrefix(param, "dir="); ok {
				value = dir
				return sbxenvLastSegment(value)
			}
		}
		value = target
	}
	return sbxenvLastSegment(value)
}

// sbxenvLastSegment returns the final path segment of a reference, stripped of
// suffixes and tags.
func sbxenvLastSegment(value string) string {
	value = strings.TrimRight(value, "/")
	if i := strings.LastIndexByte(value, '/'); i >= 0 {
		value = value[i+1:]
	}
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimSuffix(value, ".zip")
	if i := strings.LastIndexByte(value, ':'); i >= 0 {
		value = value[:i]
	}
	return value
}

// sortMapSlice returns the entries ordered by key so the document is stable.
func sortMapSlice(items yaml.MapSlice) yaml.MapSlice {
	out := append(yaml.MapSlice(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		return fmt.Sprint(out[i].Key) < fmt.Sprint(out[j].Key)
	})
	return out
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
	if c.Skills != "" {
		sbxenvUnsupported(report, "create.skills", c.Skills)
	}
	if len(c.StaticMCP) > 0 {
		sbxenvUnsupported(report, "create.staticMcp", strings.Join(cleanStrings(c.StaticMCP), ", "))
	}
	if len(c.EnvFiles) > 0 {
		sbxenvUnsupported(report, "create.envFiles", strings.Join(cleanStrings(c.EnvFiles), ", "))
	}
	if len(c.KitArgsFiles) > 0 {
		sbxenvUnsupported(report, "create.kitArgsFiles", strings.Join(cleanStrings(c.KitArgsFiles), ", "))
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
