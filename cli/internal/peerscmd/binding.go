//go:build unix

package peerscmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// ResolveOptions configures ResolveOrchestrator.
type ResolveOptions struct {
	Session string // the caller
	Cwd     string // the caller's checkout (its branch carries the binding)
	// Branch: the caller's checked-out branch, read OUTSIDE the resolution budget;
	// empty means resolve it here (branchOf(ctx, Cwd), inside whatever budget ctx
	// carries). A caller under its own timeout (Orchestrator's 2500ms) must read the
	// branch before that timeout starts - git exec fails instantly on an expired
	// context, and an unreadable branch is unbound, which re-routes to a DIFFERENT
	// orchestrator than the one this branch is actually bound to.
	Branch string
}

// Candidate is one orchestrator an unbound caller could mean.
type Candidate struct {
	Name    string `json:"name"`
	Scope   string `json:"scope,omitempty"`
	Alive   string `json:"alive"`
	Session string `json:"session"`
}

// Resolution is which orchestrator a caller's /lets:orc talks to. It never writes.
// SelfRecord is the caller's own recorded `orchestrator:` line (name resolution
// only; nil when the caller has none).
type Resolution struct {
	SelfRecord  *SelfRecord `json:"self_record"`
	Source      string      `json:"source"` // self | bound | single | ambiguous | none
	Scope       string      `json:"scope"`  // branch: the binding belongs to the branch
	Target      *Peer       `json:"target,omitempty"`
	Candidates  []Candidate `json:"candidates"`
	Reason      string      `json:"reason,omitempty"`
	Remediation string      `json:"remediation,omitempty"` // the exact line to show the user; set with every Reason
	Refused     []Refused   `json:"refused,omitempty"`
	Degraded    []Degraded  `json:"degraded"`
}

// refusalDetails is every reason the name resolution (ResolveOrchestrator) can end
// on, with the details each can carry ("" = none). resolveRemedy must answer every
// pair; remedy_test.go iterates this table and scans the package source so it
// cannot fall behind.
var refusalDetails = map[string][]string{
	"target_not_alive":  {""},
	"target_unsendable": {"name_not_unique", "liveness_unknown"},
	"branch_unreadable": {""},
	"budget_exhausted":  {""},
}

// resolveRemedy is the one line a user acts on for a name-resolution dead end; the
// orc skill prints it verbatim. Every command named here must be able to act on that
// refusal (remedy_test.go).
func resolveRemedy(reason, detail, name string) string {
	switch reason + "/" + detail {
	case "target_not_alive/":
		return "Orchestrator " + name + " is not running - start it, or bind this branch to a live one: /lets:start <id> --orc=\"<name>\""
	case "target_unsendable/name_not_unique":
		return "Two live sessions are named " + name + " - /rename one of them"
	case "target_unsendable/liveness_unknown":
		return "The Claude session registry could not be read in full - retry"
	case "branch_unreadable/":
		return "This checkout's branch could not be read - retry"
	case "budget_exhausted/":
		return "Resolution ran out of time - retry"
	}
	return ""
}

// legacyRefusalDetails is the same table for the old role-file resolution
// (resolveOrchestratorLegacy, roles.go's SetRole) and its remedy; both go with the
// role registry.
var legacyRefusalDetails = map[string][]string{
	"orchestrator_not_registered": {""},
	"target_not_alive":            {"", "bound orchestrator has no role file"},
	"target_in_other_repo":        {"", "its cwd is outside this repo", "orca_not_selected", "orca_unavailable", "repo_not_in_orca"},
	"target_unsendable":           {"", "peer_ambiguous", "session_duplicated", "no_valid_name", "name_not_unique", "non_claude_send_unsupported_v1", "claude_terminal_unjoined", "liveness_unknown"},
	"bound_ambiguous":             {"", "sibling_repos", "registered as *"},
	"branch_unreadable":           {""},
	"budget_exhausted":            {""},
	"orchestrator_needs_name":     {""},
}

