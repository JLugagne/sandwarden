package sbx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Exec runs a single command inside a sandbox via the sbx CLI, streaming
// combined output to out. The daemon's REST exec endpoint uses a hijacked
// connection; the CLI already wraps that, so it is reused here.
func (c *Client) Exec(ctx context.Context, sandbox string, command []string, out io.Writer) error {
	if strings.TrimSpace(sandbox) == "" {
		return errors.New("sandbox is required")
	}
	if len(command) == 0 {
		return errors.New("command is required")
	}
	if out == nil {
		out = io.Discard
	}
	args := append([]string{"exec", sandbox}, command...)
	cmd := cliCommand(ctx, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sbx exec failed: %w", err)
	}
	return nil
}
