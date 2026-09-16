package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestCheckoutClonesAndSwitchesRefs(t *testing.T) {
	src, _ := newSourceRepo(t)

	checkout := filepath.Join(t.TempDir(), "store", "checkout")
	if err := Checkout(context.Background(), checkout, "file://"+src, ""); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("expected default branch content: %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "feature"); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); err != nil {
		t.Fatalf("expected feature branch content: %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "v1"); err != nil {
		t.Fatalf("checkout tag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("tag checkout should not carry the feature file, got %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "missing"); err == nil {
		t.Fatal("expected an error for an unknown ref")
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("unknown ref should leave the failed clone out of the checkout, got %v", err)
	}
}

func TestCheckoutKeepsPreviousOnFailure(t *testing.T) {
	checkout := filepath.Join(t.TempDir(), "store")
	writeTestFile(t, filepath.Join(checkout, "marker.txt"), "keep me")

	if err := Checkout(context.Background(), checkout, "file:///nonexistent/repo", ""); err == nil {
		t.Fatal("expected clone failure")
	}
	raw, err := os.ReadFile(filepath.Join(checkout, "marker.txt"))
	if err != nil || string(raw) != "keep me" {
		t.Fatalf("previous checkout should survive a failed clone, got %q (%v)", raw, err)
	}
	if _, err := os.Stat(checkout + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("temporary backup should be cleaned up, got %v", err)
	}
}

func TestCheckoutRequiresURL(t *testing.T) {
	if err := Checkout(context.Background(), t.TempDir(), "  ", ""); err == nil {
		t.Fatal("expected an error for an empty url")
	}
}

func newSourceRepo(t *testing.T) (string, plumbing.Hash) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "skills", "demo", "SKILL.md"), "# Demo\n")
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add: %v", err)
	}
	first, err := worktree.Commit("initial", commitOptions())
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	base := head.Name()

	if _, err := repo.CreateTag("v1", first, nil); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	writeTestFile(t, filepath.Join(dir, "feature.txt"), "feature\n")
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add feature: %v", err)
	}
	if _, err := worktree.Commit("feature", commitOptions()); err != nil {
		t.Fatalf("commit feature: %v", err)
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Branch: base, Force: true}); err != nil {
		t.Fatalf("back to base: %v", err)
	}
	return dir, first
}

func commitOptions() *git.CommitOptions {
	return &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}
}
