//go:build unix

package worktreecmd

import (
	"fmt"

	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// Error is the typed package error. Use errors.As(err, &e) to inspect.
// Code maps to one of the Exit* constants; Kind is the snake_case
// machine-readable kind surfaced in the JSON envelope.
type Error struct {
	Code        int // one of Exit* constants
	Kind        string
	Message     string
	Remediation string
	Cause       error
}

func (e *Error) Error() string {
	if e.Remediation != "" {
		return fmt.Sprintf("%s: %s (hint: %s)", e.Kind, e.Message, e.Remediation)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// Unwrap lets errors.As reach through fmt.Errorf("...%w", err) wrappings
// and lets callers inspect the underlying cause (e.g. *exec.ExitError).
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode satisfies the cli-layer's exit-coder interface so main.go can
// translate a typed worktreecmd error into the matching process exit code
// (10..20 — see exit.go). Returns ExitGeneric when Code is zero so callers
// don't fall through to a silent 0/success on a partially-initialized Error.
func (e *Error) ExitCode() int {
	if e.Code == 0 {
		return ExitGeneric
	}
	return e.Code
}

// ErrInsideWorktree — raised when create is invoked from within a worktree.
func ErrInsideWorktree() *Error {
	return &Error{
		Code:        ExitInsideWorktree,
		Kind:        "inside_worktree",
		Message:     "cannot create a worktree from within a worktree",
		Remediation: "cd to the main repository, then retry",
	}
}

// ErrBranchCheckedOutInMain — raised when attach target is the branch
// currently checked out in the main repo. Caller can opt into
// --switch-main-if-needed (Task 4a) to flip main to $LETS_MERGE_BRANCH.
func ErrBranchCheckedOutInMain(branch string) *Error {
	return &Error{
		Code:        ExitBranchConflict,
		Kind:        "branch_checked_out_in_main",
		Message:     fmt.Sprintf("branch %q is checked out in the main repo", branch),
		Remediation: "git switch $LETS_MERGE_BRANCH in main repo, then retry; or pass --switch-main-if-needed (requires clean tree)",
	}
}

// ErrBranchInUseOtherWorktree - attach refused: the branch is checked out in another
// worktree (lets-fh3yq: read from `git worktree list --porcelain`, not git's prose).
func ErrBranchInUseOtherWorktree(branch, path string) *Error {
	return &Error{
		Code:        ExitBranchConflict,
		Kind:        "branch_in_use_other_worktree",
		Message:     fmt.Sprintf("branch %q is checked out in another worktree at %s", branch, path),
		Remediation: "lets worktree list to find it, then remove it (git worktree prune when its directory is already gone), or pick a different name",
	}
}

// ErrTeamExists - team-init found (or the filesystem's link refused over) a team
// file of that callsign; it is never overwritten.
func ErrTeamExists(path string) *Error {
	return &Error{Code: ExitTeamExists, Kind: "team_exists", Message: fmt.Sprintf("team file %s already exists", path), Remediation: "pick another callsign (--suggest-callsign)"}
}

// ErrTemplateMissing - team-init cannot read the plugin's team template.
func ErrTemplateMissing(path string) *Error {
	return &Error{Code: ExitTemplateMissing, Kind: "template_missing", Message: fmt.Sprintf("team template %s is missing", path), Remediation: "pass --plugin-root <plugins/lets>"}
}

// ErrCallsignLive - a live session already carries the callsign's lead name in this
// repository.
func ErrCallsignLive(callsign, name string) *Error {
	return &Error{Code: ExitCallsignLive, Kind: "callsign_live", Message: fmt.Sprintf("a live session is named %s (the lead of team %s)", name, callsign), Remediation: "pick another callsign (--suggest-callsign)"}
}

// ErrNotTeamWorktree - switch runs only in a standing team's worktree.
func ErrNotTeamWorktree(detail string) *Error {
	return &Error{Code: ExitNotTeamWorktree, Kind: "not_team_worktree", Message: "no team file claims this worktree" + detail, Remediation: "run it in a team worktree (lets worktree team-init), or use /lets:start to take a task"}
}

// ErrUntrackedPresent - a park would leave a new path out of the park commit.
func ErrUntrackedPresent(paths string) *Error {
	return &Error{Code: ExitUntrackedPresent, Kind: "untracked_present", Message: "new paths need an explicit --include before a park: " + paths + "; the index and HEAD are untouched", Remediation: "pass --include <path> for each path to park, or remove it"}
}

// ErrNoRemoteBase - a new branch is cut from origin/<merge> only.
func ErrNoRemoteBase(merge string) *Error {
	return &Error{Code: ExitNoRemoteBase, Kind: "no_remote_base", Message: fmt.Sprintf("origin/%s is missing; a new branch is never cut from the local %s", merge, merge), Remediation: "add the origin remote and fetch " + merge}
}

// ErrSessionHeld - the session: line of branch is held by another session, or by one
// the registry cannot prove ended (switch refuses on any held reason; task-state set
// only on a live foreign holder); nothing was written.
func ErrSessionHeld(held *taskstate.HeldError, branch string) *Error {
	return &Error{Code: ExitSessionHeld, Kind: "session_held", Message: fmt.Sprintf("the session: line of %s is held (%s) by %s", branch, held.Reason, held.Holder), Remediation: held.Holder + " still owns this task's boundary: run this from that session, after it ends, or pick another task", Cause: held}
}

// ErrTargetIsMergeBranch - a team never works on the merge-branch.
func ErrTargetIsMergeBranch(branch string) *Error {
	return &Error{Code: ExitTargetIsMergeBranch, Kind: "target_is_merge_branch", Message: fmt.Sprintf("%s is the merge-branch; a team works on task branches only", branch)}
}
