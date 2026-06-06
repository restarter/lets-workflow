//go:build unix

package cmuxcmd

// Step is one entry in the steps[] array of a result envelope.
type Step struct {
	Status  string `json:"status"` // StepOK | StepSkip | StepWarn | StepErr
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

// LaunchInfo is the outcome of an open attempt. Launched=false is NOT an
// error - it means cmux was unavailable (or errored) and the caller should
// render FallbackCommand. Reason: not_macos | cmux_not_found | cmux_error.
type LaunchInfo struct {
	Launched        bool   `json:"launched"`
	WorkspaceName   string `json:"workspace_name,omitempty"`
	Path            string `json:"path"`
	Command         string `json:"command,omitempty"`
	Reason          string `json:"reason,omitempty"`
	FallbackCommand string `json:"fallback_command,omitempty"`
}

// Envelope is the common shape across all cmux subcommand results.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
}

// NewErrorEnvelope builds a populated Envelope for early-return errors in the
// cli layer (before cmuxcmd has a chance to build one itself).
func NewErrorEnvelope(subcommand, kind, message string) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            false,
		Error:         &ErrorInfo{Kind: kind, Message: message},
		Subcommand:    subcommand,
		Steps:         []Step{},
	}
}

// OpenResult is the open-subcommand envelope.
type OpenResult struct {
	Envelope
	Launch *LaunchInfo `json:"launch,omitempty"`
}
