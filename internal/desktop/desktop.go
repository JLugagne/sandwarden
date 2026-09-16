package desktop

import (
	"context"
	"errors"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Desktop is the Wails binding surface. Every exported method becomes a
// typed frontend binding; nothing is reachable from outside the process.
type Desktop struct {
	app   *app.App
	root  context.Context
	wails *application.App
}

// New builds the binding service around the application core. root bounds
// every call that talks to the daemon.
func New(a *app.App, root context.Context) *Desktop {
	return &Desktop{app: a, root: root}
}

// Attach wires the running Wails application so the service can open native
// dialogs and emit events. It must be called before Run. It is a package
// function, not a method, so it is never exposed as a binding.
func Attach(d *Desktop, w *application.App) {
	d.wails = w
}

// Health reports the daemon socket and CLI in use.
func (d *Desktop) Health() Health {
	return Health{OK: true, Socket: sbx.SocketPath(), SbxBinary: sbx.BinaryPath()}
}

// PickFolder opens the host's native folder chooser rooted at start.
func (d *Desktop) PickFolder(start string) (string, error) {
	dialog := d.wails.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		SetTitle("Select a folder")
	if start != "" {
		dialog = dialog.SetDirectory(start)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errCancelled
	}
	return path, nil
}

var errCancelled = errors.New("folder selection cancelled")
