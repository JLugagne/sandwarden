package sbx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SecretScopeHostOnly is the scope of a registry credential used only for
// host-side pulls, injected into no sandbox. It is a display label, not a
// sandbox name, and is never passed as a scope argument.
const SecretScopeHostOnly = "(host only)"

// Secret is a stored service or registry secret (`sbx secret set`).
type Secret struct {
	Scope    string `json:"scope"`
	Type     string `json:"type"`   // "service" | "registry"
	Name     string `json:"name"`   // service name or registry host
	Masked   string `json:"masked"` // what the CLI shows; never the real secret
	Username string `json:"username,omitempty"`
	Kind     string `json:"kind,omitempty"` // resolver kind, e.g. "command" / "op" / "aws"
	Source   string `json:"source,omitempty"`
	Refresh  string `json:"refresh,omitempty"`
}

// CustomSecret is a custom proxy-injected secret (`sbx secret set-custom`).
type CustomSecret struct {
	Scope       string   `json:"scope"`
	Targets     []string `json:"targets"`
	Env         string   `json:"env"`
	Placeholder string   `json:"placeholder"`
	Masked      string   `json:"masked"`
	Kind        string   `json:"kind,omitempty"`
	Source      string   `json:"source,omitempty"`
	Refresh     string   `json:"refresh,omitempty"`
}

// SecretList is the parsed `sbx secret ls --json` output.
type SecretList struct {
	Stored []Secret       `json:"stored"`
	Custom []CustomSecret `json:"custom"`
}

type secretJSON struct {
	Scope       string   `json:"scope"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Targets     []string `json:"targets"`
	Env         string   `json:"env"`
	Placeholder string   `json:"placeholder"`
	Secret      string   `json:"secret"`
	Username    string   `json:"username"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source"`
	Refresh     string   `json:"refresh"`
}

func (s secretJSON) masked() string {
	switch {
	case s.Kind != "":
		return s.Kind + ":" + s.Source + " (" + s.Refresh + ")"
	case s.Username != "":
		return s.Username + "/" + s.Secret
	}
	return s.Secret
}

func normalizeScope(scope string) string {
	switch scope {
	case "global":
		return ""
	case "host-only":
		return SecretScopeHostOnly
	}
	return scope
}

