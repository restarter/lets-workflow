//go:build unix

package worktreecmd

import (
	"context"
	"os/exec"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// mergeRefs lists the refs a branch can be merged into, preferred first: the
// remote-tracking ref of the remote gitutil.ResolveRemote picks (no fetch; the
// local merge-branch lags in a worktree setup, because it only moves when someone
// pulls in the main checkout), then the local ref. No remote, or an ambiguous one,
// is the local ref alone - never a refusal (lets-57lb9).
func mergeRefs(ctx context.Context, root, merge string) []string {
	if r, refusal := gitutil.ResolveRemote(ctx, root, merge); refusal == nil && r.Name != "" {
		return []string{"refs/remotes/" + r.Name + "/" + merge, "refs/heads/" + merge}
	}
	return []string{"refs/heads/" + merge}
}

// mergeRefFor is the first of mergeRefs that resolves, in short form; merge itself
// when none does.
func mergeRefFor(ctx context.Context, root, merge string) string {
	for _, ref := range mergeRefs(ctx, root, merge) {
		if refExists(ctx, root, ref) {
			return shortRef(ref)
		}
	}
	return merge
}

func shortRef(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, "refs/remotes/"), "refs/heads/")
}

// refExists reports whether ref resolves in root.
func refExists(ctx context.Context, root, ref string) bool {
	return exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", ref).Run() == nil
}

// refTip returns the commit a ref points at, or "" when it does not resolve.
func refTip(ctx context.Context, root, ref string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// mergedUpstream reports whether branch is an ancestor of the resolved remote's
// <merge> (preferred) or local <merge>, and names the ref it checked. No implicit
// fetch. Squash and rebase merges are NOT detectable and report false.
func mergedUpstream(ctx context.Context, root, branch, merge string) (bool, string) {
	for _, ref := range mergeRefs(ctx, root, merge) {
		if !refExists(ctx, root, ref) {
			continue
		}
		if exec.CommandContext(ctx, "git", "-C", root, "merge-base", "--is-ancestor", "refs/heads/"+branch, ref).Run() == nil {
			return true, ref
		}
	}
	return false, ""
}

// checkedOutBranches maps every branch checked out in a worktree of root (main
// checkout included) to that worktree's path, from `git worktree list --porcelain`
// (version- and locale-independent). ok=false when the list failed - a caller
// never reads that as "nothing is checked out".
func checkedOutBranches(ctx context.Context, root string) (map[string]string, bool) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, false
	}
	held := map[string]string{}
	path := ""
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = strings.TrimSpace(p)
		}
		if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			held[strings.TrimSpace(b)] = path
		}
	}
	return held, true
}

// SweepCandidates splits the local branches that start with one of prefixes into
// merged and unmerged (per mergedUpstream). It skips the merge branch itself,
// every branch checked out in a worktree, and a branch whose tip equals a merge
// ref (never diverged: nothing was merged, so deleting it would lose nothing but
// also prove nothing).
func SweepCandidates(ctx context.Context, root, merge string, prefixes []string) (merged, unmerged []string) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "for-each-ref", "--format=%(refname:short)", "refs/heads/").Output()
	if err != nil {
		return nil, nil
	}
	checkedOut, _ := checkedOutBranches(ctx, root)
	var tips []string
	for _, ref := range mergeRefs(ctx, root, merge) {
		if tip := refTip(ctx, root, ref); tip != "" {
			tips = append(tips, tip)
		}
	}
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if _, held := checkedOut[b]; b == "" || b == merge || held || !hasAnyPrefix(b, prefixes) {
			continue
		}
		tip := refTip(ctx, root, "refs/heads/"+b)
		neverDiverged := false
		for _, t := range tips {
			if tip == t {
				neverDiverged = true
				break
			}
		}
		if neverDiverged {
			continue
		}
		if ok, _ := mergedUpstream(ctx, root, b, merge); ok {
			merged = append(merged, b)
		} else {
			unmerged = append(unmerged, b)
		}
	}
	return merged, unmerged
}

// hasAnyPrefix: an empty prefix matches every branch (a convention template that
// starts with {id} has no literal pre-filter; the caller then filters by the
// convention itself).
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
