package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

// seedSandboxConfig writes a complete sandbox config directory.
func seedSandboxConfig(t *testing.T, a *App, name string, create *fleet.SandboxCreate) {
	t.Helper()
	spec := fleet.NewMixin("")
	spec.DisplayName = name
	if _, err := a.Fleet.CreateSandbox(name, spec, fleet.SandboxApp{Sandbox: name, Create: create}); err != nil {
		t.Fatalf("seed sandbox %s: %v", name, err)
	}
}

func TestImportAdoptsDaemonSandboxesAsIncomplete(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box-a", "box-b")
	newSkillsStub(t)

	report, err := a.ImportSandboxes(ctx, nil)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Created != 2 || report.Configured != 0 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	for _, res := range report.Results {
		if res.Status != ImportCreated || !res.Incomplete {
			t.Fatalf("unexpected result: %+v", res)
		}
	}
	for _, name := range []string{"box-a", "box-b"} {
		cfg, ok := a.Fleet.SandboxByName(name)
		if !ok {
			t.Fatalf("config not written for %s", name)
		}
		if cfg.App.Create == nil || !cfg.App.Create.Incomplete {
			t.Fatalf("config for %s is not incomplete: %+v", name, cfg.App.Create)
		}
	}

	summaries, err := a.SandboxSummaries(ctx)
	if err != nil {
		t.Fatalf("summaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}
	for _, summary := range summaries {
		if !summary.Incomplete {
			t.Fatalf("summary for %s does not surface incomplete", summary.Name)
		}
	}

	detail, err := a.SandboxDetail(ctx, "box-a")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if !detail.Incomplete || !detail.Sandbox.Incomplete {
		t.Fatalf("detail does not surface incomplete: %+v", detail)
	}
}

func TestImportKeepsConfiguredSandboxesComplete(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box-a", "box-b")
	newSkillsStub(t)
	seedSandboxConfig(t, a, "box-a", &fleet.SandboxCreate{CPUs: 4, Memory: "8g", Workspaces: []string{"/w"}})

	report, err := a.ImportSandboxes(ctx, nil)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Created != 1 || report.Configured != 1 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	byName := map[string]ImportResult{}
	for _, res := range report.Results {
		byName[res.Name] = res
	}
	if res := byName["box-a"]; res.Status != ImportConfigured || res.Incomplete {
		t.Fatalf("configured sandbox altered: %+v", res)
	}
	if res := byName["box-b"]; res.Status != ImportCreated || !res.Incomplete {
		t.Fatalf("adopted sandbox not marked incomplete: %+v", res)
	}
}

func TestImportReportsOneFailureWithoutAborting(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box-a")
	newSkillsStub(t)

	report, err := a.ImportSandboxes(ctx, []string{"ghost", "box-a"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Failed != 1 || report.Created != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if res := report.Results[0]; res.Name != "ghost" || res.Status != ImportFailed || res.Error != "not in daemon" {
		t.Fatalf("unexpected failure result: %+v", res)
	}
	if res := report.Results[1]; res.Name != "box-a" || res.Status != ImportCreated {
		t.Fatalf("batch aborted after a failure: %+v", report.Results)
	}
}

func TestImportAfterPurgeMarksIncomplete(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newSkillsStub(t)
	seedSandboxConfig(t, a, "box", &fleet.SandboxCreate{CPUs: 2, Memory: "2g", Workspaces: []string{"/w"}})
	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok {
		t.Fatal("seed config missing")
	}
	if err := a.Fleet.DeleteSandbox(cfg.Slug); err != nil {
		t.Fatalf("purge config: %v", err)
	}

	report, err := a.ImportSandboxes(ctx, []string{"box"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Created != 1 || report.Failed != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !report.Results[0].Incomplete {
		t.Fatalf("re-imported sandbox not marked incomplete: %+v", report.Results[0])
	}
}

func TestCompleteSandboxConfigClearsIncomplete(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newSkillsStub(t)
	if _, err := a.ImportSandboxes(ctx, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	if err := a.CompleteSandboxConfig(ctx, "box", CompleteConfigInput{CPUs: 4, Memory: "8g", Env: []string{"A=B", "B"}}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok {
		t.Fatal("config missing after completion")
	}
	if cfg.App.Create.Incomplete {
		t.Fatal("incomplete marker not cleared")
	}
	if cfg.App.Create.CPUs != 4 || cfg.App.Create.Memory != "8g" {
		t.Fatalf("create parameters not recorded: %+v", cfg.App.Create)
	}
	env := cfg.Spec.Env()
	if env["A"] != "B" {
		t.Fatalf("environment not recorded: %+v", env)
	}
	if _, ok := env["B"]; !ok {
		t.Fatalf("key without value dropped: %+v", env)
	}

	detail, err := a.SandboxDetail(ctx, "box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Incomplete || detail.Sandbox.Incomplete {
		t.Fatalf("detail still incomplete: %+v", detail)
	}
}

func TestRecreateWarnsOnIncompleteConfig(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	newSkillsStub(t)
	if err := a.SetSandboxRunArgs(ctx, "box", "--auto"); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	detail, err := a.SandboxDetail(ctx, "box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if !detail.Incomplete {
		t.Fatal("adopted config must be incomplete")
	}

	var out bytes.Buffer
	// The warning must precede the destructive step. The fake daemon cannot
	// finish the create, so the returned error is not asserted here.
	_ = a.RecreateSandbox(ctx, "box", &out)
	if !strings.Contains(out.String(), "will not restore") {
		t.Fatalf("recreate did not warn about the incomplete config:\n%s", out.String())
	}
}
