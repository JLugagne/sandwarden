package kits

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSpec(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte("schemaVersion: \"2\"\nkind: mixin\nname: x\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

func TestDiscoverFindsTopLevelAndNestedKits(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, "code-server"))
	writeSpec(t, filepath.Join(root, "nested", "deep", "my-kit"))
	writeSpec(t, filepath.Join(root, "spec"))
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	found, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(found) != 3 {
		t.Fatalf("expected 3 kits, got %+v", found)
	}
	got := map[string]string{}
	for _, candidate := range found {
		got[candidate.RelPath] = candidate.Name
	}
	for _, want := range []string{"code-server", "nested/deep/my-kit", "spec"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing %q in %+v", want, got)
		}
	}
}

func TestDiscoverSkipsVCSAndDependencies(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, filepath.Join(root, ".git", "hidden-kit"))
	writeSpec(t, filepath.Join(root, "node_modules", "dep-kit"))
	writeSpec(t, filepath.Join(root, "real-kit"))

	found, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(found) != 1 || found[0].RelPath != "real-kit" {
		t.Fatalf("expected only real-kit, got %+v", found)
	}
}

func TestDiscoverDoesNotNestKitsInsideKits(t *testing.T) {
	root := t.TempDir()
	kit := filepath.Join(root, "outer")
	writeSpec(t, kit)
	writeSpec(t, filepath.Join(kit, "files", "inner"))

	found, err := Discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(found) != 1 || found[0].RelPath != "outer" {
		t.Fatalf("expected only outer, got %+v", found)
	}
}
