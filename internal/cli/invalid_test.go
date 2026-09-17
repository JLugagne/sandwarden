package cli

import "testing"

func TestInvalidInputFails(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantErr    string
		wantStderr string
	}{
		{
			name:    "run without sandbox or config",
			args:    []string{"run", "ghost"},
			wantErr: `sandbox "ghost" does not exist and has no config directory`,
		},
		{
			name:    "start without a sandbox name",
			args:    []string{"start"},
			wantErr: "requires at least 1 arg",
		},
		{
			name:    "apply without any config",
			args:    []string{"apply"},
			wantErr: "no sandbox configuration found",
		},
		{
			name:       "apply on an unknown sandbox",
			args:       []string{"apply", "ghost"},
			wantErr:    "1 sandbox(es) reported errors",
			wantStderr: `no configuration found for sandbox "ghost"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCLIEnv(t)
			_, stderr, err := env.execute(t, tc.args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			mustContain(t, err.Error(), tc.wantErr)
			if tc.wantStderr != "" {
				mustContain(t, stderr, tc.wantStderr)
			}
		})
	}
}
