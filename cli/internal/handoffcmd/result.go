//go:build unix

package handoffcmd

import "github.com/restarter/lets-workflow/cli/internal/agentrun"

// Step is one entry in the steps[] array of a result envelope.
type Step struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	StepOK   = "ok"
	StepSkip = "skip"
	StepWarn = "warn"
	StepErr  = "error"
)

// ErrorInfo is the first-class error object emitted when ok=false.
type ErrorInfo struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Envelope is the common shape across all handoff subcommand results.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
}

func newEnvelope(sub string) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, Subcommand: sub, Steps: []Step{}}
}

// NewErrorEnvelope builds an envelope for early-return errors in the cli layer.
func NewErrorEnvelope(subcommand, kind, message string) Envelope {
	e := newEnvelope(subcommand)
	e.Error = &ErrorInfo{Kind: kind, Message: message}
	return e
}

// Target is one agent terminal a brief can go to.
type Target struct {
	Handle       string `json:"handle"`
	Title        string `json:"title"`
	Agent        string `json:"agent"`          // claude | codex | antigravity | ...
	State        string `json:"state"`          // Orca's agent state; "" when Orca tracks none
	LastOutputAt int64  `json:"last_output_at"` // epoch ms
}

// TargetsInfo lists the agent terminals of this checkout, newest output first.
type TargetsInfo struct {
	Available bool     `json:"available"` // Orca answered
	Terminals []Target `json:"terminals"`
	Reason    string   `json:"reason,omitempty"`
}

// SendInfo is what a send proved. Delivery: proven | unproven | failed | skipped.
// InputLine: ready | busy | not_clear | unknown (the check before typing).
type SendInfo struct {
	Delivery    string   `json:"delivery"`
	Reason      string   `json:"reason,omitempty"`
	Handle      string   `json:"handle,omitempty"`
	Title       string   `json:"title,omitempty"`
	Agent       string   `json:"agent"`
	InputLine   string   `json:"input_line,omitempty"`
	Created     bool     `json:"created"`
	SentAt      string   `json:"sent_at,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	ScreenTail  []string `json:"screen_tail,omitempty"`
}

// TargetsResult is the targets envelope.
type TargetsResult struct {
	Envelope
	Targets *TargetsInfo `json:"targets,omitempty"`
}

// SendResult is the send envelope.
type SendResult struct {
	Envelope
	Send *SendInfo `json:"send,omitempty"`
}

// RunResult is the codex and await envelope.
type RunResult struct {
	Envelope
	Run *agentrun.Result `json:"run,omitempty"`
}
