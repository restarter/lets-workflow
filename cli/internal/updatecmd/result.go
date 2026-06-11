// Package updatecmd implements the `lets update` subcommand: it checks the
// four core drift-able LETS artifacts (.lets/.env, .claude/rules/lets-rules.md,
// the lets binary, the Claude Code plugin) plus the optional user-scope rules
// copy (~/.claude/rules/lets-rules.md - row omitted when absent), auto-syncs
// what it safely can, and reports actionable status for what it cannot.
// Mirrors initcmd's cobra-factory / Run() / Result-envelope structure.
package updatecmd

// SchemaVersion identifies the JSON output contract for `lets update --json`.
// Bump on field removal or semantic change of an existing field; additions are
// minor (consumers ignore unknown fields). Mirrors initcmd.SchemaVersion.
//
// v2 (lets-kaw72): `.env`/`rules` now report `in-sync` (matches their local
// source: the binary for `.env`, the plugin for `rules`) instead of `up-to-date`
// (reserved for binary/plugin == latest release). This stops two `up-to-date`
// rows at different versions from reading as self-contradictory.
const SchemaVersion = 2

// ArtifactStatus categorizes the state of one drift-able artifact.
type ArtifactStatus string

const (
	StatusUpToDate       ArtifactStatus = "up-to-date"      // matches the latest release (binary / plugin)
	StatusInSync         ArtifactStatus = "in-sync"         // matches its local source: the binary (.env) or the plugin (rules) - not necessarily the latest release
	StatusUpdated        ArtifactStatus = "updated"         // we just synced it (.env header / rules file)
	StatusOutdated       ArtifactStatus = "outdated"        // behind latest - user action needed
	StatusAhead          ArtifactStatus = "ahead"           // newer than latest stable (prerelease / dev checkout)
	StatusUnknown        ArtifactStatus = "unknown"         // couldn't determine (offline, unreadable)
	StatusNotInitialized ArtifactStatus = "not-initialized" // .env absent - project never `lets init`-ed
	StatusDev            ArtifactStatus = "dev"             // running an untagged dev binary - no comparison
	StatusDelegated      ArtifactStatus = "delegated"       // project rules deliberately absent (LETS_RULES_SCOPE=user) - rules come from ~/.claude/rules
)

// allStatuses - keep adjacent to the Status* consts. A new status MUST be
// appended here; the bucket-sum invariant test ranges over this list.
var allStatuses = []ArtifactStatus{
	StatusUpToDate, StatusInSync, StatusUpdated, StatusOutdated,
	StatusAhead, StatusUnknown, StatusNotInitialized, StatusDev,
	StatusDelegated,
}

// Artifact is the outcome of checking one drift-able artifact.
type Artifact struct {
	Name           string         `json:"name"` // ".env" | "rules" | "binary" | "plugin" | "user-rules" (only when ~/.claude/rules/lets-rules.md exists)
	Status         ArtifactStatus `json:"status"`
	CurrentVersion string         `json:"current_version,omitempty"`
	LatestVersion  string         `json:"latest_version,omitempty"`
	Action         string         `json:"action,omitempty"` // human instruction when Status needs user action
	Detail         string         `json:"detail,omitempty"` // extra context (changed keys, cache age, error reason)
}

// Summary aggregates artifact counts for at-a-glance consumption.
type Summary struct {
	UpToDate     int `json:"up_to_date"` // "in sync" bucket: up-to-date + in-sync
	Updated      int `json:"updated"`
	ActionNeeded int `json:"action_needed"` // outdated + not-initialized
	Unknown      int `json:"unknown"`       // unknown + ahead + dev
}

// Result is the structured outcome of `lets update`. Always populated, even on
// error (Artifacts carries whatever was checked before the failure).
type Result struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         string     `json:"error,omitempty"`
	ProjectRoot   string     `json:"project_root"`
	PluginRoot    string     `json:"plugin_root"`
	Artifacts     []Artifact `json:"artifacts"`
	Consistent    bool       `json:"consistent"` // binary == plugin == installed-rules frontmatter version (ignoring "dev"/"")
	Summary       Summary    `json:"summary"`
}

// NewResult initializes a Result with paths and a non-nil Artifacts slice
// (JSON consumers see "artifacts":[] not null). Consistent defaults true
// (nothing contradicts until proven otherwise).
func NewResult(projectRoot, pluginRoot string) Result {
	return Result{
		SchemaVersion: SchemaVersion,
		ProjectRoot:   projectRoot,
		PluginRoot:    pluginRoot,
		Artifacts:     []Artifact{},
		Consistent:    true,
	}
}

// Add appends an artifact and increments the matching Summary counter. Single
// invariant: every append goes through here (a direct append desyncs Summary).
func (r *Result) Add(a Artifact) {
	r.Artifacts = append(r.Artifacts, a)
	switch a.Status {
	case StatusUpToDate, StatusInSync, StatusDelegated:
		// delegated is a healthy target state (scope=user, rules come from the
		// global copy) - belongs in the "in sync" bucket, not action-needed.
		r.Summary.UpToDate++
	case StatusUpdated:
		r.Summary.Updated++
	case StatusOutdated, StatusNotInitialized:
		r.Summary.ActionNeeded++
	case StatusUnknown, StatusAhead, StatusDev:
		r.Summary.Unknown++
	}
}
