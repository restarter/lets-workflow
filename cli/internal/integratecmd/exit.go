//go:build unix

package integratecmd

// Exit codes for `lets integrate` (the 50-59 range; worktreecmd owns 27-34,
// memberscmd 40-49). The range is full: a new failure class reuses 55 with its
// own error.kind.
const (
	ExitOK                    = 0
	ExitGeneric               = 1
	ExitUsage                 = 2  // --from / --since missing, or --run / --chunk fail the grammar
	ExitConflict              = 50 // the cherry-pick conflicted; the caller tree is untouched
	ExitDirtyTree             = 51 // the caller tree has changes, untracked files included
	ExitFromMissing           = 52 // --from names no commit
	ExitSinceNotAncestor      = 53 // --since is no commit, or not an ancestor of --from
	ExitDetachedHead          = 54 // the caller's HEAD is not on a branch
	ExitGitError              = 55 // git failed, is older than 2.45, or the temp path is foreign (temp_foreign)
	ExitMergeInRange          = 56 // since..from holds a merge commit
	ExitVerifyMismatch        = 57 // the applied index / worktree differ from the picked tree (revert: the paths are not back to HEAD)
	ExitRevertConflictingEdit = 58 // --revert: the patch no longer reverses cleanly, or its paths have nothing staged (revert_nothing_staged); nothing touched
	ExitRevertIndexDiffers    = 59 // --revert: the worktree differs from the index on the patch's paths; nothing touched
)
