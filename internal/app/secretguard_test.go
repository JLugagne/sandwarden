package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/sbx"
)

func TestCreateSandboxRefusesSecretEnvValues(t *testing.T) {
	newSkillsStub(t)
	a, _ := newTestApp(t)

	var out bytes.Buffer
	err := a.CreateSandbox(context.Background(), CreateRequest{Opts: sbx.CreateOptions{
		Name:  "box",
		Agent: "claude",
		Env:   []string{"ANTHROPIC_API_KEY=sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII"},
	}}, &out)
	if err == nil {
		t.Fatalf("CreateSandbox accepted a secret-looking env value; output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("error does not name the variable: %v", err)
	}
	if !strings.Contains(err.Error(), "sbx secret") {
		t.Fatalf("error does not point to the secrets feature: %v", err)
	}
	if _, ok := a.Fleet.SandboxByName("box"); ok {
		t.Fatalf("a config was written despite the guard; spec:\n%s", out.String())
	}
}

func TestSecretReason(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"boolean", "true", false},
		{"log level", "debug", false},
		{"memory size", "8g", false},
		{"path", "/usr/local/go", false},
		{"url", "https://proxy:8080", false},
		{"database url", "postgres://user:pass@db:5432/app", false},
		{"hostname", "some.long.domain.name.example.com", false},
		{"underscored phrase", "this_is_a_long_value_with_underscores", false},
		{"uuid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"short token", "abc123", false},
		{"openai key", "sk-proj-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH", true},
		{"anthropic key", "sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH", true},
		{"quoted anthropic key", "\"sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH\"", true},
		{"github pat", "ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII", true},
		{"github oauth", "gho_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII", true},
		{"slack bot", "xoxb-" + "123456789012-abcdefghijklmnop", true},
		{"slack user", "xoxp-" + "123456789012-abcdefghijklmnop", true},
		{"aws access key id", "AKIAIOSFODNN7EXAMPLE", true},
		{"google api key", "AIzaSyA1234567890abcdefghijklmnopqrst", true},
		{"gitlab pat", "glpat-xxxxxxxxxxxxxxxxxxxx", true},
		{"pem private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA", true},
		{"random hex token", "9f8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a", true},
		{"random mixed token", "aB3dE5fG7hI9jK1lM3nO5pQ7rS9tU1vW3xY5zA7bC9dE1fG3h", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			reason := SecretReason(tt.value)
			if got := reason != ""; got != tt.want {
				t.Fatalf("SecretReason(%q) = %q, want secret=%v", tt.value, reason, tt.want)
			}
		})
	}
}

func TestCreateSandboxAllowsBareAndPlainEnvValues(t *testing.T) {
	newSkillsStub(t)
	a, _ := newTestApp(t)

	var out bytes.Buffer
	err := a.CreateSandbox(context.Background(), CreateRequest{Opts: sbx.CreateOptions{
		Name:  "box",
		Agent: "claude",
		Env:   []string{"ANTHROPIC_API_KEY", "NODE_ENV=development"},
	}}, &out)
	if err != nil {
		t.Fatalf("CreateSandbox refused legitimate env values: %v", err)
	}
	cfg, ok := a.Fleet.SandboxByName("box")
	if !ok {
		t.Fatalf("config was not written; output:\n%s", out.String())
	}
	if value, ok := cfg.Spec.Env()["ANTHROPIC_API_KEY"]; !ok || value != "" {
		t.Fatalf("bare KEY was not written as an empty value: %#v", cfg.Spec.Env())
	}
	if got := cfg.Spec.Env()["NODE_ENV"]; got != "development" {
		t.Fatalf("plain value changed: %q", got)
	}
}

func TestCreateSandboxKeepsExistingConfigSecretValues(t *testing.T) {
	newSkillsStub(t)
	a, _ := newTestApp(t)

	const value = "sk-ant-api03-AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH"
	spec := fleet.NewMixin("")
	spec.DisplayName = "box"
	spec.Requires = &fleet.SpecRequires{Agent: "claude"}
	spec.Environment = &fleet.SpecEnv{Variables: map[string]string{"ANTHROPIC_API_KEY": value}}
	existing, err := a.Fleet.CreateSandbox("box", spec, fleet.SandboxApp{Sandbox: "box"})
	if err != nil {
		t.Fatalf("seed config: %v", err)
	}

	req := CreateRequest{Opts: a.CreateOptionsFromConfig(existing)}
	if len(req.Opts.Env) == 0 {
		t.Fatal("seed config did not produce env options")
	}
	var out bytes.Buffer
	if err := a.CreateSandbox(context.Background(), req, &out); err != nil {
		t.Fatalf("CreateSandbox blocked an existing hand-written config: %v", err)
	}
}
