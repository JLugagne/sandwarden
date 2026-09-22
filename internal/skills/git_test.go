package skills

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func TestCheckoutClonesAndSwitchesRefs(t *testing.T) {
	src, _ := newSourceRepo(t)

	checkout := filepath.Join(t.TempDir(), "store", "checkout")
	if err := Checkout(context.Background(), checkout, "file://"+src, "", AuthPublic); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("expected default branch content: %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "feature", AuthPublic); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); err != nil {
		t.Fatalf("expected feature branch content: %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "v1", AuthPublic); err != nil {
		t.Fatalf("checkout tag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("tag checkout should not carry the feature file, got %v", err)
	}

	if err := Checkout(context.Background(), checkout, "file://"+src, "missing", AuthPublic); err == nil {
		t.Fatal("expected an error for an unknown ref")
	}
	if _, err := os.Stat(filepath.Join(checkout, "feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("unknown ref should leave the failed clone out of the checkout, got %v", err)
	}
}

func TestCheckoutKeepsPreviousOnFailure(t *testing.T) {
	checkout := filepath.Join(t.TempDir(), "store")
	writeTestFile(t, filepath.Join(checkout, "marker.txt"), "keep me")

	if err := Checkout(context.Background(), checkout, "file:///nonexistent/repo", "", AuthPublic); err == nil {
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
	if err := Checkout(context.Background(), t.TempDir(), "  ", "", AuthPublic); err == nil {
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

func TestCheckoutRejectsSSHAuthOverHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Checkout(ctx, t.TempDir(), "https://example.invalid/repo.git", "", AuthSSH)
	if err == nil {
		t.Fatal("expected an error for ssh auth against an http(s) url")
	}
	if !strings.Contains(err.Error(), "ssh") || !strings.Contains(err.Error(), "https://") {
		t.Fatalf("expected a clear ssh/url mismatch error, got %v", err)
	}
}

func TestCheckoutRejectsUnknownAuth(t *testing.T) {
	err := Checkout(context.Background(), t.TempDir(), "file:///nonexistent/repo", "", Auth("token"))
	if err == nil || !strings.Contains(err.Error(), "unsupported auth") {
		t.Fatalf("expected an unsupported auth error, got %v", err)
	}
}

func TestSSHAuthUsesDefaultKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestRSAKey(t, filepath.Join(home, ".ssh", "id_rsa"))

	method, err := sshAuth("git@code.example.test:team/repo.git")
	if err != nil {
		t.Fatalf("ssh auth: %v", err)
	}
	if method == nil {
		t.Fatal("expected an auth method")
	}
}

func TestAgentAuthFallsBackToXDGRuntimeSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	runtimeDir := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	keyring := agent.NewKeyring()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatalf("add key: %v", err)
	}
	serveAgent(t, keyring, filepath.Join(runtimeDir, "ssh-agent.socket"))

	signers, err := agentSigners()
	if err != nil {
		t.Fatalf("agent signers: %v", err)
	}
	if len(signers) != 1 {
		t.Fatalf("expected the agent's key to be offered, got %d signers", len(signers))
	}
}

func TestAgentAuthNoneAvailable(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if _, err := agentSigners(); err == nil {
		t.Fatal("expected an error when no agent socket is reachable")
	}
}

// shortTempDir keeps unix socket paths inside the 104-byte limit macOS enforces.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func serveAgent(t *testing.T, keyring agent.Agent, sockPath string) {
	t.Helper()
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, conn) }()
		}
	}()
}

func TestSSHAuthWithoutKeysFails(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	if _, err := sshAuth("git@code.example.test:team/repo.git"); err == nil {
		t.Fatal("expected an error when no key is available")
	}
}

func TestSSHAuthOffersEveryDefaultKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestRSAKey(t, filepath.Join(home, ".ssh", "id_rsa"))
	writeTestEd25519Key(t, filepath.Join(home, ".ssh", "id_ed25519"))

	method, err := sshAuth("git@code.example.test:team/repo.git")
	if err != nil {
		t.Fatalf("ssh auth: %v", err)
	}
	cb, ok := method.(*ssh.PublicKeysCallback)
	if !ok {
		t.Fatalf("expected every default key to be offered to the server in one attempt, got %T", method)
	}
	signers, err := cb.Callback()
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if len(signers) != 2 {
		t.Fatalf("expected both default keys to be offered, got %d", len(signers))
	}
}

func writeTestRSAKey(t *testing.T, path string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	writeTestFile(t, path, string(pem.EncodeToMemory(block)))
}

func writeTestEd25519Key(t *testing.T, path string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	writeTestFile(t, path, string(pem.EncodeToMemory(block)))
}