// remedy is the old resolution's line a user acts on for a peer dead end; the orc skill prints
// it verbatim instead of composing a workaround. detail is the refusal's own detail
// (for target_unsendable: the peer's send reason), which decides the action; sibling
// marks a target in another repo, where a command that sees only this repo is no help.
// Every command named here must be able to act on that refusal (remedy_test.go).
func remedy(reason, detail, name string, orca, sibling bool) string {
	switch reason {
	case "orchestrator_not_registered":
		return name + " has no role - in its session run /lets:start --main, or rebind this branch with /lets:start <id> --orc=\"<name>\""
	case "target_not_alive":
		if orca {
			return name + " is not running - /lets:hub wakes a stopped orchestrator, or rebind this branch with /lets:start <id> --orc=\"<name>\""
		}
		return name + " is not running - reopen it, or rebind this branch with /lets:start <id> --orc=\"<name>\""
	case "target_in_other_repo":
		switch detail {
		case "orca_unavailable":
			return name + " runs in another repo and Orca did not answer - start Orca, then retry"
		case "repo_not_in_orca":
			return name + " runs in a repo Orca does not list - add that repo to Orca, then retry"
		}
		return name + " runs in another repo - reaching it needs LETS_LAUNCHER=orca (/lets:init), or rebind this branch with /lets:start <id> --orc=\"<name>\""
	case "target_unsendable":
		return unsendableRemedy(detail, name, sibling)
	case "bound_ambiguous":
		if detail == "sibling_repos" {
			return "several live orchestrators named " + name + " across Orca repos - /rename all but one of them, or rebind this branch with /lets:start <id> --orc=\"<live name>\""
		}
		return "/lets:start <id> --orc=\"<live name>\" rebinds this branch to one of them"
	case "branch_unreadable", "budget_exhausted":
		return "resolution did not complete - retry"
	case "orchestrator_needs_name":
		return "/rename <name>, then /lets:start --main again"
	}
	return ""
}

// unsendableRemedy names the action for each reason peers() marks a peer
// unsendable; only a reason it does not know falls back to a who that lists the
// target - /lets:orc who for this repo, `lets peers who --orca-repos` for a sibling.
func unsendableRemedy(detail, name string, sibling bool) string {
	who := "/lets:orc who"
	if sibling {
		who = "lets peers who --orca-repos"
	}
	switch detail {
	case "session_duplicated":
		return name + " runs in two processes under one session id (a resumed copy beside the original) - close one of them, then retry"
	case "name_not_unique":
		return "several live sessions are named " + name + " - /rename all but one of them, then retry"
	case "no_valid_name":
		return name + " has no valid session name - /rename it in its own session, then retry"
	case "peer_ambiguous":
		return name + "'s Orca terminal is claimed twice - " + who + " shows both; close the stale pane, then retry"
	}
	return who + " shows why " + name + " cannot receive (" + nonEmpty(detail, "no reason reported") + ")"
}

// Refused is an orchestrator this caller cannot address, with the reason a human
// can act on. It never carries a session id a sender could try anyway.
type Refused struct {
	Name     string `json:"name"`
	Scope    string `json:"scope,omitempty"`
	Session6 string `json:"session6,omitempty"`
	Reason   string `json:"reason"` // target_in_other_repo | target_not_alive | target_unsendable | bound_ambiguous
	// details for target_in_other_repo: orca_not_selected | orca_unavailable | repo_not_in_orca
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
	// Sibling: the refusal concerns a target in another repo, so its remediation
	// must not name a command that only sees this repo.
	Sibling bool `json:"sibling,omitempty"`
}

// branchOf is the checked-out branch of cwd ("" when detached or unreadable).
var branchOf = func(ctx context.Context, cwd string) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", cwd, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readBinding returns the orchestrator name bound to branch (the `orc:` line of its
// .task-<slug>). An invalid name, a detached HEAD or the merge-branch is unbound.
func readBinding(root, branch string) (string, bool) {
	if branch == "" {
		return "", false
	}
	home, _ := os.UserHomeDir()
	if branch == letsconfig.ResolvedEnv(root, home, nil)["LETS_MERGE_BRANCH"] {
		return "", false
	}
	slug, ok := taskstate.Slug(branch)
	if !ok {
		return "", false
	}
	st, err := taskstate.Read(filepath.Join(root, ".lets"), slug)
	if err != nil || !ccregistry.ValidName(st.Orc) {
		return "", false
	}
	return st.Orc, true
}

func orchestratorPeer(snap ccregistry.Snapshot, f roleFile) *Peer {
	return &Peer{Role: "orchestrator", Name: liveName(snap, f), Scope: f.Scope, Session: f.Session, Session6: session6(f.Session),
		Cwd: f.Cwd, TerminalID: f.OrcaTerminal, Alive: snap.Liveness(f.Session, f.Pid).String(), Via: []string{"claude"}}
}

