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
	Name          string      `json:"name"`
	Path          string      `json:"path"`
	Branch        string      `json:"branch"`
	BranchMode    string      `json:"branch_mode,omitempty"` // "created" | "attached" (create only)
	BaseRef       string      `json:"base_ref,omitempty"`
	Kind          string      `json:"kind,omitempty"` // "interactive" | "agent" | "other"
	IsMain        bool        `json:"is_main,omitempty"`
	Locked        bool        `json:"locked,omitempty"`
	Prunable      bool        `json:"prunable,omitempty"`
	Detached      bool        `json:"detached,omitempty"`
	LetsSymlinked bool        `json:"lets_symlinked"`
	StoreLinks    []StoreLink `json:"store_links,omitempty"` // the tracker adapter's declared links and whether each is a symlink
	StoreLinked   bool        `json:"store_linked"`          // every declared link is a symlink (and there is at least one)
	// BeadsSymlinked is a DEPRECATED alias of StoreLinked, kept so consumers that read
	// beads_symlinked keep working; new code reads store_linked / store_links.
	BeadsSymlinked   bool   `json:"beads_symlinked"`
	Head             string `json:"head,omitempty"`
	ChangesClean     bool   `json:"changes_clean,omitempty"`
	ChangesModified  int    `json:"changes_modified,omitempty"`
	ChangesUntracked int    `json:"changes_untracked,omitempty"`
	Task             string `json:"task,omitempty"`             // the task-state file's task: line (validated)
	OrcaWorktreeID   string `json:"orca_worktree_id,omitempty"` // ORCA_WORKTREE_ID when info runs for its own checkout
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
	AlreadyGone           bool   `json:"already_gone,omitempty"`
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
	InWorktree    bool           `json:"in_worktree"`
	Worktree      *WorktreeInfo  `json:"worktree,omitempty"`
	MainRoot      string         `json:"main_root"`
	TaskCandidate *TaskCandidate `json:"task_candidate,omitempty"`
	Team          string         `json:"team,omitempty"` // the standing team owning this worktree (teamfile.FindByWorktree)
}

// TaskCandidate is the task id the active convention reads off a branch name in a
// shape LETS creates (info --task-candidate). A created shape is that task's branch
// by construction; accept: shapes are adopt's alone and never appear here.
type TaskCandidate struct {
	ID       string `json:"id,omitempty"`
	Template string `json:"template,omitempty"`
	Source   string `json:"source,omitempty"` // created
	Branch   string `json:"branch"`
	Reason   string `json:"reason,omitempty"` // convention_undeclared | no_match | detached_head | ref_invalid | ...
}

// BranchNameResult is the branch-name-subcommand envelope.
type BranchNameResult struct {
	Envelope
	Branch string `json:"branch,omitempty"`
	// Dir is the worktree directory name for the task: `<task-id>-<slug>` when that
	// is already a valid worktree name, else a lowered, hash-suffixed form (dirName).
	Dir      string `json:"dir,omitempty"`
	Slug     string `json:"slug,omitempty"`
	Template string `json:"template,omitempty"`
	Source   string `json:"source,omitempty"` // installed | board | plugin | default
	// Reasons carries LoadConvention's diagnosis so a source of "default" is
	// never unexplained: a board file whose keys were dropped says so here
	// (convention_undeclared + convention_keys_ignored_no_id).
	Reasons []string `json:"reasons,omitempty"`
}

// TaskStateInfo is the task-state file after (or instead of) a write.
type TaskStateInfo struct {
	Written bool     `json:"written"`
	Reason  string   `json:"reason,omitempty"` // file_absent | orc_on_merge_branch | task_mismatch
	Path    string   `json:"path"`
	Task    string   `json:"task,omitempty"`
	Start   string   `json:"start,omitempty"`
	Session string   `json:"session,omitempty"`
	Origin  string   `json:"origin,omitempty"`
	Orc     string   `json:"orc,omitempty"`
	Rebound *Rebound `json:"rebound,omitempty"`
}

// Rebound reports that --orc replaced a different binding.
type Rebound struct {
	From string `json:"from"`
}

// TaskStateResult is the task-state-subcommand envelope.
type TaskStateResult struct {
	Envelope
	TaskState *TaskStateInfo `json:"task_state,omitempty"`
}

// TaskInfo is the task adopt resolved for a worktree.
type TaskInfo struct {
	ID     string `json:"id"`
	Source string `json:"source"`           // argument | task_file | branch | dir
	Origin string `json:"origin,omitempty"` // branch | dir: an unconfirmed candidate (detect-task confirms it)
}

// AdoptResult is the adopt-subcommand envelope.
type AdoptResult struct {
	Envelope
	MainRoot   string        `json:"main_root"`
	Worktree   *WorktreeInfo `json:"worktree,omitempty"`
	Task       *TaskInfo     `json:"task,omitempty"`
	StoreLinks []StoreLink   `json:"store_links"`
	MovedAside string        `json:"moved_aside,omitempty"`
}

// ReleasedInfo is what release recorded before the worktree goes away.
type ReleasedInfo struct {
	Task     string          `json:"task,omitempty"`
	Branch   string          `json:"branch"`
	Marker   string          `json:"marker,omitempty"`
	Dirty    bool            `json:"dirty"`
	Unpushed bool            `json:"unpushed"`
	Snapshot string          `json:"snapshot,omitempty"`        // present | stale | missing
	Record   *SnapshotRecord `json:"record,omitempty"`          // the full answer behind Snapshot
	Kept     string          `json:"task_state_kept,omitempty"` // why the task-state file was left in place (Keep*), "" when removed or absent
}

// Why release left a task-state file in place.
const (
	KeepUnreadable   = "unreadable"    // the read failed
	KeepInvalidID    = "invalid_id"    // it names something that is not a task id
	KeepNoMarker     = "no_marker"     // it names a task whose marker could not be written
	KeepChanged      = "changed"       // a writer changed it between the read and the removal
	KeepRemoveFailed = "remove_failed" // the removal itself failed
)

// ReleaseResult is the release-subcommand envelope.
type ReleaseResult struct {
	Envelope
	Released *ReleasedInfo `json:"released,omitempty"`
}

// RecordResult is the record-subcommand envelope: one row per requested task.
type RecordResult struct {
	Envelope
	Tasks []TaskTrace `json:"tasks"`
}

// SweepResult is the sweep-subcommand envelope.
type SweepResult struct {
	Envelope
	Merged   []string `json:"merged"`
	Unmerged []string `json:"unmerged"` // maybe squashed: never deleted
	Deleted  []string `json:"deleted"`
	Applied  bool     `json:"applied"`
}
