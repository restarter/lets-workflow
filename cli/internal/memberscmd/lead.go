//go:build unix

package memberscmd

import (
	"fmt"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// Lead is the session recorded as the scope's lead - recorded, never inferred
// from who happens to run in the worktree. Name is its registry name (e.g.
// `<callsign>-lead`) when it has one.
type Lead struct {
	Session string    `json:"session"`
	Name    string    `json:"name"`
	Pid     int       `json:"pid"`
	Set     time.Time `json:"set"`
}

// LeadStatus is the lead as judged now: live (its session id runs, under any pid),
// rotated (re-minted in the same process, carried), gone(session_dead) or unknown.
type LeadStatus struct {
	Lead
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// refreshLeadDeadline bounds RefreshLead's wait for the lock: it runs on the
// SessionStart path beside the task-state guard's 1 s deadline.
const refreshLeadDeadline = time.Second

// judgeLead judges l and carries a rotation into it (l.Session becomes the new id);
// carried reports that l changed.
func judgeLead(l *Lead, snap ccregistry.Snapshot) (ls *LeadStatus, carried bool) {
	status, reason, ns := judgeSession(snap, l.Session, l.Pid, l.Set, true)
	if ns != "" {
		l.Session, carried = ns, true
	}
	return &LeadStatus{Lead: *l, Status: status, Reason: reason}, carried
}

func noLead(scope string) *Error {
	return &Error{Code: ExitNoLead, Kind: "no_lead", Message: scope + " has no recorded lead",
		Remediation: "the lead session runs `lets members lead --claim --scope " + scope + "`"}
}

// ShowLead reports the recorded lead, judged now (a rotation is carried).
func ShowLead(o Options) (*LeadResult, error) {
	res := &LeadResult{Envelope: newEnvelope("lead", o.Scope)}
	if !ValidScope(o.Scope) {
		return res, fail(&res.Envelope, scopeErr(o.Scope))
	}
	unlock, err := lock(o.Root, o.Scope, o.LockDeadline)
	if err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	defer unlock()
	reg, _, err := load(o.Root, o.Scope)
	if err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	if reg.Lead == nil {
		return res, fail(&res.Envelope, noLead(o.Scope))
	}
	ls, carried := judgeLead(reg.Lead, readRegistry())
	if carried {
		if err := save(o.Root, reg); err != nil {
			return res, fail(&res.Envelope, asErr(err))
		}
	}
	res.Lead = ls
	res.OK = true
	return res, nil
}

// ClaimLead records this session as the lead. Allowed when no lead is recorded,
// when the recorded lead is this session (pid / set refreshed) or was re-minted
// into it, or when the recorded lead is gone (the reopen case: a dead lead is
// taken over). Refused lead_held while another lead is live, rotated into another
// session, or cannot be judged.
func ClaimLead(o Options) (*LeadResult, error) {
	res := &LeadResult{Envelope: newEnvelope("lead", o.Scope)}
	if !ValidScope(o.Scope) {
		return res, fail(&res.Envelope, scopeErr(o.Scope))
	}
	snap := readRegistry()
	caller, e := callerEntry(snap, o.Session)
	if e != nil {
		return res, fail(&res.Envelope, e)
	}
	unlock, err := lock(o.Root, o.Scope, o.LockDeadline)
	if err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	defer unlock()
	reg, _, err := load(o.Root, o.Scope)
	if err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	if old := reg.Lead; old != nil && old.Session != o.Session {
		prev := *old
		ls, _ := judgeLead(&prev, snap)
		switch {
		case ls.Status == StatusRotated && prev.Session == o.Session:
			res.Steps = append(res.Steps, Step{Status: StepOK, Message: "the lead session was re-minted into this one; carried"})
		case ls.Status == StatusGone:
			res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("took over from %s (%s)", leadLabel(old), ls.Reason)})
		default:
			res.Lead = ls
			return res, fail(&res.Envelope, &Error{Code: ExitLeadHeld, Kind: "lead_held",
				Message:     fmt.Sprintf("%s holds the lead of %s and is %s", leadLabel(&ls.Lead), o.Scope, ls.Status),
				Remediation: "continue in that session, or close it and claim again"})
		}
	}
	reg.Lead = &Lead{Session: caller.SessionID, Name: caller.Name, Pid: caller.Pid, Set: Now().UTC()}
	if err := save(o.Root, reg); err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	res.Lead = &LeadStatus{Lead: *reg.Lead, Status: StatusLive}
	res.Claimed = true
	res.OK = true
	return res, nil
}

func leadLabel(l *Lead) string {
	if l.Name != "" {
		return fmt.Sprintf("%s (session %s)", l.Name, short(l.Session))
	}
	return "session " + short(l.Session)
}

// ReadLead returns the recorded lead of scope without judging or writing it; nil
// when none is recorded (or the scope has no registry yet).
func ReadLead(root, scope string) (*Lead, error) {
	if !ValidScope(scope) {
		return nil, scopeErr(scope)
	}
	reg, _, err := load(root, scope)
	if err != nil {
		return nil, err
	}
	return reg.Lead, nil
}

// RefreshLead refreshes the recorded lead's pid and set time when sid IS the
// recorded lead (a `claude -r` resume runs the same id under a new pid). Any other
// sid changes nothing and returns lead_held; no lead returns no_lead. Used by the
// SessionStart hook, so the lock wait is bounded and best-effort: a busy lock
// returns nil and leaves the record as it was (the next start refreshes it).
func RefreshLead(root, scope, sid string) error {
	if !ValidScope(scope) {
		return scopeErr(scope)
	}
	unlock, err := lock(root, scope, refreshLeadDeadline)
	if err != nil {
		if asErr(err).Code == ExitLockBusy {
			return nil
		}
		return err
	}
	defer unlock()
	reg, _, err := load(root, scope)
	if err != nil {
		return err
	}
	if reg.Lead == nil {
		return noLead(scope)
	}
	if sid == "" || reg.Lead.Session != sid {
		return &Error{Code: ExitLeadHeld, Kind: "lead_held", Message: fmt.Sprintf("%s is not the recorded lead of %s", "session "+short(sid), scope)}
	}
	e, cerr := callerEntry(readRegistry(), sid)
	if cerr != nil {
		return cerr
	}
	if e.Pid == reg.Lead.Pid {
		return nil
	}
	reg.Lead.Pid, reg.Lead.Set = e.Pid, Now().UTC()
	if e.Name != "" {
		reg.Lead.Name = e.Name
	}
	return save(root, reg)
}
