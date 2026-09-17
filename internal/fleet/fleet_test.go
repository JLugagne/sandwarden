package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My VM":                 "my-vm",
		"  Web dev  ":           "web-dev",
		"Go module cache":       "go-module-cache",
		"a/b\\c":                "a-b-c",
		"":                      "item",
		"---":                   "item",
		"ÄÖÜ":                   "item",
		"v1.2.3":                "v1-2-3",
		strings.Repeat("x", 80): strings.Repeat("x", 64),
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenCreatesLayout(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, sub := range []string{"sandboxes", "profiles", "caches", "stores/skills", "stores/kits"} {
		if !dirExists(filepath.Join(dir, sub)) {
			t.Errorf("missing %s", sub)
		}
	}
	if len(f.Sandboxes()) != 0 || len(f.Profiles()) != 0 || len(f.Caches()) != 0 {
		t.Fatal("expected an empty fleet")
	}
}

func TestSandboxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	spec := NewMixin("")
	spec.DisplayName = "My VM"
	spec.Description = "test"
	spec.Requires = &SpecRequires{Agent: "claude"}
	spec.Environment = &SpecEnv{Variables: map[string]string{"FOO": "bar"}}
	spec.Permissions = &SpecPermission{Network: &SpecNetwork{Deny: []string{"telemetry.example.com"}}}
	spec.Ports = []SpecPort{{Container: 8080, Protocol: "tcp", Name: "web"}}
	app := SandboxApp{
		Create:   &SandboxCreate{CPUs: 4, Memory: "8g", Workspaces: []string{"/tmp/src"}, Publish: []string{"8080:8080"}},
		Profiles: []string{"web-dev"},
		Caches:   []string{"go-mod"},
		Skills:   []SkillRef{{Store: "anthropics-skills", Kind: "skill", Name: "pdf"}},
		Mounts:   []MountRef{{HostPath: "/host", TargetPath: "/src", ReadOnly: true}},
		RunArgs:  "--model opus",
		OptOuts:  &SandboxOptOut{Caches: []string{"go-mod"}},
	}
	created, err := f.CreateSandbox("My VM", spec, app)
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if created.Slug != "my-vm" || created.Spec.Name != "my-vm" || created.Spec.Kind != "mixin" || created.Spec.SchemaVersion != "2" {
		t.Fatalf("unexpected normalized sandbox: %+v", created.Spec)
	}
	if created.App.Sandbox != "My VM" {
		t.Fatalf("canonical name lost: %+v", created.App)
	}

	f2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := f2.SandboxByName("My VM")
	if !ok {
		t.Fatal("sandbox not found by canonical name")
	}
	if got.Spec.DisplayName != "My VM" || got.Spec.Requires.Agent != "claude" ||
		got.Spec.Env()["FOO"] != "bar" || got.Spec.NetworkDeny()[0] != "telemetry.example.com" {
		t.Fatalf("spec round-trip mismatch: %+v", got.Spec)
	}
	if got.App.Create.CPUs != 4 || got.App.Profiles[0] != "web-dev" || len(got.App.OptOuts.Caches) != 1 {
		t.Fatalf("sidecar round-trip mismatch: %+v", got.App)
	}
	if got.App.Mounts[0].EffectiveTarget() != "/src" || got.App.Mounts[0].Key() != "/host:/src" {
		t.Fatalf("mount helpers mismatch: %+v", got.App.Mounts[0])
	}
}

func TestUnknownSpecFieldRejected(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	path := filepath.Join(dir, "sandboxes", "bad")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("spec.yaml", "schemaVersion: \"2\"\nkind: mixin\nname: bad\nx-sandwarden: nope\n")
	write("sandwarden.yaml", "sandbox: bad\n")
	if err := f.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(f.Errors()) == 0 {
		t.Fatal("expected a strict decode error")
	}
	if _, ok := f.Sandbox("bad"); ok {
		t.Fatal("broken sandbox must not be indexed")
	}
	if !strings.Contains(f.Errors()[0].Error(), "x-sandwarden") {
		t.Fatalf("error should name the unknown field: %v", f.Errors()[0])
	}
}

