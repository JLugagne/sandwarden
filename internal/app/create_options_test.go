package app

import (
	"slices"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

func TestCreateOptionsFromConfigRoundTripsNewFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	a := newExportApp(t)
	spec := fleet.NewMixin("")
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	create := &fleet.SandboxCreate{
		KitArgs:      []string{"repo=acme/app", "mcp.strict=true"},
		KitArgsFiles: []string{"~/kit-args.txt"},
		EnvFiles:     []string{"~/secrets.env"},
		Skills:       "readwrite",
		StaticMCP:    []string{"notion", "none"},
	}
	s, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{Sandbox: "box", Create: create})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	opts := a.CreateOptionsFromConfig(s)
	if !slices.Equal(opts.KitArgs, create.KitArgs) {
		t.Fatalf("KitArgs = %v, want %v", opts.KitArgs, create.KitArgs)
	}
	if !slices.Equal(opts.StaticMCP, create.StaticMCP) {
		t.Fatalf("StaticMCP = %v, want %v", opts.StaticMCP, create.StaticMCP)
	}
	if opts.Skills != "readwrite" {
		t.Fatalf("Skills = %q, want readwrite", opts.Skills)
	}
	wantKitArgsFile := home + "/kit-args.txt"
	if !slices.Equal(opts.KitArgsFiles, []string{wantKitArgsFile}) {
		t.Fatalf("KitArgsFiles = %v, want [%s]", opts.KitArgsFiles, wantKitArgsFile)
	}
	wantEnvFile := home + "/secrets.env"
	if !slices.Equal(opts.EnvFiles, []string{wantEnvFile}) {
		t.Fatalf("EnvFiles = %v, want [%s]", opts.EnvFiles, wantEnvFile)
	}
}

func TestRecordCreatedSandboxPersistsNewCreateFields(t *testing.T) {
	a := newExportApp(t)
	req := CreateRequest{Opts: sbx.CreateOptions{
		Agent:        "claude",
		Name:         "box",
		KitArgs:      []string{"repo=acme/app"},
		KitArgsFiles: []string{"/tmp/kit-args"},
		EnvFiles:     []string{"/tmp/secrets.env"},
		Skills:       "readonly",
		StaticMCP:    []string{"notion,atlassian"},
	}}
	s, err := a.recordCreatedSandbox("box", req)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if s.App.Create == nil {
		t.Fatal("create block was not written")
	}
	c := s.App.Create
	if !slices.Equal(c.KitArgs, req.Opts.KitArgs) {
		t.Fatalf("KitArgs = %v, want %v", c.KitArgs, req.Opts.KitArgs)
	}
	if !slices.Equal(c.KitArgsFiles, req.Opts.KitArgsFiles) {
		t.Fatalf("KitArgsFiles = %v, want %v", c.KitArgsFiles, req.Opts.KitArgsFiles)
	}
	if !slices.Equal(c.EnvFiles, req.Opts.EnvFiles) {
		t.Fatalf("EnvFiles = %v, want %v", c.EnvFiles, req.Opts.EnvFiles)
	}
	if c.Skills != "readonly" {
		t.Fatalf("Skills = %q, want readonly", c.Skills)
	}
	if !slices.Equal(c.StaticMCP, req.Opts.StaticMCP) {
		t.Fatalf("StaticMCP = %v, want %v", c.StaticMCP, req.Opts.StaticMCP)
	}

	fresh, err := fleet.Open(a.Fleet.Dir())
	if err != nil {
		t.Fatalf("reopen fleet: %v", err)
	}
	saved, ok := fresh.SandboxByName("box")
	if !ok || saved.App.Create == nil {
		t.Fatal("box missing after reopen")
	}
	if !slices.Equal(saved.App.Create.KitArgs, req.Opts.KitArgs) {
		t.Fatalf("saved KitArgs = %v, want %v", saved.App.Create.KitArgs, req.Opts.KitArgs)
	}
}
