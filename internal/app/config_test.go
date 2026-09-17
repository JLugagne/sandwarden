package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestConfigStalenessReportsOutsideEdits(t *testing.T) {
	a, _ := newTestApp(t)
	ctx := context.Background()

	spec := fleet.NewMixin("")
	spec.DisplayName = "My VM"
	sb, err := a.Fleet.CreateSandbox("My VM", spec, fleet.SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	report, err := a.ConfigStaleness(ctx)
	if err != nil {
		t.Fatalf("ConfigStaleness: %v", err)
	}
	if report.Stale || len(report.Changed) != 0 {
		t.Fatalf("fresh fleet must not be stale: %+v", report)
	}

	sidecar := filepath.Join(sb.Dir, "sandwarden.yaml")
	if err := os.WriteFile(sidecar, []byte("sandbox: My VM\nrunArgs: --model sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = a.ConfigStaleness(ctx)
	if err != nil {
		t.Fatalf("ConfigStaleness: %v", err)
	}
	if !report.Stale || len(report.Changed) != 1 {
		t.Fatalf("expected one stale file: %+v", report)
	}
	if file := report.Changed[0]; file.Kind != fleet.EntitySandbox || file.Name != "My VM" {
		t.Fatalf("unexpected stale file: %+v", file)
	}

	if _, err := a.ReloadFleet(ctx); err != nil {
		t.Fatalf("ReloadFleet: %v", err)
	}
	if report, err = a.ConfigStaleness(ctx); err != nil || report.Stale {
		t.Fatalf("reload must clear staleness: %+v (%v)", report, err)
	}
}

func mustReadSandboxConfig(t *testing.T, a *App, name string) []ConfigFile {
	t.Helper()
	files, err := a.ReadSandboxConfig(context.Background(), name)
	if err != nil {
		t.Fatalf("ReadSandboxConfig(%q): %v", name, err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two config files, got %d", len(files))
	}
	return files
}

func configFileByName(t *testing.T, files []ConfigFile, name string) ConfigFile {
	t.Helper()
	for _, file := range files {
		if file.Name == name {
			return file
		}
	}
	t.Fatalf("config file %q not found in %+v", name, files)
	return ConfigFile{}
}

func TestReadSandboxConfigReturnsBothFiles(t *testing.T) {
	a, _ := newTestApp(t)
	sb, err := a.Fleet.CreateSandbox("My VM", fleet.NewMixin(""), fleet.SandboxApp{RunArgs: "--model sonnet"})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}

	for _, selector := range []string{"My VM", sb.Slug} {
		files := mustReadSandboxConfig(t, a, selector)
		if files[0].Name != "spec.yaml" || files[1].Name != "sandwarden.yaml" {
			t.Fatalf("unexpected file order: %+v", files)
		}
		if !strings.Contains(files[0].Content, "name: "+sb.Slug) {
			t.Fatalf("spec content missing the kit name: %q", files[0].Content)
		}
		if !strings.Contains(files[1].Content, "runArgs: --model sonnet") {
			t.Fatalf("sidecar content missing the run args: %q", files[1].Content)
		}
		for _, file := range files {
			if file.Error != "" {
				t.Fatalf("unexpected error for %s: %s", file.Name, file.Error)
			}
			if want := filepath.Join(sb.Dir, file.Name); file.Path != want {
				t.Fatalf("path = %q, want %q", file.Path, want)
			}
		}
	}
}

func TestReadSandboxConfigRejectsForeignSelectors(t *testing.T) {
	a, _ := newTestApp(t)
	profile, err := a.Fleet.CreateProfile("golang", fleet.NewMixin(""), fleet.ProfileApp{})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	for _, selector := range []string{"", "   ", "..", "../profiles/x", "missing", profile.Slug} {
		if _, err := a.ReadSandboxConfig(context.Background(), selector); err == nil {
			t.Fatalf("selector %q must not resolve to a sandbox", selector)
		}
	}
}

func TestReadSandboxConfigCapsFileSize(t *testing.T) {
	a, _ := newTestApp(t)
	sb, err := a.Fleet.CreateSandbox("big", fleet.NewMixin(""), fleet.SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	oversized := append([]byte("# "), bytes.Repeat([]byte("x"), maxConfigFileBytes)...)
	oversized = append(oversized, '\n')
	if err := os.WriteFile(filepath.Join(sb.Dir, "sandwarden.yaml"), oversized, 0o644); err != nil {
		t.Fatal(err)
	}

	files := mustReadSandboxConfig(t, a, sb.Slug)
	sidecar := configFileByName(t, files, "sandwarden.yaml")
	if sidecar.Content != "" {
		t.Fatalf("oversized file must not be loaded, got %d bytes", len(sidecar.Content))
	}
	if !strings.Contains(sidecar.Error, "256 KiB viewer limit") {
		t.Fatalf("unexpected error: %q", sidecar.Error)
	}
}

func TestReadSandboxConfigReportsInvalidYAML(t *testing.T) {
	a, _ := newTestApp(t)
	sb, err := a.Fleet.CreateSandbox("alpha", fleet.NewMixin(""), fleet.SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	broken := "sandbox: alpha\nrunArgs: [unterminated\n"
	if err := os.WriteFile(filepath.Join(sb.Dir, "sandwarden.yaml"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := a.ReadSandboxConfig(context.Background(), sb.Slug)
	if err != nil {
		t.Fatalf("a broken sidecar must not fail the call: %v", err)
	}
	sidecar := configFileByName(t, files, "sandwarden.yaml")
	if sidecar.Content != broken {
		t.Fatalf("raw content must be preserved, got %q", sidecar.Content)
	}
	if sidecar.Error == "" {
		t.Fatal("invalid YAML must be reported per file")
	}
	if spec := configFileByName(t, files, "spec.yaml"); spec.Error != "" {
		t.Fatalf("valid spec must not be reported: %q", spec.Error)
	}
}

func TestReadSandboxConfigSurvivesBrokenFilesAfterReload(t *testing.T) {
	a, _ := newTestApp(t)
	sb, err := a.Fleet.CreateSandbox("alpha", fleet.NewMixin(""), fleet.SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sb.Dir, "spec.yaml"), []byte("schemaVersion: \"2\"\nkind: mixin\nname: alpha\nx-sandwarden: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.Dir, "sandwarden.yaml"), []byte("sandbox: alpha\nrunArgs: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReloadFleet(context.Background()); err != nil {
		t.Fatalf("ReloadFleet: %v", err)
	}
	if _, ok := a.Fleet.Sandbox(sb.Slug); ok {
		t.Fatal("a broken sandbox must not be indexed")
	}

	files, err := a.ReadSandboxConfig(context.Background(), sb.Slug)
	if err != nil {
		t.Fatalf("unindexed broken sandbox must stay readable: %v", err)
	}
	spec := configFileByName(t, files, "spec.yaml")
	if !strings.Contains(spec.Error, "x-sandwarden") {
		t.Fatalf("strict decode error expected, got %q", spec.Error)
	}
	sidecar := configFileByName(t, files, "sandwarden.yaml")
	if sidecar.Content == "" || sidecar.Error == "" {
		t.Fatalf("broken sidecar must keep its raw content and error: %+v", sidecar)
	}
}

func TestReadSandboxConfigReportsMissingFile(t *testing.T) {
	a, _ := newTestApp(t)
	sb, err := a.Fleet.CreateSandbox("alpha", fleet.NewMixin(""), fleet.SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if err := os.Remove(filepath.Join(sb.Dir, "sandwarden.yaml")); err != nil {
		t.Fatal(err)
	}

	files := mustReadSandboxConfig(t, a, sb.Slug)
	sidecar := configFileByName(t, files, "sandwarden.yaml")
	if sidecar.Error != "file not found" || sidecar.Content != "" {
		t.Fatalf("missing file must be reported empty: %+v", sidecar)
	}
}

func TestReadProfileConfigReturnsBothFiles(t *testing.T) {
	a, _ := newTestApp(t)
	profile, err := a.Fleet.CreateProfile("golang", fleet.NewMixin(""), fleet.ProfileApp{Default: true})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	files, err := a.ReadProfileConfig(context.Background(), profile.Slug)
	if err != nil {
		t.Fatalf("ReadProfileConfig: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected two config files, got %d", len(files))
	}
	sidecar := configFileByName(t, files, "sandwarden.yaml")
	if !strings.Contains(sidecar.Content, "default: true") || sidecar.Error != "" {
		t.Fatalf("unexpected sidecar: %+v", sidecar)
	}
	if _, err := a.ReadProfileConfig(context.Background(), "missing"); err == nil {
		t.Fatal("unknown profile must be rejected")
	}

	if err := os.WriteFile(filepath.Join(profile.Dir, "sandwarden.yaml"), []byte("cachez: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReloadFleet(context.Background()); err != nil {
		t.Fatalf("ReloadFleet: %v", err)
	}
	files, err = a.ReadProfileConfig(context.Background(), profile.Slug)
	if err != nil {
		t.Fatalf("unindexed broken profile must stay readable: %v", err)
	}
	if broken := configFileByName(t, files, "sandwarden.yaml"); broken.Error == "" {
		t.Fatalf("invalid profile sidecar must be reported: %+v", broken)
	}
}
