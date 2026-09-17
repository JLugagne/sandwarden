package cli

import (
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestRunCreatesOrStartsSandbox(t *testing.T) {
	cases := []struct {
		name       string
		running    []string
		wantCreate bool
		wantStart  bool
		wantOut    string
	}{
		{"creates from config when the sandbox is missing", nil, true, false, "config: saved"},
		{"starts and applies when the sandbox exists", []string{"box"}, false, true, "apply: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCLIEnv(t, tc.running...)
			fl := env.fleet(t)
			mustCreateSandbox(t, fl, "box", fleet.SandboxApp{})

			stdout, _, err := env.execute(t, "run", "box", "--", "--verbose")
			if err != nil {
				t.Fatalf("run: %v\n%s", err, stdout)
			}
			mustContain(t, stdout, tc.wantOut)
			env.requireSbxCall(t, "run --name box -- --verbose")
			if tc.wantCreate {
				env.requireSbxCall(t, "create claude --name box")
			} else {
				env.requireNoSbxCall(t, "create ")
			}
			if got := env.daemon.called("POST", "/sandbox/box/start"); got != tc.wantStart {
				t.Errorf("start called = %v, want %v", got, tc.wantStart)
			}
		})
	}
}
