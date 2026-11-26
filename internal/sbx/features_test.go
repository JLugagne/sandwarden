package sbx

import (
	"slices"
	"strings"
	"testing"
)

// featureIDs is a test helper: the stable ID of each required feature.
func featureIDs(fs []Feature) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

func TestCreateOptionsRequiredFeatures(t *testing.T) {
	tests := []struct {
		name string
		opts CreateOptions
		want []string
	}{
		{name: "plain create", opts: CreateOptions{Agent: "claude", Workspaces: []string{"/host/src"}}},
		{name: "read-only workspace has no target", opts: CreateOptions{Workspaces: []string{"/host/src:ro"}}},
		{
			name: "workspace target",
			opts: CreateOptions{Workspaces: []string{"/host/src:/srv/src"}},
			want: []string{FeatureCreateWorkspaces},
		},
		{
			name: "read-only workspace target",
			opts: CreateOptions{Workspaces: []string{"/host/src:/srv/src:ro"}},
			want: []string{FeatureCreateWorkspaces},
		},
		{
			name: "skills mode",
			opts: CreateOptions{Skills: "readonly"},
			want: []string{FeatureCreateSkills},
		},
		{
			name: "kit args",
			opts: CreateOptions{KitArgs: []string{"repo=acme/app"}},
			want: []string{FeatureCreateKitArgs},
		},
		{
			name: "kit args files",
			opts: CreateOptions{KitArgsFiles: []string{"/tmp/args"}},
			want: []string{FeatureCreateKitArgsFiles},
		},
		{
			name: "env files",
			opts: CreateOptions{EnvFiles: []string{"/tmp/env"}},
			want: []string{FeatureCreateEnvFiles},
		},
		{
			name: "static mcp",
			opts: CreateOptions{StaticMCP: []string{"notion"}},
			want: []string{FeatureStaticMCP},
		},
		{
			name: "static mcp none",
			opts: CreateOptions{StaticMCP: []string{"none"}},
			want: []string{FeatureStaticMCP, FeatureStaticMCPNone},
		},
		{
			name: "static mcp none inside a list",
			opts: CreateOptions{StaticMCP: []string{"notion, none"}},
			want: []string{FeatureStaticMCP, FeatureStaticMCPNone},
		},
		{
			name: "all new options",
			opts: CreateOptions{
				Workspaces: []string{"/host:/srv"},
				Skills:     "off",
				KitArgs:    []string{"a=b"},
				StaticMCP:  []string{"none"},
			},
			want: []string{FeatureCreateSkills, FeatureCreateKitArgs, FeatureStaticMCP, FeatureStaticMCPNone, FeatureCreateWorkspaces},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := featureIDs(tt.opts.RequiredFeatures())
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("RequiredFeatures = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKitAddFeatures(t *testing.T) {
	if got := KitAddFeatures(nil); len(got) != 0 {
		t.Fatalf("no args = %v, want none", got)
	}
	if got := KitAddFeatures([]string{"  "}); len(got) != 0 {
		t.Fatalf("blank args = %v, want none", got)
	}
	got := featureIDs(KitAddFeatures([]string{"repo=acme/app"}))
	if !slices.Equal(got, []string{FeatureKitAddKitArg}) {
		t.Fatalf("KitAddFeatures = %v, want [%s]", got, FeatureKitAddKitArg)
	}
}

func TestIncompatibleAndFormat(t *testing.T) {
	old := ParseVersion("sbx version: v0.43.0 abc")
	required := CreateOptions{
		Workspaces: []string{"/host:/srv"},
		Skills:     "readonly",
		StaticMCP:  []string{"none"},
	}.RequiredFeatures()

	got := featureIDs(Incompatible(old, required))
	want := []string{FeatureStaticMCPNone, FeatureCreateWorkspaces}
	if !slices.Equal(got, want) {
		t.Fatalf("Incompatible = %v, want %v", got, want)
	}

	err := FormatIncompatible(old, required)
	if err == nil {
		t.Fatal("expected an error for an old sbx")
	}
	for _, needle := range []string{
		"workspace mount target (create.workspaces) requires sbx v0.51.0 or later; installed sbx is v0.43.0",
		"static MCP `none` (create.staticMcp.none) requires sbx v0.51.0",
	} {
		if !strings.Contains(err.Error(), needle) {
			t.Fatalf("error %q does not contain %q", err.Error(), needle)
		}
	}

	if err := FormatIncompatible(ParseVersion("v0.51.0"), required); err != nil {
		t.Fatalf("v0.51.0 must support the required features: %v", err)
	}
	if err := FormatIncompatible(Version{}, required); err != nil {
		t.Fatalf("unknown version must allow everything: %v", err)
	}
}

func TestFeatureTableCoversDocumentedMinimums(t *testing.T) {
	min := map[string]string{}
	for _, f := range Features() {
		min[f.ID] = f.MinVersion
	}
	want := map[string]string{
		FeatureCreateSkills:          "v0.43.0",
		FeatureCreateKitArgs:         "v0.43.0",
		FeatureCreateKitArgsFiles:    "v0.43.0",
		FeatureCreateEnvFiles:        "v0.43.0",
		FeatureStaticMCP:             "v0.43.0",
		FeatureKitAddKitArg:          "v0.43.0",
		FeatureStaticMCPNone:         "v0.51.0",
		FeatureCreateWorkspaces:      "v0.51.0",
		FeatureSbxenvWorkspaceTarget: "v0.51.0",
		FeatureSbxPort:               "v0.54.0",
	}
	for id, version := range want {
		if min[id] != version {
			t.Errorf("%s min version = %q, want %q", id, min[id], version)
		}
	}
}
