//go:build unix

package peerscmd

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
)

// The peers dir <root>/.lets/sessions/peers holds the last-seen files of
// orchestrators (lastseen.go) under .lets/locks/peers.lock. Legacy <sid>.role files
// an older binary wrote there are never read or written: the /clear proof is the
// task-state session_proof key (lets-gbfpd).

var now = time.Now

func peersDir(root string) string { return filepath.Join(root, ".lets", "sessions", "peers") }

var errPeersLockBusy = errors.New("peers_lock_busy")

// lockPeers takes .lets/locks/peers.lock, blocking or until deadline.
func lockPeers(root string, deadline time.Duration) (func(), error) {
	dir := filepath.Join(root, ".lets", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "peers.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if deadline > 0 {
		err = fsutil.TryLockFile(f, now().Add(deadline))
	} else {
		err = fsutil.LockFile(f)
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, errPeersLockBusy
		}
		return nil, err
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

func session6(sid string) string {
	if len(sid) >= 6 {
		return sid[:6]
	}
	return sid
}
