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
// error - it means cmux was unavailable/errored OR a workspace already targets
// this path; the caller renders FallbackCommand (or points at the existing
// workspace). Reason: not_macos | cmux_not_found | cmux_error | already_open.
type LaunchInfo struct {
	Launched        bool   `json:"launched"`
	WorkspaceName   string `json:"workspace_name,omitempty"`
	Path            string `json:"path"`
	Command         string `json:"command,omitempty"`
	Reason          string `json:"reason,omitempty"`
	FallbackCommand string `json:"fallback_command,omitempty"`
	// Set when Reason == "already_open": the workspace already bound to Path.
	ExistingRef   string `json:"existing_ref,omitempty"`
	ExistingTitle string `json:"existing_title,omitempty"`
}

// RenameInfo is the outcome of a rename attempt. Renamed=false is NOT an error
// when Reason is set (not_macos | cmux_not_found | workspace_not_found |
// cmux_error) - cmux is optional, so the caller degrades quietly.
type RenameInfo struct {
	Renamed  bool   `json:"renamed"`
	Ref      string `json:"ref,omitempty"`
	Title    string `json:"title,omitempty"`
	OldTitle string `json:"old_title,omitempty"`
	Reason   string `json:"reason,omitempty"`
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

// RenameResult is the rename-subcommand envelope.
type RenameResult struct {
	Envelope
	Rename *RenameInfo `json:"rename,omitempty"`
}