func TestUniqueSlug(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first, err := f.CreateSandbox("My VM", NewMixin(""), SandboxApp{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.CreateSandbox("my vm", NewMixin(""), SandboxApp{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "my-vm" || second.Slug != "my-vm-2" {
		t.Fatalf("slugs = %q, %q", first.Slug, second.Slug)
	}
}

func TestCacheDefaults(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	created, err := f.CreateCache("Go module cache", CacheApp{HostPath: "/host/cache", TargetPath: "/cache"})
	if err != nil {
		t.Fatal(err)
	}
	if !created.App.AutoAttaches() || !created.App.IsEnabled() {
		t.Fatal("absent booleans must default to true")
	}
	f2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := f2.Cache("go-module-cache")
	if !ok || got.App.Name != "Go module cache" || !got.App.AutoAttaches() {
		t.Fatalf("cache round-trip mismatch: %+v", got)
	}
}

func TestStoreRegistrations(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	reg, err := f.CreateStore(StoreReg{Kind: StoreSkills, Name: "Anthropic skills", URL: "https://github.com/anthropics/skills", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if reg.Slug != "anthropic-skills" {
		t.Fatalf("slug = %q", reg.Slug)
	}
	got, ok := f.Store(StoreSkills, reg.Slug)
	if !ok || got.URL != reg.URL || got.Kind != StoreSkills {
		t.Fatalf("store round-trip mismatch: %+v", got)
	}
	if _, ok := f.Store(StoreKits, reg.Slug); ok {
		t.Fatal("store kinds must stay separate")
	}
	if err := f.DeleteStore(StoreSkills, reg.Slug); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Store(StoreSkills, reg.Slug); ok {
		t.Fatal("store still indexed after delete")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if cfg := f.Config(); cfg.Terminals.Enabled != nil {
		t.Fatal("default terminals must be nil (all enabled)")
	}
	enabled := []string{"kitty", "gnome-terminal"}
	if err := f.SetConfig(AppConfig{Terminals: TerminalPrefs{Enabled: &enabled, Default: "kitty"}, Notifications: true}); err != nil {
		t.Fatal(err)
	}
	f2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := f2.Config()
	if cfg.Terminals.Enabled == nil || len(*cfg.Terminals.Enabled) != 2 || cfg.Terminals.Default != "kitty" || !cfg.Notifications {
		t.Fatalf("config round-trip mismatch: %+v", cfg)
	}
	if err := f2.SetConfig(AppConfig{Notifications: false}); err != nil {
		t.Fatal(err)
	}
	f3, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f3.Config().Terminals.Enabled != nil {
		t.Fatal("nil terminals must survive a round trip")
	}
}

func TestProfileAndReferences(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	spec := NewMixin("")
	spec.DisplayName = "Web dev"
	spec.Permissions = &SpecPermission{Network: &SpecNetwork{Allow: []string{"api.github.com"}}}
	p, err := f.CreateProfile("Web dev", spec, ProfileApp{Default: true, Mounts: []MountRef{{HostPath: "/src"}}, Caches: []string{"go-mod"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "web-dev" || p.Label() != "Web dev" || !p.App.Default {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if err := f.DeleteProfile(p.Slug); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Profile(p.Slug); ok {
		t.Fatal("profile still indexed after delete")
	}
}

func TestHomePathsAreExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cache, err := f.CreateCache("Go module cache", CacheApp{HostPath: "~/go/pkg/mod"})
	if err != nil {
		t.Fatalf("CreateCache: %v", err)
	}
	if want := filepath.Join(home, "go/pkg/mod"); cache.App.HostPath != want {
		t.Fatalf("cache host path = %q, want %q", cache.App.HostPath, want)
	}
	spec := NewMixin("")
	sb, err := f.CreateSandbox("demo", spec, SandboxApp{
		Mounts: []MountRef{{HostPath: "~/src"}},
		Create: &SandboxCreate{Workspaces: []string{"~/work"}},
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if want := filepath.Join(home, "src"); sb.App.Mounts[0].HostPath != want {
		t.Fatalf("mount host path = %q, want %q", sb.App.Mounts[0].HostPath, want)
	}
	if want := filepath.Join(home, "work"); sb.App.Create.Workspaces[0] != want {
		t.Fatalf("workspace = %q, want %q", sb.App.Create.Workspaces[0], want)
	}

	f2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := f2.Cache(cache.Slug)
	if !ok {
		t.Fatal("cache missing after reload")
	}
	if want := filepath.Join(home, "go/pkg/mod"); got.App.HostPath != want {
		t.Fatalf("reloaded cache host path = %q, want %q", got.App.HostPath, want)
	}
}

func TestStoreRegistrationsSurviveReload(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	skillReg, err := f.CreateStore(StoreReg{Kind: StoreSkills, Name: "Anthropic skills", URL: "https://github.com/anthropics/skills", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	kitReg, err := f.CreateStore(StoreReg{Kind: StoreKits, Name: "Base kits", URL: "https://example.com/kits"})
	if err != nil {
		t.Fatal(err)
	}
	f2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := f2.Store(StoreSkills, skillReg.Slug); !ok {
		t.Fatalf("skill store %q lost on reload", skillReg.Slug)
	}
	if _, ok := f2.Store(StoreKits, kitReg.Slug); !ok {
		t.Fatalf("kit store %q lost on reload", kitReg.Slug)
	}
}

func TestStalenessDetectsOutsideEdits(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	spec := NewMixin("")
	spec.DisplayName = "My VM"
	sb, err := f.CreateSandbox("My VM", spec, SandboxApp{})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	if got := f.CheckStaleness(); got.Stale || len(got.Changed) != 0 {
		t.Fatalf("fresh fleet must not be stale: %+v", got)
	}

	sb.App.RunArgs = "--model opus"
	if err := f.SaveSandbox(sb); err != nil {
		t.Fatalf("SaveSandbox: %v", err)
	}
	if got := f.CheckStaleness(); got.Stale {
		t.Fatalf("the app's own writes must not be stale: %+v", got)
	}

	sidecar := filepath.Join(sb.Dir, sidecarFile)
	if err := os.WriteFile(sidecar, []byte("sandbox: My VM\nrunArgs: --model sonnet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := f.CheckStaleness()
	if !got.Stale || len(got.Changed) != 1 {
		t.Fatalf("expected exactly the edited sidecar to be stale: %+v", got)
	}
	if file := got.Changed[0]; file.Kind != EntitySandbox || file.Slug != "my-vm" ||
		file.File != sidecarFile || file.Name != "My VM" || file.Path != sidecar {
		t.Fatalf("unexpected stale file: %+v", file)
	}

	if err := f.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := f.CheckStaleness(); got.Stale {
		t.Fatalf("reload must clear staleness: %+v", got)
	}
	if reloaded, ok := f.Sandbox("my-vm"); !ok || reloaded.App.RunArgs != "--model sonnet" {
		t.Fatalf("reload lost the new value: %+v", reloaded)
	}
}

func TestStalenessCoversEveryEntityKind(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := f.CreateCache("Go module cache", CacheApp{HostPath: "/host/cache", TargetPath: "/cache"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CreateStore(StoreReg{Kind: StoreSkills, Name: "skills", URL: "https://example.com/skills"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetConfig(AppConfig{Notifications: true}); err != nil {
		t.Fatal(err)
	}
	if got := f.CheckStaleness(); got.Stale {
		t.Fatalf("app writes must not be stale: %+v", got)
	}

	config := filepath.Join(dir, configFile)
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, append(raw, []byte("# outside edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	got := f.CheckStaleness()
	if !got.Stale || len(got.Changed) != 1 || got.Changed[0].Kind != EntityConfig || got.Changed[0].Name != "global configuration" {
		t.Fatalf("unexpected staleness: %+v", got)
	}

	if err := f.DeleteCache("go-module-cache"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteStore(StoreSkills, "skills"); err != nil {
		t.Fatal(err)
	}
	got = f.CheckStaleness()
	if !got.Stale || len(got.Changed) != 1 || got.Changed[0].Kind != EntityConfig {
		t.Fatalf("deletes must drop their fingerprints: %+v", got)
	}

	if err := f.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := f.CheckStaleness(); got.Stale {
		t.Fatalf("reload must clear staleness: %+v", got)
	}

	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if got := f.CheckStaleness(); !got.Stale || got.Changed[0].Kind != EntityConfig {
		t.Fatalf("a removed file must be reported: %+v", got)
	}
}
