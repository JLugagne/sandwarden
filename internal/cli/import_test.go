package cli

import (
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestImportAdoptsDaemonSandboxes(t *testing.T) {
	env := newCLIEnv(t, "box-a", "box-b")

	stdout, stderr, err := env.execute(t, "import")
	if err != nil {
		t.Fatalf("import: %v (stderr: %s)", err, stderr)
	}
	mustContain(t, stdout, "box-a: created config (incomplete: CPU, memory and env were not recoverable)")
	mustContain(t, stdout, "box-b: created config (incomplete: CPU, memory and env were not recoverable)")
	mustContain(t, stdout, "import: 2 created, 0 already configured, 0 failed")

	fl := env.fleet(t)
	for _, name := range []string{"box-a", "box-b"} {
		cfg, ok := fl.SandboxByName(name)
		if !ok {
			t.Fatalf("config not written for %s", name)
		}
		if cfg.App.Create == nil || !cfg.App.Create.Incomplete {
			t.Fatalf("config for %s is not incomplete: %+v", name, cfg.App.Create)
		}
	}
}

func TestImportKeepsConfiguredSandboxes(t *testing.T) {
	env := newCLIEnv(t, "box-a", "box-b")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box-a", fleet.SandboxApp{Create: &fleet.SandboxCreate{CPUs: 4, Memory: "8g"}})

	stdout, _, err := env.execute(t, "import")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	mustContain(t, stdout, "box-a: already configured")
	mustContain(t, stdout, "box-b: created config (incomplete")
	mustContain(t, stdout, "import: 1 created, 1 already configured, 0 failed")
}

func TestImportReportsFailureWithoutAborting(t *testing.T) {
	env := newCLIEnv(t, "box")

	stdout, _, err := env.execute(t, "import", "ghost", "box")
	if err == nil {
		t.Fatal("expected import to fail when a named sandbox is not in the daemon")
	}
	mustContain(t, stdout, "ghost: failed: not in daemon")
	mustContain(t, stdout, "box: created config")
	mustContain(t, stdout, "import: 1 created, 0 already configured, 1 failed")
}

func TestImportJSONReportsOutcomes(t *testing.T) {
	env := newCLIEnv(t, "box-a", "box-b")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box-a", fleet.SandboxApp{Create: &fleet.SandboxCreate{CPUs: 4, Memory: "8g"}})

	stdout, _, err := env.execute(t, "import", "--json")
	if err != nil {
		t.Fatalf("import --json: %v", err)
	}
	doc := mustDecodeJSON(t, stdout)
	requireJSONKeys(t, doc, "schemaVersion", "created", "alreadyConfigured", "failed", "results")
	if doc["created"] != float64(1) || doc["alreadyConfigured"] != float64(1) || doc["failed"] != float64(0) {
		t.Fatalf("unexpected counts: %v", doc)
	}
	results := jsonTestArray(t, doc, "results")
	if len(results) != 2 {
		t.Fatalf("results = %v, want 2 entries", results)
	}
	for _, res := range results {
		requireJSONKeys(t, res, "name", "status", "incomplete")
		switch res["name"] {
		case "box-a":
			if res["status"] != "already configured" || res["incomplete"] != false {
				t.Errorf("box-a result = %v", res)
			}
		case "box-b":
			if res["status"] != "created" || res["incomplete"] != true {
				t.Errorf("box-b result = %v", res)
			}
		default:
			t.Errorf("unexpected result %v", res)
		}
	}
}

func TestImportAfterPurgeMarksIncomplete(t *testing.T) {
	env := newCLIEnv(t, "box")
	fl := env.fleet(t)
	mustCreateSandbox(t, fl, "box", fleet.SandboxApp{Create: &fleet.SandboxCreate{CPUs: 2, Memory: "2g", Workspaces: []string{"/w"}}})

	if _, _, err := env.execute(t, "rm", "--purge", "box"); err != nil {
		t.Fatalf("rm --purge: %v", err)
	}
	if _, ok := env.fleet(t).SandboxByName("box"); ok {
		t.Fatal("rm --purge left the config directory behind")
	}
	// The sandbox is recreated outside sandwarden: only the daemon knows it.
	env.daemon.mu.Lock()
	env.daemon.sandboxes["box"] = true
	env.daemon.mu.Unlock()

	stdout, _, err := env.execute(t, "import", "box")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	mustContain(t, stdout, "box: created config (incomplete")

	statusOut, _, err := env.execute(t, "status", "box")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	mustContain(t, statusOut, "config=incomplete")
	mustContain(t, statusOut, "recreate will not restore")

	recreateOut, _, _ := env.execute(t, "recreate", "box")
	mustContain(t, recreateOut, "will not restore")
}