// addressable answers the sender's question with the sender's own computation: the
// target must be a row of THIS repo's peer set with a usable Send. Anything else is
// refused by name rather than returned as a target the send path cannot reach.
// Cross-repo reach is decided by siblingOrchestrator, for a bound name only.
func addressable(ctx context.Context, rc *repoContext, f roleFile) (*Peer, *Refused) {
	name := liveName(rc.snap, f)
	ref := &Refused{Name: name, Scope: f.Scope, Session6: session6(f.Session)}
	for _, p := range rc.peers(ctx) {
		if p.Session != f.Session {
			continue
		}
		if p.Send == "none" || p.Send == "" {
			ref.Reason, ref.Detail = "target_unsendable", p.Reason
			return nil, ref
		}
		out := p
		out.Role, out.Scope = "orchestrator", f.Scope
		return &out, nil
	}
	if _, ok := rc.snap.Find(f.Session); ok {
		ref.Reason = "target_in_other_repo"
		ref.Detail = "its cwd is outside this repo"
		return nil, ref
	}
	ref.Reason = "target_not_alive"
	return nil, ref
}

// candidate pairs a live orchestrator's role file with its already-resolved,
// addressable Peer, so ResolveOrchestrator never rebuilds it.
type candidate struct {
	f roleFile
	p *Peer
}

// resolveOrchestratorLegacy is the old role-file resolution, kept until the role
// registry goes (its tests and the heal / resume tests still drive it). It decides
// the caller's orchestrator: its own role first (self),
// then the branch binding (bound; never re-routed to another orchestrator when the
// bound one is gone), then the only live orchestrator (single); several, or any whose
// liveness is unknown, is ambiguous; nothing is none. A candidate this repo cannot
// address (cross-repo, dead, or present but unsendable) is refused by name, never
// returned as a target.
func resolveOrchestratorLegacy(ctx context.Context, rc *repoContext, o ResolveOptions) *Resolution {
	res := &Resolution{Scope: "branch", Candidates: []Candidate{}, Degraded: rc.degraded, Refused: []Refused{}}
	// Every dead end leaves with its remedy; res is a pointer, so this sees the final value.
	defer func() {
		if res.Reason != "" && res.Remediation == "" {
			name, detail, sibling := "", "", false
			if len(res.Refused) > 0 {
				name, detail, sibling = res.Refused[0].Name, res.Refused[0].Detail, res.Refused[0].Sibling
			} else if res.Target != nil {
				name = res.Target.Name
			}
			res.Remediation = remedy(res.Reason, detail, name, orcaSelected(rc.root, false), sibling)
		}
	}()
	files, snap := rc.roles, rc.snap
	if self, ok := files[o.Session]; ok && self.Role == "orchestrator" {
		res.Source, res.Target = "self", orchestratorPeer(snap, self)
		if !ccregistry.ValidName(res.Target.Name) {
			res.Reason = "orchestrator_needs_name"
		}
		return res
	}
	orchs := []roleFile{}
	for sid, f := range files {
		if sid != o.Session && f.Role == "orchestrator" {
			orchs = append(orchs, f)
		}
	}
	sort.Slice(orchs, func(i, j int) bool { return liveName(snap, orchs[i]) < liveName(snap, orchs[j]) })
	branch := o.Branch
	if branch == "" {
		branch = branchOf(ctx, o.Cwd)
	}
	if name, ok := readBinding(rc.root, branch); ok {
		res.Source = "bound"
		// Pass 1: the live holder of the name (the name is the address). Pass 2, only
		// when nobody holds it live: a role file registered under it - a resumed
		// session that came back under another registry name. Several there = refuse.
		var byLive, byRegistered []roleFile
		for _, f := range orchs {
			if liveName(snap, f) == name {
				byLive = append(byLive, f)
			} else if f.Name == name {
				byRegistered = append(byRegistered, f)
			}
		}
		cands := byLive
		if len(cands) == 0 {
			cands = byRegistered
		}
		// sibling applies the cross-repo rule (bound only - the unbound path below
		// never calls it); false = no sibling holds the name either.
		sibling := func(local *roleFile) bool {
			p, ref, ok := siblingOrchestrator(ctx, rc, name, local)
			switch {
			case !ok:
				return false
			case p != nil:
				res.Target = p
			case ref.Reason == errBudget:
				res.Reason = errBudget
				res.Degraded = append(res.Degraded, Degraded{Source: "context", Reason: "deadline_exceeded"})
			default:
				res.Reason, res.Refused = ref.Reason, append(res.Refused, *ref)
			}
			return true
		}
		switch len(cands) {
		case 0:
			if sibling(nil) {
				return res
			}
			res.Reason = "orchestrator_not_registered"
			res.Refused = append(res.Refused, Refused{Name: name, Reason: "target_not_alive", Detail: "bound orchestrator has no role file"})
		case 1:
			p, ref := addressable(ctx, rc, cands[0])
			switch {
			case p != nil:
				res.Target = p
			case ref.Reason == "target_in_other_repo" && sibling(&cands[0]):
			default:
				res.Reason, res.Refused = ref.Reason, append(res.Refused, *ref)
			}
		default:
			res.Reason = "bound_ambiguous"
			for _, f := range cands {
				res.Refused = append(res.Refused, Refused{Name: liveName(snap, f), Scope: f.Scope, Session6: session6(f.Session), Reason: "bound_ambiguous", Detail: "registered as " + name, Hint: "/lets:start <id> --orc=\"<live name>\" rebinds this branch"})
			}
		}
		return res
	}
	// An exhausted budget must degrade loudly, never silently fall through to the
	// unbound path below and pick a DIFFERENT live orchestrator - whether the branch
	// itself came back unreadable, or the branch is perfectly readable but the budget
	// was already spent inside loadRepo (a failed worktree list narrows the peer set
	// the unbound loop reads from addressable()/rc.peers(), the same silent-misroute
	// risk reached through a different door). A genuinely detached HEAD or a
	// genuinely unbound branch, both with a healthy ctx, still fall through unchanged.
	if ctx.Err() != nil {
		res.Source = "none"
		if branch == "" {
			res.Reason = "branch_unreadable"
			res.Degraded = append(res.Degraded, Degraded{Source: "git", Reason: "branch_unreadable"})
		} else {
			res.Reason = "budget_exhausted"
			res.Degraded = append(res.Degraded, Degraded{Source: "context", Reason: "deadline_exceeded"})
		}
		return res
	}
	var live []candidate
	unknown := false
	for _, f := range orchs {
		l := snap.Liveness(f.Session, f.Pid)
		if l == ccregistry.Dead {
			continue
		}
		// Uncertainty counts toward ambiguity BEFORE addressability is even asked:
		// an unknown-liveness orchestrator is always unaddressable today (peers()
		// reports it Send=none), so testing addressable() first would make `unknown`
		// dead and let a single addressable candidate silently outrank one this repo
		// genuinely cannot vouch for.
		if l == ccregistry.Unknown {
			unknown = true
		}
		p, ref := addressable(ctx, rc, f)
		if p == nil {
			res.Refused = append(res.Refused, *ref)
			continue
		}
		live = append(live, candidate{f: f, p: p})
		res.Candidates = append(res.Candidates, Candidate{Name: liveName(snap, f), Scope: f.Scope, Alive: l.String(), Session: f.Session})
	}
	switch {
	case len(live) == 1 && !unknown:
		res.Source, res.Target = "single", live[0].p
	case len(live) > 0:
		res.Source = "ambiguous"
	default:
		res.Source = "none"
		if len(res.Refused) > 0 {
			res.Reason = res.Refused[0].Reason
		}
	}
	return res
}

