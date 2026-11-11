//go:build unix

package worktreecmd

import "testing"

func TestClassifyWorktreeAddErr(t *testing.T) {
	const wt = "/x/.worktrees/foo"
	for _, c := range []struct {
		msg, branch, kind string
		code              int
	}{
		{"fatal: 'feat' is already used by worktree at '/x/w'", "feat", "branch_in_use_other_worktree", ExitBranchConflict},                                      // git >= 2.42
		{"Preparing worktree (checking out 'feat')\nfatal: 'feat' is already checked out at '/x/w'", "feat", "branch_in_use_other_worktree", ExitBranchConflict}, // git 2.39
		{"fatal: '/x/.worktrees/foo' already exists", "worktree-foo", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: a branch named 'worktree-foo' already exists", "worktree-foo", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: invalid reference: nope", "worktree-foo", "git_worktree_add_failed", ExitGitFailed},
		{"cache already exists\nhook failed", "worktree-foo", "git_worktree_add_failed", ExitGitFailed},                                                        // hook output, not git's
		{"Preparing worktree (new branch 'x')\nwarning: index already exists\nfatal: hook exited 1", "worktree-foo", "git_worktree_add_failed", ExitGitFailed}, // only fatal: lines count
		{"fatal: a branch named 'cache' already exists\nhook failed", "worktree-foo", "git_worktree_add_failed", ExitGitFailed},                                // a hook's own git call
		{"fatal: '/elsewhere' already exists", "worktree-foo", "git_worktree_add_failed", ExitGitFailed},                                                       // another path
		{"fatal: 'other' is already checked out at '/x/w'", "feat", "git_worktree_add_failed", ExitGitFailed},
		{"fatal: '/x/.worktrees/foo' is a missing but already registered worktree;\nuse 'add -f' to override, or 'prune' or 'remove' to clear", "worktree-foo", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: '/x/.worktrees/foo' is a missing but locked worktree;\nuse 'add -f -f' to override, or 'unlock' and 'prune' or 'remove' to clear", "worktree-foo", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: '/elsewhere' is a missing but already registered worktree;", "worktree-foo", "git_worktree_add_failed", ExitGitFailed}, // another branch
	} {
		kind, code, rem := classifyWorktreeAddErr(c.msg, "foo", wt, c.branch)
		if kind != c.kind || code != c.code || rem == "" {
			t.Errorf("%q (branch %s) -> %s/%d/%q, want %s/%d", c.msg, c.branch, kind, code, rem, c.kind, c.code)
		}
	}
}
