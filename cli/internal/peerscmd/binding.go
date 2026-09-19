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
	Source     string      `json:"source"` // self | bound | single | ambiguous | none
	Scope      string      `json:"scope"`  // branch: the binding belongs to the branch
	Target     *Peer       `json:"target,omitempty"`
	Candidates []Candidate `json:"candidates"`
	Reason     string      `json:"reason,omitempty"`
	Refused    []Refused   `json:"refused,omitempty"`
	Degraded   []Degraded  `json:"degraded"`
}

// Refused is an orchestrator this caller cannot address, with the reason a human
// can act on. It never carries a session id a sender could try anyway.
type Refused struct {
	Name     string `json:"name"`
	Scope    string `json:"scope,omitempty"`
	Session6 string `json:"session6,omitempty"`
	Reason   string `json:"reason"` // target_in_other_repo | target_not_alive | target_unsendable
	Detail   string `json:"detail,omitempty"`
	Hint     string `json:"hint,omitempty"`
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
		if orcaSelected(rc.root, false) {
			ref.Hint = "cross-repo messaging goes through /lets:hub"
		}
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
		for _, f := range orchs {
			if liveName(snap, f) == name {
				if p, ref := addressable(ctx, rc, f); p != nil {
					res.Target = p
				} else {
					res.Reason, res.Refused = ref.Reason, append(res.Refused, *ref)
				}
				return res
			}
		}
		res.Reason = "orchestrator_not_registered"
		res.Refused = append(res.Refused, Refused{Name: name, Reason: "target_not_alive", Detail: "bound orchestrator has no role file"})
		return res
	}
	// An exhausted budget must degrade loudly, never silently re-route to a different
	// orchestrator: branchOf(ctx, ...) fails instantly once ctx is done, which looks
	// exactly like a detached HEAD (also "") to readBinding above. Tell them apart by
	// ctx.Err() - a genuinely detached HEAD with a healthy ctx still falls through.
	if branch == "" && ctx.Err() != nil {
		res.Source = "none"
		res.Reason = "branch_unreadable"
		res.Degraded = append(res.Degraded, Degraded{Source: "git", Reason: "branch_unreadable"})
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
