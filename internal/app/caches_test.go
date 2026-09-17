package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateCacheExpandsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newFakeSbx(t)

	cache, err := a.CreateCache(ctx, CacheInput{
		Name:       "go-mod",
		HostPath:   "~/go/pkg/mod",
		TargetPath: "/home/agent/go/pkg/mod",
		AutoAttach: true,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateCache with a ~ path must succeed: %v", err)
	}
	if want := filepath.Join(home, "go/pkg/mod"); cache.App.HostPath != want {
		t.Fatalf("host path = %q, want %q", cache.App.HostPath, want)
	}

	if err := a.AddMountAt(ctx, "box", "~/src", "/src", true); err != nil {
		t.Fatalf("AddMountAt with a ~ path must succeed: %v", err)
	}
	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok {
		t.Fatal("sandbox config missing")
	}
	if want := filepath.Join(home, "src"); cfg.App.Mounts[0].HostPath != want {
		t.Fatalf("mount host path = %q, want %q", cfg.App.Mounts[0].HostPath, want)
	}
}
