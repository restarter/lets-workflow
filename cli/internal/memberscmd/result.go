//go:build unix

package memberscmd

import "fmt"

// SchemaVersion is this package's JSON envelope version; a bump is a breaking
// change for member-run and every command that reads `lets members`.
const SchemaVersion = 1

// Step is one entry in the steps[] array of a result envelope.
type Step struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	StepOK   = "ok"
	StepSkip = "skip"
	StepWarn = "warn"
)

// ErrorInfo is the first-class error object emitted when ok=false.
type ErrorInfo struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Envelope is the common shape of every members result.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Scope         string     `json:"scope"`
	Steps         []Step     `json:"steps"`
}

func newEnvelope(sub, scope string) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, Subcommand: sub, Scope: scope, Steps: []Step{}}
}

// NewErrorEnvelope builds an envelope for early-return errors in the cli layer.
func NewErrorEnvelope(subcommand, scope, kind, message string) Envelope {
	e := newEnvelope(subcommand, scope)
	e.Error = &ErrorInfo{Kind: kind, Message: message}
	return e
}

// AddResult is `lets members add`.
type AddResult struct {
	Envelope
	Member *Member `json:"member,omitempty"`
}

// DismissResult is `lets members dismiss`.
type DismissResult struct {
	Envelope
	Dismissed []string `json:"dismissed"`
}

// StatusResult is `lets members status`: the lead (null when none is recorded)
// and every member, each judged now.
type StatusResult struct {
	Envelope
	Lead    *LeadStatus `json:"lead"`
	Members []Member    `json:"members"`
}

// LeadResult is `lets members lead [--claim]`.
type LeadResult struct {
	Envelope
	Lead    *LeadStatus `json:"lead,omitempty"`
	Claimed bool        `json:"claimed"`
}

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

// fail records e in env and returns it.
func fail(env *Envelope, e *Error) error {
	env.OK = false
	env.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
	return e
}
