//go:build unix

package integratecmd

import "fmt"

// SchemaVersion is this package's JSON envelope version; a bump is a breaking
// change for /lets:execute, which reads `lets integrate --json`.
const SchemaVersion = 1

// Step is one entry in the steps[] array of a result envelope.
type Step struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	StepOK   = "ok"
	StepSkip = "skip"
	StepWarn = "warn"
)

// ErrorInfo is the first-class error object emitted when ok=false.
type ErrorInfo struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Conflict names the commit whose pick conflicted and the conflicting paths.
type Conflict struct {
	Commit string   `json:"commit"`
	Files  []string `json:"files"`
}

// Result is the `lets integrate` envelope. Picked holds the commits the pick
// created in the temporary worktree, oldest first (a commit that became empty is
// dropped and absent); the last one is the tree the caller's index was verified
// against. Files are the paths the patch touches (both sides of a rename).
type Result struct {
	SchemaVersion int        `json:"schema_version"`
	OK            bool       `json:"ok"`
	Error         *ErrorInfo `json:"error,omitempty"`
	Subcommand    string     `json:"subcommand"`
	Steps         []Step     `json:"steps"`
	Picked        []string   `json:"picked"`
	Files         []string   `json:"files"`
	PatchPath     string     `json:"patch_path"`
	Conflict      *Conflict  `json:"conflict,omitempty"`
}

func newResult(sub string) *Result {
	return &Result{SchemaVersion: SchemaVersion, Subcommand: sub, Steps: []Step{}, Picked: []string{}, Files: []string{}}
}

// NewErrorResult builds an envelope for early-return errors in the cli layer.
func NewErrorResult(subcommand, kind, message string) *Result {
	r := newResult(subcommand)
	r.Error = &ErrorInfo{Kind: kind, Message: message}
	return r
}

func (r *Result) step(status, msg string) {
	r.Steps = append(r.Steps, Step{Status: status, Message: msg})
}

// Error is the typed package error; Code is one of the Exit* constants.
type Error struct {
	Code        int
	Kind        string
	Message     string
	Remediation string
}

func (e *Error) Error() string {
	if e.Remediation != "" {
		return fmt.Sprintf("%s: %s (hint: %s)", e.Kind, e.Message, e.Remediation)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// ExitCode satisfies the cli-layer exit-coder interface.
func (e *Error) ExitCode() int {
	if e.Code == 0 {
		return ExitGeneric
	}
	return e.Code
}

// fail records e in r and returns it.
func fail(r *Result, e *Error) error {
	r.OK = false
	r.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
	return e
}
