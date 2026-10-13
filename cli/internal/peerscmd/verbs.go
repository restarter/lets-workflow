//go:build unix

package peerscmd

import (
	"context"
	"os"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

func rootOf(cwd string) (string, *Error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	root := gitutil.ProjectRoot(cwd, 2*time.Second)
	if root == "" {
		return "", &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	return root, nil
}

// RoleOptions configures RoleSet: a v0.10.0 caller's flags, accepted and ignored -
// nothing resolves roles or records a proof any more.
type RoleOptions struct {
	Session  string
	Role     string
	Task     string
	Scope    string
	Cwd      string
	Takeover bool
}

// RoleSet is the deprecated `lets peers role set`: a v0.10.0 caller's call that
// records nothing (the /clear proof is the task-state session_proof key, lets-gbfpd)
// and takes no lock. Every flag still parses; a valid --session answers ok=true
// granted=true with deprecated, so a v0.10.0 plugin's gates pass.
func RoleSet(_ context.Context, _ string, o RoleOptions) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	res.Deprecated = deprecatedText
	if !ccregistry.ValidSession(o.Session) {
		e := &Error{Code: ExitUsage, Kind: "usage", Message: "role set needs a valid --session"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	res.OK, res.Role = true, &RoleInfo{Role: o.Role, Granted: true}
	return res, nil
}

// RoleClear is the deprecated `lets peers role clear`: records nothing, removes
// nothing (a legacy .role file is never read or written), answers ok=true.
func RoleClear(_ context.Context, _ string, session string) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	res.Deprecated = deprecatedText
	if !ccregistry.ValidSession(session) {
		e := &Error{Code: ExitUsage, Kind: "usage", Message: "role clear needs a valid --session"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	res.OK = true
	return res, nil
}

// Orchestrator is `lets peers orchestrator`: ResolveOrchestrator over this repo's
// registry rows and merge-branch records (no roles, no Orca), on the hot path of
// every session start, done, end and Orchestrator offer. It refreshes the last-seen
// file of each live orchestrator row it sees.
func Orchestrator(ctx context.Context, cwd, session string) (*OrchestratorResult, error) {
	return OrchestratorWith(ctx, cwd, session, StandingProbe{})
}

// OrchestratorWith is Orchestrator answering, for a probe (the harness envelope of a
// message this worker or lead received), whether its bound orchestrator sent it under
// a standing grant: standing_ok / standing_reason (standingFromOrchestrator).
func OrchestratorWith(ctx context.Context, cwd, session string, probe StandingProbe) (*OrchestratorResult, error) {
	res := &OrchestratorResult{Envelope: newEnvelope("orchestrator"), Candidates: []Candidate{}}
	if probe.given() {
		// every return carries the answer once the probe was passed: an error is false
		defer func() { standingOnError(&res.Standing, res.Error) }()
	}
	if e := probe.usage(); e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	// Read the branch on the INCOMING, unbounded ctx, before the budget below starts.
	// exec.CommandContext fails instantly on an expired context, and an unreadable
	// branch looks unbound - which would re-route to a DIFFERENT live orchestrator
	// instead of the one this branch is bound to (lets-cbmg7).
	branch := branchOf(ctx, cwd)
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	mainRoot, e := mainRootOf(cwd)
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	v := loadNameView(ctx, mainRoot)
	r := ResolveOrchestrator(ctx, v, ResolveOptions{Session: session, Cwd: cwd, Branch: branch})
	refreshLastSeen(mainRoot, v.rows, v.pids)
	res.OK = true
	res.Source, res.Scope, res.Target, res.Candidates, res.Reason, res.Refused, res.Degraded = r.Source, r.Scope, r.Target, r.Candidates, r.Reason, r.Refused, r.Degraded
	res.Remediation, res.SelfRecord = r.Remediation, r.SelfRecord
	if probe.given() {
		res.Standing = standingOf(standingFromOrchestrator(v, r, probe))
	}
	return res, nil
}
