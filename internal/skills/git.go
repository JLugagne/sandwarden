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
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Auth selects how a skill store is fetched.
type Auth string

const (
	// AuthPublic clones without credentials; the URL may still embed a token.
	AuthPublic Auth = ""
	// AuthSSH authenticates with the user's ssh-agent or the default private
	// keys of ~/.ssh; host keys are verified against ~/.ssh/known_hosts.
	AuthSSH Auth = "ssh"
)

// Checkout replaces dir with a fresh clone of url, optionally checked out at a
// branch or tag ref. The clone lands in a sibling temporary directory first, so
// a failed fetch never destroys the previous checkout.
func Checkout(ctx context.Context, dir, url, ref string, auth Auth) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("git url is required")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("checkout directory is required")
	}
	if err := ValidateAuthURL(url, auth); err != nil {
		return err
	}
	options := &git.CloneOptions{URL: url}
	if auth == AuthSSH {
		method, err := sshAuth()
		if err != nil {
			return err
		}
		options.Auth = method
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
	repo, err := git.PlainCloneContext(ctx, tmp, false, options)
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

// ValidateAuthURL rejects auth/url combinations go-git cannot serve: SSH
// authentication paired with an http(s) URL fails deep inside the HTTP
// transport with an opaque "invalid auth method" error instead of clearly
// naming the mismatch, and an unrecognized auth value would otherwise only
// surface once a clone is attempted.
func ValidateAuthURL(url string, auth Auth) error {
	switch auth {
	case AuthPublic:
		return nil
	case AuthSSH:
		if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
			return fmt.Errorf("ssh authentication requires an ssh:// or git@host:path url, not %s", url)
		}
		return nil
	default:
		return fmt.Errorf("unsupported auth %q", auth)
	}
}

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

// sshAuth resolves an SSH auth method from the user's environment: the agent
// when SSH_AUTH_SOCK is set, otherwise the first default private key of
// ~/.ssh. Host keys are verified against ~/.ssh/known_hosts by go-git.
func sshAuth() (transport.AuthMethod, error) {
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		if method, err := ssh.NewSSHAgentAuth("git"); err == nil {
			return method, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		path := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		method, err := ssh.NewPublicKeysFromFile("git", path, "")
		if err != nil {
			lastErr = err
			continue
		}
		return method, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("load ssh key: %w", lastErr)
	}
	return nil, errors.New("no ssh key found in ~/.ssh (looked for id_ed25519, id_ecdsa, id_rsa)")
}
