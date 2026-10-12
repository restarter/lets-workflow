//go:build unix

package peerscmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
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
// SelfRecord is the caller's own recorded `orchestrator:` line (nil when none).
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

// refusalDetails is every reason a resolution can end on, with the details each
// can carry ("" = none). remedy must answer every pair; remedy_test.go iterates this
// table and scans the package source so it cannot fall behind.
var refusalDetails = map[string][]string{
	"target_not_alive":  {""},
	"target_unsendable": {"name_not_unique", "liveness_unknown"},
	"branch_unreadable": {""},
	"budget_exhausted":  {""},
}

// remedy is the one line a user acts on for a resolution dead end; the orc skill
// prints it verbatim. Every command named here must be able to act on that refusal
// (remedy_test.go).
func remedy(reason, detail, name string) string {
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

// Refused is an orchestrator this caller cannot address, with the reason a human
// can act on. It never carries a session id a sender could try anyway.
type Refused struct {
	Name     string `json:"name"`
	Scope    string `json:"scope,omitempty"`
	Session6 string `json:"session6,omitempty"`
	Reason   string `json:"reason"` // target_not_alive | target_unsendable
	Detail   string `json:"detail,omitempty"`
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
	v.rows, v.pids = whoRows(ctx, mainRoot, v.merge, entries, snap, recs, true)
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
			res.Remediation = remedy(res.Reason, detail, name)
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
	switch {
	case len(orchs) == 0:
		res.Source = "none"
	case len(orchs) == 1 && v.snap.Degraded == nil:
		t := orchs[0]
		res.Source, res.Target = "single", &t
	default:
		// several, or one beside a live registry entry that could not be read (it may
		// be another orchestrator): never let one candidate outrank the unknown
		res.Source = "ambiguous"
	}
	return res
}

// boundTarget is the Peer a bound name resolves to: this repo's row when the SAME
// process runs here (row session AND row name equal the registry entry's), else a
// bare row with its cwd - another repo, reached by name and always without standing
// (cross-repo stays per-message). Two live processes can share one session id
// (`claude -r` beside a running original) under different names and repos, so a
// session match alone would hand back another process's name and standing.
func boundTarget(v *nameView, e ccregistry.Entry) *Peer {
	for _, p := range v.rows {
		if p.Session == e.SessionID && p.Name == e.Name {
			t := p
			return &t
		}
	}
	return &Peer{Name: e.Name, Session: e.SessionID, Session6: session6(e.SessionID), Cwd: e.Cwd, State: e.Status, Alive: "alive"}
}
