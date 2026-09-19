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

// Orchestrator is `lets peers orchestrator`. Bound like Who: it now pays peers()'s
// cost (a git subprocess and a transcript stat per registry row) on the hot path of
// every session start, done, end and Orchestrator offer.
func Orchestrator(ctx context.Context, cwd, session string) (*OrchestratorResult, error) {
	res := &OrchestratorResult{Envelope: newEnvelope("orchestrator"), Candidates: []Candidate{}}
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	rc, err := loadRepo(ctx, cwd, false)
	if err != nil {
		e := err.(*Error)
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if cwd == "" {
		cwd = rc.root
	}
	r := ResolveOrchestrator(ctx, rc, ResolveOptions{Session: session, Cwd: cwd})
	res.OK = true
	res.Source, res.Scope, res.Target, res.Candidates, res.Reason, res.Refused, res.Degraded = r.Source, r.Scope, r.Target, r.Candidates, r.Reason, r.Refused, r.Degraded
	return res, nil
}
