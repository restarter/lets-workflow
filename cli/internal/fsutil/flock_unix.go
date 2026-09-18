//go:build unix

// Package fsutil holds small filesystem helpers shared by several packages
// (advisory file locks). Leaf package: standard library only.
package fsutil

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// ErrLockBusy: TryLockFile gave up at its deadline because another process
// (or another descriptor in this one) holds the lock.
var ErrLockBusy = errors.New("lock busy")

// LockFile takes an exclusive advisory file lock via flock(2). Blocks until
// acquired or the descriptor is invalidated. The caller releases it with
// UnlockFile (typically via defer).
//
// flock semantics: per-fd (not per-pid), released on close. Callers rely on this
// for cross-process serialization; the lock files live under .lets/locks/ and
// outlive the process.
func LockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

// UnlockFile releases a lock taken by LockFile or TryLockFile.
func UnlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

// TryLockFile takes the lock without blocking, retrying every 50 ms until
// deadline, then returns ErrLockBusy. A zero deadline tries exactly once.
func TryLockFile(f *os.File, deadline time.Time) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if !time.Now().Add(50 * time.Millisecond).Before(deadline) {
			return ErrLockBusy
		}
		time.Sleep(50 * time.Millisecond)
	}
}
