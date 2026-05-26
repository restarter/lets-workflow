//go:build unix

package worktreecmd

import "errors"

// Exit codes returned by `lets worktree` subcommands. 0-2 are the standard
// shell conventions; 10..21 are typed failure classes that scripts can
// branch on without parsing prose. 22..29 reserved for future `adopt` and
// related subcommands (see backlog on lets-rqep4).
const (
	ExitOK                   = 0
	ExitGeneric              = 1
	ExitUsage                = 2
	ExitNotInRepo            = 10
	ExitInsideWorktree       = 11
	ExitWorktreeExists       = 12
	ExitBranchConflict       = 13 // overloaded; parse error.kind for specifics. See decisions table.
	ExitDirtyWorktree        = 14
	ExitBranchUnmerged       = 15
	ExitGitFailed            = 16
	ExitFilesystem           = 17
	ExitStaleWorktreePath    = 18
	ExitSymlinkSourceMissing = 19
	ExitVerifyFailed         = 20
	ExitUnpushedCommits      = 21
	// Reserved 22..29 for future `lets worktree adopt` and related subcommands.
	// Adopt would re-register an externally-created worktree path with LETS-managed
	// symlinks. See lets-rqep4 backlog comment.
)

// ExitCode maps an error to its numeric exit code via errors.As.
// Returns ExitOK for nil, the typed code for a *Error (even when wrapped
// via fmt.Errorf("...%w", ...)), and ExitGeneric otherwise.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ExitGeneric
}