// ListSecrets returns every stored secret across all scopes.
func (c *Client) ListSecrets(ctx context.Context) (SecretList, error) {
	raw, err := c.runCLI(ctx, nil, nil, "secret", "ls", "--json")
	if err != nil {
		return SecretList{}, err
	}
	var payload struct {
		Secrets       *[]secretJSON `json:"secrets"`
		CustomSecrets *[]secretJSON `json:"custom_secrets"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return SecretList{}, fmt.Errorf("decode secret list: %w", err)
	}
	if payload.Secrets == nil || payload.CustomSecrets == nil {
		return SecretList{}, errors.New("unexpected secret list output")
	}
	var out SecretList
	for _, s := range *payload.Secrets {
		out.Stored = append(out.Stored, Secret{
			Scope:    normalizeScope(s.Scope),
			Type:     s.Type,
			Name:     s.Name,
			Masked:   s.masked(),
			Username: s.Username,
			Kind:     s.Kind,
			Source:   s.Source,
			Refresh:  s.Refresh,
		})
	}
	for _, s := range *payload.CustomSecrets {
		out.Custom = append(out.Custom, CustomSecret{
			Scope:       normalizeScope(s.Scope),
			Targets:     s.Targets,
			Env:         s.Env,
			Placeholder: s.Placeholder,
			Masked:      s.masked(),
			Kind:        s.Kind,
			Source:      s.Source,
			Refresh:     s.Refresh,
		})
	}
	return out, nil
}

// checkNotStored rejects a create when an equivalent entry already exists and
// overwrite is false. Without it the CLI would prompt on a non-TTY stdin and
// silently no-op.
func (c *Client) checkNotStored(ctx context.Context, scope, typ, name string, overwrite bool) error {
	if overwrite {
		return nil
	}
	list, err := c.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("cannot check existing secrets: %w", err)
	}
	for _, s := range list.Stored {
		if s.Type == typ && s.Name == name && s.Scope == scope {
			return fmt.Errorf("%s %q already exists in this scope; enable overwrite to replace it", typ, name)
		}
	}
	return nil
}

func scopeArgs(scope string) []string {
	if scope == "" || scope == SecretScopeHostOnly {
		return nil
	}
	return []string{"--sandbox", scope}
}

// ServiceSecretSpec describes a service-secret write. Exactly one of Value, Ref
// or Command must be set; Value is piped on stdin and never appears in argv.
type ServiceSecretSpec struct {
	Service   string
	Scope     string // "" global, or a sandbox name
	Value     string
	Ref       string
	Command   string
	Refresh   string
	Overwrite bool
}

// SetServiceSecret stores or replaces a service secret.
func (c *Client) SetServiceSecret(ctx context.Context, spec ServiceSecretSpec) error {
	if strings.TrimSpace(spec.Service) == "" {
		return errors.New("service is required")
	}
	sources := countSources(spec.Value, spec.Ref, spec.Command)
	if sources != 1 {
		return errors.New("provide exactly one of value, ref or command")
	}
	if err := c.checkNotStored(ctx, spec.Scope, "service", spec.Service, spec.Overwrite); err != nil {
		return err
	}
	args := append([]string{"secret", "set", spec.Service}, scopeArgs(spec.Scope)...)
	if spec.Overwrite {
		args = append(args, "--force")
	}
	var stdin io.Reader
	switch {
	case spec.Ref != "":
		args = append(args, "--ref", spec.Ref)
	case spec.Command != "":
		args = append(args, "--command", spec.Command)
	default:
		stdin = strings.NewReader(spec.Value + "\n")
	}
	if spec.Refresh != "" && (spec.Ref != "" || spec.Command != "") {
		args = append(args, "--refresh", spec.Refresh)
	}
	_, err := c.runCLI(ctx, stdin, nil, args...)
	return err
}

// RegistrySecretSpec describes a registry pull credential. Scope is
// SecretScopeHostOnly (default), "" for global (injected into every sandbox),
// or a sandbox name. Password is piped on stdin.
type RegistrySecretSpec struct {
	Host      string
	Username  string
	Password  string
	Scope     string
	Overwrite bool
}

// SetRegistrySecret stores or replaces a registry credential.
func (c *Client) SetRegistrySecret(ctx context.Context, spec RegistrySecretSpec) error {
	if strings.TrimSpace(spec.Host) == "" {
		return errors.New("registry host is required")
	}
	if spec.Password == "" {
		return errors.New("registry password is required")
	}
	if err := c.checkNotStored(ctx, spec.Scope, "registry", spec.Host, spec.Overwrite); err != nil {
		return err
	}
	args := []string{"secret", "set", "--registry", spec.Host}
	if spec.Username != "" {
		args = append(args, "--username", spec.Username)
	}
	args = append(args, "--password-stdin")
	switch spec.Scope {
	case SecretScopeHostOnly:
	case "":
		args = append(args, "--all-sandboxes")
	default:
		args = append(args, "--sandbox", spec.Scope)
	}
	if spec.Overwrite {
		args = append(args, "--force")
	}
	_, err := c.runCLI(ctx, strings.NewReader(spec.Password+"\n"), nil, args...)
	return err
}

// CustomSecretSpec describes a custom proxy-injected secret. Exactly one of
// Value, Ref or Command must be set. Unlike the other writers, set-custom has no
// stdin path, so a literal Value is passed on the command line (sbx documents
// this as less secure); prefer Ref or Command for real secrets.
type CustomSecretSpec struct {
	Hosts       []string
	Env         string
	Value       string
	Ref         string
	Command     string
	Placeholder string
	Refresh     string
	Scope       string
	Overwrite   bool
}

// SetCustomSecret stores or replaces a custom secret. Overwrite removes the
// existing entry by placeholder first, since set-custom has no --force.
func (c *Client) SetCustomSecret(ctx context.Context, spec CustomSecretSpec) error {
	if len(spec.Hosts) == 0 {
		return errors.New("at least one host is required")
	}
	if strings.TrimSpace(spec.Env) == "" {
		return errors.New("env var name is required")
	}
	if countSources(spec.Value, spec.Ref, spec.Command) != 1 {
		return errors.New("provide exactly one of value, ref or command")
	}
	if spec.Overwrite && spec.Placeholder != "" {
		rmArgs := append([]string{"secret", "rm"}, scopeArgs(spec.Scope)...)
		rmArgs = append(rmArgs, "--placeholder", spec.Placeholder, "-f")
		_, _ = c.runCLI(ctx, nil, nil, rmArgs...)
	}

	args := append([]string{"secret", "set-custom"}, scopeArgs(spec.Scope)...)
	for _, host := range spec.Hosts {
		args = append(args, "--host", host)
	}
	args = append(args, "--env", spec.Env)
	if spec.Placeholder != "" {
		args = append(args, "--placeholder", spec.Placeholder)
	}
	switch {
	case spec.Ref != "":
		args = append(args, "--ref", spec.Ref)
	case spec.Command != "":
		args = append(args, "--command", spec.Command)
	default:
		args = append(args, "--value", spec.Value)
	}
	if spec.Refresh != "" && (spec.Ref != "" || spec.Command != "") {
		args = append(args, "--refresh", spec.Refresh)
	}
	_, err := c.runCLI(ctx, nil, nil, args...)
	return err
}

// RemoveSecret deletes a service secret in scope ("" global, or a sandbox name).
func (c *Client) RemoveSecret(ctx context.Context, scope, service string) error {
	if strings.TrimSpace(service) == "" {
		return errors.New("service is required")
	}
	args := append([]string{"secret", "rm"}, scopeArgs(scope)...)
	args = append(args, service, "-f")
	_, err := c.runCLI(ctx, nil, nil, args...)
	return err
}

// RemoveCustomSecret deletes a custom secret by placeholder (the key that works
// regardless of how many hosts it covers).
func (c *Client) RemoveCustomSecret(ctx context.Context, scope, placeholder string) error {
	if strings.TrimSpace(placeholder) == "" {
		return errors.New("placeholder is required")
	}
	args := append([]string{"secret", "rm"}, scopeArgs(scope)...)
	args = append(args, "--placeholder", placeholder, "-f")
	_, err := c.runCLI(ctx, nil, nil, args...)
	return err
}

// ImportSecrets runs `sbx secret import`, streaming output to stream.
func (c *Client) ImportSecrets(ctx context.Context, service string, all, dryRun, force bool, stream io.Writer) error {
	args := []string{"secret", "import"}
	if strings.TrimSpace(service) != "" {
		args = append(args, service)
	}
	if all {
		args = append(args, "--all")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if force {
		args = append(args, "--force")
	}
	_, err := c.runCLI(ctx, nil, stream, args...)
	return err
}

func countSources(value, ref, command string) int {
	n := 0
	for _, s := range []string{value, ref, command} {
		if s != "" {
			n++
		}
	}
	return n
}

// RemoveRegistrySecret deletes registry pull credentials for host.
// Scope "" targets the all-sandboxes credential, SecretScopeHostOnly the
// host-only one, and any other value a sandbox name.
func (c *Client) RemoveRegistrySecret(ctx context.Context, scope, host string) error {
	if strings.TrimSpace(host) == "" {
		return errors.New("registry host is required")
	}
	args := []string{"secret", "rm", "--registry", host}
	switch scope {
	case SecretScopeHostOnly:
	case "":
		args = append(args, "--all-sandboxes")
	default:
		args = append(args, "--sandbox", scope)
	}
	args = append(args, "-f")
	_, err := c.runCLI(ctx, nil, nil, args...)
	return err
}