// nameView is what the name resolution reads: this repo's rows (as `who` builds
// them), the machine-wide registry snapshot, the merge-branch `orchestrator:`
// records, and the merge-branch name.
type nameView struct {
	mainRoot, merge string
	rows            []Peer
	pids            map[string]int
	snap            ccregistry.Snapshot
	recs            []taskstate.Orchestrator
	degraded        []Degraded
}

// loadNameView builds the view of the repo whose main checkout is mainRoot.
func loadNameView(ctx context.Context, mainRoot string) *nameView {
	v := &nameView{mainRoot: mainRoot, merge: mergeBranchOf(mainRoot), degraded: []Degraded{}}
	entries, _, _, snap, degraded := registryPeers(ctx, mainRoot)
	v.snap = snap
	v.degraded = append(v.degraded, degraded...)
	recs, rdeg := orchestratorRecords(mainRoot, v.merge)
	v.recs = recs
	v.degraded = append(v.degraded, rdeg...)
	v.rows, v.pids = whoRows(ctx, mainRoot, v.merge, entries, snap, recs)
	return v
}

// boundName is the `orc:` binding of branch in mainRoot's task-state; the
// merge-branch, a detached HEAD or an invalid name is unbound.
func boundName(mainRoot, merge, branch string) (string, bool) {
	slug, ok := taskstate.Slug(branch)
	if !ok || branch == merge {
		return "", false
	}
	st, err := taskstate.Read(filepath.Join(mainRoot, ".lets"), slug)
	if err != nil || !ccregistry.ValidName(st.Orc) {
		return "", false
	}
	return st.Orc, true
}

