//go:build unix

package fleet

import (
	"errors"
	"os"
	"syscall"
)

// flockTry attempts a non-blocking exclusive flock on file, the cross-process
// half of AcquireLock.
func flockTry(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errLockBusy
	}
	return err
}

// flockUnlock releases the flock on file.
func flockUnlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
