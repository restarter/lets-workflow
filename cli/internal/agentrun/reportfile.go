//go:build unix

package agentrun

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// reportFile is the provider for an agent LETS has no transcript for (Antigravity,
// Claude, any other agent Orca names). The --send brief asks the agent to write its
// final report to AgentReport(base) and then create AgentDone(base); the done file
// ends the wait. Nothing proves the agent wrote what it was asked: the report stays
// untrusted and unverified, like every other.
type reportFile struct{ name string }

func (p reportFile) Name() string { return p.name }

// Run: a headless run exists only for Codex.
func (p reportFile) Run(context.Context, Request) Result {
	return Result{Provider: p.name, Reason: ReasonHeadlessUnsupported, Warnings: []string{}}
}

// Await polls for the done file (a regular file, not older than the send) with a
// 1-5 s backoff, then copies the agent's report into the LETS report path.
func (p reportFile) Await(ctx context.Context, r AwaitRequest) Result {
	report, _, _ := Outputs(r.OutBase)
	res := Result{Provider: p.name, Ran: true, ReportPath: report, Warnings: []string{}}
	if _, err := os.Lstat(report); err == nil {
		res.Reason = ReasonReportExists
		return res
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	start := now()
	deadline := start.Add(timeout)
	floor := r.Since.Add(-5 * time.Second)
	backoff := time.Second
	for {
		if fi, err := os.Lstat(AgentDone(r.OutBase)); err == nil && fi.Mode().IsRegular() && !fi.ModTime().Before(floor) {
			res.DurationMS = now().Sub(start).Milliseconds()
			text, err := readAgentReport(AgentReport(r.OutBase))
			switch {
			case err != nil:
				res.Reason, res.StderrTail = ReasonReportUnreadable, redact.Control(err.Error())
				return res
			case strings.TrimSpace(text) == "":
				res.Reason = ReasonReportEmpty
				return res
			}
			if err := writeNew(report, clean(text)); err != nil {
				res.Reason, res.StderrTail = ReasonIO, redact.Control(err.Error())
				return res
			}
			checkDrift(&res, r)
			res.Complete = true
			return res
		}
		if ctx.Err() != nil || !now().Before(deadline) {
			res.DurationMS = now().Sub(start).Milliseconds()
			res.Reason = ReasonTimeout
			if ctx.Err() != nil && now().Before(deadline) {
				res.Reason = ReasonCanceled
			}
			res.Warnings = append(res.Warnings, "no "+AgentDone(r.OutBase)+": the agent may still be working, lack write permission, or have skipped the report instruction")
			return res
		}
		sleep(backoff)
		if backoff < 5*time.Second {
			backoff += time.Second
		}
	}
}

// readAgentReport opens the agent's report without following a symlink (the agent
// chooses what it writes; a link could point at any file) and reads at most
// 2 x ReportCap bytes; clean caps it again.
func readAgentReport(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", errors.New("the agent report is not a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, 2*ReportCap))
	return string(b), err
}
