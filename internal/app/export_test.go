package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func newExportApp(t *testing.T) *App {
	t.Helper()
	fl, err := fleet.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open fleet: %v", err)
	}
	return &App{Fleet: fl}
}

func exportFixture(t *testing.T) (*App, *fleet.Sandbox) {
	t.Helper()
	core := newExportApp(t)
	spec := fleet.NewMixin("")
	spec.DisplayName = "Web dev"
	spec.Description = "Frontend work"
	spec.SourceURL = "https://example.com/kit"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	spec.Environment = &fleet.SpecEnv{Variables: map[string]string{
		"LOG_LEVEL": "debug",
		"NODE_ENV":  "development",
		"FROM_ENV":  "",
	}}
	spec.Ports = []fleet.SpecPort{{Container: 3000, Protocol: "tcp", Name: "http"}}
	spec.Permissions = &fleet.SpecPermission{Network: &fleet.SpecNetwork{
		Allow: []string{"api.github.com"},
		Deny:  []string{"telemetry.example.com"},
	}}
	spec.Sandbox = &fleet.SpecSandbox{
		Image:      "ubuntu:24.04",
		Entrypoint: []string{"/bin/bash"},
		Command:    &fleet.SpecCommands{Default: "bash"},
		Resources:  &fleet.SpecResource{CPU: 2, Memory: "4g"},
	}
	spec.Setup = &fleet.SpecSetup{
		Install: []fleet.SpecCommand{{Command: "make setup"}},
		Files:   []fleet.SpecFile{{Path: "/etc/motd"}},
	}
	spec.Credentials = []any{"github"}
	spec.Arguments = map[string]any{"model": "opus"}
	s, err := core.Fleet.CreateSandbox("web-dev", spec, fleet.SandboxApp{
		Create: &fleet.SandboxCreate{
			CPUs:       4,
			Memory:     "8g",
			Workspaces: []string{"/home/you/src/web", "/home/you/src/api:ro"},
			Template:   "ubuntu-24.04",
			Publish:    []string{"3000:3000", "5353/udp", "127.0.0.1:8080:8080"},
			Kits:       []string{"./mixins/base", "git+https://github.com/acme/sbx-kits#dir=node&ref=v1"},
		},
		Profiles: []string{"web-dev"},
		Caches:   []string{"go-mod-cache"},
		Skills:   []fleet.SkillRef{{Store: "anthropics-skills", Kind: "command", Name: "review"}},
		Mounts:   []fleet.MountRef{{HostPath: "/home/you/data", TargetPath: "/data", ReadOnly: true}},
		RunArgs:  "--model opus",
		OptOuts:  &fleet.SandboxOptOut{Mounts: []string{"/home/you/src/shared:/shared"}, Caches: []string{"npm-cache"}},
	})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	return core, s
}

const sbxenvGolden = `schemaVersion: 1
name: web-dev
agent: claude
kits:
- ./mixins/base
- "git+https://github.com/acme/sbx-kits#dir=node&ref=v1"
workspace:
  path: /home/you/src/web
env:
  LOG_LEVEL: debug
  NODE_ENV: development
ports:
- sandbox: 3000
  host: 3000
- sandbox: 5353
  protocol: udp
`

