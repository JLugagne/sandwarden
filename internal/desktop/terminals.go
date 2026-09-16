package desktop

import (
	"errors"
	"strings"

	"github.com/JLugagne/sandwarden/internal/terminal"
)

// ListTerminals reports the terminal emulators found on this host, most
// common first. It feeds the Open button dropdown and the Settings panel.
func (d *Desktop) ListTerminals() []terminal.Terminal {
	return terminal.Detect()
}

// OpenInTerminal opens dir in the chosen terminal and runs command there, so
// sandbox connect commands start in the workspace directory.
func (d *Desktop) OpenInTerminal(terminalID, dir, command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("no command to run")
	}
	return terminal.Launch(terminalID, dir, command)
}
