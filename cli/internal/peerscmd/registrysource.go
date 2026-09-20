//go:build unix

package peerscmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// repoWorktrees lists the resolved root of every worktree of the repo whose main
// checkout is mainRoot (the main checkout first). Resolved once per call site. On a
// git failure it still returns the [mainRoot] fallback (never nothing), but names the
// failure: silently narrowing the worktree set to one is what makes every OTHER live
// worktree's session look foreign (lets-cbmg7 FIX D).
func repoWorktrees(ctx context.Context, mainRoot string) ([]string, *Degraded) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", mainRoot, "worktree", "list", "--porcelain").Output()
	roots := []string{}
	if err != nil {
		if r, err := filepath.EvalSymlinks(mainRoot); err == nil {
			roots = append(roots, r)
		}
		return roots, &Degraded{Source: "git", Reason: "worktrees_unreadable", Detail: err.Error()}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			if r, err := filepath.EvalSymlinks(strings.TrimSpace(p)); err == nil {
				roots = append(roots, r)
			}
		}
	}
	return roots, nil
}

// worktreeOf returns the worktree root that contains cwd (cwd is that root or below
// it, both resolved through symlinks), or "".
func worktreeOf(cwd string, roots []string) string {
	r, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return ""
	}
	best := ""
	for _, root := range roots {
		if r == root || strings.HasPrefix(r, root+string(filepath.Separator)) {
			if len(root) > len(best) { // a nested worktree wins over its parent
				best = root
			}
		}
	}
	return best
}

// registryPeers reads the registry and keeps the entries whose cwd lies in a
// worktree of this repo. A degraded registry, or a worktree list this repo could not
// read (repoWorktrees then falls back to just mainRoot), still contributes its
// readable rows - never silently, so a narrowed worktree set is named in degraded[]
// rather than making every other worktree's session look like a foreign repo. The
// resolved roots are returned too, so a caller never re-derives them with a second
// `git worktree list` call of its own.
func registryPeers(ctx context.Context, mainRoot string) ([]ccregistry.Entry, map[string]string, []string, ccregistry.Snapshot, []Degraded) {
	snap := ccregistry.Read(ccregistry.HomeDir())
	roots, wtd := repoWorktrees(ctx, mainRoot)
	var kept []ccregistry.Entry
	rootOf := map[string]string{}
	for _, e := range snap.Entries {
		if wt := worktreeOf(e.Cwd, roots); wt != "" {
			kept = append(kept, e)
			rootOf[e.SessionID] = wt
		}
	}
	var degraded []Degraded
	if snap.Degraded != nil {
		degraded = append(degraded, Degraded{Source: "claude", Reason: snap.Degraded.Reason, Detail: snap.Degraded.Detail})
	}
	if wtd != nil {
		degraded = append(degraded, *wtd)
	}
	return kept, rootOf, roots, snap, degraded
}
