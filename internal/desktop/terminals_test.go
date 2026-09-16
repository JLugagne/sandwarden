package desktop

import "testing"

func TestOpenInTerminalRejectsEmptyCommand(t *testing.T) {
	d, _, _ := newTestDesktop(t)
	if err := d.OpenInTerminal("xterm", "/tmp", "   "); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

func TestListTerminalsIsNeverNil(t *testing.T) {
	d, _, _ := newTestDesktop(t)
	if d.ListTerminals() == nil {
		t.Fatal("expected a non-nil terminal list")
	}
}
