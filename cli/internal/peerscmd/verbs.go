//go:build unix

package peerscmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

// RoleOptions configures RoleSet. Role, Task, Scope and Takeover are a v0.10.0
// caller's flags: accepted and ignored - nothing resolves roles any more.
type RoleOptions struct {
	Session  string
	Role     string
	Task     string
	Scope    string
	Cwd      string
	Takeover bool
}

// sessionNotInRegistry is the remedy when the registry does not list the caller.
const sessionNotInRegistry = "Claude Code's session registry does not list this session - restart it (or update Claude Code), then run /lets:start again"

// RoleSet is the deprecated `lets peers role set`: it records this session's proof
// (pid and set time, from the registry) for the SessionStart /clear carry, until
// lets-gbfpd replaces it. No name, uniqueness or role semantics. A session the
// registry does not list gets ok=false and no file: a pid-less proof could never
// prove a carry. Exit 0 either way - a v0.10.0 caller prints a non-ok envelope as
// one line.
func RoleSet(ctx context.Context, cwd string, o RoleOptions) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	res.Deprecated = deprecatedText
	if !ccregistry.ValidSession(o.Session) {
		e := &Error{Code: ExitUsage, Kind: "usage", Message: "role set needs a valid --session"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	root, e := rootOf(cwd)
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	CarryProof(ctx, root) // the lazy proof carry, before this session's own write
	info := &RoleInfo{Role: o.Role}
	res.Role = info
	self, ok := ccregistry.Read(ccregistry.HomeDir()).Find(o.Session)
	if !ok {
		info.Reason, info.Remediation = "session_not_in_registry", sessionNotInRegistry
		res.Error = &ErrorInfo{Kind: "session_not_in_registry", Message: "the session registry does not list this session", Remediation: sessionNotInRegistry}
		return res, nil
	}
	info.Registered = true
	unlock, err := lockPeers(root, proofLockWait)
	if err != nil {
		kind := "proof_write_failed"
		if errors.Is(err, errPeersLockBusy) {
			kind = "peers_lock_busy"
		}
		res.Error = &ErrorInfo{Kind: kind, Message: err.Error()}
		return res, &Error{Code: ExitGeneric, Kind: kind, Message: err.Error(), Cause: err}
	}
	defer unlock()
	if err := writeProof(root, o.Session, self.Pid, now()); err != nil {
		res.Error = &ErrorInfo{Kind: "proof_write_failed", Message: err.Error()}
		return res, &Error{Code: ExitGeneric, Kind: "proof_write_failed", Message: err.Error(), Cause: err}
	}
	res.OK, info.Granted = true, true
	return res, nil
}

// RoleClear is the deprecated `lets peers role clear`: it removes this session's
// proof file.
func RoleClear(ctx context.Context, cwd, session string) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	res.Deprecated = deprecatedText
	fail := func(e *Error) (*RoleResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if !ccregistry.ValidSession(session) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "role clear needs a valid --session"})
	}
	root, e := rootOf(cwd)
	if e != nil {
		return fail(e)
	}
	unlock, err := lockPeers(root, 0)
	if err != nil {
		return fail(&Error{Code: ExitGeneric, Kind: "role_clear_failed", Message: err.Error(), Cause: err})
	}
	defer unlock()
	if err := os.Remove(filepath.Join(peersDir(root), session+".role")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(&Error{Code: ExitGeneric, Kind: "role_clear_failed", Message: err.Error(), Cause: err})
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
	CarryProof(ctx, mainRoot) // the lazy proof carry: recovers one the hook missed
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
