package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/JLugagne/sandwarden/internal/app"
	"github.com/JLugagne/sandwarden/internal/desktop"
	"github.com/JLugagne/sandwarden/internal/sbx"
	"github.com/JLugagne/sandwarden/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:web/dist
var assets embed.FS

// version is the release tag injected at build time (dev for local builds).
var version = "dev"

func main() {
	socket := flag.String("socket", "", "sandboxd unix socket path (default: $DOCKER_SANDBOXES_API or XDG)")
	dbPath := flag.String("db", "", "path to the SQLite database (default: XDG state dir)")
	flag.Parse()

	if *dbPath == "" {
		*dbPath = defaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	client := sbx.New(*socket)
	if _, err := client.ListSandboxes(context.Background()); err != nil {
		log.Printf("warning: cannot reach sandboxd (%s): %v", sbx.SocketPath(), err)
	}

	core := app.New(client, st)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := core.Reconcile(ctx); err != nil {
		log.Printf("initial reconcile: %v", err)
	}
	core.Start(ctx)

	service := desktop.New(core, ctx, version)
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

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

func defaultDBPath() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "sandwarden", "sandwarden.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "sandwarden.db"
	}
	return filepath.Join(home, ".local", "state", "sandwarden", "sandwarden.db")
}
