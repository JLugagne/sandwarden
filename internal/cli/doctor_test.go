package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const doctorSecret = "sk-ant-api03-BBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII"

func writeDoctorFixture(t *testing.T, env *cliEnv, slug, spec, sidecar string) {
	t.Helper()
	dir := filepath.Join(env.opts.ConfigDir, "sandboxes", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", slug, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec %s: %v", slug, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sandwarden.yaml"), []byte(sidecar), 0o644); err != nil {
		t.Fatalf("write sidecar %s: %v", slug, err)
	}
}

func TestDoctorReportsSecretEnvValuesWithoutPrintingThem(t *testing.T) {
	env := newCLIEnv(t)
	writeDoctorFixture(t, env, "leaky", `schemaVersion: "2"
kind: mixin
name: leaky
environment:
  variables:
    ANTHROPIC_API_KEY: `+doctorSecret+`
    LOG_LEVEL: debug
`, "sandbox: leaky\n")
	writeDoctorFixture(t, env, "clean", `schemaVersion: "2"
kind: mixin
name: clean
environment:
  variables:
    NODE_ENV: development
    PROXY: https://proxy:8080
`, "sandbox: clean\n")

	stdout, _, err := env.execute(t, "doctor")
	if err == nil {
		t.Fatalf("doctor exited zero despite a secret-looking value:\n%s", stdout)
	}
	mustContain(t, stdout, "sandbox leaky: spec.yaml:6: ANTHROPIC_API_KEY looks like a secret")
	if strings.Contains(stdout, doctorSecret) {
		t.Fatalf("doctor printed the secret value:\n%s", stdout)
	}
	if strings.Contains(stdout, "clean:") {
		t.Fatalf("doctor flagged a clean sandbox:\n%s", stdout)
	}
}

func TestDoctorExitsZeroWhenNoSecretsFound(t *testing.T) {
	env := newCLIEnv(t)
	writeDoctorFixture(t, env, "clean", `schemaVersion: "2"
kind: mixin
name: clean
environment:
  variables:
    NODE_ENV: development
`, "sandbox: clean\n")

	stdout, _, err := env.execute(t, "doctor")
	if err != nil {
		t.Fatalf("doctor failed on a clean fleet: %v\n%s", err, stdout)
	}
	mustContain(t, stdout, "no likely secrets found")
}
