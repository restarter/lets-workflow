//go:build unix

package orcacmd

import (
	"context"
	"fmt"
	"os"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// phaseStatus maps a LETS phase to the Orca workspace status it sets ("" leaves the
// status and writes only the comment).
var phaseStatus = map[string]string{"start": "in-progress", "pr": "in-review", "closed": "completed", "end": "", "blocked": "", "gate": ""}

// CardOptions configures Card.
type CardOptions struct {
	Phase    string
	Comment  string
	Launcher string // LETS_LAUNCHER as the CLI resolved it; Card never reads config itself
}

// CardInfo is what Card did.
type CardInfo struct {
	Updated bool   `json:"updated"`
	Phase   string `json:"phase"`
	Status  string `json:"status,omitempty"`
	Target  string `json:"target,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// CardResult is the card-subcommand envelope.
type CardResult struct {
	Envelope
	Card *CardInfo `json:"card,omitempty"`
}

// Card mirrors a LETS phase onto this worktree's Orca card, best effort. Orca is an
// opt-in addon: without LETS_LAUNCHER=orca, or outside an Orca terminal, it returns
// a reason and runs nothing. Only an unknown phase is a hard error.
func Card(ctx context.Context, o CardOptions) (*CardResult, error) {
	res := &CardResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "card", Steps: []Step{}}}
	st, known := phaseStatus[o.Phase]
	if !known {
		res.Error = &ErrorInfo{Kind: "phase_invalid", Message: "unknown --phase " + o.Phase}
		return res, &Error{Code: ExitUsage, Kind: "phase_invalid", Message: "unknown --phase " + o.Phase + " (start|pr|closed|end|blocked|gate)"}
	}
	info := &CardInfo{Phase: o.Phase}
	res.Card, res.OK = info, true
	degrade := func(reason string) (*CardResult, error) {
		info.Reason = reason
		res.Steps = append(res.Steps, Step{Status: StepSkip, Message: "orca card skipped: " + reason})
		return res, nil
	}
	if o.Launcher != "orca" {
		return degrade(ReasonNotEnabled)
	}
	if os.Getenv("ORCA_WORKTREE_ID") == "" {
		return degrade(ReasonEnvAbsent)
	}
	c, f := NewClient()
	if f != nil {
		return degrade(f.Reason)
	}
	id, f := c.SelfWorktree(ctx)
	if f != nil {
		return degrade(f.Reason)
	}
	args := []string{"worktree", "set", "--worktree", "id:" + id}
	if cm := Truncate(redact.Control(redact.Text(o.Comment)), commentCap); cm != "" {
		args = append(args, "--comment", cm)
	}
	if st != "" {
		args = append(args, "--workspace-status", st)
	}
	if f := c.RunChecked(ctx, "worktree set", append(args, "--json")...); f != nil {
		return degrade(f.Reason)
	}
	info.Updated, info.Status, info.Target = true, st, id
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("card %s on %s", o.Phase, id)})
	return res, nil
}
