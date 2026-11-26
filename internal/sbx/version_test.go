package sbx

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		known bool
		major int
		minor int
		patch int
		pre   string
	}{
		{name: "release", input: "sbx version: v0.43.0 79805a6e3c6667520dc2da4f6bdeddae9b700969", known: true, minor: 43},
		{name: "pre-release", input: "sbx version: v0.51.0-rc2 deadbeef", known: true, minor: 51, pre: "rc2"},
		{name: "no v prefix", input: "sbx version: 1.2.3", known: true, major: 1, minor: 2, patch: 3},
		{name: "surrounding noise", input: "WARN: keychain fallback\nsbx version: v0.54.12 abc\n", known: true, minor: 54, patch: 12},
		{name: "dev build", input: "sbx version: dev-79805a6e", known: false},
		{name: "empty", input: "", known: false},
		{name: "sha only", input: "79805a6e3c6667520dc2da4f6bdeddae9b700969", known: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseVersion(tt.input)
			if got.Known != tt.known {
				t.Fatalf("Known = %v, want %v (raw %q)", got.Known, tt.known, got.Raw)
			}
			if !tt.known {
				return
			}
			if got.Major != tt.major || got.Minor != tt.minor || got.Patch != tt.patch {
				t.Fatalf("version = %d.%d.%d, want %d.%d.%d", got.Major, got.Minor, got.Patch, tt.major, tt.minor, tt.patch)
			}
			if got.Pre != tt.pre {
				t.Fatalf("Pre = %q, want %q", got.Pre, tt.pre)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	if got := ParseVersion("sbx version: v0.43.0 abc").String(); got != "v0.43.0" {
		t.Fatalf("String = %q, want v0.43.0", got)
	}
	if got := ParseVersion("sbx version: v0.51.0-rc1 abc").String(); got != "v0.51.0-rc1" {
		t.Fatalf("String = %q, want v0.51.0-rc1", got)
	}
	if got := (Version{}).String(); got != "unknown" {
		t.Fatalf("zero String = %q, want unknown", got)
	}
	if got := (Version{Raw: "dev-abc"}).String(); got != "dev-abc" {
		t.Fatalf("unknown String = %q, want dev-abc", got)
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		version string
		min     string
		want    bool
	}{
		{name: "older minor", version: "v0.43.0", min: "v0.51.0", want: false},
		{name: "equal", version: "v0.51.0", min: "v0.51.0", want: true},
		{name: "newer patch", version: "v0.51.2", min: "v0.51.0", want: true},
		{name: "newer minor", version: "v0.54.0", min: "v0.51.0", want: true},
		{name: "numeric not lexical", version: "v0.9.0", min: "v0.10.0", want: false},
		{name: "pre-release counts as release", version: "v0.43.0-rc1", min: "v0.43.0", want: true},
		{name: "unknown allows", version: "dev-abc", min: "v99.0.0", want: true},
		{name: "unparseable minimum allows", version: "v1.0.0", min: "dev", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseVersion(tt.version).AtLeast(tt.min); got != tt.want {
				t.Fatalf("AtLeast(%q) = %v, want %v", tt.min, got, tt.want)
			}
		})
	}
}
