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
	ExitLetsDirConflict      = 22 // adopt: a real .lets with non-cache content (never deleted)
	ExitNotLinkedWorktree    = 23 // adopt / release: the directory is not a linked worktree
	ExitStoreLinkFailed      = 24 // a declared store link could not be made (foreign file at the link path)
	ExitTaskFileConflict     = 25 // adopt: .task-<slug> already names a different task
	ExitTaskStateLockBusy    = 26 // task-state: the lock was still held at the --wait deadline
	ExitTeamExists           = 27 // team-init: .lets/teams/<callsign>.md exists (the filesystem refused the link)
	ExitTemplateMissing      = 28 // team-init: <plugin-root>/templates/team.md is missing
	ExitNotTeamWorktree      = 29 // switch: no team file claims this worktree
	ExitUntrackedPresent     = 30 // switch --park: a new path (untracked or staged) has no --include
	ExitNoRemoteBase         = 31 // switch: origin/<merge> is missing, and a new branch is never cut from local <merge>
	ExitMembersLive          = 32 // switch: a member working in this worktree is still live
	ExitTargetIsMergeBranch  = 33 // switch: the target branch is the merge-branch
	ExitCallsignLive         = 34 // team-init: a live session is named <callsign>-lead
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
