//go:build unix

package worktreecmd

import (
	"context"
	"os/exec"
	"strings"
)

// mergeRefs lists the refs a branch can be merged into, preferred first: the
// remote-tracking ref (the local merge-branch lags in a worktree setup, because
// it only moves when someone pulls in the main checkout), then the local ref.
func mergeRefs(merge string) []string {
	return []string{"refs/remotes/origin/" + merge, "refs/heads/" + merge}
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

// mergedUpstream reports whether branch is an ancestor of origin/<merge> (preferred)
// or local <merge>, and names the ref it checked. No implicit fetch. Squash and
// rebase merges are NOT detectable and report false.
func mergedUpstream(ctx context.Context, root, branch, merge string) (bool, string) {
	for _, ref := range mergeRefs(merge) {
		if !refExists(ctx, root, ref) {
			continue
		}
		if exec.CommandContext(ctx, "git", "-C", root, "merge-base", "--is-ancestor", "refs/heads/"+branch, ref).Run() == nil {
			return true, ref
		}
	}
	return false, ""
}

// checkedOutBranches returns the short names of every branch checked out in any
// worktree of root (main checkout included).
func checkedOutBranches(ctx context.Context, root string) map[string]bool {
	out, _ := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if b, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			set[strings.TrimSpace(b)] = true
		}
	}
	return set
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
	checkedOut := checkedOutBranches(ctx, root)
	var tips []string
	for _, ref := range mergeRefs(merge) {
		if tip := refTip(ctx, root, ref); tip != "" {
			tips = append(tips, tip)
		}
	}
	for _, b := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if b == "" || b == merge || checkedOut[b] || !hasAnyPrefix(b, prefixes) {
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

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
