package statuslinecmd

import (
	"fmt"
	"io"
)

// SchemaVersion is this package's JSON envelope schema version. Per-package per
// the JSON-envelope convention; pinned by TestResult_SchemaContract.
const SchemaVersion = 1

// Step is one entry in steps[].
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

// Appearance is the resolved render-flag state.
type Appearance struct {
	Light   bool `json:"light"`
	Compact bool `json:"compact"`
	NoTip   bool `json:"no_tip"`
	NoDir   bool `json:"no_dir"`
	NoTask  bool `json:"no_task"`
}

// Result is the `lets statusline config` envelope. Valid JSON even on ok=false.
type Result struct {
	SchemaVersion int         `json:"schema_version"`
	OK            bool        `json:"ok"`
	Error         *ErrorInfo  `json:"error,omitempty"`
	Subcommand    string      `json:"subcommand"`
	ProjectRoot   string      `json:"project_root"`
	Steps         []Step      `json:"steps"`
	SettingsPath  string      `json:"settings_path,omitempty"`
	Command       string      `json:"command,omitempty"` // the persisted statusLine command
	Appearance    *Appearance `json:"appearance,omitempty"`
	Changed       bool        `json:"changed"` // true when a write happened (set), false for show / no-op
}

func newResult(root string) Result {
	return Result{
		SchemaVersion: SchemaVersion,
		Subcommand:    "config",
		ProjectRoot:   root,
		Steps:         []Step{},
	}
}

func errInfo(e *Error) *ErrorInfo {
	return &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
}

// NewErrorResult builds an ok=false envelope for early cli-layer bailouts
// (not_in_repo, usage) before Apply/Show run.
func NewErrorResult(root string, e *Error) Result {
	res := newResult(root)
	res.OK = false
	res.Error = errInfo(e)
	res.Steps = append(res.Steps, Step{Status: StepErr, Message: e.Message})
	return res
}

// RenderHuman writes a short human-readable summary of a config result.
func RenderHuman(w io.Writer, res Result) {
	if !res.OK {
		if res.Error != nil {
			fmt.Fprintf(w, "statusline config: %s\n", res.Error.Message)
			if res.Error.Remediation != "" {
				fmt.Fprintf(w, "  hint: %s\n", res.Error.Remediation)
			}
		}
		return
	}
	verb := "Current"
	if res.Changed {
		verb = "Saved"
	}
	fmt.Fprintf(w, "%s statusline appearance: %s\n", verb, res.Command)
	if res.SettingsPath != "" {
		fmt.Fprintf(w, "  %s\n", res.SettingsPath)
	}
	if res.Changed {
		fmt.Fprintln(w, "  Restart Claude Code to apply (the statusLine command is read on session start).")
	}
}
