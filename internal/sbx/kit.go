package sbx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// KitSpec is the parsed `sbx kit inspect --json` payload the kit catalog and
// detail views render. Fields the UI does not show are ignored.
type KitSpec struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Kind          string                 `json:"kind"`
	Name          string                 `json:"name"`
	Version       string                 `json:"version,omitempty"`
	DisplayName   string                 `json:"displayName,omitempty"`
	Description   string                 `json:"description,omitempty"`
	SourceURL     string                 `json:"sourceURL,omitempty"`
	Requires      KitRequires            `json:"requires,omitempty"`
	Sandbox       KitSandbox             `json:"sandbox,omitempty"`
	Environment   KitEnvironment         `json:"environment,omitempty"`
	Permissions   KitPermissions         `json:"permissions,omitempty"`
	Ports         []KitPort              `json:"ports,omitempty"`
	Setup         KitSetup               `json:"setup,omitempty"`
	Arguments     map[string]KitArgument `json:"arguments,omitempty"`
}

// KitRequires names the base agent a mixin composes onto.
type KitRequires struct {
	Agent string `json:"agent,omitempty"`
}

// KitSandbox is the sandbox image block of a kind=sandbox kit.
type KitSandbox struct {
	Image      string        `json:"image,omitempty"`
	Entrypoint []string      `json:"entrypoint,omitempty"`
	Command    KitCommandSet `json:"command,omitempty"`
}

// KitCommandSet is the default and interactive argv of an agent kit.
type KitCommandSet struct {
	Default     []string `json:"default,omitempty"`
	Interactive []string `json:"interactive,omitempty"`
}

// KitEnvironment holds the static environment variables a kit sets.
type KitEnvironment struct {
	Variables map[string]string `json:"variables,omitempty"`
}

// KitPermissions is the egress policy a kit declares.
type KitPermissions struct {
	Network KitNetworkPolicy `json:"network,omitempty"`
}

// KitNetworkPolicy lists the allowed and denied egress hosts.
type KitNetworkPolicy struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// KitPort is one container port a kit exposes.
type KitPort struct {
	Container int    `json:"container"`
	Protocol  string `json:"protocol,omitempty"`
	Name      string `json:"name,omitempty"`
}

// KitSetup is the install/startup hooks and injected files of a kit.
type KitSetup struct {
	Install []KitCommand `json:"install,omitempty"`
	Startup []KitCommand `json:"startup,omitempty"`
	Files   []KitFile    `json:"files,omitempty"`
}

// KitCommand is one setup hook. Command is normalized to a single string
// whether the spec declared it as a string or an argv array.
type KitCommand struct {
	Command     string `json:"command,omitempty"`
	User        string `json:"user,omitempty"`
	Description string `json:"description,omitempty"`
}

// UnmarshalJSON accepts both hook shapes: a string command or an argv array.
func (k *KitCommand) UnmarshalJSON(data []byte) error {
	var raw struct {
		Command     json.RawMessage `json:"command"`
		User        string          `json:"user"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	k.User = raw.User
	k.Description = raw.Description
	var single string
	if err := json.Unmarshal(raw.Command, &single); err == nil {
		k.Command = single
		return nil
	}
	var argv []string
	if err := json.Unmarshal(raw.Command, &argv); err == nil {
		k.Command = strings.Join(argv, " ")
	}
	return nil
}

// KitFile is one static file a kit injects; its content is never surfaced.
type KitFile struct {
	Path        string `json:"path"`
	Mode        string `json:"mode,omitempty"`
	Description string `json:"description,omitempty"`
}

// KitArgument is one caller-supplied input a kit declares.
type KitArgument struct {
	Default     *string  `json:"default,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
}

// KitInspect loads a kit artifact through `sbx kit inspect --json`. The
// reference may be a directory, a ZIP, a git+ URL or an OCI reference.
func (c *Client) KitInspect(ctx context.Context, ref string) (KitSpec, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return KitSpec{}, errors.New("kit reference is required")
	}
	raw, err := c.runCLI(ctx, nil, nil, "kit", "inspect", "--json", ref)
	if err != nil {
		return KitSpec{}, err
	}
	var spec KitSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return KitSpec{}, fmt.Errorf("decode kit inspect output: %w", err)
	}
	return spec, nil
}

// KitValidate checks a kit artifact and returns the CLI's verdict text. An
// invalid artifact is reported through err; the verdict is always returned.
func (c *Client) KitValidate(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("kit reference is required")
	}
	return c.runCLI(ctx, nil, nil, "kit", "validate", ref)
}

// KitAdd runs `sbx kit add SANDBOX REF`: sbx recreates the sandbox's container
// with the reference appended to its kit list, preserving kit-owned volumes
// (agent session state) and --clone workspaces. Combined output is teed to
// stream when it is non-nil; a refusal is returned verbatim.
func (c *Client) KitAdd(ctx context.Context, sandbox, ref string, stream io.Writer) (string, error) {
	sandbox = strings.TrimSpace(sandbox)
	if sandbox == "" {
		return "", errors.New("sandbox name is required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("kit reference is required")
	}
	return c.runCLI(ctx, nil, stream, "kit", "add", sandbox, ref)
}
