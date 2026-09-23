package sbx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// envBinary overrides the sbx executable path.
const envBinary = "SBX_BINARY"

// BinaryPath resolves the sbx CLI executable. Precedence: SBX_BINARY, then "sbx"
// on PATH.
func BinaryPath() string {
	if p := os.Getenv(envBinary); p != "" {
		return p
	}
	return "sbx"
}

// CreateOptions describes a `sbx create` invocation. The daemon exposes a REST
// create route, but its request is gated behind a feature flag and streams a
// long image pull; the CLI owns naming, progress and lazy pulls, so creation is
// delegated to it.
type CreateOptions struct {
	Agent       string
	Workspaces  []string
	Name        string
	CPUs        int
	Memory      string
	Profile     string
	Template    string
	Kits        []string
	Publish     []string
	Env         []string
	DenyNetwork []string
	Clone       bool
}

// Args builds the sbx argument vector for the create options.
func (o CreateOptions) Args() ([]string, error) {
	if strings.TrimSpace(o.Agent) == "" {
		return nil, errors.New("agent is required")
	}
	args := []string{"create", o.Agent}
	args = append(args, o.Workspaces...)
	if o.Name != "" {
		args = append(args, "--name", o.Name)
	}
	if o.CPUs > 0 {
		args = append(args, "--cpus", strconv.Itoa(o.CPUs))
	}
	if o.Memory != "" {
		args = append(args, "--memory", o.Memory)
	}
	if o.Profile != "" {
		args = append(args, "--profile", o.Profile)
	}
	if o.Template != "" {
		args = append(args, "--template", o.Template)
	}
	for _, kit := range o.Kits {
		if strings.TrimSpace(kit) != "" {
			args = append(args, "--kit", kit)
		}
	}
	for _, p := range o.Publish {
		args = append(args, "-p", p)
	}
	for _, e := range o.Env {
		args = append(args, "-e", e)
	}
	for _, h := range o.DenyNetwork {
		args = append(args, "--deny-network", h)
	}
	if o.Clone {
		args = append(args, "--clone")
	}
	return args, nil
}

// CreateSandbox runs `sbx create`, streaming combined stdout/stderr to out.
// It blocks until the command exits, so callers should run it asynchronously
// (for example behind an SSE endpoint) and surface out as progress.
func (c *Client) CreateSandbox(ctx context.Context, opts CreateOptions, out io.Writer) error {
	args, err := opts.Args()
	if err != nil {
		return err
	}
	if out == nil {
		out = io.Discard
	}
	cmd := cliCommand(ctx, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sbx create failed: %w", err)
	}
	return nil
}
