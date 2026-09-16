package sbx

import (
	"context"
	"strings"
)

// DaemonStatus is the parsed output of `sbx daemon status`.
type DaemonStatus struct {
	Running bool
	Socket  string
	Logs    string
}

// DaemonStatus reports whether the sandboxd daemon is running. A "stopped"
// status is a valid answer, so the status line is honoured even when the CLI
// exits non-zero.
func (c *Client) DaemonStatus(ctx context.Context) (DaemonStatus, error) {
	out, err := c.runCLI(ctx, nil, nil, "daemon", "status")
	var status DaemonStatus
	parsed := false
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "status":
			status.Running = strings.EqualFold(value, "running")
			parsed = true
		case "socket":
			status.Socket = value
		case "logs":
			status.Logs = value
		}
	}
	if !parsed && err != nil {
		return DaemonStatus{}, err
	}
	return status, nil
}

// StartDaemon runs `sbx daemon start`.
func (c *Client) StartDaemon(ctx context.Context) error {
	_, err := c.runCLI(ctx, nil, nil, "daemon", "start")
	return err
}
