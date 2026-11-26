package sbx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// newKitSbx writes a fake sbx binary that records every invocation and refuses
// `kit add` with refusal when it is non-empty.
func newKitSbx(t *testing.T, refusal string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_SBX_LOG"
if [ "$1" = "kit" ] && [ "$2" = "add" ] && [ -n "$FAKE_SBX_REFUSAL" ]; then
  printf '%s\n' "$FAKE_SBX_REFUSAL" >&2
  exit 1
fi
exit 0
`
	bin := filepath.Join(dir, "sbx")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("FAKE_SBX_LOG", logPath)
	t.Setenv("FAKE_SBX_REFUSAL", refusal)
	return logPath
}

func TestKitAddArgs(t *testing.T) {
	logPath := newKitSbx(t, "")
	if _, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", nil, nil); err != nil {
		t.Fatalf("kit add: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := string(raw), "kit add box ./mcp-postgres\n"; got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestKitAddPassesKitArgs(t *testing.T) {
	logPath := newKitSbx(t, "")
	kitArgs := []string{"repo=acme/app", "", "mcp.strict=true"}
	if _, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", kitArgs, nil); err != nil {
		t.Fatalf("kit add: %v", err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := string(raw), "kit add box ./mcp-postgres --kit-arg repo=acme/app --kit-arg mcp.strict=true\n"; got != want {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestKitAddRejectsMalformedKitArg(t *testing.T) {
	newKitSbx(t, "")
	if _, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", []string{"repo"}, nil); err == nil {
		t.Fatal("expected an error for a kit argument without =")
	}
}

func TestKitAddSurfacesRefusalVerbatim(t *testing.T) {
	refusal := "ERROR: sandbox 'box' was created before the kit-add recreate feature shipped and cannot be recreated safely"
	newKitSbx(t, refusal)
	_, err := New("").KitAdd(context.Background(), "box", "./mcp-postgres", nil, nil)
	if err == nil || err.Error() != refusal {
		t.Fatalf("err = %v, want the refusal verbatim: %q", err, refusal)
	}
}

func TestKitAddValidatesInput(t *testing.T) {
	newKitSbx(t, "")
	if _, err := New("").KitAdd(context.Background(), "  ", "./mcp-postgres", nil, nil); err == nil {
		t.Fatal("expected an error for an empty sandbox name")
	}
	if _, err := New("").KitAdd(context.Background(), "box", "  ", nil, nil); err == nil {
		t.Fatal("expected an error for an empty kit reference")
	}
}

func TestKitArgumentUnmarshal(t *testing.T) {
	cases := []struct {
		json     string
		desc     string
		required bool
		hasDef   bool
		def      string
	}{
		{
			json:     `"Simple description string"`,
			desc:     "Simple description string",
			required: false,
			hasDef:   false,
		},
		{
			json:     `{"description": "Required arg", "required": true}`,
			desc:     "Required arg",
			required: true,
			hasDef:   false,
		},
		{
			json:     `{"description": "With default", "default": ""}`,
			desc:     "With default",
			required: false,
			hasDef:   true,
			def:      "",
		},
		{
			json:     `{"description": "With bool default", "default": true}`,
			desc:     "With bool default",
			required: false,
			hasDef:   true,
			def:      "true",
		},
		{
			json:     `{"description": "With integer default", "default": 1000000}`,
			desc:     "With integer default",
			required: false,
			hasDef:   true,
			def:      "1000000",
		},
		{
			json:     `{"description": "With float default", "default": 1.5}`,
			desc:     "With float default",
			required: false,
			hasDef:   true,
			def:      "1.5",
		},
		{
			json:     `{"description": "With string default", "default": "x"}`,
			desc:     "With string default",
			required: false,
			hasDef:   true,
			def:      "x",
		},
		{
			json:     `{"description": "With null default", "default": null}`,
			desc:     "With null default",
			required: false,
			hasDef:   false,
		},
	}

	for _, tc := range cases {
		var arg KitArgument
		if err := json.Unmarshal([]byte(tc.json), &arg); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.json, err)
		}
		if arg.Description != tc.desc {
			t.Errorf("desc = %q, want %q", arg.Description, tc.desc)
		}
		if arg.Required != tc.required {
			t.Errorf("required = %v, want %v", arg.Required, tc.required)
		}
		if tc.hasDef {
			if arg.Default == nil || *arg.Default != tc.def {
				t.Errorf("default = %v, want %q", arg.Default, tc.def)
			}
		} else if arg.Default != nil {
			t.Errorf("expected nil default, got %q", *arg.Default)
		}
	}
}

func TestKitCommandUnmarshal(t *testing.T) {
	cases := []struct {
		json string
		want string
	}{
		{`"npm install"`, "npm install"},
		{`["apt-get", "update"]`, "apt-get update"},
		{`{"command": "make build"}`, "make build"},
		{`{"command": ["cargo", "build"]}`, "cargo build"},
	}

	for _, tc := range cases {
		var cmd KitCommand
		if err := json.Unmarshal([]byte(tc.json), &cmd); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.json, err)
		}
		if cmd.Command != tc.want {
			t.Errorf("command = %q, want %q", cmd.Command, tc.want)
		}
	}
}
