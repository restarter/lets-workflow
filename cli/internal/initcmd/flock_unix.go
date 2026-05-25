//go:build unix

package initcmd

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory file lock via flock(2). Blocks until
// acquired or the descriptor is invalidated. Caller is responsible for
// calling unlockFile (typically via defer).
//
// flock semantics: per-fd (not per-pid), released on close. We rely on this
// for cross-process serialization of .gitignore writes; the lock file is
// .lets/locks/gitignore.lock and outlives the process.
func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
