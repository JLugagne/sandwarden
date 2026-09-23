package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/cli"
	"github.com/JLugagne/sandwarden/internal/desktop"
	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var version = "dev"

func main() {
	opts := &cli.Options{Version: version}
	root := cli.New(opts, runGUI)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runGUI launches the desktop application; it is the root command's action.
func runGUI(opts *cli.Options) error {
	dbPath := strings.TrimSpace(opts.DB)
	if dbPath == "" {
		dbPath = cli.DefaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	fl, err := fleet.Open(opts.ConfigDir)
	if err != nil {
		return fmt.Errorf("open config: %w", err)
	}

	client := sbx.New(opts.Socket)
	if _, err := client.ListSandboxes(context.Background()); err != nil {
		log.Printf("warning: cannot reach sandboxd (%s): %v", sbx.SocketPath(), err)
	}

	core := app.New(client, st, fl)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := core.Reconcile(ctx); err != nil {
		log.Printf("initial reconcile: %v", err)
	}
	core.Start(ctx)

	service := desktop.New(core, ctx, opts.Version)
	notifier := desktop.NewNotifier()

	wailsApp := application.New(application.Options{
		Name:        "sandwarden",
		Description: "Manage Docker Sandboxes",
		Services: []application.Service{
			application.NewService(service),
			application.NewService(notifier),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: desktop.AssetMiddleware(assets),
		},
		OnShutdown: func() { _ = st.Close() },
		Linux:      application.LinuxOptions{ProgramName: "sandwarden"},
	})
	desktop.Attach(service, wailsApp)

	route := "/"
	if r := strings.TrimPrefix(os.Getenv("SANDWARDEN_ROUTE"), "#"); r != "" {
		if !strings.HasPrefix(r, "/") {
			r = "/" + r
		}
		route = "/#" + r
	}

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "sandwarden",
		Width:                      1280,
		Height:                     820,
		MinWidth:                   960,
		MinHeight:                  600,
		URL:                        route,
		BackgroundColour:           application.NewRGB(6, 7, 15),
		DefaultContextMenuDisabled: true,
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionMicrophone:    application.PermissionDeny,
			application.PermissionCamera:        application.PermissionDeny,
			application.PermissionGeolocation:   application.PermissionDeny,
			application.PermissionNotifications: application.PermissionDeny,
			application.PermissionClipboardRead: application.PermissionDeny,
		},
		UseApplicationMenu: false,
	})

	go desktop.Bridge(ctx, service, func(event app.Event) {
		wailsApp.Event.Emit(desktop.EventName, event)
	})

	go func() {
		<-ctx.Done()
		wailsApp.Quit()
	}()

	return wailsApp.Run()
}
