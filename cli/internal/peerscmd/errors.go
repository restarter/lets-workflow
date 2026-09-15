package peerscmd

// Error is a typed failure that carries an exit code (main.go's exitCoder).
type Error struct {
	Code        int
	Kind        string
	Message     string
	Remediation string
	Cause       error
}

func (e *Error) Error() string { return e.Kind + ": " + e.Message }
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode is the process exit code for this error.
func (e *Error) ExitCode() int { return e.Code }
