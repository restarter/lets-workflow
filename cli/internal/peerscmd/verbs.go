//go:build unix

package peerscmd

import (
	"context"
	"errors"
	"os"
	"time"

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

// RoleSet is `lets peers role set`.
func RoleSet(ctx context.Context, cwd string, o RoleOptions) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	root, e := rootOf(cwd)
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if o.Cwd == "" {
		o.Cwd = root
	}
	info, err := SetRole(root, o)
	if err != nil {
		var pe *Error
		if errors.As(err, &pe) {
			res.Error = &ErrorInfo{Kind: pe.Kind, Message: pe.Message}
			return res, pe
		}
		kind := "role_write_failed"
		if errors.Is(err, errPeersLockBusy) {
			kind = "peers_lock_busy"
		}
		res.Error = &ErrorInfo{Kind: kind, Message: err.Error()}
		return res, &Error{Code: ExitGeneric, Kind: kind, Message: err.Error(), Cause: err}
	}
	res.OK, res.Role = true, info
	return res, nil
}

// RoleClear is `lets peers role clear`.
func RoleClear(ctx context.Context, cwd, session string) (*RoleResult, error) {
	res := &RoleResult{Envelope: newEnvelope("role")}
	root, e := rootOf(cwd)
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if err := ClearRole(root, session); err != nil {
		var pe *Error
		if !errors.As(err, &pe) {
			pe = &Error{Code: ExitGeneric, Kind: "role_clear_failed", Message: err.Error(), Cause: err}
		}
		res.Error = &ErrorInfo{Kind: pe.Kind, Message: pe.Message}
		return res, pe
	}
	res.OK = true
	return res, nil
}

// Orchestrator is `lets peers orchestrator`: ResolveOrchestrator over this repo's
// registry rows and merge-branch records (no roles, no Orca), on the hot path of
// every session start, done, end and Orchestrator offer. It refreshes the last-seen
// file of each live orchestrator row it sees.
func Orchestrator(ctx context.Context, cwd, session string) (*OrchestratorResult, error) {
	res := &OrchestratorResult{Envelope: newEnvelope("orchestrator"), Candidates: []Candidate{}}
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
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if gitutil.ProjectRoot(cwd, 2*time.Second) == "" || mainRoot == "" {
		e := &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	v := loadNameView(ctx, mainRoot)
	r := ResolveOrchestrator(ctx, v, ResolveOptions{Session: session, Cwd: cwd, Branch: branch})
	refreshLastSeen(mainRoot, v.rows, v.pids)
	res.OK = true
	res.Source, res.Scope, res.Target, res.Candidates, res.Reason, res.Refused, res.Degraded = r.Source, r.Scope, r.Target, r.Candidates, r.Reason, r.Refused, r.Degraded
	res.Remediation, res.SelfRecord = r.Remediation, r.SelfRecord
	return res, nil
}
