package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Checkout replaces dir with a fresh clone of url, optionally checked out at a
// branch or tag ref. The clone lands in a sibling temporary directory first, so
// a failed fetch never destroys the previous checkout.
func Checkout(ctx context.Context, dir, url, ref string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("git url is required")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("checkout directory is required")
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".checkout-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	repo, err := git.PlainCloneContext(ctx, tmp, false, &git.CloneOptions{URL: url})
	if err != nil {
		return fmt.Errorf("clone %s: %w", url, err)
	}
	if ref = strings.TrimSpace(ref); ref != "" {
		if err := checkoutRef(repo, ref); err != nil {
			return err
		}
	}
	previous := dir + ".previous"
	_ = os.RemoveAll(previous)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, previous); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.Rename(previous, dir)
		return err
	}
	_ = os.RemoveAll(previous)
	return nil
}

// checkoutRef switches a fresh clone to a branch or, failing that, a tag.
// checkoutRef switches a fresh clone to a branch or, failing that, a tag.
func checkoutRef(repo *git.Repository, ref string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}
	branch := plumbing.NewBranchReferenceName(ref)
	if err := worktree.Checkout(&git.CheckoutOptions{Branch: branch, Force: true}); err == nil {
		return nil
	}
	if remote, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", ref), true); err == nil {
		return worktree.Checkout(&git.CheckoutOptions{Branch: branch, Hash: remote.Hash(), Create: true, Force: true})
	}
	if tag, err := repo.Tag(ref); err == nil {
		return worktree.Checkout(&git.CheckoutOptions{Hash: tag.Hash(), Force: true})
	}
	return fmt.Errorf("ref %q is neither a branch nor a tag", ref)
}
