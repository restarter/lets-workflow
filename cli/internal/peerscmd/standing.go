//go:build unix

package peerscmd

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
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
	"standing_off", "self_name_not_unique", "not_own_name", "not_bound_here",
	"not_bound", "not_bound_target", "cross_repo",
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

// liveByName is every live registry entry (one per process) carrying name, machine-wide.
func liveByName(snap ccregistry.Snapshot, name string) []ccregistry.Entry {
	var hits []ccregistry.Entry
	for _, e := range snap.Entries {
		if e.NameOK && e.Name == name {
			hits = append(hits, e)
		}
	}
	return hits
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
	hits := liveByName(v.snap, p.FromName)
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
	e, ok := v.snap.Find(self)
	if !ok || !e.NameOK {
		return false, "standing_off" // a caller without a name holds no record
	}
	if len(liveByName(v.snap, e.Name)) != 1 {
		return false, "self_name_not_unique"
	}
	granted := false
	for _, r := range v.recs {
		if r.Name == e.Name {
			granted = r.Standing == "on"
			break
		}
	}
	switch {
	case !granted:
		return false, "standing_off"
	case own != e.Name:
		return false, "not_own_name"
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
