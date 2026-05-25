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
