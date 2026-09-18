//go:build unix

package orcacmd

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

// Envelope is the common shape across all orca subcommand results.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
}

// NewErrorEnvelope builds an envelope for early-return errors in the cli layer.
func NewErrorEnvelope(subcommand, kind, message string) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, Error: &ErrorInfo{Kind: kind, Message: message}, Subcommand: subcommand, Steps: []Step{}}
}

// LaunchInfo is the outcome of an open attempt. Launched=false is NOT an error: Orca
// was unavailable or refused, and the caller renders FallbackCommand. The first
// five keys are the cross-launcher parity fields commands/worktree.md reads for
// cmux, tmux and orca alike.
type LaunchInfo struct {
	Launched        bool   `json:"launched"`
	WorkspaceName   string `json:"workspace_name,omitempty"`
	Path            string `json:"path"`
	Reason          string `json:"reason,omitempty"`
	FallbackCommand string `json:"fallback_command,omitempty"`
	Created         bool   `json:"created"`
	Branch          string `json:"branch,omitempty"`
	OrcaWorktreeID  string `json:"orca_worktree_id,omitempty"`
}

// NotifyInfo is the outcome of a notify attempt. Notified=true means Orca accepted
// the card comment - never that a human saw it.
type NotifyInfo struct {
	Notified bool   `json:"notified"`
	Target   string `json:"target,omitempty"` // the Orca worktree id
	Title    string `json:"title,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// StatusInfo reports whether Orca is usable from here.
type StatusInfo struct {
	Bin     string `json:"bin,omitempty"`
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// OpenResult is the open-subcommand envelope.
type OpenResult struct {
	Envelope
	Launch *LaunchInfo `json:"launch,omitempty"`
}

// NotifyResult is the notify-subcommand envelope.
type NotifyResult struct {
	Envelope
	Notify *NotifyInfo `json:"notify,omitempty"`
}

// StatusResult is the status-subcommand envelope.
type StatusResult struct {
	Envelope
	Status *StatusInfo `json:"status,omitempty"`
}
