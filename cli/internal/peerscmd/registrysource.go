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
// checkout is mainRoot (the main checkout first). Resolved once per call site.
func repoWorktrees(ctx context.Context, mainRoot string) []string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", mainRoot, "worktree", "list", "--porcelain").Output()
	roots := []string{}
	if err != nil {
		if r, err := filepath.EvalSymlinks(mainRoot); err == nil {
			roots = append(roots, r)
		}
		return roots
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			if r, err := filepath.EvalSymlinks(strings.TrimSpace(p)); err == nil {
				roots = append(roots, r)
			}
		}
	}
	return roots
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
// worktree of this repo. A degraded registry still contributes its readable rows.
func registryPeers(ctx context.Context, mainRoot string) ([]ccregistry.Entry, map[string]string, ccregistry.Snapshot, *Degraded) {
	snap := ccregistry.Read(ccregistry.HomeDir())
	roots := repoWorktrees(ctx, mainRoot)
	var kept []ccregistry.Entry
	rootOf := map[string]string{}
	for _, e := range snap.Entries {
		if wt := worktreeOf(e.Cwd, roots); wt != "" {
			kept = append(kept, e)
			rootOf[e.SessionID] = wt
		}
	}
	var d *Degraded
	if snap.Degraded != nil {
		d = &Degraded{Source: "claude", Reason: snap.Degraded.Reason, Detail: snap.Degraded.Detail}
	}
	return kept, rootOf, snap, d
}
