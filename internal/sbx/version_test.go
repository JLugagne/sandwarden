package sbx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

// newVersionSbx installs a fake sbx answering `version` with the contents of
// FAKE_SBX_VERSION_FILE at call time, so one test can change the reported
// version between calls. `version --json` wraps that value in the modern JSON
// payload and appends FAKE_SBX_SERVER_JSON as the server block when set.
func newVersionSbx(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	versionFile := filepath.Join(dir, "version")
	if err := os.WriteFile(versionFile, []byte("sbx version: v0.43.0 abc"), 0o644); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"version\" ]; then\n" +
		"  if [ \"$2\" = \"--json\" ]; then\n" +
		"    printf '{\"client\":{\"version\":\"%s\"}' \"$(cat \"$FAKE_SBX_VERSION_FILE\")\"\n" +
		"    if [ -n \"${FAKE_SBX_SERVER_JSON:-}\" ]; then printf ',\"server\":%s' \"$FAKE_SBX_SERVER_JSON\"; fi\n" +
		"    printf '}\\n'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"  cat \"$FAKE_SBX_VERSION_FILE\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv(envBinary, bin)
	t.Setenv("FAKE_SBX_VERSION_FILE", versionFile)
	return versionFile
}

func setVersionOutput(t *testing.T, path, output string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}
}

func TestVersionCacheExpiresAfterTTL(t *testing.T) {
	versionFile := newVersionSbx(t)
	client := New("/nonexistent.sock")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	first, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if got := first.String(); got != "v0.43.0" {
		t.Fatalf("first version = %q, want v0.43.0", got)
	}

	setVersionOutput(t, versionFile, "sbx version: v0.54.0 abc")

	now = now.Add(time.Minute)
	if got, _ := client.Version(context.Background()); got.String() != "v0.43.0" {
		t.Fatalf("version switched to %q before the TTL expired", got)
	}

	now = now.Add(defaultVersionTTL)
	if got, _ := client.Version(context.Background()); got.String() != "v0.54.0" {
		t.Fatalf("cache did not expire: got %q, want v0.54.0", got)
	}
}

func TestRefreshVersionBypassesTheCache(t *testing.T) {
	versionFile := newVersionSbx(t)
	client := New("/nonexistent.sock")

	if got, _ := client.Version(context.Background()); got.String() != "v0.43.0" {
		t.Fatalf("version = %q, want v0.43.0", got)
	}
	setVersionOutput(t, versionFile, "sbx version: v0.54.0 abc")
	if got, _ := client.Version(context.Background()); got.String() != "v0.43.0" {
		t.Fatalf("cached version changed early: %q", got)
	}
	if got, err := client.RefreshVersion(context.Background()); err != nil || got.String() != "v0.54.0" {
		t.Fatalf("RefreshVersion = %q, %v; want v0.54.0", got, err)
	}
	if got, _ := client.Version(context.Background()); got.String() != "v0.54.0" {
		t.Fatalf("refreshed version was not cached: %q", got)
	}
}

func TestParseVersionsJSON(t *testing.T) {
	raw := `{"client":{"version":"v0.54.1","revision":"3df896d5"},"server":{"state":"running","version":"v0.43.0","revision":"79805a6e","api_version":"0.31.0"}}`
	vs := ParseVersions(raw)
	if got := vs.Client.String(); got != "v0.54.1" {
		t.Fatalf("client = %q, want v0.54.1", got)
	}
	if got := vs.Server.String(); got != "v0.43.0" {
		t.Fatalf("server = %q, want v0.43.0", got)
	}
	if vs.ServerState != "running" {
		t.Fatalf("server state = %q, want running", vs.ServerState)
	}
	if !vs.ServerRunning() {
		t.Fatal("server should be reported running")
	}
	if !vs.DaemonRestartRequired() {
		t.Fatal("a running server on a different version must require a restart")
	}
	if got := vs.Effective().String(); got != "v0.43.0" {
		t.Fatalf("effective = %q, want the lower server version v0.43.0", got)
	}
}

func TestParseVersionsJSONStoppedDaemon(t *testing.T) {
	raw := `{"client":{"version":"v0.54.1"},"server":{"state":"stopped"}}`
	vs := ParseVersions(raw)
	if got := vs.Client.String(); got != "v0.54.1" {
		t.Fatalf("client = %q, want v0.54.1", got)
	}
	if vs.Server.Known {
		t.Fatalf("stopped daemon must have an unknown version, got %q", vs.Server)
	}
	if vs.DaemonRestartRequired() {
		t.Fatal("a stopped daemon must not require a restart")
	}
	if got := vs.Effective().String(); got != "v0.54.1" {
		t.Fatalf("effective = %q, want the client version when the server is unknown", got)
	}
}

func TestParseVersionsJSONMatchingVersions(t *testing.T) {
	raw := `{"client":{"version":"v0.54.1"},"server":{"state":"running","version":"v0.54.1"}}`
	vs := ParseVersions(raw)
	if vs.DaemonRestartRequired() {
		t.Fatal("matching client and server versions must not require a restart")
	}
}

func TestParseVersionsTextFallback(t *testing.T) {
	for _, input := range []string{
		"sbx version: v0.43.0 79805a6e",
		"sbx version v0.54.1 3df896d5",
	} {
		vs := ParseVersions(input)
		if !vs.Client.Known {
			t.Fatalf("client version not parsed from %q", input)
		}
		if vs.ServerPresent {
			t.Fatalf("plain text output must not report a server block: %q", input)
		}
		if vs.DaemonRestartRequired() {
			t.Fatalf("plain text output must not require a restart: %q", input)
		}
		if got := vs.Effective(); got.String() != vs.Client.String() {
			t.Fatalf("effective = %q, want the client version", got)
		}
	}
}

func TestVersionReturnsTheLowerOfClientAndServer(t *testing.T) {
	versionFile := newVersionSbx(t)
	setVersionOutput(t, versionFile, "v0.54.0")
	t.Setenv("FAKE_SBX_SERVER_JSON", `{"state":"running","version":"v0.43.0"}`)

	client := New("/nonexistent.sock")
	got, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if got.String() != "v0.43.0" {
		t.Fatalf("effective version = %q, want the older daemon v0.43.0", got)
	}

	vs, err := client.Versions(context.Background())
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if vs.Client.String() != "v0.54.0" || vs.Server.String() != "v0.43.0" {
		t.Fatalf("versions = client %q, server %q", vs.Client, vs.Server)
	}
	if !vs.DaemonRestartRequired() {
		t.Fatal("restart should be required when the daemon lags the CLI")
	}
}

func TestVersionsFallsBackWhenJSONIsUnsupported(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"version\" ]; then\n" +
		"  if [ \"$2\" = \"--json\" ]; then echo 'unknown flag: --json' >&2; exit 2; fi\n" +
		"  echo 'sbx version: v0.54.1 abc'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sbx: %v", err)
	}
	t.Setenv(envBinary, bin)

	vs, err := New("/nonexistent.sock").Versions(context.Background())
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if vs.Client.String() != "v0.54.1" {
		t.Fatalf("client = %q, want the text-fallback v0.54.1", vs.Client)
	}
	if vs.ServerPresent {
		t.Fatal("text fallback must leave the server unknown")
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
