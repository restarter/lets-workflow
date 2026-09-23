//go:build unix

package peerscmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// healLockDeadline bounds HealSelf's wait for peers.lock: it runs on hot paths
// (orchestrator, frame, the SessionStart hook) and must never stall them.
var healLockDeadline = time.Second

// HealSelf writes rc's pending moves and, when the calling session has no role file
// of its own, restores one from its anchor - whatever lost it (a re-minted id the
// reconcile could not prove, a new agent after a crash, a harness duplicate):
//   - worker: the task: of its branch's .task-<slug> (never on the merge-branch);
//   - orchestrator: a name the USER set, no live holder of it, and exactly one dead
//     role file or last-seen record under it (a derived name proves nothing).
//
// Own repo only. Best-effort: a busy lock or a failed write is reported in the
// returned degraded entries; rc.roles is corrected in memory either way.
func HealSelf(rc *repoContext, sid, cwd, branch string) []Degraded {
	restored, stale, lastSeen, ambiguous := planRestore(rc, sid, cwd, branch)
	if len(rc.moves) == 0 && restored == nil && !ambiguous {
		return nil
	}
	var out []Degraded
	if ambiguous {
		self, _ := rc.snap.Find(sid)
		out = append(out, Degraded{Source: "roles", Reason: "reclaim_ambiguous",
			Detail: "several dead orchestrators were registered as " + self.Name + " - /lets:start --main registers this session"})
	}
	if restored != nil {
		if stale != "" {
			delete(rc.roles, stale)
		}
		rc.roles[sid] = *restored
		rc.peersCache = nil
	}
	if len(rc.moves) == 0 && restored == nil {
		return out
	}
	unlock, err := lockPeers(rc.root, healLockDeadline)
	if err != nil {
		return append(out, Degraded{Source: "roles", Reason: "peers_lock_busy"})
	}
	defer unlock()
	if err := persistMoves(rc.root, rc.moves); err != nil {
		return append(out, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
	}
	rc.moves = nil
	if restored == nil {
		return out
	}
	if err := writeRole(rc.root, *restored); err != nil {
		return append(out, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
	}
	if stale != "" {
		_ = os.Remove(filepath.Join(peersDir(rc.root), stale+".role"))
	}
	if lastSeen != "" {
		if err := os.Remove(lastSeen); err != nil && !errors.Is(err, os.ErrNotExist) {
			out = append(out, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
		}
	}
	return out
}

// planRestore decides, without writing, which role the caller gets back: the role
// file to write, the dead holder's file it replaces, the last-seen file it consumes.
func planRestore(rc *repoContext, sid, cwd, branch string) (restored *roleFile, stale, lastSeen string, ambiguous bool) {
	if _, has := rc.roles[sid]; has {
		return nil, "", "", false
	}
	self, ok := rc.snap.Find(sid)
	if !ok {
		return nil, "", "", false
	}
	base := roleFile{Session: sid, Name: self.Name, Pid: self.Pid, Cwd: cwd, OrcaTerminal: os.Getenv(orcaTerminalVar),
		Set: now().UTC().Format(time.RFC3339), path: filepath.Join(peersDir(rc.root), sid+".role")}
	if task := branchTask(rc.root, branch); task != "" {
		base.Role, base.Task = "worker", task
		return &base, "", "", false
	}
	if self.NameSource != "user" || !ccregistry.ValidName(self.Name) {
		return nil, "", "", false
	}
	var dead []roleFile
	for osid, f := range rc.roles {
		if f.Role != "orchestrator" {
			continue
		}
		live := rc.snap.Liveness(osid, f.Pid) != ccregistry.Dead
		if live && liveName(rc.snap, f) == self.Name {
			return nil, "", "", false // a live holder: name_held territory, never silent
		}
		if !live && f.Name == self.Name {
			dead = append(dead, f)
		}
	}
	base.Role = "orchestrator"
	switch {
	case len(dead) > 1:
		return nil, "", "", true
	case len(dead) == 1:
		base.Scope = dead[0].Scope
		return &base, dead[0].Session, lastSeenFile(peersDir(rc.root), self.Name), false
	}
	p := lastSeenFile(peersDir(rc.root), self.Name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, "", "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, ": "); ok && k == "scope" {
			base.Scope = cleanScope(v)
		}
	}
	return &base, "", p, false
}

// branchTask is the task: of branch's .task-<slug>; "" on the merge-branch, a
// detached HEAD, or a missing / invalid id.
func branchTask(root, branch string) string {
	if branch == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	if branch == letsconfig.ResolvedEnv(root, home, nil)["LETS_MERGE_BRANCH"] {
		return ""
	}
	slug, ok := taskstate.Slug(branch)
	if !ok {
		return ""
	}
	st, err := taskstate.Read(filepath.Join(root, ".lets"), slug)
	if err != nil || !taskid.Valid(st.Task) {
		return ""
	}
	return st.Task
}

// Heal is the SessionStart hook's entry: load this checkout, heal sid, report nothing.
func Heal(ctx context.Context, cwd, sid string) {
	if !ccregistry.ValidSession(sid) {
		return
	}
	branch := branchOf(ctx, cwd)
	rc, err := loadRepo(ctx, cwd, false)
	if err != nil {
		return
	}
	_ = HealSelf(rc, sid, cwd, branch)
}
