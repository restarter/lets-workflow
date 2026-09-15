//go:build unix

package peerscmd

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

// WhoOptions configures Who.
type WhoOptions struct {
	Cwd            string
	Role           string // filter: orchestrator | worker | peer
	Orc            string // filter: workers bound to this orchestrator name
	ExcludeSession string
	Session        string // self lookup
	Prune          bool
	ProbeOrca      bool
	Timeout        time.Duration // total budget (default 2500ms)
}

// newOrcaOps resolves the Orca binary (a seam: tests fake it and spy on the lookup).
var newOrcaOps = func() (orcaOps, *orcacmd.Failure) {
	c, f := orcacmd.NewClient()
	if f != nil {
		return nil, f
	}
	return clientOps{c: c}, nil
}

// orcaSelected: Orca is an opt-in addon. It is consulted only on an explicit
// --probe-orca or LETS_LAUNCHER=orca (project over user env); ORCA_WORKTREE_ID alone
// never selects it, so a non-Orca user never pays the binary lookup.
func orcaSelected(root string, probe bool) bool {
	if probe {
		return true
	}
	home, _ := os.UserHomeDir()
	return letsconfig.ResolvedEnv(root, home, nil)["LETS_LAUNCHER"] == "orca"
}

// repoContext is what every peers verb resolves once: the checkout, the main
// checkout, the registry rows of this repo, role files, and (when selected) Orca.
type repoContext struct {
	root, mainRoot string
	entries        []ccregistry.Entry
	rootOf         map[string]string
	roots          []string
	snap           ccregistry.Snapshot
	roles          map[string]roleFile
	ops            orcaOps
	terms          []orcaTerm
	degraded       []Degraded
}

