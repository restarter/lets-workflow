//go:build unix

package worktreecmd_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// initRepoWithOrigin returns a repo on main with a bare `origin` that has main pushed.
func initRepoWithOrigin(t *testing.T) string {
	t.Helper()
	repo := initRepo(t)
	bare := filepath.Join(realTempDir(t), "origin.git")
	runIn(t, repo, "git", "init", "--bare", bare)
	runIn(t, repo, "git", "remote", "add", "origin", bare)
	runIn(t, repo, "git", "push", "-q", "origin", "main")
	runIn(t, repo, "git", "fetch", "-q", "origin")
	return repo
}

// commitOnBranch creates branch from start (or checks it out) in a scratch worktree
// and adds one empty commit, leaving the main checkout untouched.
func commitOnBranch(t *testing.T, repo, branch, start, msg string) {
	t.Helper()
	wt := filepath.Join(realTempDir(t), "wt")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", branch, wt, start)
	runIn(t, wt, "git", "commit", "-q", "--allow-empty", "-m", msg)
	runIn(t, repo, "git", "worktree", "remove", "--force", wt)
}

func TestMergedUpstream_OriginAheadOfLocalMain(t *testing.T) {
	repo := initRepoWithOrigin(t)
	commitOnBranch(t, repo, "feature/a", "main", "a")
	runIn(t, repo, "git", "push", "-q", "origin", "feature/a:main") // merged upstream; local main lags
	runIn(t, repo, "git", "fetch", "-q", "origin")

	ok, ref := worktreecmd.MergedUpstreamForTesting(context.Background(), repo, "feature/a", "main")
	if !ok || ref != "refs/remotes/origin/main" {
		t.Fatalf("mergedUpstream = %v, %q; want true, refs/remotes/origin/main", ok, ref)
	}
}

func TestMergedUpstream_Unmerged(t *testing.T) {
	repo := initRepoWithOrigin(t)
	commitOnBranch(t, repo, "feature/b", "main", "b")
	if ok, ref := worktreecmd.MergedUpstreamForTesting(context.Background(), repo, "feature/b", "main"); ok {
		t.Fatalf("unmerged branch reported merged into %q", ref)
	}
}

func TestMergedUpstream_SquashMergeNotDetected(t *testing.T) {
	repo := initRepoWithOrigin(t)
	commitOnBranch(t, repo, "feature/c", "main", "c")
	wt := filepath.Join(realTempDir(t), "squash")
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "tmp-squash", wt, "origin/main")
	runIn(t, wt, "git", "merge", "--squash", "feature/c")
	runIn(t, wt, "git", "commit", "-q", "--allow-empty", "-m", "squash c")
	runIn(t, wt, "git", "push", "-q", "origin", "tmp-squash:main")
	runIn(t, repo, "git", "worktree", "remove", "--force", wt)
	runIn(t, repo, "git", "fetch", "-q", "origin")

	if ok, _ := worktreecmd.MergedUpstreamForTesting(context.Background(), repo, "feature/c", "main"); ok {
		t.Fatal("a squash-merged branch must report false (not detectable)")
	}
}

func TestSweepCandidates(t *testing.T) {
	repo := initRepoWithOrigin(t)
	ctx := context.Background()
	commitOnBranch(t, repo, "feature/merged", "main", "m")
	runIn(t, repo, "git", "push", "-q", "origin", "feature/merged:main")
	// main moves on after the merge; a branch sitting exactly at the merge tip is
	// indistinguishable from one that never diverged, and is skipped (feature/at-tip).
	commitOnBranch(t, repo, "tmp-after", "feature/merged", "after")
	runIn(t, repo, "git", "push", "-q", "origin", "tmp-after:main")
	runIn(t, repo, "git", "branch", "-D", "tmp-after")
	runIn(t, repo, "git", "fetch", "-q", "origin")
	commitOnBranch(t, repo, "worktree-unmerged", "origin/main", "u")
	runIn(t, repo, "git", "branch", "feature/at-tip", "origin/main") // never diverged
	commitOnBranch(t, repo, "feature/checked-out", "main", "co")
	runIn(t, repo, "git", "push", "-q", "origin", "feature/checked-out:refs/heads/side")
	wt := filepath.Join(realTempDir(t), "live")
	runIn(t, repo, "git", "worktree", "add", "-q", wt, "feature/checked-out")
	commitOnBranch(t, repo, "release/1.0", "main", "r") // outside the prefixes

	merged, unmerged := worktreecmd.SweepCandidates(ctx, repo, "main", []string{"feature/", "worktree-"})
	if !slices.Equal(merged, []string{"feature/merged"}) {
		t.Errorf("merged = %v, want [feature/merged]", merged)
	}
	if !slices.Equal(unmerged, []string{"worktree-unmerged"}) {
		t.Errorf("unmerged = %v, want [worktree-unmerged]", unmerged)
	}
	all := strings.Join(append(merged, unmerged...), " ")
	for _, never := range []string{"feature/at-tip", "feature/checked-out", "release/1.0", "main"} {
		if strings.Contains(" "+all+" ", " "+never+" ") {
			t.Errorf("%s must not be a sweep candidate (got %s)", never, all)
		}
	}
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "feature/merged").Output()
	if len(out) == 0 {
		t.Error("SweepCandidates must not delete anything")
	}
}
