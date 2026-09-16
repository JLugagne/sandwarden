package sbx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const secretListFixture = `{
  "secrets": [
    {"scope":"global","type":"service","name":"github","secret":"*****"},
    {"scope":"box","type":"service","name":"openai","secret":"*****"},
    {"scope":"host-only","type":"registry","name":"ghcr.io","secret":"*****","username":"user"}
  ],
  "custom_secrets": [
    {"scope":"global","targets":["*.example.com","api.other.io"],"env":"API_KEY","placeholder":"ph-1","secret":"*****"},
    {"scope":"box","targets":["cmd.test"],"env":"X","placeholder":"ph-2","kind":"command","source":"echo","refresh":"30m0s"}
  ]
}`

// newSecretStub installs a fake sbx that logs argv and captures stdin.
func newSecretStub(t *testing.T, listJSON string) (logPath, stdinPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "args.log")
	stdinPath = filepath.Join(dir, "stdin.txt")
	outPath := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(outPath, []byte(listJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SBX_STUB_LOG\"\ncat > \"$SBX_STUB_STDIN\" 2>/dev/null\ncat \"$SBX_STUB_OUT\" 2>/dev/null\nexit 0\n"
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("SBX_STUB_LOG", logPath)
	t.Setenv("SBX_STUB_STDIN", stdinPath)
	t.Setenv("SBX_STUB_OUT", outPath)
	return logPath, stdinPath
}

func lastCall(t *testing.T, logPath string) string {
	t.Helper()
	calls := stubCalls(t, logPath)
	if len(calls) == 0 {
		return ""
	}
	return calls[len(calls)-1]
}

func TestListSecretsParsesJSON(t *testing.T) {
	newSecretStub(t, secretListFixture)

	list, err := New("/nonexistent.sock").ListSecrets(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Stored) != 3 || len(list.Custom) != 2 {
		t.Fatalf("unexpected list sizes: %+v", list)
	}
	if list.Stored[0].Scope != "" || list.Stored[0].Name != "github" || list.Stored[0].Masked != "*****" {
		t.Fatalf("unexpected stored[0]: %+v", list.Stored[0])
	}
	if list.Stored[1].Scope != "box" {
		t.Fatalf("expected sandbox scope, got %+v", list.Stored[1])
	}
	if list.Stored[2].Scope != SecretScopeHostOnly || list.Stored[2].Masked != "user/*****" {
		t.Fatalf("unexpected registry row: %+v", list.Stored[2])
	}
	if len(list.Custom[0].Targets) != 2 || list.Custom[0].Placeholder != "ph-1" {
		t.Fatalf("unexpected custom[0]: %+v", list.Custom[0])
	}
	if list.Custom[1].Masked != "command:echo (30m0s)" {
		t.Fatalf("unexpected custom[1] mask: %+v", list.Custom[1])
	}
}

func TestSetServiceSecretPipesValueOnStdin(t *testing.T) {
	logPath, stdinPath := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
	client := New("/nonexistent.sock")

	if err := client.SetServiceSecret(context.Background(), ServiceSecretSpec{
		Service: "github",
		Value:   "s3cr3t-token",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	call := lastCall(t, logPath)
	if call != "secret set github" {
		t.Fatalf("unexpected argv: %q", call)
	}
	if strings.Contains(call, "s3cr3t-token") {
		t.Fatalf("secret leaked into argv: %q", call)
	}
	raw, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}
	if strings.TrimRight(string(raw), "\n") != "s3cr3t-token" {
		t.Fatalf("unexpected stdin: %q", string(raw))
	}
}

func TestSetServiceSecretRefUsesFlagNoStdin(t *testing.T) {
	logPath, stdinPath := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
	client := New("/nonexistent.sock")

	if err := client.SetServiceSecret(context.Background(), ServiceSecretSpec{
		Service: "anthropic",
		Scope:   "box",
		Ref:     "op://Private/Anthropic/api-key",
		Refresh: "30m",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	call := lastCall(t, logPath)
	want := "secret set anthropic --sandbox box --ref op://Private/Anthropic/api-key --refresh 30m"
	if call != want {
		t.Fatalf("want %q, got %q", want, call)
	}
	if raw, _ := os.ReadFile(stdinPath); len(strings.TrimSpace(string(raw))) != 0 {
		t.Fatalf("expected no stdin, got %q", string(raw))
	}
}

func TestSetServiceSecretOverwritePrecheck(t *testing.T) {
	logPath, _ := newSecretStub(t, secretListFixture)
	client := New("/nonexistent.sock")

	// github exists globally in the fixture, so this must be rejected before any write.
	err := client.SetServiceSecret(context.Background(), ServiceSecretSpec{Service: "github", Value: "x"})
	if err == nil {
		t.Fatal("expected an already-exists error")
	}
	for _, call := range stubCalls(t, logPath) {
		if strings.HasPrefix(call, "secret set") {
			t.Fatalf("CLI should not have been invoked to set: %q", call)
		}
	}
}

func TestSetRegistrySecretScopes(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		want  string
	}{
		{"host-only", SecretScopeHostOnly, "secret set --registry ghcr.io --username u --password-stdin"},
		{"global", "", "secret set --registry ghcr.io --username u --password-stdin --all-sandboxes"},
		{"sandbox", "box", "secret set --registry ghcr.io --username u --password-stdin --sandbox box"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logPath, stdinPath := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
			err := New("/nonexistent.sock").SetRegistrySecret(context.Background(), RegistrySecretSpec{
				Host: "ghcr.io", Username: "u", Password: "pw", Scope: tc.scope,
			})
			if err != nil {
				t.Fatalf("set registry: %v", err)
			}
			if got := lastCall(t, logPath); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
			if raw, _ := os.ReadFile(stdinPath); strings.TrimSpace(string(raw)) != "pw" {
				t.Fatalf("expected password on stdin, got %q", string(raw))
			}
		})
	}
}

func TestRemoveSecretArgs(t *testing.T) {
	logPath, _ := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
	if err := New("/nonexistent.sock").RemoveSecret(context.Background(), "box", "github"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := lastCall(t, logPath); got != "secret rm --sandbox box github -f" {
		t.Fatalf("unexpected argv: %q", got)
	}
}

func TestSetCustomSecretValueGoesInArgv(t *testing.T) {
	logPath, _ := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
	err := New("/nonexistent.sock").SetCustomSecret(context.Background(), CustomSecretSpec{
		Hosts: []string{"*.example.com"}, Env: "API_KEY", Value: "literal-val",
	})
	if err != nil {
		t.Fatalf("set custom: %v", err)
	}
	// Documented limitation: set-custom has no stdin path.
	call := lastCall(t, logPath)
	if call != "secret set-custom --host *.example.com --env API_KEY --value literal-val" {
		t.Fatalf("unexpected argv: %q", call)
	}
}

func TestRemoveRegistrySecretScopes(t *testing.T) {
	logPath, _ := newSecretStub(t, `{"secrets":[],"custom_secrets":[]}`)
	client := New("/nonexistent.sock")

	if err := client.RemoveRegistrySecret(context.Background(), SecretScopeHostOnly, "ghcr.io"); err != nil {
		t.Fatalf("host-only: %v", err)
	}
	if got := lastCall(t, logPath); got != "secret rm --registry ghcr.io -f" {
		t.Fatalf("unexpected host-only argv: %q", got)
	}

	if err := client.RemoveRegistrySecret(context.Background(), "", "ghcr.io"); err != nil {
		t.Fatalf("global: %v", err)
	}
	if got := lastCall(t, logPath); got != "secret rm --registry ghcr.io --all-sandboxes -f" {
		t.Fatalf("unexpected global argv: %q", got)
	}

	if err := client.RemoveRegistrySecret(context.Background(), "box", "ghcr.io"); err != nil {
		t.Fatalf("scoped: %v", err)
	}
	if got := lastCall(t, logPath); got != "secret rm --registry ghcr.io --sandbox box -f" {
		t.Fatalf("unexpected scoped argv: %q", got)
	}
}
