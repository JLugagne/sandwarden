package fleet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrLocked reports that another sandwarden instance is mutating the same
// configuration directory. Callers match it with errors.Is and surface the
// retry hint unchanged.
var ErrLocked = errors.New("another sandwarden instance is applying changes, retry")

// lockFile is the advisory lock shared by every sandwarden process using a
// configuration directory, GUI and CLI alike. It sits next to the YAML files
// so one lock covers sandboxes, profiles, caches, stores and config.yaml.
const lockFile = ".lock"

// lockPollInterval is how often a competing flock is retried before the
// bounded wait expires.
const lockPollInterval = 10 * time.Millisecond

// errLockBusy is the contention sentinel the platform flock helpers return.
var errLockBusy = errors.New("lock busy")

// Lock is a held advisory lock on a fleet directory. Read paths never take it.
type Lock struct {
	file *os.File
	gate chan struct{}

	once sync.Once
	err  error
}

// AcquireLock takes the fleet lock of dir, waiting up to wait for a competing
// process. A zero or negative wait makes it a single non-blocking try. On
// contention the returned error matches ErrLocked.
//
// Goroutines of this process queue on an in-process gate, processes contend on
// flock(2) over <dir>/.lock. AcquireLock is not reentrant: a nested call from
// the same goroutine would wait on its own holder, so locked entry points call
// unlocked internals (see docs/locking.md).
func AcquireLock(dir string, wait time.Duration) (*Lock, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(dir, lockFile)
	key, err := filepath.Abs(path)
	if err != nil {
		key = path
	}

	gate := lockGate(key)
	deadline := time.Now().Add(wait)
	if !lockGateWait(gate, wait) {
		return nil, ErrLocked
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		<-gate
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockFileWait(file, deadline); err != nil {
		_ = file.Close()
		<-gate
		if errors.Is(err, errLockBusy) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return &Lock{file: file, gate: gate}, nil
}

// Release drops the lock. It is safe to call more than once.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		unlockErr := flockUnlock(l.file)
		closeErr := l.file.Close()
		<-l.gate
		if unlockErr != nil {
			l.err = unlockErr
			return
		}
		l.err = closeErr
	})
	return l.err
}

// lockGates serializes acquisitions inside one process, keyed by absolute lock
// path, so goroutines queue instead of fighting over the file lock.
var lockGates = struct {
	sync.Mutex
	m map[string]chan struct{}
}{m: map[string]chan struct{}{}}

// lockGate returns the in-process gate of one lock path, creating it once.
func lockGate(key string) chan struct{} {
	lockGates.Lock()
	defer lockGates.Unlock()
	gate, ok := lockGates.m[key]
	if !ok {
		gate = make(chan struct{}, 1)
		lockGates.m[key] = gate
	}
	return gate
}

// lockGateWait takes the in-process gate, giving up after wait.
func lockGateWait(gate chan struct{}, wait time.Duration) bool {
	if wait <= 0 {
		select {
		case gate <- struct{}{}:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case gate <- struct{}{}:
		return true
	case <-timer.C:
		return false
	}
}

// lockFileWait retries the platform flock until deadline.
func lockFileWait(file *os.File, deadline time.Time) error {
	for {
		err := flockTry(file)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errLockBusy) {
			return err
		}
		if !time.Now().Before(deadline) {
			return errLockBusy
		}
		time.Sleep(lockPollInterval)
	}
}