func exportContains(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func TestExportSbxenvPinsTheDocument(t *testing.T) {
	core, _ := exportFixture(t)
	result, err := core.ExportSbxenv("web-dev")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if result.Slug != "web-dev" || result.Name != "web-dev" {
		t.Fatalf("result identity = %q/%q, want web-dev/web-dev", result.Slug, result.Name)
	}
	if got := string(result.Document); got != sbxenvGolden {
		t.Fatalf("sbxenv document mismatch\n--- want ---\n%s\n--- got ---\n%s", sbxenvGolden, got)
	}
}

func TestExportSbxenvReportsUnsupportedFields(t *testing.T) {
	core, _ := exportFixture(t)
	result, err := core.ExportSbxenv("web-dev")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	want := []string{
		"create.kits[0] (./mixins/base): sbxenv resolves a relative kit source against the sbxenv.yaml file",
		"create.workspaces[1] (/home/you/src/api:ro) cannot be expressed in sbxenv.yaml (sbxenv mounts one read-write workspace)",
		"create.publish[2] (127.0.0.1:8080:8080) cannot be expressed in sbxenv.yaml (a host IP binding cannot be expressed)",
		"environment.variables[FROM_ENV] cannot be expressed in sbxenv.yaml (a bare key takes its value from the host environment)",
		"create.cpus cannot be expressed in sbxenv.yaml (4)",
		"create.memory cannot be expressed in sbxenv.yaml (8g)",
		"create.template cannot be expressed in sbxenv.yaml (ubuntu-24.04)",
		"displayName cannot be expressed in sbxenv.yaml (Web dev)",
		"description cannot be expressed in sbxenv.yaml (Frontend work)",
		"sourceURL cannot be expressed in sbxenv.yaml (https://example.com/kit)",
		"spec.sandbox.image cannot be expressed in sbxenv.yaml (ubuntu:24.04)",
		"spec.sandbox.entrypoint cannot be expressed in sbxenv.yaml (/bin/bash)",
		"spec.sandbox.command cannot be expressed in sbxenv.yaml",
		"spec.sandbox.resources.cpu cannot be expressed in sbxenv.yaml (2)",
		"spec.sandbox.resources.memory cannot be expressed in sbxenv.yaml (4g)",
		"spec.ports cannot be expressed in sbxenv.yaml (3000/tcp)",
		"permissions.network.allow cannot be expressed in sbxenv.yaml (api.github.com)",
		"permissions.network.deny cannot be expressed in sbxenv.yaml (telemetry.example.com)",
		"spec.setup.install cannot be expressed in sbxenv.yaml (1 entry)",
		"spec.setup.files cannot be expressed in sbxenv.yaml (1 entry)",
		"spec.credentials cannot be expressed in sbxenv.yaml (1 entry)",
		"spec.arguments cannot be expressed in sbxenv.yaml (model)",
		"profiles cannot be expressed in sbxenv.yaml (web-dev)",
		"caches cannot be expressed in sbxenv.yaml (go-mod-cache)",
		"skills cannot be expressed in sbxenv.yaml (anthropics-skills:command:review)",
		"mounts cannot be expressed in sbxenv.yaml (/home/you/data:/data)",
		"runArgs cannot be expressed in sbxenv.yaml (--model opus)",
		"optOuts.mounts cannot be expressed in sbxenv.yaml (/home/you/src/shared:/shared)",
		"optOuts.caches cannot be expressed in sbxenv.yaml (npm-cache)",
	}
	for _, line := range want {
		if !exportContains(result.Unsupported, line) {
			t.Errorf("unsupported report is missing %q\ngot:\n%s", line, strings.Join(result.Unsupported, "\n"))
		}
	}
	if len(result.Unsupported) != len(want) {
		t.Errorf("unsupported report has %d lines, want %d\ngot:\n%s", len(result.Unsupported), len(want), strings.Join(result.Unsupported, "\n"))
	}
}

func TestExportSbxenvCoversEveryMappedField(t *testing.T) {
	core, _ := exportFixture(t)
	result, err := core.ExportSbxenv("web-dev")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	doc := string(result.Document)
	for _, want := range []string{
		"schemaVersion: 1",
		"name: web-dev",
		"agent: claude",
		"kits:",
		"./mixins/base",
		"workspace:",
		"path: /home/you/src/web",
		"env:",
		"LOG_LEVEL: debug",
		"NODE_ENV: development",
		"ports:",
		"sandbox: 3000",
		"host: 3000",
		"protocol: udp",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("sbxenv document is missing %q\ndocument:\n%s", want, doc)
		}
	}
	for _, want := range []string{
		"permissions.network.allow cannot be expressed in sbxenv.yaml (api.github.com)",
		"permissions.network.deny cannot be expressed in sbxenv.yaml (telemetry.example.com)",
	} {
		if !exportContains(result.Unsupported, want) {
			t.Errorf("unsupported report is missing %q", want)
		}
	}
}

