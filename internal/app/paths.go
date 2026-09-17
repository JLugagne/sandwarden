package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

const (
	skillStoresDataDir = "skill-stores"
	kitStoresDataDir   = "kit-stores"
)

// dataHome resolves $XDG_DATA_HOME, falling back to ~/.local/share.
func dataHome() string {
	if root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "sandwarden")
	}
	return filepath.Join(home, ".local", "share")
}

// storeCheckoutPath is the deterministic checkout directory of a store: it is
// derived from the store slug, never stored.
func storeCheckoutPath(kind fleet.StoreKind, slug string) string {
	sub := skillStoresDataDir
	if kind == fleet.StoreKits {
		sub = kitStoresDataDir
	}
	return filepath.Join(dataHome(), "sandwarden", sub, slug)
}

// sandboxConfigPath is the spec file of a sandbox, for display.
func (a *App) sandboxConfigPath(name string) string {
	if s, ok := a.Fleet.SandboxByName(name); ok {
		return s.Dir
	}
	return ""
}

// storePaths maps every registered store checkout key ("skill:<slug>",
// "kit:<slug>") to its deterministic directory.
func (a *App) storePaths() map[string]string {
	out := map[string]string{}
	for _, reg := range a.Fleet.Stores(fleet.StoreSkills) {
		out["skill:"+reg.Slug] = storeCheckoutPath(fleet.StoreSkills, reg.Slug)
	}
	for _, reg := range a.Fleet.Stores(fleet.StoreKits) {
		out["kit:"+reg.Slug] = storeCheckoutPath(fleet.StoreKits, reg.Slug)
	}
	return out
}
