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

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/peerscmd"
	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// selfHealLockDeadline bounds how long a session start waits for another adopt
// (the orca.yaml setup hook may be running the same adopt right now).
var selfHealLockDeadline = 3 * time.Second

// selfHealFn is the SessionStart self-heal; a var so tests can spy on it.
var selfHealFn = selfHeal

// selfHeal adopts an unlinked linked worktree of an initialized LETS project
// before LETS Config is built - an Orca worktree whose setup hook did not run (GUI
// PATH, async hook, uncommitted orca.yaml) otherwise gets a Config from a .lets/.env
// that does not exist yet. Gates run cheapest first, and any miss returns "" with
// no file touched. It returns a Notice message only when adopt failed or waited out
// its lock; a successful adopt says nothing (the Config that follows is proof).
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
	// 2. not linked yet
	if fi, err := os.Lstat(filepath.Join(root, ".lets")); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return ""
	}
	// 3. an initialized LETS project: a repo that never ran `lets init` is left
	//    alone even with a user-scoped plugin
	if _, err := os.Stat(filepath.Join(mainRoot, ".lets", ".env")); err != nil {
		return ""
	}
	// 4. not an agent worktree (/lets:team, Agent isolation)
	if under(root, filepath.Join(mainRoot, ".claude", "worktrees")) {
		return ""
	}
	pluginRoot := ""
	if rulesPath != "" {
		pluginRoot = filepath.Dir(filepath.Dir(rulesPath)) // <plugin>/rules/lets-rules.md
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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

// peersHealFn restores this session's peer role on SessionStart; a var so tests spy.
var peersHealFn = peersHeal

// peersHeal carries this session's role to a re-minted id and restores it from its
// anchor, the moment the harness hands the session its id (startup, resume, /clear).
// Best-effort like selfHeal: bounded, silent, and skipped in a repo that never used
// peers - the lazy path in every `lets peers` call covers whatever this misses (the
// registry may not show the new id yet when the hook fires).
func peersHeal(root, sid string) {
	if root == "" || sid == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".lets", "sessions", "peers")); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	peerscmd.Heal(ctx, root, sid)
}
