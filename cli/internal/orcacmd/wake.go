//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

// WakeOptions configures Wake.
type WakeOptions struct {
	Repo    string // a main checkout
	Session string // the last-seen session of the orchestrator to resume
	Pid     int    // its recorded pid (0 unknown)
	Title   string // the orchestrator name (terminal title)
}

// WakeInfo is what Wake did.
type WakeInfo struct {
	Woken     bool   `json:"woken"`
	Handle    string `json:"handle,omitempty"`
	Satisfied bool   `json:"satisfied"` // the resumed session reached its idle prompt
	Reason    string `json:"reason,omitempty"`
}

// WakeResult is the wake-subcommand envelope.
type WakeResult struct {
	Envelope
	Wake *WakeInfo `json:"wake,omitempty"`
}

// Wake resumes an orchestrator that is not running in a visible Orca terminal, so a
// human can press the gates there. It refuses a session that is alive (main_alive)
// or whose liveness cannot be established (liveness_unknown): a second process on a
// live MAIN would fork its state.
func Wake(ctx context.Context, o WakeOptions) (*WakeResult, error) {
	res := &WakeResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "wake", Steps: []Step{}}}
	fail := func(kind, msg string) (*WakeResult, error) {
		res.Error = &ErrorInfo{Kind: kind, Message: msg}
		return res, &Error{Code: ExitUsage, Kind: kind, Message: msg}
	}
	if fi, err := os.Stat(o.Repo); o.Repo == "" || err != nil || !fi.IsDir() {
		res.Error = &ErrorInfo{Kind: "repo_invalid", Message: "--repo is not a directory"}
		return res, &Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: "--repo is not a directory"}
	}
	if inWt, main := gitutil.DetectInsideWorktreeAt(o.Repo); inWt || main == "" || !fsutil.SameDir(main, o.Repo) {
		res.Error = &ErrorInfo{Kind: "repo_invalid", Message: "--repo is not a main checkout"}
		return res, &Error{Code: ExitRepoInvalid, Kind: "repo_invalid", Message: "--repo is not a main checkout"}
	}
	if !ccregistry.ValidSession(o.Session) || o.Pid < 0 || !ccregistry.ValidName(o.Title) {
		return fail("usage", "wake needs a session id, a non-negative --pid and a valid --title")
	}
	info := &WakeInfo{}
	res.Wake, res.OK = info, true
	switch ccregistry.Read(ccregistry.HomeDir()).Liveness(o.Session, o.Pid) {
	case ccregistry.Alive:
		info.Reason = "main_alive"
		return res, nil
	case ccregistry.Unknown:
		info.Reason = "liveness_unknown"
		return res, nil
	}
	c, f := NewClient()
	if f != nil {
		info.Reason = f.Reason
		return res, nil
	}
	if _, f := c.Status(ctx); f != nil {
		info.Reason = f.Reason
		return res, nil
	}
	h, f := c.CreateTerminal(ctx, o.Repo, o.Title, "claude -r "+o.Session)
	if f != nil {
		info.Reason = f.Reason
		return res, nil
	}
	info.Woken, info.Handle = true, h
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("resumed %s in a new Orca terminal", o.Title)})
	h, up, f := c.WaitStartup(ctx, h, o.Repo, o.Title)
	info.Handle, info.Satisfied = h, up
	if f != nil {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "startup wait: " + f.Error()})
	}
	return res, nil
}
