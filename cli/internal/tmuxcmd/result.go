//go:build unix

package tmuxcmd

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

// LaunchInfo is the outcome of an open attempt. Launched=false is NOT an error -
// it means tmux was unavailable/errored OR a pane already lives at this path;
// the caller renders FallbackCommand (or points at the existing target).
// Reason: tmux_not_found | tmux_error | already_open.
//
// WorkspaceName (the tmux SESSION name) is kept for cross-launcher parity with
// cmuxcmd.LaunchInfo, so commands/worktree.md can read one field name for both
// launchers. Target/AttachCommand/InExistingSession are tmux-specific.
type LaunchInfo struct {
	Launched      bool   `json:"launched"`
	WorkspaceName string `json:"workspace_name,omitempty"` // tmux session name
	Target        string `json:"target,omitempty"`         // "session:window_index"
	Description   string `json:"description,omitempty"`
	Path          string `json:"path"`
	Command       string `json:"command,omitempty"`
	// InExistingSession is true when the caller already ran inside tmux ($TMUX
	// set) and we added a window to the current session - no attach needed.
	InExistingSession bool   `json:"in_existing_session"`
	AttachCommand     string `json:"attach_command,omitempty"`
	Reason            string `json:"reason,omitempty"`
	FallbackCommand   string `json:"fallback_command,omitempty"`
	// Set when Reason == "already_open": the pane already living at Path.
	ExistingTarget string `json:"existing_target,omitempty"`
	ExistingTitle  string `json:"existing_title,omitempty"`
}

// RenameInfo is the outcome of a rename attempt. Renamed=false is NOT an error
// when Reason is set (tmux_not_found | pane_not_found | tmux_error) - tmux is
// optional, so the caller degrades quietly.
type RenameInfo struct {
	Renamed  bool   `json:"renamed"`
	Target   string `json:"target,omitempty"`
	Title    string `json:"title,omitempty"`
	OldTitle string `json:"old_title,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// NotifyInfo is the outcome of a notify attempt. Notified=true means the message
// was DISPLAYED on >=1 attached client (Clients says how many) - NOT that a human
// saw it. Notified=false is NOT an error when Reason is set (tmux_not_found |
// no_client | tmux_error). Callers must keep an in-band signal too (the gate
// halts visibly regardless).
//
// no_client is the load-bearing reason: a tmux server can be running with our
// DETACHED worktree session in it and still have zero humans attached. tmux exits
// 0 on a display-message nobody can see, so without this the envelope would lie.
type NotifyInfo struct {
	Notified bool   `json:"notified"`
	Clients  int    `json:"clients,omitempty"` // attached clients the message reached
	Target   string `json:"target,omitempty"`  // the gated window, when resolvable
	Title    string `json:"title,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Envelope is the common shape across all tmux subcommand results.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
}

// NewErrorEnvelope builds a populated Envelope for early-return errors in the
// cli layer (before tmuxcmd has a chance to build one itself).
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

// NotifyResult is the notify-subcommand envelope.
type NotifyResult struct {
	Envelope
	Notify *NotifyInfo `json:"notify,omitempty"`
}
