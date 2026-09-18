//go:build unix

// Package agentrun runs a prompt through an external agent and captures the agent's
// final report. It is the provider-neutral seam lets-9yz68 builds on: a Provider
// starts its agent headless (Run), and learns that a turn started in a session
// someone else runs has finished and where its final report is (Await). Codex reads
// back through its rollout; every other named agent through the report-file
// contract.
//
// A leaf package (standard library and redact only, pinned by TestLeafPackages), so
// any command package can import it without a cycle.
package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// Reasons a Result carries. A provider's own absence is "<name>_not_found".
const (
	ReasonTimeout             = "timeout"
	ReasonCanceled            = "canceled"
	ReasonExitNonZero         = "exit_nonzero"
	ReasonTurnFailed          = "turn_failed"
	ReasonTurnAborted         = "turn_aborted"
	ReasonReportEmpty         = "report_empty"
	ReasonReportExists        = "report_exists"
	ReasonReportUnreadable    = "report_unreadable"
	ReasonMarkerNotFound      = "marker_not_found"
	ReasonMarkerAmbiguous     = "marker_ambiguous"
	ReasonHeadlessUnsupported = "headless_unsupported"
	ReasonIO                  = "io_error"
)

// DefaultTimeout bounds a run or a wait; a real Codex plan review took 454 s.
const DefaultTimeout = 30 * time.Minute

// ReportCap bounds a written report in bytes; redact.Cap marks what it drops.
const ReportCap = 128 << 10

// Request is one headless run.
type Request struct {
	PromptFile string        // its content is the prompt, sent on stdin
	Dir        string        // the repository root the agent reads and never writes
	OutBase    string        // outputs: OutBase + "-report.md" / "-events.jsonl" / "-stderr.txt"
	Timeout    time.Duration // <= 0 means DefaultTimeout
}

// AwaitRequest waits for the end of the turn a delivered prompt started in a
// session the provider did not start (an Orca tab).
type AwaitRequest struct {
	Marker      string        // unique to the delivered prompt: the brief's absolute path
	Since       time.Time     // when the prompt was sent
	OutBase     string        // the report goes to OutBase + "-report.md"
	Dir         string        // the checkout, for the drift check
	Fingerprint string        // Fingerprint(Dir) at send time; "" skips the drift check
	Timeout     time.Duration // <= 0 means DefaultTimeout
}

// Result is what a run or a wait established. Ran=false is a named degrade, never
// an error; Complete=true means ReportPath holds the agent's final report, redacted
// and capped.
type Result struct {
	Provider         string   `json:"provider"`
	Ran              bool     `json:"ran"`
	Complete         bool     `json:"complete"`
	Reason           string   `json:"reason,omitempty"`
	ExitCode         int      `json:"exit_code"`
	SessionID        string   `json:"session_id,omitempty"`
	ReportPath       string   `json:"report_path,omitempty"`
	EventsPath       string   `json:"events_path,omitempty"`
	StderrPath       string   `json:"stderr_path,omitempty"`
	RolloutPath      string   `json:"rollout_path,omitempty"`
	StderrTail       string   `json:"stderr_tail,omitempty"`
	DurationMS       int64    `json:"duration_ms"`
	WorkspaceChanged bool     `json:"workspace_changed"`
	Warnings         []string `json:"warnings"`
}

// Provider is one external agent.
type Provider interface {
	Name() string
	Run(ctx context.Context, r Request) Result
	Await(ctx context.Context, r AwaitRequest) Result
}

// agentNameRe is the shape of an agent name Orca reports (claude, codex,
// antigravity, ...).
var agentNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Lookup returns the provider for an agent name as Orca reports it. Codex keeps a
// transcript LETS reads; any other named agent returns its report through the
// report-file contract.
func Lookup(name string) (Provider, bool) {
	switch {
	case name == "codex":
		return codex{}, true
	case agentNameRe.MatchString(name):
		return reportFile{name: name}, true
	}
	return nil, false
}

// Outputs derives the LETS-written output paths from a base.
func Outputs(base string) (report, events, stderr string) {
	return base + "-report.md", base + "-events.jsonl", base + "-stderr.txt"
}

// AgentReport and AgentDone are the two files the report-file contract asks an
// agent to write (the brief names them).
func AgentReport(base string) string { return base + "-agent-report.md" }
func AgentDone(base string) string   { return base + "-agent-report.done" }

// clean redacts and caps agent output before it is written where LETS reads it:
// secrets and URL credentials, then control bytes (an escape sequence in a relayed
// report would restyle or rewrite the reader's terminal), then the cap.
func clean(text string) string {
	return redact.Cap(redact.Control(redact.Text(redact.Creds(text))), ReportCap)
}

// writeNew writes a file that must not exist yet, mode 0600.
func writeNew(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// replace rewrites path atomically: a 0600 sibling temp file, then rename.
func replace(path, text string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agentrun-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// checkDrift marks a wait whose checkout no longer has the send-time fingerprint.
func checkDrift(res *Result, r AwaitRequest) {
	if r.Fingerprint != "" && r.Dir != "" && Fingerprint(r.Dir) != r.Fingerprint {
		res.WorkspaceChanged = true
		res.Warnings = append(res.Warnings, "the working tree changed between the send and the report")
	}
}
