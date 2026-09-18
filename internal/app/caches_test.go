package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/JLugagne/sandwarden/internal/sbx"
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

// TestCreateSandboxAttachesAutoCaches reproduces the reported bug: a cache
// with auto-attach set is not attached to a freshly created sandbox.
func TestCreateSandboxAttachesAutoCaches(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	fake := newFakeSbx(t)

	cache, err := a.CreateCache(ctx, CacheInput{
		Name:       "go-build",
		HostPath:   t.TempDir(),
		TargetPath: "/home/agent/.cache/go-build",
		AutoAttach: true,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateCache: %v", err)
	}

	req := CreateRequest{
		Opts:         sbx.CreateOptions{Agent: "codex", Name: "box"},
		AttachCaches: true,
	}
	var out bytes.Buffer
	if err := a.CreateSandbox(ctx, req, &out); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok {
		t.Fatalf("sandbox config missing; output: %s", out.String())
	}
	if !slices.Contains(cfg.App.Caches, cache.Slug) {
		t.Fatalf("sandbox caches = %v, want auto-attach cache %q attached; output: %s", cfg.App.Caches, cache.Slug, out.String())
	}
	if mounts := fake.mounts(t, "box"); !hasMount(mounts, cache.App.HostPath, cache.App.TargetPath, false) {
		t.Fatalf("sandbox mounts = %v, want auto-attach cache mounted; output: %s", mounts, out.String())
	}
}
