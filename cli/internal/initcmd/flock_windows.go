//go:build windows

package initcmd

import "os"

// Best-effort no-op on Windows. Concurrent .gitignore writes are not
// serialized; in practice the LETS flow does not run two `lets`
// invocations against the same repo simultaneously. If this becomes a
// real problem, switch to LockFileEx via golang.org/x/sys/windows.
func lockFile(f *os.File) error { _ = f; return nil }

func unlockFile(f *os.File) error { _ = f; return nil }