// ResolveOrchestrator decides, by session NAME, whom the caller's /lets:orc talks
// to; first match wins:
//
//   - bound: the branch's `orc:<name>` - first, also for a recorded orchestrator.
//     The target is the one live registry entry with that name anywhere on the
//     machine; none is refused target_not_alive, several target_unsendable.
//   - self: the caller's own row is an orchestrator row of this repo.
//   - single / ambiguous: one / several orchestrator rows of this repo.
//   - none.
//
// An orchestrator row is a name recorded in the merge-branch task-state, or (only
// when no recorded name is live) a <repo>-orc[-scope] name. SelfRecord is always
// the caller's own record, whatever the source. It never writes.
func ResolveOrchestrator(ctx context.Context, v *nameView, o ResolveOptions) *Resolution {
	res := &Resolution{Scope: "branch", Candidates: []Candidate{}, Refused: []Refused{}, Degraded: v.degraded}
	defer func() {
		if res.Reason != "" && res.Remediation == "" {
			name, detail := "", ""
			if len(res.Refused) > 0 {
				name, detail = res.Refused[0].Name, res.Refused[0].Detail
			}
			res.Remediation = resolveRemedy(res.Reason, detail, name)
		}
	}()
	var self *Peer
	for i := range v.rows {
		if v.rows[i].Session == o.Session {
			self = &v.rows[i]
		}
	}
	if e, ok := v.snap.Find(o.Session); ok && e.NameOK {
		for _, r := range v.recs {
			if r.Name == e.Name {
				res.SelfRecord = &SelfRecord{Name: r.Name, Scope: r.Scope, Standing: r.Standing}
				break
			}
		}
	}
	branch := o.Branch
	if branch == "" {
		branch = branchOf(ctx, o.Cwd)
	}
	if name, ok := boundName(v.mainRoot, v.merge, branch); ok {
		res.Source = "bound"
		var hits []ccregistry.Entry
		seen := map[string]bool{}
		for _, e := range v.snap.Entries {
			if e.NameOK && e.Name == name && !seen[e.SessionID] {
				seen[e.SessionID] = true
				hits = append(hits, e)
			}
		}
		switch {
		case len(hits) == 1:
			res.Target = boundTarget(v, hits[0])
		case len(hits) > 1:
			res.Reason = "target_unsendable"
			res.Refused = append(res.Refused, Refused{Name: name, Reason: "target_unsendable", Detail: "name_not_unique"})
		case v.snap.Degraded != nil:
			// a live session the registry could not read may carry the name
			res.Reason = "target_unsendable"
			res.Refused = append(res.Refused, Refused{Name: name, Reason: "target_unsendable", Detail: "liveness_unknown"})
		default:
			res.Reason = "target_not_alive"
			res.Refused = append(res.Refused, Refused{Name: name, Reason: "target_not_alive"})
		}
		return res
	}
	if self != nil && self.Orchestrator {
		t := *self
		res.Source, res.Target = "self", &t
		return res
	}
	// An exhausted budget degrades loudly, never falls through to a DIFFERENT live
	// orchestrator: an unreadable branch may be a bound one (lets-cbmg7).
	if ctx.Err() != nil {
		res.Source = "none"
		if branch == "" {
			res.Reason = "branch_unreadable"
			res.Degraded = append(res.Degraded, Degraded{Source: "git", Reason: "branch_unreadable"})
		} else {
			res.Reason = "budget_exhausted"
			res.Degraded = append(res.Degraded, Degraded{Source: "context", Reason: "deadline_exceeded"})
		}
		return res
	}
	var orchs []Peer
	for _, p := range v.rows {
		if p.Orchestrator && p.Session != o.Session {
			orchs = append(orchs, p)
			res.Candidates = append(res.Candidates, Candidate{Name: p.Name, Scope: p.Scope, Alive: p.Alive, Session: p.Session})
		}
	}
	switch len(orchs) {
	case 0:
		res.Source = "none"
	case 1:
		t := orchs[0]
		res.Source, res.Target = "single", &t
	default:
		res.Source = "ambiguous"
	}
	return res
}

// boundTarget is the Peer a bound name resolves to: this repo's row when the
// session runs here, else a bare row with its cwd - another repo, reached by name
// and always without standing (cross-repo stays per-message).
func boundTarget(v *nameView, e ccregistry.Entry) *Peer {
	for _, p := range v.rows {
		if p.Session == e.SessionID {
			t := p
			return &t
		}
	}
	return &Peer{Name: e.Name, Session: e.SessionID, Session6: session6(e.SessionID), Cwd: e.Cwd, State: e.Status, Alive: "alive"}
}
