// Package statuslinecmd persists the LETS statusline appearance (the render
// flags --light/--compact/--no-tip/--no-dir/--no-task) to a project's personal
// .claude/settings.local.json, so a choice sticks across sessions without
// hand-editing settings. It mirrors the worktreecmd conventions: a typed
// package Error carrying an exit code + machine-readable kind, and a JSON
// envelope (Result) valid even on ok=false.
package statuslinecmd

import "fmt"

// Exit codes for `lets statusline config`. 0-2 follow shell convention; 10 and
// 30..32 are typed failure classes scripts can branch on without parsing prose.
// The 30s range is statuslinecmd's own slice of the shared binary exit space
// (worktreecmd owns 10..21).
const (
	ExitOK         = 0
	ExitGeneric    = 1
	ExitUsage      = 2
	ExitNotInRepo  = 10
	ExitForeign    = 30
	ExitMalformed  = 31
	ExitFilesystem = 32
)

// Error is the typed package error. Use errors.As(err, &e) to inspect. Code
// maps to an Exit* constant; Kind is the snake_case kind in the JSON envelope.
type Error struct {
	Code        int
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

// Unwrap lets errors.As reach the underlying cause through fmt.Errorf wrapping.
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode satisfies the cli-layer exit-coder interface (main.go) so a typed
// statuslinecmd error becomes the matching process exit code.
func (e *Error) ExitCode() int {
	if e.Code == 0 {
		return ExitGeneric
	}
	return e.Code
}

// ErrNotInRepo — project root could not be resolved (not inside a git repo).
func ErrNotInRepo() *Error {
	return &Error{
		Code:        ExitNotInRepo,
		Kind:        "not_in_repo",
		Message:     "could not resolve the project root (not inside a git repository)",
		Remediation: "run from inside the project, or run `lets init` first",
	}
}

// ErrUsage — invalid invocation (e.g. no flags and no --show).
func ErrUsage(message string) *Error {
	return &Error{Code: ExitUsage, Kind: "usage", Message: message}
}

// ErrForeign — settings.local.json already has a non-LETS statusLine command;
// refuse to clobber it unless --force.
func ErrForeign(command string) *Error {
	return &Error{
		Code:        ExitForeign,
		Kind:        "foreign_statusline",
		Message:     fmt.Sprintf("settings.local.json has a foreign statusLine command (%q)", command),
		Remediation: "re-run with --force to overwrite it",
	}
}

// ErrMalformed — settings.local.json is not valid JSON; refuse to mutate.
func ErrMalformed(path string, cause error) *Error {
	return &Error{
		Code:        ExitMalformed,
		Kind:        "malformed_settings",
		Message:     fmt.Sprintf("%s is not valid JSON", path),
		Remediation: "fix or remove the file, then retry",
		Cause:       cause,
	}
}

// ErrFilesystem — a read/write/marshal step failed.
func ErrFilesystem(message string, cause error) *Error {
	return &Error{Code: ExitFilesystem, Kind: "filesystem", Message: message, Cause: cause}
}
