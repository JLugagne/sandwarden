package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JLugagne/sandwarden/internal/fleet"
	"github.com/JLugagne/sandwarden/internal/store"
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
	result, err := a.AttachKit(ctx, "alpha", "git+"+reg.URL+"#dir=kits/go-agent&ref=v1", &out)
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
