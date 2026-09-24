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
type Resolution struct {
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
// can carry ("" = none; "x*" = the literal prefix "x" followed by a name). remedy
// must answer every pair; remedy_test.go iterates this table and scans the package
// source so it cannot fall behind.
var refusalDetails = map[string][]string{
	"orchestrator_not_registered": {""},
	"target_not_alive":            {"", "bound orchestrator has no role file"},
	"target_in_other_repo":        {"", "its cwd is outside this repo", "orca_not_selected", "orca_unavailable", "repo_not_in_orca"},
	"target_unsendable":           {"", "peer_ambiguous", "session_duplicated", "no_valid_name", "name_not_unique", "non_claude_send_unsupported_v1", "claude_terminal_unjoined", "liveness_unknown"},
	"bound_ambiguous":             {"", "sibling_repos", "registered as *"},
	"branch_unreadable":           {""},
	"budget_exhausted":            {""},
	"orchestrator_needs_name":     {""},
}

// remedy is the one line a user acts on for a peer dead end; the orc skill prints
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

// ResolveOrchestrator decides the caller's orchestrator: its own role first (self),
// then the branch binding (bound; never re-routed to another orchestrator when the
// bound one is gone), then the only live orchestrator (single); several, or any whose
// liveness is unknown, is ambiguous; nothing is none. A candidate this repo cannot
// address (cross-repo, dead, or present but unsendable) is refused by name, never
// returned as a target.
func ResolveOrchestrator(ctx context.Context, rc *repoContext, o ResolveOptions) *Resolution {
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
