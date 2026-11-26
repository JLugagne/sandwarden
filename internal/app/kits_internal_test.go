package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/store"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// registerKitStore registers a git-hosted kit store holding one discovered kit
// and returns the registration with the local checkout path of that kit.
func registerKitStore(t *testing.T, a *App, url string) (*fleet.StoreReg, string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreKits, Name: "acme", URL: url, Ref: "v1", Auth: "ssh"})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	items := []store.KitItem{{Store: reg.Slug, Name: "go-agent", DisplayName: "Go agent", RelPath: "kits/go-agent"}}
	if err := a.Store.ReplaceKitItems(context.Background(), reg.Slug, items); err != nil {
		t.Fatalf("seed kit items: %v", err)
	}
	local := filepath.Join(storeCheckoutPath(fleet.StoreKits, reg.Slug), "kits", "go-agent")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatalf("seed checkout: %v", err)
	}
	return reg, local
}

func TestListKitItemsReferencesLocalCheckout(t *testing.T) {
	a, _ := newTestApp(t)
	_, local := registerKitStore(t, a, "git@github.com:acme/kits.git")

	items, err := a.ListKitItems(context.Background(), "")
	if err != nil {
		t.Fatalf("list kit items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 kit, got %d", len(items))
	}
	if strings.HasPrefix(items[0].Ref, "git+") {
		t.Fatalf("catalog kit still references the remote: %q", items[0].Ref)
	}
	if items[0].Ref != local {
		t.Fatalf("ref = %q, want the local checkout %q", items[0].Ref, local)
	}
}

func TestCreateOptionsFromConfigResolvesRemoteKitToCheckout(t *testing.T) {
	a, _ := newTestApp(t)
	reg, local := registerKitStore(t, a, "git@github.com:acme/kits.git")

	spec := fleet.NewMixin("")
	spec.DisplayName = "alpha"
	app := fleet.SandboxApp{
		Sandbox: "alpha",
		Create:  &fleet.SandboxCreate{Kits: []string{"git+" + reg.URL + "#dir=kits/go-agent&ref=v1"}},
	}
	s, err := a.Fleet.CreateSandbox("alpha", spec, app)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	opts := a.CreateOptionsFromConfig(s)
	if !slices.Equal(opts.Kits, []string{local}) {
		t.Fatalf("kits = %v, want %v", opts.Kits, []string{local})
	}
}

func TestCreateOptionsFromConfigKeepsUnknownKitReference(t *testing.T) {
	a, _ := newTestApp(t)
	registerKitStore(t, a, "git@github.com:acme/kits.git")

	remote := "git+https://github.com/other/kits#dir=node"
	spec := fleet.NewMixin("")
	spec.DisplayName = "beta"
	app := fleet.SandboxApp{
		Sandbox: "beta",
		Create:  &fleet.SandboxCreate{Kits: []string{remote, "./local-mixin"}},
	}
	s, err := a.Fleet.CreateSandbox("beta", spec, app)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	opts := a.CreateOptionsFromConfig(s)
	if !slices.Equal(opts.Kits, []string{remote, "./local-mixin"}) {
		t.Fatalf("kits = %v, want them unchanged", opts.Kits)
	}
}

func TestAttachKitResolvesRemoteKitToCheckout(t *testing.T) {
	ctx := context.Background()
	a, logPath := newKitAddApp(t)
	reg, local := registerKitStore(t, a, "git@github.com:acme/kits.git")

	var out strings.Builder
	result, err := a.AttachKit(ctx, "alpha", "git+"+reg.URL+"#dir=kits/go-agent&ref=v1", nil, &out)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if result.Ref != local {
		t.Fatalf("result ref = %q, want %q", result.Ref, local)
	}
	calls := skillCalls(t, logPath)
	if !slices.Contains(calls, "kit add alpha "+local) {
		t.Fatalf("sbx did not receive the local checkout: %v", calls)
	}
	for _, call := range calls {
		if strings.Contains(call, "git+") {
			t.Fatalf("sbx received a remote reference: %q", call)
		}
	}
	if got, want := sandboxKits(t, a, "alpha"), []string{"base", local}; !slices.Equal(got, want) {
		t.Fatalf("create.kits = %v, want %v", got, want)
	}
}

func TestCreateOptionsFromConfigKeepsRemoteKitWhenCheckoutIsGone(t *testing.T) {
	a, _ := newTestApp(t)
	reg, local := registerKitStore(t, a, "git@github.com:acme/kits.git")
	if err := os.RemoveAll(storeCheckoutPath(fleet.StoreKits, reg.Slug)); err != nil {
		t.Fatalf("drop checkout: %v", err)
	}

	remote := "git+" + reg.URL + "#dir=kits/go-agent&ref=v1"
	spec := fleet.NewMixin("")
	spec.DisplayName = "gamma"
	app := fleet.SandboxApp{
		Sandbox: "gamma",
		Create:  &fleet.SandboxCreate{Kits: []string{remote}},
	}
	s, err := a.Fleet.CreateSandbox("gamma", spec, app)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	opts := a.CreateOptionsFromConfig(s)
	if !slices.Equal(opts.Kits, []string{remote}) {
		t.Fatalf("kits = %v, want the remote reference kept while %s is missing", opts.Kits, local)
	}
}

func TestResolveKitRefMatchesAcrossURLSchemes(t *testing.T) {
	a, _ := newTestApp(t)
	_, local := registerKitStore(t, a, "git@github.com:acme/kits.git")

	for _, ref := range []string{
		"git+https://github.com/acme/kits#dir=kits/go-agent",
		"git+https://github.com/acme/kits.git#dir=kits/go-agent&ref=v1",
		"git+ssh://git@github.com/acme/kits.git#dir=kits/go-agent",
	} {
		if got := a.resolveKitRef(ref); got != local {
			t.Fatalf("resolveKitRef(%q) = %q, want %q", ref, got, local)
		}
	}
}

