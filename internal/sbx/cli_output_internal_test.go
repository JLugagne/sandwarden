package sbx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newNoisySbx installs a fake sbx that mimics the real CLI writing diagnostics
// to stderr before the JSON payload lands on stdout. runCLI merges both streams,
// so the captured output starts with the noise.
func newNoisySbx(t *testing.T, stdout string) {
	t.Helper()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(outPath, []byte(stdout), 0o644); err != nil {
		t.Fatalf("write stub output: %v", err)
	}
	script := "#!/bin/sh\n" +
		"echo 'WARN: marlin: auth-store verifier unavailable' 1>&2\n" +
		"echo 'keychain backend unavailable: D-Bus session bus unavailable' 1>&2\n" +
		"cat \"$SBX_STUB_OUT\"\n" +
		"exit 0\n"
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("SBX_STUB_OUT", outPath)
}

func TestInspectDetailIgnoresCLIDiagnostics(t *testing.T) {
	newNoisySbx(t, `{"workspace":"/w","image":"img:1","runtime_mounts":[{"host_path":"/data/one","read_only":true}]}`)

	detail, err := New("/nonexistent.sock").InspectDetail(context.Background(), "box")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.Workspace != "/w" || detail.Image != "img:1" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if len(detail.RuntimeMounts) != 1 || detail.RuntimeMounts[0].HostPath != "/data/one" {
		t.Fatalf("unexpected mounts: %+v", detail.RuntimeMounts)
	}
}

func TestListTemplatesIgnoresCLIDiagnostics(t *testing.T) {
	newNoisySbx(t, `{"images":[{"id":"abc","repository":"repo","tag":"v1"}]}`)

	templates, err := New("/nonexistent.sock").ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	if len(templates) != 1 || templates[0].ID != "abc" {
		t.Fatalf("unexpected templates: %+v", templates)
	}
}

func TestListSecretsIgnoresCLIDiagnostics(t *testing.T) {
	newNoisySbx(t, `{"secrets":[{"name":"one","scope":"global"}],"custom_secrets":[]}`)

	list, err := New("/nonexistent.sock").ListSecrets(context.Background())
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	if len(list.Stored) != 1 || list.Stored[0].Name != "one" {
		t.Fatalf("unexpected secrets: %+v", list)
	}
}

func TestKitInspectIgnoresCLIDiagnostics(t *testing.T) {
	newNoisySbx(t, `{"name":"kit-one","version":"1.0"}`)

	spec, err := New("/nonexistent.sock").KitInspect(context.Background(), "./kit")
	if err != nil {
		t.Fatalf("kit inspect: %v", err)
	}
	if spec.Name != "kit-one" {
		t.Fatalf("unexpected kit: %+v", spec)
	}
}
