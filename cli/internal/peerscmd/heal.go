//go:build unix

package peerscmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

// HealSelf carries re-minted roles and, when the calling session has no role file
// of its own, restores one from its anchor - whatever lost it (a re-minted id the
// reconcile could not prove, a new agent after a crash, a harness duplicate):
//   - worker: the task: of its branch's .task-<slug> (never on the merge-branch);
//   - orchestrator: a name the USER set, no live holder of it, and exactly one dead
//     role file or last-seen record under it (a derived name proves nothing).
//
// The view rc was loaded with only decides whether to take peers.lock at all; every
// write is decided again from the files and registry read under the lock, so two
// sessions healing at once cannot both claim one name, and a move planned earlier
// never overwrites a role written since. Own repo only. Best-effort: a busy lock or
// a failed write is reported in the returned degraded entries.
func HealSelf(rc *repoContext, sid, cwd, branch string) []Degraded {
	restored, _, _, ambiguous := planRestore(rc.root, rc.roles, rc.snap, sid, cwd, branch)
	if len(rc.moves) == 0 && restored == nil {
		return ambiguousDegraded(ambiguous)
	}
	unlock, err := lockPeers(rc.root, healLockDeadline)
	if err != nil {
		return []Degraded{{Source: "roles", Reason: "peers_lock_busy"}}
	}
	defer unlock()
	files, _, snap, err := reconcileLocked(rc.root)
	rc.roles, rc.moves, rc.peersCache = files, nil, nil
	if err != nil {
		return []Degraded{{Source: "roles", Reason: "role_write_failed", Detail: err.Error()}}
	}
	restored, stale, lastSeen, ambiguous := planRestore(rc.root, files, snap, sid, cwd, branch)
	out := ambiguousDegraded(ambiguous)
	if restored == nil {
		return out
	}
	if err := writeRole(rc.root, *restored); err != nil {
		return append(out, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
	}
	if stale != "" {
		delete(files, stale)
		_ = os.Remove(filepath.Join(peersDir(rc.root), stale+".role"))
	}
	files[sid] = *restored
	if lastSeen != "" {
		if err := os.Remove(lastSeen); err != nil && !errors.Is(err, os.ErrNotExist) {
			out = append(out, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
		}
	}
	return out
}

// ambiguousDegraded reports a restore that no anchor could decide; detail carries
// the remedy, "" means nothing was ambiguous.
func ambiguousDegraded(detail string) []Degraded {
	if detail == "" {
		return nil
	}
	return []Degraded{{Source: "roles", Reason: "reclaim_ambiguous", Detail: detail}}
}

// planRestore decides, without writing, which role the caller gets back: the role
// file to write, the dead holder's file it replaces, the last-seen file it consumes.
// ambiguous is the remedy when no anchor can decide ("" otherwise).
func planRestore(root string, roles map[string]roleFile, snap ccregistry.Snapshot, sid, cwd, branch string) (restored *roleFile, stale, lastSeen, ambiguous string) {
	if _, has := roles[sid]; has {
		return nil, "", "", ""
	}
	self, ok := snap.Find(sid)
	if !ok {
		return nil, "", "", ""
	}
	// A file still rotating to sid after reconcile is a tie movesTo refused to break:
	// this process held several roles under earlier ids, set in the same second, and
	// its own history cannot say which is current - so no anchor may pick one either.
	for osid, f := range roles {
		if set, err := time.Parse(time.RFC3339, f.Set); err == nil {
			if to, ok := snap.Rotated(osid, f.Pid, set); ok && to == sid {
				return nil, "", "", "this session held several roles under earlier session ids, registered in the same second - /lets:start <id> (worker) or /lets:start --main (orchestrator) registers the one you mean"
			}
		}
	}
	base := roleFile{Session: sid, Name: self.Name, Pid: self.Pid, Cwd: cwd, OrcaTerminal: os.Getenv(orcaTerminalVar),
		Set: now().UTC().Format(time.RFC3339), path: filepath.Join(peersDir(root), sid+".role")}
	if task, holder := branchTask(root, branch); task != "" {
		// The task-state session: names who owns this branch's task. A live foreign
		// holder (a teammate pane opened in the lead's worktree) gets no worker role -
		// it would be listed as the task's worker to the orchestrator.
		if _, live := snap.Find(holder); live && holder != sid {
			return nil, "", "", ""
		}
		base.Role, base.Task = "worker", task
		return &base, "", "", ""
	}
	if self.NameSource != "user" || !ccregistry.ValidName(self.Name) {
		return nil, "", "", ""
	}
	var dead []roleFile
	for osid, f := range roles {
		if f.Role != "orchestrator" {
			continue
		}
		live := snap.Liveness(osid, f.Pid) != ccregistry.Dead
		if live && liveName(snap, f) == self.Name {
			return nil, "", "", "" // a live holder: name_held territory, never silent
		}
		if !live && f.Name == self.Name {
			dead = append(dead, f)
		}
	}
	base.Role = "orchestrator"
	switch {
	case len(dead) > 1:
		return nil, "", "", "several dead orchestrators were registered as " + self.Name + " - /lets:start --main registers this session"
	case len(dead) == 1:
		base.Scope = dead[0].Scope
		return &base, dead[0].Session, lastSeenFile(peersDir(root), self.Name), ""
	}
	p := lastSeenFile(peersDir(root), self.Name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, "", "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, ": "); ok && k == "scope" {
			base.Scope = cleanScope(v)
		}
	}
	return &base, "", p, ""
}

// branchTask is the task: of branch's .task-<slug> and the session id its session:
// line records ("" when none); task is "" on the merge-branch, a detached HEAD, or a
// missing / invalid id. A plain read, no lock: the hook's hot path.
func branchTask(root, branch string) (task, holder string) {
	if branch == "" {
		return "", ""
	}
	home, _ := os.UserHomeDir()
	if branch == letsconfig.ResolvedEnv(root, home, nil)["LETS_MERGE_BRANCH"] {
		return "", ""
	}
	slug, ok := taskstate.Slug(branch)
	if !ok {
		return "", ""
	}
	st, err := taskstate.Read(filepath.Join(root, ".lets"), slug)
	if err != nil || !taskid.Valid(st.Task) {
		return "", ""
	}
	return st.Task, taskstate.StoredSession(st)
}

// RoleProof returns the pid and set time sid's role file recorded - the proof a
// later session id is the same process re-minted (ccregistry.Snapshot.Rotated).
// Read-only: the SessionStart hook reads it BEFORE peersHeal, whose reconcile moves
// a re-minted role file to the new id and deletes the old one.
func RoleProof(root, sid string) (pid int, set time.Time, ok bool) {
	if !ccregistry.ValidSession(sid) {
		return 0, time.Time{}, false
	}
	data, err := os.ReadFile(filepath.Join(peersDir(root), sid+".role"))
	if err != nil {
		return 0, time.Time{}, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, cut := strings.Cut(line, ": ")
		switch {
		case !cut:
		case k == "pid":
			pid, _ = strconv.Atoi(strings.TrimSpace(v))
		case k == "set":
			set, _ = time.Parse(time.RFC3339, strings.TrimSpace(v))
		}
	}
	return pid, set, pid > 0 && !set.IsZero()
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
