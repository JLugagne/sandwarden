package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"

	"github.com/JLugagne/sandwarden/internal/app"
)

func exportSbxenvFixture(t *testing.T, fl *fleet.Fleet, name string) {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	spec.Environment = &fleet.SpecEnv{Variables: map[string]string{"FOO": "bar"}}
	spec.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{Deny: []string{"telemetry.example.com"}}}
	if _, err := fl.CreateSandbox(name, spec, fleet.SandboxApp{
		Create: &fleet.SandboxCreate{
			Workspaces: []string{"/srv/www"},
			Publish:    []string{"8080:80"},
			Kits:       []string{"git+https://example.com/kits"},
		},
		Caches: []string{"gomod"},
	}); err != nil {
		t.Fatalf("create sandbox %s: %v", name, err)
	}
}

func exportedSbxenv(t *testing.T, fl *fleet.Fleet, name string) []byte {
	t.Helper()
	result, err := (&app.App{Fleet: fl}).ExportSbxenv(name)
	if err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return result.Document
}

func TestExportSbxenvPrintsTheDocumentToStdout(t *testing.T) {
	env := newCLIEnv(t)
	fl := env.fleet(t)
	exportSbxenvFixture(t, fl, "web-dev")
	want := string(exportedSbxenv(t, fl, "web-dev"))
	stdout, stderr, err := env.execute(t, "export", "--sbxenv", "web-dev")
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}
	if stdout != want {
		t.Fatalf("stdout mismatch\n--- want ---\n%s\n--- got ---\n%s", want, stdout)
	}
	mustContain(t, stderr, "export: web-dev: caches cannot be expressed in sbxenv.yaml (gomod)")
	mustContain(t, stderr, "export: web-dev: permissions.network.deny cannot be expressed in sbxenv.yaml (telemetry.example.com)")
	env.requireNoSbxCall(t, "env")
}

func TestExportSbxenvWritesAFile(t *testing.T) {
	env := newCLIEnv(t)
	fl := env.fleet(t)
	exportSbxenvFixture(t, fl, "web-dev")
	want := exportedSbxenv(t, fl, "web-dev")
	path := filepath.Join(t.TempDir(), "sbxenv.yaml")
	stdout, stderr, err := env.execute(t, "export", "--sbxenv", "-o", path, "web-dev")
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if string(data) != string(want) {
		t.Fatalf("file mismatch\n--- want ---\n%s\n--- got ---\n%s", want, data)
	}
	mustContain(t, stdout, "wrote "+path)
	mustContain(t, stderr, "export: web-dev: caches cannot be expressed")
}

func TestExportSbxenvWritesOneFilePerSandbox(t *testing.T) {
	env := newCLIEnv(t)
	fl := env.fleet(t)
	exportSbxenvFixture(t, fl, "web-dev")
	exportSbxenvFixture(t, fl, "api")
	if _, err := fl.CreateSandbox("agentless", fleet.NewMixin(""), fleet.SandboxApp{}); err != nil {
		t.Fatalf("create agentless: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "out")
	stdout, stderr, err := env.execute(t, "export", "--sbxenv", "-o", dir)
	if err == nil {
		t.Fatal("a sandbox without an agent must fail the export")
	}
	for _, name := range []string{"api", "web-dev"} {
		path := filepath.Join(dir, name+".sbxenv.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if want := exportedSbxenv(t, fl, name); string(data) != string(want) {
			t.Fatalf("%s mismatch\n--- want ---\n%s\n--- got ---\n%s", path, want, data)
		}
		mustContain(t, stdout, "wrote "+path)
	}
	if _, err := os.Stat(filepath.Join(dir, "agentless.sbxenv.yaml")); !os.IsNotExist(err) {
		t.Fatalf("the agentless sandbox must not produce a file (stat err=%v)", err)
	}
	mustContain(t, stderr, "export: agentless:")
}

func TestExportSbxenvResolvesSlugAndCanonicalName(t *testing.T) {
	env := newCLIEnv(t)
	fl := env.fleet(t)
	spec := fleet.NewMixin("")
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	if _, err := fl.CreateSandbox("My VM", spec, fleet.SandboxApp{}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if _, _, err := env.execute(t, "export", "--sbxenv", "My VM"); err != nil {
		t.Fatalf("export by canonical name: %v", err)
	}
	bySlug, _, err := env.execute(t, "export", "--sbxenv", "my-vm")
	if err != nil {
		t.Fatalf("export by slug: %v", err)
	}
	if strings.Contains(bySlug, "name:") {
		t.Fatalf("an invalid canonical name must be omitted:\n%s", bySlug)
	}
}

func TestExportSbxenvRejectsMissingTargets(t *testing.T) {
	env := newCLIEnv(t)
	if _, _, err := env.execute(t, "export"); err == nil {
		t.Fatal("export without --sbxenv must fail")
	}
	if _, _, err := env.execute(t, "export", "--sbxenv"); err == nil {
		t.Fatal("export --sbxenv without a sandbox or -o DIR must fail")
	}
	if _, _, err := env.execute(t, "export", "--sbxenv", "missing"); err == nil {
		t.Fatal("export of an unknown sandbox must fail")
	}
}
