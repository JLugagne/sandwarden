package skills

import (
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestSSHAuthUsesConfiguredIdentityFile(t *testing.T) {
	home := newSSHHome(t)
	writeTestFile(t, filepath.Join(home, ".ssh", "config"), "Host code.example.test\n    IdentityFile ~/.ssh/work_key\n")
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "work_key"))

	if _, err := sshAuth("git@code.example.test:team/repo.git"); err != nil {
		t.Fatalf("ssh auth should use the identity file configured for the host: %v", err)
	}
}

func TestSSHAuthCombinesAgentAndConfiguredKeys(t *testing.T) {
	home := newSSHHome(t)
	writeTestFile(t, filepath.Join(home, ".ssh", "config"), "Host code.example.test\n    IdentityFile ~/.ssh/work_key\n")
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "work_key"))
	serveTestAgent(t, filepath.Join(home, "agent.sock"))

	method, err := sshAuth("git@code.example.test:team/repo.git")
	if err != nil {
		t.Fatalf("ssh auth: %v", err)
	}
	signers, err := callbackSigners(t, method)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if len(signers) != 2 {
		t.Fatalf("expected the agent key and the configured identity file to be offered, got %d", len(signers))
	}
}

func TestSSHAuthHonorsIdentitiesOnly(t *testing.T) {
	home := newSSHHome(t)
	writeTestFile(t, filepath.Join(home, ".ssh", "config"), "Host code.example.test\n    IdentityFile ~/.ssh/work_key\n    IdentitiesOnly yes\n")
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "work_key"))
	serveTestAgent(t, filepath.Join(home, "agent.sock"))

	method, err := sshAuth("git@code.example.test:team/repo.git")
	if err != nil {
		t.Fatalf("ssh auth: %v", err)
	}
	signers, err := callbackSigners(t, method)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("IdentitiesOnly should keep the agent keys out, got %d signers", len(signers))
	}
}

func TestSSHAuthResolvesLoginName(t *testing.T) {
	home := newSSHHome(t)
	writeTestFile(t, filepath.Join(home, ".ssh", "config"), "Host code.example.test\n    IdentityFile ~/.ssh/work_key\n    User deploy\n")
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "work_key"))
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "id_ed25519"))

	for _, tc := range []struct {
		url  string
		want string
	}{
		{"ssh://code.example.test/team/repo.git", "deploy"},
		{"admin@code.example.test:team/repo.git", "admin"},
		{"ssh://other.example.test/team/repo.git", defaultSSHUser},
	} {
		method, err := sshAuth(tc.url)
		if err != nil {
			t.Fatalf("ssh auth %s: %v", tc.url, err)
		}
		cb, ok := method.(*ssh.PublicKeysCallback)
		if !ok {
			t.Fatalf("expected a PublicKeysCallback auth method, got %T", method)
		}
		if cb.User != tc.want {
			t.Fatalf("%s: expected login name %q, got %q", tc.url, tc.want, cb.User)
		}
	}
}

func callbackSigners(t *testing.T, method transport.AuthMethod) ([]gossh.Signer, error) {
	t.Helper()
	cb, ok := method.(*ssh.PublicKeysCallback)
	if !ok {
		t.Fatalf("expected a PublicKeysCallback auth method, got %T", method)
	}
	return cb.Callback()
}

func newSSHHome(t *testing.T) string {
	t.Helper()
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	return home
}

func serveTestAgent(t *testing.T, sock string) {
	t.Helper()
	keyring := agent.NewKeyring()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatalf("add key: %v", err)
	}
	serveAgent(t, keyring, sock)
	t.Setenv("SSH_AUTH_SOCK", sock)
}
