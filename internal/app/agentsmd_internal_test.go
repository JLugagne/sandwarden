package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAgentsFileDefaultsToTemplate(t *testing.T) {
	a, _ := newTestApp(t)

	doc, err := a.AgentsFile(context.Background())
	if err != nil {
		t.Fatalf("agents file: %v", err)
	}
	if !doc.Default || doc.Content != defaultAgentsFile {
		t.Fatalf("expected the default template, got default=%v", doc.Default)
	}
	if _, err := os.Stat(doc.Path); err != nil {
		t.Fatalf("default file not written: %v", err)
	}
}

func TestSaveAgentsFileRewritesInPlace(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	doc, err := a.AgentsFile(ctx)
	if err != nil {
		t.Fatalf("agents file: %v", err)
	}
	before, err := os.Stat(doc.Path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	saved, err := a.SaveAgentsFile(ctx, "# Custom\n")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	after, err := os.Stat(doc.Path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("save replaced the file, which detaches it from running sandboxes")
	}
	if saved.Default || saved.Content != "# Custom\n" {
		t.Fatalf("unexpected saved doc: %+v", saved)
	}

	reset, err := a.ResetAgentsFile(ctx)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !reset.Default || reset.Content != defaultAgentsFile {
		t.Fatal("reset did not restore the default template")
	}
}

func TestApplyMountsAgentsFileReadOnly(t *testing.T) {
	a, _ := seedLockTest(t)
	logPath := newSkillsStub(t)
	doc, err := a.AgentsFile(context.Background())
	if err != nil {
		t.Fatalf("agents file: %v", err)
	}

	if _, err := a.Apply(context.Background(), "box"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	calls := skillCalls(t, logPath)
	want := "mount box " + doc.Path + ":" + sandboxAgentsFile + ":ro"
	if countCalls(calls, want) != 1 {
		t.Fatalf("agents file not mounted read-only, calls: %v", calls)
	}
	for _, call := range calls {
		if strings.HasPrefix(call, "exec box mkdir -p "+sandboxAgentsFile) {
			t.Fatalf("created a directory where the file is mounted: %q", call)
		}
	}
}
