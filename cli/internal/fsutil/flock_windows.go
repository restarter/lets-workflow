//go:build windows

package fsutil

import (
	"errors"
	"os"
	"time"
)

// ErrLockBusy mirrors the unix sentinel so callers compile on every platform.
var ErrLockBusy = errors.New("lock busy")

// Best-effort no-ops on Windows. Concurrent writers are not serialized; in
// practice the LETS flow does not run two `lets` invocations against the same
// repo simultaneously. If this becomes a real problem, switch to LockFileEx via
// golang.org/x/sys/windows.
func LockFile(f *os.File) error { _ = f; return nil }

func UnlockFile(f *os.File) error { _ = f; return nil }

func TryLockFile(f *os.File, deadline time.Time) error { _, _ = f, deadline; return nil }
