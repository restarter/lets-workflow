//go:build unix

// Package memberscmd is `lets members`: the registry of the members a lead spawned
// in one scope - a standing team (`<callsign>`) or one delegated execute run
// (`run-<RUN>`) - and of the scope's lead. It is the ONE place liveness is judged:
// a member is live only while its session id AND its pid still match the Claude
// Code session registry; a same-process re-mint (/clear) is carried to the new id;
// anything else is gone with a named reason. The model never edits the file.
//
// Store `.lets/execution/members-<scope>.json`, written only under
// `.lets/locks/members-<scope>.lock` through fsutil.AtomicWriteBytes.
//
// Leaf package: standard library plus the ccregistry, fsutil and teamfile leaves.
package memberscmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// ShippedRoles are the agents a member may run as: every shipped lets:* agent
// except actor (a meta-agent that needs an explicit personality). Pinned against
// plugins/lets/agents/*.md by TestShippedRoles_MatchAgents.
var ShippedRoles = []string{
	"architect", "backend", "compliance", "database", "devops", "docs", "explorer", "frontend",
	"git-historian", "implementer", "pragmatist", "qa", "security", "skeptic",
}

// Stored member states; the judged ones below are what status reports.
const (
	StatusActive    = "active"
	StatusDismissed = "dismissed"

	StatusLive    = "live"
	StatusRotated = "rotated"
	StatusGone    = "gone"
	StatusUnknown = "unknown" // the registry cannot prove either way - never treated as gone
)

// Gone reasons.
const (
	ReasonSessionDead     = "session_dead"
	ReasonWorktreeMissing = "worktree_missing"
	ReasonPrunable        = "prunable"
	ReasonDirMissing      = "dir_missing"
	ReasonDismissed       = "dismissed"
)

// Member kinds: a pane member is its own registry session; an in-process member
// runs inside the lead's process and is recorded with the lead's session.
const (
	KindPane      = "pane"
	KindInProcess = "in_process"
)

// Links: team = reachable as the lead's own teammate; peer = only across sessions
// (the lead that spawned it is gone or was replaced).
const (
	LinkTeam = "team"
	LinkPeer = "peer"
)

// Member is one registry entry. Session / Pid / Set are the session the member's
// liveness is judged by (its own for a pane member, the lead's for an in-process
// one); Status is stored as active | dismissed and reported as judged.
type Member struct {
	Name           string    `json:"name"`
	AgentName      string    `json:"agent_name"`
	Role           string    `json:"role"`
	Model          string    `json:"model"`
	Isolation      string    `json:"isolation"`
	WorktreePath   string    `json:"worktree_path"`
	WorktreeBranch string    `json:"worktree_branch"`
	Kind           string    `json:"kind"`
	Session        string    `json:"session"`
	Pid            int       `json:"pid"`
	Set            time.Time `json:"set"`
	CallerToplevel string    `json:"caller_toplevel"`
	LeadPid        int       `json:"lead_pid"`
	Link           string    `json:"link"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason,omitempty"`
}

// Options is what every verb needs; Session and CallerToplevel are computed by
// the caller (the cli layer: $CLAUDE_CODE_SESSION_ID and the git toplevel), never
// taken from a flag.
type Options struct {
	Root           string // the checkout whose .lets holds the registry (the main checkout)
	CallerToplevel string
	Session        string
	Scope          string
	LockDeadline   time.Duration // zero = defaultLockDeadline
}

// AddOptions are add's flags.
type AddOptions struct {
	Name, Role, Model, Isolation, WorktreePath, WorktreeBranch, Link string
	Cwd                                                              string // where a pane member's session runs; default below
}

const (
	defaultLockDeadline = 5 * time.Second
	lookupAttempts      = 5
	lookupInterval      = time.Second
)

// Seams. Now and Sleep are exported for the external test package.
var (
	Now          = time.Now
	Sleep        = time.Sleep
	readRegistry = func() ccregistry.Snapshot { return ccregistry.Read(ccregistry.HomeDir()) }
)

