package fleet

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const lockHelperDirEnv = "SANDWARDEN_TEST_LOCK_DIR"

func TestAcquireLockCreatesLockFile(t *testing.T) {
	dir := t.TempDir()
	lock, err := AcquireLock(dir, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !fileExists(filepath.Join(dir, lockFile)) {
		t.Fatalf("lock file %s missing", filepath.Join(dir, lockFile))
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestAcquireLockContention(t *testing.T) {
	dir := t.TempDir()
	held, err := AcquireLock(dir, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if _, err := AcquireLock(dir, 0); !errors.Is(err, ErrLocked) {
		t.Fatalf("contended AcquireLock error = %v, want ErrLocked", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	again, err := AcquireLock(dir, 0)
	if err != nil {
		t.Fatalf("AcquireLock after Release: %v", err)
	}
	if err := again.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquireLockSerializes(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireLock(dir, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	second := make(chan error, 1)
	go func() {
		lock, err := AcquireLock(dir, 5*time.Second)
		if err == nil {
			err = lock.Release()
		}
		second <- err
	}()
	select {
	case err := <-second:
		t.Fatalf("second acquisition finished while the first was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second acquisition: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second acquisition did not run after Release")
	}
}

func TestAcquireLockBlocksOtherProcess(t *testing.T) {
	dir := t.TempDir()
	held, err := AcquireLock(dir, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if out := runLockHelper(t, dir); !strings.Contains(out, "helper: locked") {
		t.Fatalf("helper acquired a lock held by this process: %s", out)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if out := runLockHelper(t, dir); !strings.Contains(out, "helper: acquired") {
		t.Fatalf("helper failed to acquire after release: %s", out)
	}
}

func TestAcquireLockHelperProcess(t *testing.T) {
	dir := os.Getenv(lockHelperDirEnv)
	if dir == "" {
		t.Skip("lock helper process")
	}
	lock, err := AcquireLock(dir, 0)
	if errors.Is(err, ErrLocked) {
		fmt.Println("helper: locked")
		return
	}
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	_ = lock.Release()
	fmt.Println("helper: acquired")
}

func runLockHelper(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAcquireLockHelperProcess$")
	cmd.Env = append(os.Environ(), lockHelperDirEnv+"="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lock helper: %v\n%s", err, out)
	}
	return string(out)
}
