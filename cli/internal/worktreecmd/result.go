//go:build unix

package worktreecmd

// Step is one entry in the steps[] array of a result envelope.
// Use keyed literals at construction sites.
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

// RollbackInfo describes what was undone after a partial-create failure.
// Residual paths surface in the JSON envelope (no separate log file).
type RollbackInfo struct {
	Attempted bool     `json:"attempted"`
	Succeeded bool     `json:"succeeded"`
	Residual  []string `json:"residual,omitempty"`
}

// WorktreeInfo is one worktree row.
type WorktreeInfo struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	BranchMode       string `json:"branch_mode,omitempty"` // "created" | "attached" (create only)
	BaseRef          string `json:"base_ref,omitempty"`
	Kind             string `json:"kind,omitempty"` // "interactive" | "agent" | "other"
	IsMain           bool   `json:"is_main,omitempty"`
	Locked           bool   `json:"locked,omitempty"`
	Prunable         bool   `json:"prunable,omitempty"`
	Detached         bool   `json:"detached,omitempty"`
	LetsSymlinked    bool   `json:"lets_symlinked"`
	BeadsSymlinked   bool   `json:"beads_symlinked"`
	Head             string `json:"head,omitempty"`
	ChangesClean     bool   `json:"changes_clean,omitempty"`
	ChangesModified  int    `json:"changes_modified,omitempty"`
	ChangesUntracked int    `json:"changes_untracked,omitempty"`
}

// NextSteps gives callers actionable follow-up.
type NextSteps struct {
	AbsolutePath string `json:"absolute_path"`
}

// Envelope is the common shape across all subcommand results.
type Envelope struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	ProjectRoot   string     `json:"project_root"`
	Steps         []Step     `json:"steps"`
}

// NewErrorEnvelope builds a populated Envelope for early-return errors
// in the cli layer (before worktreecmd has a chance to build one itself).
// Use when --json is set and the RunE bails out before calling worktreecmd
// (flag_conflict, not_in_repo, getwd_failed). Without this, scripts that
// expect a JSON envelope on --json would receive plain text on stderr only.
func NewErrorEnvelope(subcommand, kind, message string) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            false,
		Error:         &ErrorInfo{Kind: kind, Message: message},
		Subcommand:    subcommand,
		Steps:         []Step{},
	}
}

// CreateResult is the create-subcommand envelope.
type CreateResult struct {
	Envelope
	Worktree  *WorktreeInfo `json:"worktree,omitempty"`
	NextSteps *NextSteps    `json:"next_steps,omitempty"`
	Rollback  *RollbackInfo `json:"rollback,omitempty"`
}

// RemoveResult is the remove-subcommand envelope.
type RemoveResult struct {
	Envelope
	Removed *RemovedInfo `json:"removed,omitempty"`
}

// RemovedInfo describes what `lets worktree remove` removed.
type RemovedInfo struct {
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	Branch                string `json:"branch"`
	BranchDeleted         bool   `json:"branch_deleted"`
	HadUncommittedChanges bool   `json:"had_uncommitted_changes"`
	Forced                bool   `json:"forced"`
}

// ListResult is the list-subcommand envelope.
type ListResult struct {
	Envelope
	Worktrees []WorktreeInfo `json:"worktrees"`
	Main      *WorktreeInfo  `json:"main,omitempty"`
}

// InfoResult is the info-subcommand envelope.
type InfoResult struct {
	Envelope
	InWorktree bool          `json:"in_worktree"`
	Worktree   *WorktreeInfo `json:"worktree,omitempty"`
	MainRoot   string        `json:"main_root"`
}