func TestExportSbxenvOmitsAnInvalidName(t *testing.T) {
	core := newExportApp(t)
	spec := fleet.NewMixin("")
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	if _, err := core.Fleet.CreateSandbox("Web dev", spec, fleet.SandboxApp{}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	result, err := core.ExportSbxenv("Web dev")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if strings.Contains(string(result.Document), "name:") {
		t.Fatalf("document must not carry an invalid name:\n%s", result.Document)
	}
	want := `name "Web dev" cannot be expressed in sbxenv.yaml (sbxenv names match ^[a-zA-Z0-9][a-zA-Z0-9.-]+$, need at least two characters and cannot be "default")`
	if !exportContains(result.Unsupported, want) {
		t.Errorf("unsupported report is missing %q\ngot:\n%s", want, strings.Join(result.Unsupported, "\n"))
	}
}

func TestExportSbxenvMapsCloneToTheWorkspace(t *testing.T) {
	core := newExportApp(t)
	spec := fleet.NewMixin("")
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	if _, err := core.Fleet.CreateSandbox("clone-me", spec, fleet.SandboxApp{
		Create: &fleet.SandboxCreate{Workspaces: []string{"/home/you/src/app"}, Clone: true},
	}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	result, err := core.ExportSbxenv("clone-me")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(result.Document), "clone: true") {
		t.Fatalf("document must carry workspace.clone:\n%s", result.Document)
	}
	for _, line := range result.Unsupported {
		if strings.HasPrefix(line, "create.clone") {
			t.Fatalf("clone must not be reported as unsupported when a workspace exists: %q", line)
		}
	}

	if _, err := core.Fleet.CreateSandbox("clone-nowhere", spec, fleet.SandboxApp{
		Create: &fleet.SandboxCreate{Clone: true},
	}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	result, err = core.ExportSbxenv("clone-nowhere")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	want := "create.clone cannot be expressed in sbxenv.yaml (workspace.clone needs a workspace path)"
	if !exportContains(result.Unsupported, want) {
		t.Errorf("unsupported report is missing %q\ngot:\n%s", want, strings.Join(result.Unsupported, "\n"))
	}
}

func TestSbxenvParsePort(t *testing.T) {
	cases := []struct {
		raw     string
		want    sbxenvPort
		wantErr bool
	}{
		{raw: "3000", want: sbxenvPort{Sandbox: 3000}},
		{raw: "3000:3000", want: sbxenvPort{Host: 3000, Sandbox: 3000}},
		{raw: "8080:3000/tcp", want: sbxenvPort{Host: 8080, Sandbox: 3000, Protocol: "tcp"}},
		{raw: "5353/udp", want: sbxenvPort{Sandbox: 5353, Protocol: "udp"}},
		{raw: "127.0.0.1:8080:3000", wantErr: true},
		{raw: "[::1]:8080:3000", wantErr: true},
		{raw: "3000/sctp", wantErr: true},
		{raw: "70000", wantErr: true},
		{raw: "0", wantErr: true},
		{raw: "3000:0", wantErr: true},
		{raw: "http", wantErr: true},
	}
	for _, tc := range cases {
		got, err := sbxenvParsePort(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("sbxenvParsePort(%q) = %+v, want error", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("sbxenvParsePort(%q): %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("sbxenvParsePort(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}
}

func TestExportSbxenvErrors(t *testing.T) {
	core := newExportApp(t)
	if _, err := core.ExportSbxenv("missing"); err == nil {
		t.Fatal("export of an unknown sandbox must fail")
	}
	spec := fleet.NewMixin("")
	if _, err := core.Fleet.CreateSandbox("agentless", spec, fleet.SandboxApp{}); err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if _, err := core.ExportSbxenv("agentless"); err == nil {
		t.Fatal("export of a sandbox without requires.agent must fail")
	}
}
