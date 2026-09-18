//go:build unix

package handoffcmd

import (
	"context"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/agentrun"
)

// RunOptions configures Codex.
type RunOptions struct {
	Root, Brief string
	Timeout     time.Duration
}

// AwaitOptions configures Await.
type AwaitOptions struct {
	Root, Brief, Agent string
	Since              time.Time
	Fingerprint        string // send.fingerprint; "" skips the drift check
	Timeout            time.Duration
}

// Codex runs the brief through Codex headless (read-only sandbox) and writes the
// report next to it. The envelope goes to stdout only: the command reads the
// background run's own output, so no result file can go stale.
func Codex(ctx context.Context, o RunOptions) (*RunResult, error) {
	res := &RunResult{Envelope: newEnvelope("codex")}
	if err := CheckBrief(o.Root, o.Brief); err != nil {
		return res, briefInvalid(&res.Envelope, err)
	}
	p, _ := agentrun.Lookup("codex")
	r := p.Run(ctx, agentrun.Request{PromptFile: o.Brief, Dir: o.Root, OutBase: OutBase(o.Brief), Timeout: o.Timeout})
	res.OK, res.Run = true, &r
	return res, nil
}

// Await waits for the report of a brief sent to an agent's tab: Codex through its
// rollout, any other agent through the report-file contract. An agent name outside
// the provider shape is ok=true, ran=false, await_unsupported_agent.
func Await(ctx context.Context, o AwaitOptions) (*RunResult, error) {
	res := &RunResult{Envelope: newEnvelope("await")}
	if err := CheckBrief(o.Root, o.Brief); err != nil {
		return res, briefInvalid(&res.Envelope, err)
	}
	if o.Since.IsZero() || o.Agent == "" {
		return res, usage(&res.Envelope, "--agent and --since are required (send.agent, send.sent_at)")
	}
	p, ok := agentrun.Lookup(o.Agent)
	if !ok {
		res.OK, res.Run = true, &agentrun.Result{Provider: o.Agent, Reason: "await_unsupported_agent", Warnings: []string{}}
		return res, nil
	}
	r := p.Await(ctx, agentrun.AwaitRequest{Marker: o.Brief, Since: o.Since, OutBase: OutBase(o.Brief), Dir: o.Root, Fingerprint: o.Fingerprint, Timeout: o.Timeout})
	res.OK, res.Run = true, &r
	return res, nil
}
