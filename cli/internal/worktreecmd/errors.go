//go:build unix

package worktreecmd

import "fmt"

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

// ErrTeamExists - team-init found (or the filesystem's link refused over) a team
// file of that callsign; it is never overwritten.
func ErrTeamExists(path string) *Error {
	return &Error{Code: ExitTeamExists, Kind: "team_exists", Message: fmt.Sprintf("team file %s already exists", path), Remediation: "pick another callsign (--suggest-callsign)"}
}

// ErrTemplateMissing - team-init cannot read the plugin's team template.
func ErrTemplateMissing(path string) *Error {
	return &Error{Code: ExitTemplateMissing, Kind: "template_missing", Message: fmt.Sprintf("team template %s is missing", path), Remediation: "pass --plugin-root <plugins/lets>"}
}

// ErrCallsignLive - a live session already runs as the callsign's lead.
func ErrCallsignLive(callsign string) *Error {
	return &Error{Code: ExitCallsignLive, Kind: "callsign_live", Message: fmt.Sprintf("a live session is named %s-lead", callsign), Remediation: "pick another callsign (--suggest-callsign)"}
}
