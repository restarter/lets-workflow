//go:build unix

package orcacmd

import "fmt"

// Error is the typed package error. Code maps to one of the Exit* constants; Kind
// is the snake_case machine-readable kind surfaced in the JSON envelope.
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

// Unwrap lets errors.As reach the underlying cause.
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode satisfies the cli-layer exit-coder interface.
func (e *Error) ExitCode() int {
	if e.Code == 0 {
		return ExitGeneric
	}
	return e.Code
}
