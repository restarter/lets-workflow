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
	Degraded   []Degraded  `json:"degraded"`
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

// ResolveOrchestrator decides the caller's orchestrator: its own role first (self),
// then the branch binding (bound; never re-routed to another orchestrator when the
// bound one is gone), then the only live orchestrator (single); several, or any whose
// liveness is unknown, is ambiguous; nothing is none.
func ResolveOrchestrator(ctx context.Context, root string, o ResolveOptions) *Resolution {
	res := &Resolution{Scope: "branch", Candidates: []Candidate{}, Degraded: []Degraded{}}
	files, invalid := loadRoles(root)
	for _, name := range invalid {
		res.Degraded = append(res.Degraded, Degraded{Source: "roles", Reason: "role_file_invalid", Detail: name})
	}
	snap := ccregistry.Read(ccregistry.HomeDir())
	if snap.Degraded != nil {
		res.Degraded = append(res.Degraded, Degraded{Source: "claude", Reason: snap.Degraded.Reason, Detail: snap.Degraded.Detail})
	}
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
	if name, ok := readBinding(root, branchOf(ctx, o.Cwd)); ok {
		res.Source = "bound"
		for _, f := range orchs {
			if liveName(snap, f) == name {
				res.Target = orchestratorPeer(snap, f)
				if res.Target.Alive == "dead" {
					res.Reason = "orchestrator_dead"
				}
				return res
			}
		}
		res.Target = &Peer{Role: "orchestrator", Name: name, Alive: "dead", Via: []string{}, Send: "none"}
		res.Reason = "orchestrator_not_registered"
		return res
	}
	var live []roleFile
	unknown := false
	for _, f := range orchs {
		l := snap.Liveness(f.Session, f.Pid)
		if l == ccregistry.Dead {
			continue
		}
		if l == ccregistry.Unknown {
			unknown = true
		}
		live = append(live, f)
		res.Candidates = append(res.Candidates, Candidate{Name: liveName(snap, f), Scope: f.Scope, Alive: l.String(), Session: f.Session})
	}
	switch {
	case len(live) == 1 && !unknown:
		res.Source, res.Target = "single", orchestratorPeer(snap, live[0])
	case len(live) > 0:
		res.Source = "ambiguous"
	default:
		res.Source = "none"
	}
	return res
}
