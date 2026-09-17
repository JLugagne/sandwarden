package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func mustDecodeJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	return out
}

func jsonTestArray(t *testing.T, doc map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := doc[key].([]any)
	if !ok {
		t.Fatalf("%q is not a JSON array in %v", key, doc)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%q item is not a JSON object: %v", key, item)
		}
		out = append(out, obj)
	}
	return out
}

func jsonTestSandbox(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	for _, item := range jsonTestArray(t, doc, "sandboxes") {
		if item["name"] == name {
			return item
		}
	}
	t.Fatalf("sandbox %q not found in %v", name, doc)
	return nil
}

func jsonTestKeys(t *testing.T, obj map[string]any) []string {
	t.Helper()
	out := make([]string, 0, len(obj))
	for key := range obj {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func requireJSONKeys(t *testing.T, obj map[string]any, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := jsonTestKeys(t, obj); !slices.Equal(got, want) {
		t.Errorf("JSON keys = %v, want %v", got, want)
	}
}

func writeBrokenConfig(t *testing.T, env *cliEnv, fl *fleet.Fleet, name string) {
	t.Helper()
	mustCreateSandbox(t, fl, name, fleet.SandboxApp{})
	path := filepath.Join(env.opts.ConfigDir, "sandboxes", name, "sandwarden.yaml")
	if err := os.WriteFile(path, []byte("sandbox: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("break config: %v", err)
	}
}

func TestLsJSONReportsConfigAndDaemonState(t *testing.T) {
	env := newCLIEnv(t, "alpha", "beta")
	fl := env.fleet(t)
	profile := mustCreateProfile(t, fl, "dev", []string{"example.com"}, fleet.ProfileApp{})
	cache, err := fl.CreateCache("go-mod", fleet.CacheApp{
		Name:       "go-mod",
		HostPath:   t.TempDir(),
		TargetPath: "/home/agent/go/pkg/mod",
		Enabled:    boolPtr(true),
	})
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	skill, _ := seedSkillStore(t, env, fl, "internal-skills", "pdf")
	mustCreateSandbox(t, fl, "alpha", fleet.SandboxApp{
		Profiles: []string{profile.Slug},
		Caches:   []string{cache.Slug},
		Skills:   []fleet.SkillRef{skill},
		Mounts:   []fleet.MountRef{{HostPath: t.TempDir(), TargetPath: "/src", ReadOnly: true}},
	})
	mustCreateSandbox(t, fl, "beta", fleet.SandboxApp{Create: &fleet.SandboxCreate{Incomplete: true}})

	stdout, stderr, err := env.execute(t, "ls", "--json")
	if err != nil {
		t.Fatalf("ls --json: %v (stderr: %s)", err, stderr)
	}
	doc := mustDecodeJSON(t, stdout)
	if doc["schemaVersion"] != float64(jsonSchemaVersion) {
		t.Errorf("schemaVersion = %v, want %d", doc["schemaVersion"], jsonSchemaVersion)
	}
	if got := jsonTestArray(t, doc, "configErrors"); len(got) != 0 {
		t.Errorf("configErrors = %v, want empty", got)
	}

	alpha := jsonTestSandbox(t, doc, "alpha")
	if alpha["status"] != "running" || alpha["running"] != true {
		t.Errorf("alpha status = %v/%v, want running/true", alpha["status"], alpha["running"])
	}
	if alpha["configSlug"] != "alpha" {
		t.Errorf("alpha configSlug = %v, want alpha", alpha["configSlug"])
	}
	if want := filepath.Join(env.opts.ConfigDir, "sandboxes", "alpha"); alpha["configPath"] != want {
		t.Errorf("alpha configPath = %v, want %s", alpha["configPath"], want)
	}
	if got, ok := alpha["profiles"].([]any); !ok || len(got) != 1 || got[0] != profile.Slug {
		t.Errorf("alpha profiles = %v, want [%s]", alpha["profiles"], profile.Slug)
	}
	for key, want := range map[string]float64{"caches": 1, "skills": 1, "mounts": 1} {
		if _, ok := alpha[key].(float64); !ok {
			t.Errorf("alpha %s = %v (%T), want a JSON number", key, alpha[key], alpha[key])
		}
		if alpha[key] != want {
			t.Errorf("alpha %s = %v, want %v", key, alpha[key], want)
		}
	}
	if alpha["incomplete"] != false {
		t.Errorf("alpha incomplete = %v, want false", alpha["incomplete"])
	}

	beta := jsonTestSandbox(t, doc, "beta")
	if beta["incomplete"] != true {
		t.Errorf("beta incomplete = %v, want true", beta["incomplete"])
	}
	if got, ok := beta["profiles"].([]any); !ok || len(got) != 0 {
		t.Errorf("beta profiles = %v, want []", beta["profiles"])
	}
	for _, key := range []string{"caches", "skills", "mounts"} {
		if beta[key] != float64(0) {
			t.Errorf("beta %s = %v, want 0", key, beta[key])
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestLsJSONWithUnreachableDaemon(t *testing.T) {
	env := newCLIEnv(t)
	env.opts.Socket = filepath.Join(t.TempDir(), "missing.sock")

	stdout, stderr, err := env.execute(t, "ls", "--json")
	if err == nil {
		t.Fatal("expected ls --json to fail when the daemon is unreachable")
	}
	doc := mustDecodeJSON(t, stdout)
	if got := jsonTestArray(t, doc, "sandboxes"); len(got) != 0 {
		t.Errorf("sandboxes = %v, want empty", got)
	}
	jsonTestArray(t, doc, "configErrors")
	mustContain(t, stderr, "warning:")
	mustContain(t, stderr, err.Error())
}

func TestStatusJSONReportsConfigAndDaemonErrors(t *testing.T) {
	env := newCLIEnv(t, "box")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{})
	mustCreateSandbox(t, fl, "ghost", fleet.SandboxApp{})
	writeBrokenConfig(t, env, fl, "broken")

	stdout, stderr, err := env.execute(t, "status", "--json")
	if err == nil {
		t.Fatal("expected status --json to fail for a sandbox missing from the daemon")
	}
	doc := mustDecodeJSON(t, stdout)
	if doc["configDir"] != env.opts.ConfigDir {
		t.Errorf("configDir = %v, want %s", doc["configDir"], env.opts.ConfigDir)
	}
	configErrors := jsonTestArray(t, doc, "configErrors")
	if len(configErrors) != 1 {
		t.Fatalf("configErrors = %v, want exactly 1 entry", configErrors)
	}
	if path := configErrors[0]["path"].(string); !strings.HasSuffix(path, filepath.Join("sandboxes", "broken", "sandwarden.yaml")) {
		t.Errorf("configErrors path = %v", path)
	}
	if configErrors[0]["error"] == "" {
		t.Error("configErrors error is empty")
	}

	box := jsonTestSandbox(t, doc, "box")
	if box["status"] != "running" || box["running"] != true {
		t.Errorf("box status = %v/%v, want running/true", box["status"], box["running"])
	}
	if _, ok := box["detail"].(map[string]any); !ok {
		t.Errorf("box detail = %v, want a JSON object", box["detail"])
	}

	ghost := jsonTestSandbox(t, doc, "ghost")
	errors, ok := ghost["errors"].([]any)
	if !ok || len(errors) != 1 || !strings.Contains(errors[0].(string), "not in daemon") {
		t.Errorf("ghost errors = %v, want one not-in-daemon error", ghost["errors"])
	}
	if _, ok := ghost["detail"]; ok {
		t.Errorf("ghost detail = %v, want it omitted", ghost["detail"])
	}
	if ghost["configSlug"] != "ghost" || ghost["incomplete"] != false {
		t.Errorf("ghost config fields = %v/%v", ghost["configSlug"], ghost["incomplete"])
	}
	mustContain(t, stderr, "not found in daemon")
}

func TestJSONSchemaKeysArePinned(t *testing.T) {
	lsSandboxKeys := []string{"caches", "configPath", "configSlug", "id", "incomplete", "mounts", "name", "profiles", "running", "skills", "status"}
	statusSandboxKeys := append(append([]string{}, lsSandboxKeys...), "detail", "errors")
	slices.Sort(lsSandboxKeys)
	slices.Sort(statusSandboxKeys)

	env := newCLIEnv(t, "box")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{})
	mustCreateSandbox(t, fl, "ghost", fleet.SandboxApp{})
	writeBrokenConfig(t, env, fl, "broken")

	lsStdout, _, err := env.execute(t, "ls", "--json")
	if err != nil {
		t.Fatalf("ls --json: %v", err)
	}
	lsDoc := mustDecodeJSON(t, lsStdout)
	requireJSONKeys(t, lsDoc, "schemaVersion", "sandboxes", "configErrors")
	if lsDoc["schemaVersion"] != float64(1) {
		t.Errorf("schemaVersion = %v, want 1", lsDoc["schemaVersion"])
	}
	requireJSONKeys(t, jsonTestSandbox(t, lsDoc, "box"), lsSandboxKeys...)
	lsConfigErrors := jsonTestArray(t, lsDoc, "configErrors")
	if len(lsConfigErrors) != 1 {
		t.Fatalf("configErrors = %v, want exactly 1 entry", lsConfigErrors)
	}
	requireJSONKeys(t, lsConfigErrors[0], "path", "error")

	statusStdout, _, statusErr := env.execute(t, "status", "--json")
	if statusErr == nil {
		t.Fatal("expected status --json to fail for ghost")
	}
	statusDoc := mustDecodeJSON(t, statusStdout)
	requireJSONKeys(t, statusDoc, "schemaVersion", "configDir", "sandboxes", "configErrors")
	requireJSONKeys(t, jsonTestSandbox(t, statusDoc, "box"), statusSandboxKeys...)
}

func TestApplyJSONReports(t *testing.T) {
	env := newCLIEnv(t, "box")
	fl := env.fleet(t)
	profile := mustCreateProfile(t, fl, "dev", []string{"example.com"}, fleet.ProfileApp{})
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{Profiles: []string{profile.Slug}})
	mustCreateSandbox(t, fl, "ghost", fleet.SandboxApp{})

	stdout, stderr, err := env.execute(t, "apply", "--json")
	if err == nil {
		t.Fatal("expected apply --json to fail for a sandbox missing from the daemon")
	}
	doc := mustDecodeJSON(t, stdout)
	requireJSONKeys(t, doc, "schemaVersion", "reports")
	reports := jsonTestArray(t, doc, "reports")
	if len(reports) != 2 {
		t.Fatalf("reports = %v, want 2 entries", reports)
	}
	for _, report := range reports {
		switch report["name"] {
		case "box":
			requireJSONKeys(t, report, "name", "report")
			applied := report["report"].(map[string]any)
			if applied["rules_applied"] != float64(1) {
				t.Errorf("box rules_applied = %v, want 1", applied["rules_applied"])
			}
			for _, key := range []string{"warnings", "errors"} {
				if _, ok := applied[key].([]any); !ok {
					t.Errorf("box report %s = %v, want a JSON array", key, applied[key])
				}
			}
		case "ghost":
			requireJSONKeys(t, report, "name", "error")
			if report["error"] == "" {
				t.Error("ghost error is empty")
			}
		default:
			t.Errorf("unexpected report %v", report)
		}
	}
	mustContain(t, stderr, "reported errors")
}

func TestStartJSONReports(t *testing.T) {
	env := newCLIEnv(t, "box")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{})

	stdout, stderr, err := env.execute(t, "start", "box", "--json")
	if err != nil {
		t.Fatalf("start --json: %v (stderr: %s)", err, stderr)
	}
	doc := mustDecodeJSON(t, stdout)
	requireJSONKeys(t, doc, "schemaVersion", "sandboxes")
	sandboxes := jsonTestArray(t, doc, "sandboxes")
	if len(sandboxes) != 1 {
		t.Fatalf("sandboxes = %v, want 1 entry", sandboxes)
	}
	requireJSONKeys(t, sandboxes[0], "name", "started", "report")
	if sandboxes[0]["name"] != "box" || sandboxes[0]["started"] != true {
		t.Errorf("start result = %v, want box/true", sandboxes[0])
	}
	if _, ok := sandboxes[0]["report"].(map[string]any); !ok {
		t.Errorf("start report = %v, want a JSON object", sandboxes[0]["report"])
	}
}
