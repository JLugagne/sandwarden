package sbx

import (
	"slices"
	"testing"
)

func TestCreateOptionsArgs(t *testing.T) {
	cases := []struct {
		name string
		opts CreateOptions
		want []string
	}{
		{
			name: "agent only",
			opts: CreateOptions{Agent: "claude"},
			want: []string{"create", "claude"},
		},
		{
			name: "kit arguments and files",
			opts: CreateOptions{
				Agent:        "claude",
				Kits:         []string{"./mixin"},
				KitArgs:      []string{"repo=acme/app", "linter.strict=true"},
				KitArgsFiles: []string{"/tmp/kit-args", "/tmp/more-args"},
			},
			want: []string{
				"create", "claude",
				"--kit", "./mixin",
				"--kit-arg", "repo=acme/app",
				"--kit-arg", "linter.strict=true",
				"--kit-args-file", "/tmp/kit-args",
				"--kit-args-file", "/tmp/more-args",
			},
		},
		{
			name: "env files, skills and static MCP",
			opts: CreateOptions{
				Agent:     "claude",
				EnvFiles:  []string{"/tmp/api.env"},
				Skills:    "readonly",
				StaticMCP: []string{"notion,atlassian", "none"},
			},
			want: []string{
				"create", "claude",
				"--skills", "readonly",
				"--static-mcp", "notion,atlassian",
				"--static-mcp", "none",
				"--env-file", "/tmp/api.env",
			},
		},
		{
			name: "blank entries are skipped",
			opts: CreateOptions{
				Agent:        "claude",
				KitArgs:      []string{"", "   ", "repo=acme/app"},
				KitArgsFiles: []string{"", "/tmp/kit-args"},
				EnvFiles:     []string{"  "},
				Skills:       "  ",
				StaticMCP:    []string{"", "notion"},
			},
			want: []string{
				"create", "claude",
				"--kit-arg", "repo=acme/app",
				"--kit-args-file", "/tmp/kit-args",
				"--static-mcp", "notion",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.opts.Args()
			if err != nil {
				t.Fatalf("Args() error: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Args() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateOptionsArgsRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		opts CreateOptions
	}{
		{name: "kit argument without equals", opts: CreateOptions{Agent: "claude", KitArgs: []string{"repo"}}},
		{name: "unknown skills mode", opts: CreateOptions{Agent: "claude", Skills: "write"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.opts.Args(); err == nil {
				t.Fatal("Args() accepted an invalid value")
			}
		})
	}
}

func TestCreateOptionsArgsRequiresAgent(t *testing.T) {
	_, err := (CreateOptions{}).Args()
	if err == nil {
		t.Fatal("Args() accepted a missing agent")
	}
}
