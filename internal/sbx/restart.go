package sbx

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrDaemonRestartRequired is returned when sbx refuses a command because the
// upgraded CLI needs sandboxd to restart. Use errors.Is to test for it.
var ErrDaemonRestartRequired = errors.New("sbx daemon restart required")

// DaemonRestartError is the typed error for ErrDaemonRestartRequired. It
// carries the client and server versions when sbx printed them.
type DaemonRestartError struct {
	ClientVersion Version
	ServerVersion Version
}

// Error renders the actionable message shown to the user.
func (e *DaemonRestartError) Error() string {
	if e.ClientVersion.Known && e.ServerVersion.Known {
		return fmt.Sprintf(
			"sbx was updated (CLI %s, daemon %s) and sandboxd must restart; run `sbx daemon restart` (this stops running sandboxes)",
			e.ClientVersion, e.ServerVersion,
		)
	}
	return "sbx was updated and sandboxd must restart; run `sbx daemon restart` (this stops running sandboxes)"
}

// Unwrap lets errors.Is(err, ErrDaemonRestartRequired) match.
func (e *DaemonRestartError) Unwrap() error { return ErrDaemonRestartRequired }

// daemonRestartNotice reports whether sbx output says the daemon must restart
// after an update. sbx prints, among other lines:
//
//	warning: Docker Sandboxes has been updated and needs to restart; all running sandboxes will be stopped
//	error: ensure daemon: cannot prompt for restart: stdin is not a terminal; run the command in an interactive terminal to confirm the restart
func daemonRestartNotice(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "needs to restart") || strings.Contains(lower, "cannot prompt for restart")
}

// restartVersionPattern matches the "Client Version:" and "Server Version:"
// lines sbx prints before refusing a command.
var restartVersionPattern = regexp.MustCompile(`(?im)^\s*(client|server) version:\s*(\S+)`)

// newDaemonRestartError extracts the client and server versions from the
// captured output, when present, and builds the typed error.
func newDaemonRestartError(output string) *DaemonRestartError {
	err := &DaemonRestartError{}
	for _, match := range restartVersionPattern.FindAllStringSubmatch(output, -1) {
		switch strings.ToLower(match[1]) {
		case "client":
			err.ClientVersion = ParseVersion(match[2])
		case "server":
			err.ServerVersion = ParseVersion(match[2])
		}
	}
	return err
}
