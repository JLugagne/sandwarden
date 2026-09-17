package cli

import "testing"

func TestSbxPassthroughForwardsArguments(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			"forwards flags verbatim without parsing them",
			[]string{"sbx", "--version", "--config", "/nope", "-x"},
			"--version --config /nope -x",
		},
		{
			"forwards a run command and its own flags",
			[]string{"sbx", "run", "--name", "box", "--", "ls", "-la"},
			"run --name box -- ls -la",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCLIEnv(t)
			stdout, stderr, err := env.execute(t, tc.args...)
			if err != nil {
				t.Fatalf("sbx: %v\n%s%s", err, stdout, stderr)
			}
			env.requireSbxCall(t, tc.want)
		})
	}
}