var (
	nameRe     = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	runScopeRe = regexp.MustCompile(`^run-[a-z0-9-]{1,36}$`)
	modelRe    = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]{0,64}$`)
)

// ValidScope: a team callsign (teamfile.ValidName) or an execute scope run-<RUN>.
func ValidScope(s string) bool { return teamfile.ValidName(s) || runScopeRe.MatchString(s) }

// executeScope reports an execute run's scope; any other valid scope is a team's.
func executeScope(s string) bool { return strings.HasPrefix(s, "run-") }

// ValidName is the member-name grammar.
func ValidName(s string) bool { return nameRe.MatchString(s) }

// AgentName is the Agent name a member runs under: `<callsign>-<name>` in a team
// scope (machine-unique pane names), the bare name in an execute scope (the run
// id is already in it, e.g. impl-<RUN>-c1).
func AgentName(scope, name string) string {
	if executeScope(scope) {
		return name
	}
	return scope + "-" + name
}

// normalizeRole maps `lets:<r>` or `<r>` to `lets:<r>` when r is a shipped role.
func normalizeRole(r string) (string, bool) {
	bare := strings.TrimPrefix(r, "lets:")
	if !slices.Contains(ShippedRoles, bare) {
		return "", false
	}
	return "lets:" + bare, true
}

type registryFile struct {
	Schema  int      `json:"schema"`
	Scope   string   `json:"scope"`
	Lead    *Lead    `json:"lead"`
	Members []Member `json:"members"`
}

func registryPath(root, scope string) string {
	return filepath.Join(root, ".lets", "execution", "members-"+scope+".json")
}

// load reads the registry; a missing file is an empty registry (exists=false).
func load(root, scope string) (reg registryFile, exists bool, err error) {
	reg = registryFile{Schema: 1, Scope: scope, Members: []Member{}}
	data, err := os.ReadFile(registryPath(root, scope))
	if errors.Is(err, os.ErrNotExist) {
		return reg, false, nil
	}
	if err != nil {
		return reg, false, ioErr(err)
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, true, ioErr(fmt.Errorf("%s: %w", registryPath(root, scope), err))
	}
	if reg.Scope != scope {
		return reg, true, ioErr(fmt.Errorf("%s names scope %q", registryPath(root, scope), reg.Scope))
	}
	if reg.Members == nil {
		reg.Members = []Member{}
	}
	return reg, true, nil
}

func save(root string, reg registryFile) error {
	path := registryPath(root, reg.Scope)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ioErr(err)
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return ioErr(err)
	}
	if err := fsutil.AtomicWriteBytes(path, append(data, '\n'), 0o644); err != nil {
		return ioErr(err)
	}
	return nil
}

// lock takes .lets/locks/members-<scope>.lock until deadline.
func lock(root, scope string, deadline time.Duration) (func(), error) {
	if deadline == 0 {
		deadline = defaultLockDeadline
	}
	dir := filepath.Join(root, ".lets", "locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, ioErr(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "members-"+scope+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, ioErr(err)
	}
	if err := fsutil.TryLockFile(f, time.Now().Add(deadline)); err != nil {
		_ = f.Close()
		if errors.Is(err, fsutil.ErrLockBusy) {
			return nil, &Error{Code: ExitLockBusy, Kind: "lock_busy", Message: "another lets members call holds " + f.Name(), Remediation: "retry"}
		}
		return nil, ioErr(err)
	}
	return func() { _ = fsutil.UnlockFile(f); _ = f.Close() }, nil
}

func ioErr(err error) *Error {
	return &Error{Code: ExitIOError, Kind: "io_error", Message: err.Error()}
}

func scopeErr(scope string) *Error {
	return &Error{Code: ExitInvalidScope, Kind: "invalid_scope", Message: fmt.Sprintf("%q is neither a team name nor run-<RUN>", scope)}
}

func usageErr(msg string) *Error { return &Error{Code: ExitUsage, Kind: "usage", Message: msg} }

// callerEntry is this session's registry entry: the only source of its pid.
func callerEntry(snap ccregistry.Snapshot, sid string) (ccregistry.Entry, *Error) {
	if !ccregistry.ValidSession(sid) {
		return ccregistry.Entry{}, &Error{Code: ExitRegistryUnavailable, Kind: "registry_unavailable",
			Message: "no valid session id ($CLAUDE_CODE_SESSION_ID)", Remediation: "run inside a Claude Code session"}
	}
	if snap.Degraded != nil && snap.Degraded.Reason == "registry_absent" {
		return ccregistry.Entry{}, &Error{Code: ExitRegistryUnavailable, Kind: "registry_unavailable",
			Message: "Claude Code's session registry is absent", Remediation: "a Claude Code with a session registry is required"}
	}
	e, ok := snap.Find(sid)
	if !ok {
		return ccregistry.Entry{}, &Error{Code: ExitRegistryUnavailable, Kind: "registry_unavailable",
			Message: "session " + short(sid) + " is not in Claude Code's session registry"}
	}
	return e, nil
}

func short(sid string) string {
	if len(sid) > 8 {
		return sid[:8]
	}
	return sid
}

// samePath compares two paths after symlink resolution.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return resolve(a) == resolve(b)
}

func resolve(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// findPane looks for the member's own session: a registry entry named agentName
// whose cwd is cwd, other than the caller. Bounded: lookupAttempts reads,
// lookupInterval apart - a pane registers itself a moment after the Agent call.
func findPane(agentName, cwd, callerSid string) (ccregistry.Entry, bool) {
	for i := 0; i < lookupAttempts; i++ {
		if i > 0 {
			Sleep(lookupInterval)
		}
		var best ccregistry.Entry
		found := false
		for _, e := range readRegistry().Entries {
			if e.Name == agentName && e.SessionID != callerSid && samePath(e.Cwd, cwd) && (!found || e.StartedAt > best.StartedAt) {
				best, found = e, true
			}
		}
		if found {
			return best, true
		}
	}
	return ccregistry.Entry{}, false
}

// Add records a member the caller just spawned. A pane member (its own registry
// session named AgentName at the cwd) is recorded with its own session; otherwise
// it is in-process and recorded with the caller's session, whose liveness it then
// shares. The name is refused while a member of that name is live, rotated or
// unknown; a gone or dismissed entry of that name is replaced.
func Add(o Options, a AddOptions) (*AddResult, error) {
	res := &AddResult{Envelope: newEnvelope("add", o.Scope)}
	if !ValidScope(o.Scope) {
		return res, fail(&res.Envelope, scopeErr(o.Scope))
	}
	if !ValidName(a.Name) {
		return res, fail(&res.Envelope, &Error{Code: ExitNameInvalid, Kind: "name_invalid", Message: fmt.Sprintf("--name %q does not match [a-z0-9-]{1,40}", a.Name)})
	}
	role, ok := normalizeRole(a.Role)
	if !ok {
		return res, fail(&res.Envelope, &Error{Code: ExitRoleNotAllowed, Kind: "role_not_allowed",
			Message: fmt.Sprintf("--role %q is not a shipped lets:* agent (actor excluded)", a.Role), Remediation: "one of lets:" + strings.Join(ShippedRoles, ", lets:")})
	}
	link := a.Link
	if link == "" {
		link = LinkTeam
	}
	switch {
	case link != LinkTeam && link != LinkPeer:
		return res, fail(&res.Envelope, usageErr("--link must be team or peer"))
	case a.Isolation != "" && a.Isolation != "worktree":
		return res, fail(&res.Envelope, usageErr("--isolation must be worktree or empty"))
	case a.Isolation != "" && !filepath.IsAbs(a.WorktreePath):
		return res, fail(&res.Envelope, usageErr("--isolation worktree needs an absolute --worktree-path"))
	case !modelRe.MatchString(a.Model):
		return res, fail(&res.Envelope, usageErr(fmt.Sprintf("--model %q is not a model name", a.Model)))
	case strings.ContainsFunc(a.WorktreeBranch+a.WorktreePath+a.Cwd, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return res, fail(&res.Envelope, usageErr("a path or branch carries a control character"))
	}

	caller, e := callerEntry(readRegistry(), o.Session)
	if e != nil {
		return res, fail(&res.Envelope, e)
	}
	cwd := a.Cwd
	if cwd == "" {
		cwd = o.CallerToplevel
		if a.Isolation != "" {
			cwd = a.WorktreePath
		}
	}
	agent := AgentName(o.Scope, a.Name)
	m := Member{
		Name: a.Name, AgentName: agent, Role: role, Model: a.Model, Isolation: a.Isolation,
		WorktreePath: a.WorktreePath, WorktreeBranch: a.WorktreeBranch, CallerToplevel: o.CallerToplevel,
		LeadPid: caller.Pid, Link: link, Status: StatusActive, Set: Now().UTC(),
	}
	if pane, ok := findPane(agent, cwd, o.Session); ok {
		m.Kind, m.Session, m.Pid = KindPane, pane.SessionID, pane.Pid
		res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("pane member: %s runs as session %s (pid %d)", agent, short(pane.SessionID), pane.Pid)})
	} else {
		if link == LinkPeer {
			return res, fail(&res.Envelope, &Error{Code: ExitRegistryUnavailable, Kind: "registry_unavailable",
				Message: fmt.Sprintf("--link peer needs %s's own session at %s; none is in the registry", agent, cwd)})
		}
		m.Kind, m.Session, m.Pid = KindInProcess, caller.SessionID, caller.Pid
		res.Steps = append(res.Steps, Step{Status: StepOK, Message: fmt.Sprintf("in-process member: no session named %s at %s after %d lookups; liveness follows this session", agent, cwd, lookupAttempts)})
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
	snap := readRegistry()
	idx := -1
	for i := range reg.Members {
		if reg.Members[i].Name != a.Name {
			continue
		}
		old := reg.Members[i]
		judgeMember(&old, snap, reg.Lead, worktreeList(o.Root, []Member{old}))
		if old.Status != StatusGone {
			return res, fail(&res.Envelope, &Error{Code: ExitNameLive, Kind: "name_live",
				Message:     fmt.Sprintf("member %s is %s (session %s)", a.Name, old.Status, short(old.Session)),
				Remediation: "dismiss it first, or spawn under another name"})
		}
		idx = i
	}
	if idx >= 0 {
		reg.Members[idx] = m
	} else {
		reg.Members = append(reg.Members, m)
	}
	if err := save(o.Root, reg); err != nil {
		return res, fail(&res.Envelope, asErr(err))
	}
	m.Status = StatusLive
	res.Member = &m
	res.OK = true
	return res, nil
}

// Dismiss marks one member (or every member) dismissed. The harness can still
// resume a dismissed agent by name; LETS refuses to message it.
func Dismiss(o Options, name string, all bool) (*DismissResult, error) {
	res := &DismissResult{Envelope: newEnvelope("dismiss", o.Scope), Dismissed: []string{}}
	if !ValidScope(o.Scope) {
		return res, fail(&res.Envelope, scopeErr(o.Scope))
	}
	if all == (name != "") {
		return res, fail(&res.Envelope, usageErr("pass exactly one of --name or --all"))
	}
	if !all && !ValidName(name) {
		return res, fail(&res.Envelope, &Error{Code: ExitNameInvalid, Kind: "name_invalid", Message: fmt.Sprintf("--name %q does not match [a-z0-9-]{1,40}", name)})
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
	found := false
	for i := range reg.Members {
		m := &reg.Members[i]
		if !all && m.Name != name {
			continue
		}
		found = true
		if m.Status != StatusDismissed {
			m.Status = StatusDismissed
			res.Dismissed = append(res.Dismissed, m.Name)
		}
	}
	if !all && !found {
		return res, fail(&res.Envelope, &Error{Code: ExitNameInvalid, Kind: "name_invalid", Message: fmt.Sprintf("%s is not a member of %s", name, o.Scope)})
	}
	if len(res.Dismissed) > 0 {
		if err := save(o.Root, reg); err != nil {
			return res, fail(&res.Envelope, asErr(err))
		}
	}
	res.OK = true
	return res, nil
}

// Status judges the lead and every member now. A rotation is carried to the new
// session id and a link flip is recorded, both under the lock; a scope with no
// registry file is empty and nothing is written.
func Status(o Options, name string) (*StatusResult, error) {
	res := &StatusResult{Envelope: newEnvelope("status", o.Scope), Members: []Member{}}
	if !ValidScope(o.Scope) {
		return res, fail(&res.Envelope, scopeErr(o.Scope))
	}
	if name != "" && !ValidName(name) {
		return res, fail(&res.Envelope, &Error{Code: ExitNameInvalid, Kind: "name_invalid", Message: fmt.Sprintf("--name %q does not match [a-z0-9-]{1,40}", name)})
	}
	if _, err := os.Stat(registryPath(o.Root, o.Scope)); errors.Is(err, os.ErrNotExist) {
		res.OK = true
		return res, nil
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
	snap := readRegistry()
	if snap.Degraded != nil {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "session registry degraded: " + snap.Degraded.Reason + " " + snap.Degraded.Detail})
	}
	changed := false
	if reg.Lead != nil {
		ls, carried := judgeLead(reg.Lead, snap)
		changed = changed || carried
		res.Lead = ls
	}
	wts := worktreeList(o.Root, reg.Members)
	for i := range reg.Members {
		stored := reg.Members[i]
		judged := stored
		judgeMember(&judged, snap, reg.Lead, wts)
		// Persist only what judging proved: a carried session id and a flipped link.
		if judged.Session != stored.Session || judged.Link != stored.Link {
			reg.Members[i].Session, reg.Members[i].Link = judged.Session, judged.Link
			changed = true
		}
		if name == "" || judged.Name == name {
			res.Members = append(res.Members, judged)
		}
	}
	if name != "" && len(res.Members) == 0 {
		return res, fail(&res.Envelope, &Error{Code: ExitNameInvalid, Kind: "name_invalid", Message: fmt.Sprintf("%s is not a member of %s", name, o.Scope)})
	}
	if changed {
		if err := save(o.Root, reg); err != nil {
			return res, fail(&res.Envelope, asErr(err))
		}
	}
	res.OK = true
	return res, nil
}

func asErr(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ioErr(err)
}

// judgeSession: live = the session id runs under the recorded pid (anyPid: under
// any pid - a lead resumed with `claude -r` is still the lead); rotated = the same
// process re-minted the id (newSid set); unknown = the registry cannot tell; else
// gone(session_dead). A member whose session runs under another pid was resumed in a
// new process: its in-process teammates and harness link are gone with the old one.
func judgeSession(snap ccregistry.Snapshot, sid string, pid int, set time.Time, anyPid bool) (status, reason, newSid string) {
	if snap.Degraded != nil && snap.Degraded.Reason == "registry_absent" {
		return StatusUnknown, "registry_absent", ""
	}
	if e, ok := snap.Find(sid); ok {
		if anyPid || e.Pid == pid {
			return StatusLive, "", ""
		}
		return StatusGone, ReasonSessionDead, ""
	}
	if ns, ok := snap.Rotated(sid, pid, set); ok {
		return StatusRotated, "", ns
	}
	if snap.Liveness(sid, pid) == ccregistry.Unknown {
		reason = "liveness_unknown"
		if snap.Degraded != nil {
			reason = snap.Degraded.Reason
		}
		return StatusUnknown, reason, ""
	}
	return StatusGone, ReasonSessionDead, ""
}

// worktreeState of one isolated member's worktree.
type worktreeState struct{ listed, prunable bool }

// worktreeList reads `git worktree list --porcelain` once, only when a member is
// isolated; nil = not needed, or git failed (the members are then judged unknown).
func worktreeList(root string, members []Member) map[string]worktreeState {
	if !slices.ContainsFunc(members, func(m Member) bool { return m.Isolation != "" }) {
		return nil
	}
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	wts := map[string]worktreeState{}
	cur := ""
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur = resolve(strings.TrimPrefix(line, "worktree "))
			wts[cur] = worktreeState{listed: true}
		case strings.HasPrefix(line, "prunable") && cur != "":
			wts[cur] = worktreeState{listed: true, prunable: true}
		}
	}
	return wts
}

// judgeMember sets m's reported Status / Reason, carries a rotated session and
// flips link team -> peer once the lead that spawned a pane member is no longer
// the lead (a recorded lead with another pid, or with no lead record, the spawning
// pid is dead). wts nil with an isolated member = the worktree list failed.
func judgeMember(m *Member, snap ccregistry.Snapshot, lead *Lead, wts map[string]worktreeState) {
	m.Reason = ""
	if m.Kind == KindPane && m.Link == LinkTeam && m.LeadPid > 0 {
		if (lead != nil && lead.Pid != m.LeadPid) || (lead == nil && !ccregistry.ProcAlive(m.LeadPid)) {
			m.Link = LinkPeer
		}
	}
	if m.Status == StatusDismissed {
		m.Status, m.Reason = StatusGone, ReasonDismissed
		return
	}
	if m.Isolation != "" {
		if wts == nil {
			m.Status, m.Reason = StatusUnknown, "worktree_list_failed"
			return
		}
		st := wts[resolve(m.WorktreePath)]
		switch {
		case !st.listed:
			m.Status, m.Reason = StatusGone, ReasonWorktreeMissing
			return
		case st.prunable:
			m.Status, m.Reason = StatusGone, ReasonPrunable
			return
		}
		if fi, err := os.Stat(m.WorktreePath); err != nil || !fi.IsDir() {
			m.Status, m.Reason = StatusGone, ReasonDirMissing
			return
		}
	}
	status, reason, ns := judgeSession(snap, m.Session, m.Pid, m.Set, false)
	if ns != "" {
		m.Session = ns
	}
	m.Status, m.Reason = status, reason
}
