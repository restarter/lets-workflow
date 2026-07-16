//go:build unix

package tmuxcmd

import "errors"

// Exit codes for `lets tmux`. tmux UNavailability is NOT an error (graceful
// fallback, exit 0). Only bad usage/path is a hard error.
const (
	ExitOK          = 0
	ExitGeneric     = 1
	ExitUsage       = 2
	ExitPathInvalid = 10 // --path missing or not a directory
	// 11 reserved (tmux_error currently degrades to a fallback, not an exit).
)

// ExitCode maps an error to its numeric exit code via errors.As.
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
