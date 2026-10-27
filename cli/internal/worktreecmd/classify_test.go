//go:build unix

package worktreecmd

import "testing"

func TestClassifyWorktreeAddErr(t *testing.T) {
	for _, c := range []struct {
		msg, kind string
		code      int
	}{
		{"fatal: 'feat' is already used by worktree at '/x/w'", "branch_in_use_other_worktree", ExitBranchConflict},                                      // git >= 2.42
		{"Preparing worktree (checking out 'feat')\nfatal: 'feat' is already checked out at '/x/w'", "branch_in_use_other_worktree", ExitBranchConflict}, // git 2.39
		{"fatal: '/x/.worktrees/foo' already exists", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: a branch named 'worktree-foo' already exists", "worktree_path_exists", ExitWorktreeExists},
		{"fatal: invalid reference: nope", "git_worktree_add_failed", ExitGitFailed},
	} {
		kind, code, rem := classifyWorktreeAddErr(c.msg, "foo")
		if kind != c.kind || code != c.code || rem == "" {
			t.Errorf("%q -> %s/%d/%q, want %s/%d", c.msg, kind, code, rem, c.kind, c.code)
		}
	}
}
