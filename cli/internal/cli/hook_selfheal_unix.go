//go:build unix

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/peerscmd"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// selfHealLockDeadline bounds how long a session start waits for another adopt
// (the orca.yaml setup hook may be running the same adopt right now).
var selfHealLockDeadline = 3 * time.Second

// selfHealFn is the SessionStart self-heal; a var so tests can spy on it.
var selfHealFn = selfHeal

// selfHeal adopts an unlinked linked worktree of an initialized LETS project before
// LETS Config is built - an Orca worktree whose setup hook did not run (GUI PATH, async
// hook, uncommitted orca.yaml) otherwise gets a Config from a .lets/.env that does not
// exist yet - and then, in every linked worktree (just adopted, or linked before
// lets-urmfa), declares the main .lets/ as an additional directory so the NEXT session
// writes .lets/ without a prompt. Gates run cheapest first, and any miss returns "" with
// no file touched. It returns a Notice only when adopt failed or waited out its lock, or
// when the declaration could not be made (a tracked settings file or a .gitignore
// negation excepted - the project's choice, reported by create / adopt when they run).
func selfHeal(root, rulesPath string) string {
	if root == "" {
		return ""
	}
	// 1. a linked worktree (a .git file with gitdir: + commondir; no fork; a
	//    submodule is not one)
	mainRoot, ok := gitutil.LinkedWorktreeMain(root)
	if !ok {
		return ""
	}
	// 2. an initialized LETS project: a repo that never ran `lets init` is left
	//    alone even with a user-scoped plugin
	if _, err := os.Stat(filepath.Join(mainRoot, ".lets", ".env")); err != nil {
		return ""
	}
	// 3. not an agent worktree (/lets:execute --parallel, Agent isolation)
	if under(root, filepath.Join(mainRoot, ".claude", "worktrees")) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 4. not linked yet: adopt
	if fi, err := os.Lstat(filepath.Join(root, ".lets")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		if notice := adoptNotice(ctx, root, rulesPath); notice != "" {
			return notice
		}
	}
	// 5. linked: the main .lets/ as an additional directory (lets-urmfa). After an adopt
	//    that already declared it this is one read and one check-ignore.
	if st, kind := worktreecmd.EnsureLetsAdditionalDir(ctx, root, mainRoot); st.Status == worktreecmd.StepWarn && kind != worktreecmd.SettingsLocalTracked && kind != worktreecmd.SettingsLocalNotIgnored {
		return st.Message
	}
	return ""
}

// adoptNotice runs the self-heal's adopt: "" when it linked the worktree, else the
// Notice (lock busy, or the failure with its remediation).
func adoptNotice(ctx context.Context, root, rulesPath string) string {
	pluginRoot := ""
	if rulesPath != "" {
		pluginRoot = filepath.Dir(filepath.Dir(rulesPath)) // <plugin>/rules/lets-rules.md
	}
	res, err := worktreecmd.Adopt(ctx, root, worktreecmd.AdoptOptions{PluginRoot: pluginRoot, LockDeadline: selfHealLockDeadline})
	if err == nil && res != nil && res.OK {
		return ""
	}
	var e *worktreecmd.Error
	if errors.As(err, &e) && e.Kind == "adopt_lock_busy" {
		return fmt.Sprintf("adopt skipped: %s - /lets:start retries", e.Message)
	}
	kind, msg, remediation := "adopt_failed", "", ""
	if res != nil && res.Error != nil {
		kind, msg, remediation = res.Error.Kind, res.Error.Message, res.Error.Remediation
	} else if err != nil {
		msg = err.Error()
	}
	out := fmt.Sprintf("This worktree is not linked to the LETS project yet and adopt failed (%s): %s", kind, msg)
	if remediation != "" {
		out += " - " + remediation
	}
	return out + ". Nothing was deleted or forced."
}

// under reports whether path lies inside dir (both resolved through symlinks).
func under(path, dir string) bool {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if d, err := filepath.EvalSymlinks(dir); err == nil {
		dir = d
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// peersHealFn carries the session proof on SessionStart; a var so tests spy.
var peersHealFn = peersHeal

// peersHeal carries the session proof (peerscmd.CarryProof) the moment the harness
// hands the session its id (startup, resume, /clear): a proof whose id was re-minted
// in the same process moves to the new id, a resumed one gets its new pid, so the
// next /clear is provable too. Best-effort like selfHeal: bounded, silent, and
// skipped in a repo that never wrote a proof - the lazy carry in every `lets peers
// who`, `orchestrator` and `role set` covers whatever this misses (the registry may
// not show the new id yet when the hook fires). lets-gbfpd replaces it.
func peersHeal(root, sid string) {
	if root == "" || sid == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".lets", "sessions", "peers")); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	peerscmd.CarryProof(ctx, root)
}

// sessionGuardFn builds the task-state session guard for the session sid; a var so
// tests spy.
var sessionGuardFn = sessionGuard

// sessionGuard judges from ONE registry snapshot: liveness, a re-mint proven by the
// stored session_proof, a parked session continued in THIS caller (lets-56ymc), and
// the caller's own proof. No registry at all keeps the unguarded refresh.
func sessionGuard(sid string) taskstate.SessionGuard {
	snap := ccregistry.Read(ccregistry.HomeDir())
	if snap.Degraded != nil && snap.Degraded.Reason == "registry_absent" {
		return taskstate.SessionGuard{}
	}
	return taskstate.SessionGuard{
		Liveness: func(s string) taskstate.Liveness { return sessionLiveness(snap, s) },
		Rotated: func(s string, p taskstate.Proof) (string, bool) {
			if snap.ContinuedIn(s, sid) {
				return sid, true
			}
			if p.Session != s || p.Pid <= 0 {
				return "", false
			}
			if !snap.SameProcess(p.Pid, p.Started.UnixMilli()) {
				return "", false // the proof's process is gone or the pid was reused
			}
			return snap.Rotated(s, p.Pid, p.Started)
		},
		Self: func(own string) (taskstate.Proof, bool) {
			e, ok := snap.Find(own)
			if !ok || e.Pid <= 0 || e.StartedAt <= 0 {
				return taskstate.Proof{}, false // registry lag: no proof now, a later Go write fills it
			}
			return taskstate.Proof{Session: own, Pid: e.Pid, Started: time.UnixMilli(e.StartedAt)}, true
		},
	}
}

// sessionLiveness: a session runs when the registry lists it, under peerProtocol 1
// or (Loose) another one. Not listed is dead only when the registry was read in
// full; an unparseable live entry might be it, and any other degraded read proves
// nothing - both unknown, so the guard holds.
func sessionLiveness(snap ccregistry.Snapshot, sid string) taskstate.Liveness {
	if _, ok := snap.Find(sid); ok {
		return taskstate.LiveAlive
	}
	if _, ok := snap.Loose[sid]; ok {
		return taskstate.LiveAlive
	}
	if snap.Degraded == nil {
		return taskstate.LiveDead
	}
	if snap.Degraded.Reason != "registry_protocol_unknown" {
		return taskstate.LiveUnknown
	}
	for _, why := range snap.Unrecognized {
		if why == "unparseable" {
			return taskstate.LiveUnknown
		}
	}
	return taskstate.LiveDead
}