// newKitRepoWithSpecs builds a local git repository holding one kit directory
// per spec, keyed by relative path.
func newKitRepoWithSpecs(t *testing.T, specs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	for rel, content := range specs {
		kitDir := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(kitDir, 0o755); err != nil {
			t.Fatalf("mkdir kit: %v", err)
		}
		if err := os.WriteFile(filepath.Join(kitDir, "spec.yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write spec: %v", err)
		}
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := worktree.Commit("seed", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

// newKitInspectErrorsStub installs a fake sbx whose `kit inspect` fails with the
// required-argument message for a kit directory ending in needs-args, and with
// an invalid-manifest message for one ending in badkind.
func newKitInspectErrorsStub(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := `#!/bin/sh
if [ "$1" = "kit" ] && [ "$2" = "inspect" ]; then
  case "$4" in
    */badkind)
      printf '%s\n' 'ERROR: manifest: invalid kind "nonsense" (must be "sandbox" or "mixin")' >&2
      exit 1
      ;;
    */needs-args)
      printf '%s\n' 'ERROR: kit "'"$4"'" requires an argument:' >&2
      printf '\n' >&2
      printf '%s\n' '  repo  spec.yaml:7  GitHub repository to clone' >&2
      printf '\n' >&2
      printf '%s\n' 'Supply it and re-run:' >&2
      printf '%s\n' '  --kit-arg repo=<value>' >&2
      exit 1
      ;;
  esac
  printf '%s\n' '{"schemaVersion":"2","kind":"mixin","name":"plain"}'
  exit 0
fi
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write kit inspect stub: %v", err)
	}
	t.Setenv("SBX_BINARY", bin)
}

func TestSyncKitStoreFallsBackOnlyForRequiredArguments(t *testing.T) {
	ctx := context.Background()
	a, _ := newTestApp(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	newKitInspectErrorsStub(t)

	src := newKitRepoWithSpecs(t, map[string]string{
		"badkind":    "schemaVersion: \"2\"\nkind: nonsense\nname: bad-kind\n",
		"needs-args": "schemaVersion: \"2\"\nkind: mixin\nname: needs-args\nargs:\n  repo:\n    required: true\n",
	})
	reg, err := a.Fleet.CreateStore(fleet.StoreReg{Kind: fleet.StoreKits, Name: "kitbox", URL: "file://" + src})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if _, err := a.syncKitStore(ctx, reg); err != nil {
		t.Fatalf("sync: %v", err)
	}

	items, err := a.Store.ListKitItems(ctx, reg.Slug)
	if err != nil {
		t.Fatalf("list kit items: %v", err)
	}
	if len(items) != 1 || items[0].Name != "needs-args" {
		t.Fatalf("catalog = %+v, want only the required-argument kit", items)
	}

	states, err := a.Store.StoreStates(ctx, string(fleet.StoreKits))
	if err != nil {
		t.Fatalf("store states: %v", err)
	}
	msg := states[reg.Slug].Error
	if !strings.Contains(msg, "badkind") || !strings.Contains(msg, "invalid kind") {
		t.Fatalf("the invalid kit was not reported: %q", msg)
	}
	if strings.Contains(msg, "requires an argument") {
		t.Fatalf("the required-argument kit was skipped: %q", msg)
	}
}

func TestParseKitSpecWithRequiredArgs(t *testing.T) {
	dir := t.TempDir()
	specContent := `schemaVersion: "2"
kind: mixin
name: github-clone
displayName: GitHub Clone
description: Clones a GitHub repository into the sandbox at create time
args:
  repo:
    required: true
    description: GitHub repository to clone
  ref:
    default: ""
    description: Optional git ref
permissions:
  network:
    allow:
      - api.github.com
      - github.com
setup:
  install:
    - command: echo "installing"
`
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	spec, err := parseKitSpec(dir)
	if err != nil {
		t.Fatalf("parseKitSpec failed: %v", err)
	}
	if spec.Name != "github-clone" {
		t.Errorf("spec.Name = %q, want github-clone", spec.Name)
	}
	if spec.DisplayName != "GitHub Clone" {
		t.Errorf("spec.DisplayName = %q, want GitHub Clone", spec.DisplayName)
	}
	if len(spec.Arguments) != 2 {
		t.Fatalf("spec.Arguments length = %d, want 2", len(spec.Arguments))
	}
	repoArg, ok := spec.Arguments["repo"]
	if !ok || !repoArg.Required {
		t.Errorf("spec.Arguments[repo] = %+v, want required=true", repoArg)
	}
	refArg, ok := spec.Arguments["ref"]
	if !ok || refArg.Default == nil || *refArg.Default != "" {
		t.Errorf("spec.Arguments[ref] = %+v, want default empty string", refArg)
	}
	if len(spec.Permissions.Network.Allow) != 2 {
		t.Errorf("network allow = %v, want 2 entries", spec.Permissions.Network.Allow)
	}
}

func TestSandboxMatches(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"box", "box", true},
		{"box", "BOX", true},
		{"my-sandbox", "my_sandbox", true},
		{"my-sandbox-1", "My-Sandbox-1", true},
		{"box-1", "box-2", false},
		{"", "", true},
		{"box", "", false},
	}
	for _, tc := range cases {
		if got := sandboxMatches(tc.a, tc.b); got != tc.want {
			t.Errorf("sandboxMatches(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
