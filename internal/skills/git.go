package skills

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/kevinburke/ssh_config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// defaultSSHUser is the login name git hosts expect when neither the url nor
// ~/.ssh/config names one.
const defaultSSHUser = "git"

// defaultSSHKeyNames are the key names ssh loads when no IdentityFile applies.
var defaultSSHKeyNames = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

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
		method, err := sshAuth(url)
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

// agentSigners returns the keys held by a running ssh-agent, trying
// SSH_AUTH_SOCK first and falling back to the socket paths a desktop session
// commonly starts one at: a process launched from a desktop entry (rather than
// a login shell) often does not inherit SSH_AUTH_SOCK even though an agent is
// running. The returned signers keep the agent connection alive for as long as
// they are used.
func agentSigners() ([]gossh.Signer, error) {
	var candidates []string
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		candidates = append(candidates, sock)
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		candidates = append(candidates,
			filepath.Join(runtimeDir, "ssh-agent.socket"),
			filepath.Join(runtimeDir, "keyring", "ssh"),
		)
	}
	var lastErr error
	for _, sock := range candidates {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			lastErr = err
			continue
		}
		signers, err := agent.NewClient(conn).Signers()
		if err != nil {
			_ = conn.Close()
			lastErr = err
			continue
		}
		return signers, nil
	}
	if lastErr != nil {
		return nil, errors.Join(errors.New("connect to ssh agent"), lastErr)
	}
	return nil, errors.New("no ssh agent socket found")
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

// sshAuth resolves an SSH auth method for url the way ssh itself would: the
// identity files ~/.ssh/config names for that host, then the keys held by a
// running ssh-agent, then the default ~/.ssh keys — all offered in one attempt
// so the server picks whichever it recognizes (a single wrong-but-present key
// would otherwise be tried alone and rejected, even though a working key sits
// right next to it). "IdentitiesOnly yes" drops the agent keys, as in OpenSSH.
// The login name comes from url, else ~/.ssh/config, else "git". Host keys are
// verified against ~/.ssh/known_hosts by go-git.
func sshAuth(url string) (transport.AuthMethod, error) {
	host, user := sshEndpoint(url)
	config := sshConfig()
	if user == "" {
		user = strings.TrimSpace(config.Get(host, "User"))
	}
	if user == "" {
		user = defaultSSHUser
	}
	signers, err := sshSigners(host, config)
	if err != nil {
		return nil, err
	}
	return &ssh.PublicKeysCallback{
		User:     user,
		Callback: func() ([]gossh.Signer, error) { return signers, nil },
	}, nil
}

// sshConfig reads ~/.ssh/config relative to the current HOME. The package
// default resolves the home directory through /etc/passwd instead, which
// ignores a HOME override.
func sshConfig() *ssh_config.UserSettings {
	settings := &ssh_config.UserSettings{IgnoreErrors: true}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	settings.ConfigFinder(func() string { return filepath.Join(home, ".ssh", "config") })
	return settings
}

// sshEndpoint extracts the host and the login name url carries, tolerating a
// url go-git cannot parse: the clone itself will report that failure.
func sshEndpoint(url string) (host, user string) {
	endpoint, err := transport.NewEndpoint(url)
	if err != nil {
		return "", ""
	}
	return endpoint.Host, endpoint.User
}

// sshSigners collects every key worth offering for host, deduplicated by
// fingerprint so a key present both on disk and in the agent does not burn two
// of the server's authentication attempts.
func sshSigners(host string, config *ssh_config.UserSettings) ([]gossh.Signer, error) {
	var signers []gossh.Signer
	seen := make(map[string]bool)
	add := func(candidates ...gossh.Signer) {
		for _, signer := range candidates {
			fingerprint := gossh.FingerprintSHA256(signer.PublicKey())
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			signers = append(signers, signer)
		}
	}
	var failures []error
	for _, path := range sshIdentityFiles(host, config) {
		key, err := ssh.NewPublicKeysFromFile(defaultSSHUser, path, "")
		if err != nil {
			failures = append(failures, errors.Join(fmt.Errorf("load %s", path), err))
			continue
		}
		add(key.Signer)
	}
	if !strings.EqualFold(strings.TrimSpace(config.Get(host, "IdentitiesOnly")), "yes") {
		fromAgent, err := agentSigners()
		if err != nil {
			failures = append(failures, err)
		}
		add(fromAgent...)
	}
	if len(signers) == 0 {
		return nil, errors.Join(append([]error{sshNoKeyError(host)}, failures...)...)
	}
	return signers, nil
}

// sshIdentityFiles lists the existing private keys to offer for host: those
// ~/.ssh/config declares first, then the default names, so a host-specific key
// is tried before the generic ones.
func sshIdentityFiles(host string, config *ssh_config.UserSettings) []string {
	var paths []string
	seen := make(map[string]bool)
	for _, entry := range config.GetAll(host, "IdentityFile") {
		if path := expandSSHPath(entry); path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range defaultSSHKeyNames {
			path := filepath.Join(home, ".ssh", name)
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return slices.DeleteFunc(paths, func(path string) bool {
		info, err := os.Stat(path)
		return err != nil || info.IsDir()
	})
}

// expandSSHPath resolves the ~ and quoting ssh_config permits in an
// IdentityFile entry; a path relative to nothing else resolves under ~/.ssh,
// as ssh does.
func expandSSHPath(entry string) string {
	entry = strings.Trim(strings.TrimSpace(entry), `"`)
	if entry == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	switch {
	case entry == "~":
		return home
	case strings.HasPrefix(entry, "~/"):
		return filepath.Join(home, entry[2:])
	case filepath.IsAbs(entry):
		return filepath.Clean(entry)
	default:
		return filepath.Join(home, ".ssh", entry)
	}
}

// sshNoKeyError names the three places a key was looked for, so the message
// tells the user which one to populate.
func sshNoKeyError(host string) error {
	if host == "" {
		host = "this host"
	}
	return fmt.Errorf("no usable ssh key for %s: nothing in ssh-agent, no IdentityFile in ~/.ssh/config for that host, and no %s in ~/.ssh",
		host, strings.Join(defaultSSHKeyNames, "/"))
}
