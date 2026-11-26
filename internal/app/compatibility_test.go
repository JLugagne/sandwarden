package app

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

func TestCreateSandboxRefusesWorkspaceTargetOnOldSbx(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.43.0 79805a6e")

	req := CreateRequest{Opts: sbx.CreateOptions{
		Agent:      "claude",
		Name:       "box",
		Workspaces: []string{"/host/project:/srv/project"},
	}}
	err := a.CreateSandbox(context.Background(), req, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "workspace mount target (create.workspaces) requires sbx v0.51.0 or later; installed sbx is v0.43.0") {
		t.Fatalf("err = %v, want the workspace-target refusal", err)
	}
	if _, ok := a.Fleet.SandboxByName("box"); ok {
		t.Fatal("the config was recorded despite the refusal")
	}
}

func TestCreateSandboxAllowsWorkspaceTargetOnNewSbx(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.51.0 abcdef")

	req := CreateRequest{Opts: sbx.CreateOptions{
		Agent:      "claude",
		Name:       "box",
		Workspaces: []string{"/host/project:/srv/project"},
	}}
	if err := a.CreateSandbox(context.Background(), req, io.Discard); err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok || cfg.App.Create == nil {
		t.Fatal("the config was not recorded")
	}
	if !slices.Equal(cfg.App.Create.Workspaces, []string{"/host/project:/srv/project"}) {
		t.Fatalf("workspaces = %v", cfg.App.Create.Workspaces)
	}
}

func TestCreateSandboxAllowsUnknownSbxVersion(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: dev-79805a6e")

	req := CreateRequest{Opts: sbx.CreateOptions{
		Agent:      "claude",
		Name:       "box",
		Workspaces: []string{"/host/project:/srv/project"},
		Skills:     "readonly",
	}}
	if err := a.CreateSandbox(context.Background(), req, io.Discard); err != nil {
		t.Fatalf("an unknown sbx version must not block create: %v", err)
	}
}

func TestRecreateRefusesUnsupportedConfigBeforeDeleting(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.43.0 abcdef")

	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	if _, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{
		Sandbox: "box",
		Create:  &fleet.SandboxCreate{Workspaces: []string{"/host:/srv"}},
	}); err != nil {
		t.Fatalf("create sandbox config: %v", err)
	}

	err := a.RecreateSandbox(context.Background(), "box", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "create.workspaces") {
		t.Fatalf("err = %v, want a compatibility refusal", err)
	}
}

func TestAttachKitRefusesKitArgsOnOldSbx(t *testing.T) {
	a, logPath := newKitAddApp(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.42.0 abcdef")

	_, err := a.AttachKit(context.Background(), "alpha", "./mcp-postgres", []string{"repo=acme/app"}, nil)
	if err == nil || !strings.Contains(err.Error(), "kit add --kit-arg (kit.add.kitArg) requires sbx v0.43.0") {
		t.Fatalf("err = %v, want a kit-arg compatibility refusal", err)
	}
	if calls := skillCalls(t, logPath); countCalls(calls, "kit add ") != 0 {
		t.Fatalf("sbx kit add ran despite the refusal: %v", calls)
	}
}

func TestAttachKitAllowsKitArgsOnUnknownSbx(t *testing.T) {
	a, _ := newKitAddApp(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: dev-abc")

	if _, err := a.AttachKit(context.Background(), "alpha", "./mcp-postgres", []string{"repo=acme/app"}, nil); err != nil {
		t.Fatalf("an unknown sbx version must not block kit add: %v", err)
	}
}

func TestSandboxCompatibilityReportsMessages(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "sbx version: v0.43.0 abcdef")

	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	s, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{
		Sandbox: "box",
		Create: &fleet.SandboxCreate{
			Workspaces: []string{"/host/project:/srv/project"},
			Skills:     "readonly", // supported since v0.43.0
		},
	})
	if err != nil {
		t.Fatalf("create sandbox config: %v", err)
	}

	messages := a.SandboxCompatibility(context.Background(), s)
	if len(messages) != 1 {
		t.Fatalf("messages = %v, want exactly the workspace target", messages)
	}
	if !strings.Contains(messages[0], "workspace mount target (create.workspaces) requires sbx v0.51.0 or later; installed sbx is v0.43.0") {
		t.Fatalf("message = %q", messages[0])
	}
}

func TestSandboxCompatibilityAllowsUnknownVersion(t *testing.T) {
	a, _ := newTestApp(t)
	newFakeSbx(t)
	t.Setenv("FAKE_SBX_VERSION", "dev-abc")

	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	s, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{
		Sandbox: "box",
		Create:  &fleet.SandboxCreate{Workspaces: []string{"/host:/srv"}},
	})
	if err != nil {
		t.Fatalf("create sandbox config: %v", err)
	}
	if messages := a.SandboxCompatibility(context.Background(), s); len(messages) != 0 {
		t.Fatalf("messages = %v, want none for an unknown version", messages)
	}
}

func TestSbxenvCompatibilityReportsAdditionalTargets(t *testing.T) {
	core := newExportApp(t)
	spec := fleet.NewMixin("")
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	s, err := core.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{
		Sandbox: "box",
		Create:  &fleet.SandboxCreate{Workspaces: []string{"/host/primary", "/host/api:/srv/api"}},
	})
	if err != nil {
		t.Fatalf("create sandbox config: %v", err)
	}
	result, err := renderSbxenv(s)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !slices.Equal(result.additionalTargets, []string{"/srv/api"}) {
		t.Fatalf("additionalTargets = %v, want [/srv/api]", result.additionalTargets)
	}

	annotateSbxenvCompatibility(&result, sbx.ParseVersion("sbx version: v0.43.0 abc"))
	if len(result.Unsupported) != 1 || !strings.Contains(result.Unsupported[0], "additionalWorkspaces[].target /srv/api requires sbx v0.51.0") {
		t.Fatalf("unsupported = %v, want the target incompatibility", result.Unsupported)
	}

	newer := SbxenvExport{additionalTargets: []string{"/srv/api"}}
	annotateSbxenvCompatibility(&newer, sbx.ParseVersion("v0.51.0"))
	if len(newer.Unsupported) != 0 {
		t.Fatalf("unsupported = %v, want none on v0.51.0", newer.Unsupported)
	}
}
