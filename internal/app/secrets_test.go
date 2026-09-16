package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JLugagne/sandwarden/internal/sbx"
)

func TestSetSandboxSecretNotifiesSandboxDetail(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "fake-sbx")
	content := "#!/bin/sh\ncase \"$*\" in\n  \"secret ls --json\") printf '{\"secrets\":[],\"custom_secrets\":[]}' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(stub, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", stub)

	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	if err := a.SetServiceSecret(ctx, sbx.ServiceSecretSpec{Service: "github", Scope: "box", Value: "dummy"}); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	waitForTopic(t, events, TopicSandbox("box"))
}

func TestRemoveSandboxSecretNotifiesSandboxDetail(t *testing.T) {
	stub := filepath.Join(t.TempDir(), "fake-sbx")
	content := "#!/bin/sh\ncase \"$*\" in\n  \"secret ls --json\") printf '{\"secrets\":[],\"custom_secrets\":[]}' ;;\nesac\nexit 0\n"
	if err := os.WriteFile(stub, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv("SBX_BINARY", stub)

	ctx := context.Background()
	a, _ := newTestApp(t, "box")
	events, unsubscribe := a.Hub.Subscribe()
	defer unsubscribe()

	if err := a.RemoveSecret(ctx, "box", "github"); err != nil {
		t.Fatalf("remove secret: %v", err)
	}
	waitForTopic(t, events, TopicSandbox("box"))
}
