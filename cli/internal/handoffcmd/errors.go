//go:build unix

package handoffcmd

import "fmt"

// Error is the typed package error; Code is one of the Exit* constants.
type Error struct {
	Code        int
	Kind        string
	Message     string
	Remediation string
}

func (e *Error) Error() string {
	if e.Remediation != "" {
		return fmt.Sprintf("%s: %s (hint: %s)", e.Kind, e.Message, e.Remediation)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// ExitCode satisfies the cli-layer exit-coder interface.
func (e *Error) ExitCode() int {
	if e.Code == 0 {
		return ExitGeneric
	}
	return e.Code
}

func usage(env *Envelope, msg string) error {
	env.Error = &ErrorInfo{Kind: "usage", Message: msg}
	return &Error{Code: ExitUsage, Kind: "usage", Message: msg}
}

func briefInvalid(env *Envelope, err error) error {
	const hint = "save the brief through artifact-path kind=handoff"
	env.Error = &ErrorInfo{Kind: "brief_invalid", Message: err.Error(), Remediation: hint}
	return &Error{Code: ExitBriefInvalid, Kind: "brief_invalid", Message: err.Error(), Remediation: hint}
}