func loadRepo(ctx context.Context, cwd string, probeOrca bool) (*repoContext, error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	root := gitutil.ProjectRoot(cwd, 2*time.Second)
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if root == "" || mainRoot == "" {
		return nil, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	rc := &repoContext{root: root, mainRoot: mainRoot, degraded: []Degraded{}}
	var d *Degraded
	rc.entries, rc.rootOf, rc.snap, d = registryPeers(ctx, mainRoot)
	if d != nil {
		rc.degraded = append(rc.degraded, *d)
	}
	rc.roots = repoWorktrees(ctx, mainRoot)
	var invalid []string
	rc.roles, invalid = loadRoles(root)
	for _, name := range invalid {
		rc.degraded = append(rc.degraded, Degraded{Source: "roles", Reason: "role_file_invalid", Detail: name})
	}
	if orcaSelected(root, probeOrca) {
		ops, f := newOrcaOps()
		if f != nil {
			rc.degraded = append(rc.degraded, Degraded{Source: "orca", Reason: f.Reason, Detail: f.Detail})
		} else {
			terms, f := ops.Terminals(ctx)
			if f != nil {
				rc.degraded = append(rc.degraded, Degraded{Source: "orca", Reason: f.Reason, Detail: f.Detail})
			} else {
				rc.ops = ops
				for _, t := range terms {
					if worktreeOf(t.Path, rc.roots) != "" {
						rc.terms = append(rc.terms, t)
					}
				}
			}
		}
	}
	return rc, nil
}

// peers merges the sources. A registry session and an Orca terminal are one peer
// only when the terminal handle equals the `orca_terminal:` the session reported
// about itself in its role file AND the terminal's worktree is the session's
// worktree root; the pair must be unique on both sides. Titles are never identity.
func (rc *repoContext) peers(ctx context.Context) []Peer {
	joinedTerm := map[string]string{} // handle -> sid
	termsByHandle := map[string][]orcaTerm{}
	for _, t := range rc.terms {
		termsByHandle[t.Handle] = append(termsByHandle[t.Handle], t)
	}
	claimed := map[string]int{}
	for _, e := range rc.entries {
		if f, ok := rc.roles[e.SessionID]; ok && f.OrcaTerminal != "" {
			claimed[f.OrcaTerminal]++
		}
	}
	nameCount := map[string]int{}
	for _, e := range rc.snap.Entries { // SendMessage names are machine-wide
		if e.NameOK {
			nameCount[e.Name]++
		}
	}
	out := []Peer{}
	for _, e := range rc.entries {
		p := Peer{Session: e.SessionID, Session6: session6(e.SessionID), Cwd: e.Cwd, State: e.Status, Alive: "alive", Via: []string{"claude"}}
		if e.NameOK {
			p.Name = e.Name
		}
		p.Branch = branchOf(ctx, e.Cwd)
		if path, d := LocateTranscript(ccregistry.HomeDir(), e.Cwd, e.SessionID); d == nil {
			if m, err := statMtime(path); err == nil {
				p.LastActivity = m.UTC().Format(time.RFC3339)
			}
		}
		if f, ok := rc.roles[e.SessionID]; ok {
			p.Role, p.Task, p.Scope = f.Role, f.Task, f.Scope
			if f.Role == "worker" {
				if name, ok := readBinding(rc.root, p.Branch); ok {
					p.Orc = name
				}
			}
			if f.OrcaTerminal != "" {
				cands := []orcaTerm{}
				for _, t := range termsByHandle[f.OrcaTerminal] {
					if fsutil.SameDir(t.Path, rc.rootOf[e.SessionID]) {
						cands = append(cands, t)
					}
				}
				switch {
				case len(cands) == 1 && claimed[f.OrcaTerminal] == 1:
					p.TerminalID, p.AgentType = f.OrcaTerminal, cands[0].AgentType
					p.Via = append(p.Via, "orca")
					joinedTerm[f.OrcaTerminal] = e.SessionID
				case len(cands) > 1 || (len(cands) == 1 && claimed[f.OrcaTerminal] > 1):
					p.Reason = "peer_ambiguous"
				}
			}
		}
		switch {
		case p.TerminalID != "":
			p.Send = "orca"
		case !e.NameOK:
			p.Send, p.Reason = "none", nonEmpty(p.Reason, "no_valid_name")
		case nameCount[e.Name] != 1:
			p.Send, p.Reason = "none", nonEmpty(p.Reason, "name_not_unique")
		default:
			p.Send = "claude"
		}
		out = append(out, p)
	}
	// Orca rows no registry session joined: a non-Claude agent is read-only in v1.
	registryRoots := map[string]bool{}
	for _, r := range rc.rootOf {
		registryRoots[r] = true
	}
	for _, t := range rc.terms {
		if _, ok := joinedTerm[t.Handle]; ok {
			continue
		}
		wt := worktreeOf(t.Path, rc.roots)
		if t.AgentType == "claude" && registryRoots[wt] {
			continue // the registry row of that worktree already stands for this session
		}
		p := Peer{Name: t.Title, TerminalID: t.Handle, Cwd: t.Path, AgentType: t.AgentType, State: t.State, Alive: "alive", Via: []string{"orca"}, Send: "none"}
		p.Reason = "non_claude_send_unsupported_v1"
		if t.AgentType == "claude" {
			p.Reason = "claude_terminal_unjoined"
		}
		out = append(out, p)
	}
	// Role files whose holder the registry cannot show (pid unknown / unrecognized).
	for sid, f := range rc.roles {
		if _, ok := rc.snap.Find(sid); ok {
			continue
		}
		if rc.snap.Liveness(sid, f.Pid) != ccregistry.Unknown {
			continue
		}
		out = append(out, Peer{Role: f.Role, Name: f.Name, Scope: f.Scope, Task: f.Task, Session: sid, Session6: session6(sid), Cwd: f.Cwd, Alive: "unknown", Via: []string{}, Send: "none", Reason: "liveness_unknown"})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return roleRank(out[i].Role) < roleRank(out[j].Role)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func roleRank(r string) int {
	switch r {
	case "orchestrator":
		return 0
	case "worker":
		return 1
	case "peer":
		return 2
	}
	return 3
}

// Who lists the repo's live peers.
func Who(ctx context.Context, o WhoOptions) (*WhoResult, error) {
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}}
	if o.Timeout <= 0 {
		o.Timeout = 2500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	rc, err := loadRepo(ctx, o.Cwd, o.ProbeOrca)
	if err != nil {
		e, _ := err.(*Error)
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, err
	}
	res.Degraded = rc.degraded
	if o.Prune {
		if unlock, err := lockPeers(rc.root, max(time.Until(deadlineOf(ctx)), 50*time.Millisecond)); err == nil {
			pruneRoles(rc.roles, rc.snap, "")
			unlock()
		} else {
			res.Degraded = append(res.Degraded, Degraded{Source: "roles", Reason: "peers_lock_busy"})
		}
	}
	for _, p := range rc.peers(ctx) {
		if o.Session != "" && p.Session == o.Session {
			self := p
			res.Self = &self
		}
		if (o.ExcludeSession != "" && p.Session == o.ExcludeSession) || (o.Role != "" && p.Role != o.Role) || (o.Orc != "" && (p.Role != "worker" || p.Orc != o.Orc)) {
			continue
		}
		res.Peers = append(res.Peers, p)
	}
	res.OK = true
	return res, nil
}

func deadlineOf(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(time.Second)
}
