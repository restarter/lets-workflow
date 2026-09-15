//go:build unix

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTryLockFile_BusyThenFree(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	a, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()

	if err := LockFile(a); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := TryLockFile(b, time.Now().Add(100*time.Millisecond)); !errors.Is(err, ErrLockBusy) {
		t.Fatalf("held lock: err = %v, want ErrLockBusy", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("TryLockFile overran its deadline")
	}
	if err := UnlockFile(a); err != nil {
		t.Fatal(err)
	}
	if err := TryLockFile(b, time.Now().Add(100*time.Millisecond)); err != nil {
		t.Fatalf("free lock: %v", err)
	}
	_ = UnlockFile(b)
}
