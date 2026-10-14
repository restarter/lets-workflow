//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// StandingProbe is the harness envelope of one inbound message, passed by the orc
// skill as --from-name / --from-addr: from-name is the sender's claimed session name,
// from-addr its socket (`uds:/tmp/cc-socks/<pid>.sock`), which carries the pid of the
// sending Claude Code process. Both or neither.
type StandingProbe struct {
	FromName string
	FromAddr string
}

// given reports whether the probe was passed at all.
func (p StandingProbe) given() bool { return p.FromName != "" || p.FromAddr != "" }

// usage refuses a probe with one half missing.
func (p StandingProbe) usage() *Error {
	if (p.FromName == "") != (p.FromAddr == "") {
		return &Error{Code: ExitUsage, Kind: "usage", Message: "--from-name and --from-addr go together"}
	}
	return nil
}

// standingReasons is every reason a standing answer can be false with; the README
// and the orc skill name the same set.
var standingReasons = []string{
	"from_addr_unparseable", "liveness_unknown", "sender_not_live", "name_not_unique", "pid_mismatch",
	"no_self_record", "not_own_name", "self_name_not_unique", "standing_off", "not_bound_here",
	"not_bound", "not_bound_target", "cross_repo",
}

// standingOnError fills a probe's answer on a return that never reached the decision:
// standing_ok=false, standing_reason = the error kind ("error" without one).
func standingOnError(s *Standing, e *ErrorInfo) {
	if s.StandingOK != nil {
		return
	}
	reason := "error"
	if e != nil && e.Kind != "" {
		reason = e.Kind
	}
	*s = standingOf(false, reason)
}

// senderPid parses the pid out of an envelope from attribute: `uds:` + a path whose
// base is `<digits>.sock`.
func senderPid(addr string) (int, bool) {
	path, ok := strings.CutPrefix(addr, "uds:")
	if !ok || path == "" {
		return 0, false
	}
	digits, ok := strings.CutSuffix(filepath.Base(path), ".sock")
	if !ok || digits == "" {
		return 0, false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	pid, err := strconv.Atoi(digits)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// standingSender is the process that sent the message: the ONE live entry named
// p.FromName, and its pid must be the pid of p.FromAddr. A name alone is a claim any
// session can make; the socket pid is the harness's.
func standingSender(v *nameView, p StandingProbe) (ccregistry.Entry, string) {
	pid, ok := senderPid(p.FromAddr)
	if !ok {
		return ccregistry.Entry{}, "from_addr_unparseable"
	}
	if v.snap.Degraded != nil { // an unreadable live entry may carry the name
		return ccregistry.Entry{}, "liveness_unknown"
	}
	hits := v.snap.Named(p.FromName)
	switch {
	case len(hits) == 0:
		return ccregistry.Entry{}, "sender_not_live"
	case len(hits) > 1:
		return ccregistry.Entry{}, "name_not_unique"
	case hits[0].Pid != pid:
		return ccregistry.Entry{}, "pid_mismatch"
	}
	return hits[0], ""
}

// repoRowOf is this repo's row of process e: same session, same name, and the row was
// built from that very pid. nil for a process outside this repo.
func repoRowOf(v *nameView, e ccregistry.Entry) *Peer {
	for i := range v.rows {
		if p := &v.rows[i]; p.Session == e.SessionID && p.Name == e.Name && v.pids[e.SessionID] == e.Pid {
			return p
		}
	}
	return nil
}

// standingFromWorker is the orchestrator side (`who --orc <own>`): may this session,
// holding a standing grant, answer the message without asking its user? Only when the
// sender is the process it claims to be, the caller is the unique holder of a
// standing=on record named own, and the sender is a row of this repo bound to own.
func standingFromWorker(v *nameView, self, own string, p StandingProbe) (bool, string) {
	sender, why := standingSender(v, p)
	if why != "" {
		return false, why
	}
	// the caller's identity and record first, so the reason names the real cause;
	// the grant itself last
	e, ok := v.snap.Find(self)
	if !ok || !e.NameOK {
		return false, "no_self_record" // a caller without a registry name holds no record
	}
	if own != e.Name {
		return false, "not_own_name"
	}
	if len(v.snap.Named(e.Name)) != 1 {
		return false, "self_name_not_unique"
	}
	var rec *taskstate.Orchestrator
	for i := range v.recs {
		if v.recs[i].Name == e.Name {
			rec = &v.recs[i]
			break
		}
	}
	switch {
	case rec == nil:
		return false, "no_self_record"
	case rec.Standing != "on":
		return false, "standing_off"
	}
	if row := repoRowOf(v, sender); row == nil || row.Orc != own {
		return false, "not_bound_here"
	}
	return true, ""
}

// standingFromOrchestrator is the worker / lead side (`orchestrator`): may this
// session act on its bound orchestrator's message without asking its user? Only when
// the sender is the process it claims to be, it is this branch's bound target, it runs
// in this repo (cross-repo is never standing), and its record has standing=on.
func standingFromOrchestrator(v *nameView, r *Resolution, p StandingProbe) (bool, string) {
	sender, why := standingSender(v, p)
	if why != "" {
		return false, why
	}
	if r.Source != "bound" || r.Target == nil {
		return false, "not_bound"
	}
	if p.FromName != r.Target.Name {
		return false, "not_bound_target"
	}
	row := repoRowOf(v, sender)
	if row == nil {
		return false, "cross_repo"
	}
	if !row.Standing {
		return false, "standing_off"
	}
	return true, ""
}

// Sender is `lets peers sender`: provenance of one inbound message, any repo.
// Both probe halves are required; every failure answers sender_ok=false with the
// reason (fail closed). Mirrors OrchestratorWith's error handling.
func Sender(ctx context.Context, cwd string, p StandingProbe) (*SenderResult, error) {
	res := &SenderResult{Envelope: newEnvelope("sender")}
	if !p.given() {
		e := &Error{Code: ExitUsage, Kind: "usage", Message: "--from-name and --from-addr are both required"}
		res.Error, res.SenderReason = &ErrorInfo{Kind: e.Kind, Message: e.Message}, e.Kind
		return res, e
	}
	if e := p.usage(); e != nil {
		res.Error, res.SenderReason = &ErrorInfo{Kind: e.Kind, Message: e.Message}, e.Kind
		return res, e
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	mainRoot, e := mainRootOf(cwd)
	if e != nil {
		res.Error, res.SenderReason = &ErrorInfo{Kind: e.Kind, Message: e.Message}, e.Kind
		return res, e
	}
	v := loadNameView(ctx, mainRoot)
	res.OK, res.Degraded = true, v.degraded
	sender, why := standingSender(v, p)
	if why != "" {
		res.SenderReason = why
		return res, nil
	}
	res.SenderOK = true
	res.Name, res.Cwd = sender.Name, sender.Cwd
	if len(sender.SessionID) >= 6 {
		res.Session6 = sender.SessionID[:6]
	}
	res.InRepo = repoRowOf(v, sender) != nil
	return res, nil
}
