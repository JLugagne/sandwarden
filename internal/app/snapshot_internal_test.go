package app

import (
	"context"
	"testing"
)

func TestSetSandboxRunArgsAppendedToConnectRun(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t, "box")

	summaries, err := a.SandboxSummaries(ctx)
	if err != nil {
		t.Fatalf("summaries: %v", err)
	}
	if summaries[0].Connect.Run != "sbx run --name box" {
		t.Fatalf("expected no custom args yet, got %q", summaries[0].Connect.Run)
	}
	if summaries[0].RunArgs != "" {
		t.Fatalf("expected empty run args, got %q", summaries[0].RunArgs)
	}

	if err := a.SetSandboxRunArgs(ctx, "box", "--auto"); err != nil {
		t.Fatalf("set run args: %v", err)
	}

	summaries, err = a.SandboxSummaries(ctx)
	if err != nil {
		t.Fatalf("summaries after set: %v", err)
	}
	if summaries[0].Connect.Run != "sbx run --name box -- --auto" {
		t.Fatalf("unexpected connect run: %q", summaries[0].Connect.Run)
	}
	if summaries[0].RunArgs != "--auto" {
		t.Fatalf("unexpected run args: %q", summaries[0].RunArgs)
	}

	detail, err := a.SandboxDetail(ctx, "box")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Sandbox.Connect.Run != "sbx run --name box -- --auto" {
		t.Fatalf("unexpected detail connect run: %q", detail.Sandbox.Connect.Run)
	}

	if err := a.SetSandboxRunArgs(ctx, "box", ""); err != nil {
		t.Fatalf("clear run args: %v", err)
	}
	summaries, _ = a.SandboxSummaries(ctx)
	if summaries[0].Connect.Run != "sbx run --name box" {
		t.Fatalf("expected cleared connect run, got %q", summaries[0].Connect.Run)
	}
}
