package initcmd

import (
	"github.com/restarter/lets-workflow/cli/internal/drift"
)

// SchemaVersion identifies the JSON output contract for `lets init --json`.
// Bump on field removal or semantic change of existing fields. Additions
// are minor (consumers ignore unknown fields). Schema contract test
// (TestResult_SchemaContract) ensures bumps are noticed.
const SchemaVersion = 1

// Result is the structured outcome of `lets init`. Always populated, even on
// partial failure (Steps slice carries work completed before the error).
type Result struct {
	SchemaVersion int         `json:"schema_version"`
	OK            bool        `json:"ok"`
	Error         string      `json:"error,omitempty"`
	ProjectRoot   string      `json:"project_root"`
	PluginRoot    string      `json:"plugin_root"`
	Steps         []Step      `json:"steps"`
	EnvAction     EnvAction   `json:"env_action"`
	Drift         DriftReport `json:"drift"`
	Summary       Summary     `json:"summary"`
}

// DriftReport mirrors drift.Result with JSON tags + the canonical message.
//
// Note on disambiguation: InstalledVersion is "" both when state is "missing"
// (file absent) AND "unknown" (file present but unparseable). Consumers must
// check `state` to disambiguate.
type DriftReport struct {
	Detected         bool        `json:"detected"`
	State            drift.State `json:"state"`
	InstalledVersion string      `json:"installed_version"`
	PluginVersion    string      `json:"plugin_version"`
	Message          string      `json:"message,omitempty"` // canonical, from drift.Message(r)
}

// Summary aggregates step counts for at-a-glance consumption.
type Summary struct {
	OKCount      int `json:"ok_count"`
	SkipCount    int `json:"skip_count"`
	WarnCount    int `json:"warn_count"`
	ErrCount     int `json:"err_count"`
	MigrateCount int `json:"migrate_count"`
}

// Add adds a step and increments the matching summary counter. Single-source
// invariant: every step append goes through here. Direct `result.Steps = append(...)`
// is forbidden — would silently desync Summary counts.
func (r *Result) Add(s Step) {
	r.Steps = append(r.Steps, s)
	switch s.Status {
	case StepOK:
		r.Summary.OKCount++
	case StepSkip:
		r.Summary.SkipCount++
	case StepWarn:
		r.Summary.WarnCount++
	case StepErr:
		r.Summary.ErrCount++
	case StepMigrate:
		r.Summary.MigrateCount++
	}
}

// NewResult initializes a Result with project/plugin paths and empty slice
// (never nil — JSON consumers see "steps":[] not "steps":null).
func NewResult(projectRoot, pluginRoot string) Result {
	return Result{
		SchemaVersion: SchemaVersion,
		ProjectRoot:   projectRoot,
		PluginRoot:    pluginRoot,
		Steps:         []Step{},
		EnvAction:     EnvAction{ChangedKeys: []string{}},
	}
}
