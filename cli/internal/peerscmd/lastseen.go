//go:build unix

package peerscmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// The last-seen files: one per orchestrator name under .lets/sessions/peers/last,
// read by the hub to wake or ask an orchestrator that is not running. The resolvers
// (`who`, `orchestrator`) refresh them for every live orchestrator row they see.

// lastSeenLockWait bounds the refresh's wait for peers.lock; a miss is silent.
const lastSeenLockWait = 300 * time.Millisecond

// lastDir holds one last-seen file per orchestrator name.
func lastDir(peers string) string { return filepath.Join(peers, "last") }

// lastSeenFile names a name's file by the lowercase hex of its UTF-8 bytes, so
// "MAIN PWA" and "MAIN_PWA" never share a file and no name reaches a path unescaped.
func lastSeenFile(peers, name string) string {
	return filepath.Join(lastDir(peers), hex.EncodeToString([]byte(name))+".last")
}

// lastSeen is one last-seen record.
type lastSeen struct {
	Session, Name, Scope, Cwd, Set string
	Pid                            int
}

// writeLastSeen records a live orchestrator (so the hub can wake it later),
// atomically, one file per name; a later write of the same name overwrites only it.
func writeLastSeen(peers string, l lastSeen) error {
	if !ccregistry.ValidName(l.Name) {
		return nil
	}
	dir := lastDir(peers)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	body := fmt.Sprintf("session: %s\npid: %d\nname: %s\nscope: %s\ncwd: %s\nset: %s\n", l.Session, l.Pid, l.Name, l.Scope, l.Cwd, l.Set)
	tmp, err := os.CreateTemp(dir, ".last-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	_ = tmp.Chmod(0o600)
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), lastSeenFile(peers, l.Name))
}

// refreshLastSeen writes one last-seen file per orchestrator row of this repo
// (best-effort: a missing .lets, a busy lock or a failed write is silent). pids maps
// a row's session to its registry pid.
func refreshLastSeen(mainRoot string, rows []Peer, pids map[string]int) {
	var orch []Peer
	for _, p := range rows {
		if p.Orchestrator && ccregistry.ValidName(p.Name) && ccregistry.ValidSession(p.Session) {
			orch = append(orch, p)
		}
	}
	if len(orch) == 0 {
		return
	}
	if _, err := os.Stat(filepath.Join(mainRoot, ".lets")); err != nil {
		return // not a LETS project: who never creates .lets
	}
	unlock, err := lockPeers(mainRoot, lastSeenLockWait)
	if err != nil {
		return
	}
	defer unlock()
	set := now().UTC().Format(time.RFC3339)
	for _, p := range orch {
		_ = writeLastSeen(peersDir(mainRoot), lastSeen{Session: p.Session, Pid: pids[p.Session], Name: p.Name, Scope: p.Scope, Cwd: p.Cwd, Set: set})
	}
}

// readLastSeen reads every last-seen file under peers, sorted by file name; a file
// whose name is not a valid peer name is skipped. Read-only.
func readLastSeen(peers string) []LastOrchestrator {
	out := []LastOrchestrator{}
	files, _ := filepath.Glob(filepath.Join(lastDir(peers), "*.last"))
	sort.Strings(files)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		fields := map[string]string{}
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(line, ": "); ok {
				fields[k] = v
			}
		}
		if !ccregistry.ValidName(fields["name"]) {
			continue
		}
		out = append(out, lastOrch(fields["name"], fields["scope"], fields["session"], fields["pid"], "last_seen"))
	}
	return out
}
