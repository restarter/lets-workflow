//go:build unix

package peerscmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// The last-seen files: one per orchestrator name under .lets/sessions/peers/last,
// read by the hub to wake or ask an orchestrator that is not running. The resolvers
// (`who`, `orchestrator`) refresh them for every live orchestrator they see; the
// file layout (lastDir, lastSeenFile, writeLastSeen) is still roles.go's until the
// role registry is deleted.

// lastSeenLockWait bounds the refresh's wait for peers.lock; a miss is silent.
const lastSeenLockWait = 300 * time.Millisecond

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
		_ = writeLastSeen(peersDir(mainRoot), roleFile{Session: p.Session, Pid: pids[p.Session], Name: p.Name, Scope: p.Scope, Cwd: p.Cwd, Set: set})
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
