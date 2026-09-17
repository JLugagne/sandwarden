package cli

import (
	"os"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
)

func TestRmKeepsOrPurgesConfig(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		purge   bool
		wantOut string
	}{
		{"keeps the config directory by default", []string{"rm", "box"}, false, "box: removed (config kept)"},
		{"purges the config directory with --purge", []string{"rm", "box", "--purge"}, true, "box: removed (config purged)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCLIEnv(t, "box")
			fl := env.fleet(t)
			cfg := mustCreateSandbox(t, fl, "box", fleet.SandboxApp{})

			stdout, _, err := env.execute(t, tc.args...)
			if err != nil {
				t.Fatalf("rm: %v\n%s", err, stdout)
			}
			mustContain(t, stdout, tc.wantOut)
			if !env.daemon.called("DELETE", "/sandbox/box") {
				t.Error("expected the daemon to be asked to delete box")
			}
			_, statErr := os.Stat(cfg.Dir)
			if tc.purge && statErr == nil {
				t.Errorf("config directory %s should have been purged", cfg.Dir)
			}
			if !tc.purge && statErr != nil {
				t.Errorf("config directory %s should have been kept: %v", cfg.Dir, statErr)
			}
		})
	}
}
