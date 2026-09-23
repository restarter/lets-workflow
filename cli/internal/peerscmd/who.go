//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/redact"
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
	Repo           string        // another registered repo's main checkout (read-only: never pruned, never written)
	OrcaRepos      bool          // every repo Orca knows, validated in Go; rows carry repo_index
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
	moves          []move // reconcileRoles' corrections, already applied to roles; persisted only under peers.lock
	ops            orcaOps
	terms          []orcaTerm
	degraded       []Degraded
	peersCache     []Peer // peers() is deterministic for one context; see peers()
}

func loadRepo(ctx context.Context, cwd string, probeOrca bool) (*repoContext, error) {
	return loadRepoFor(ctx, cwd, func(root string) bool { return orcaSelected(root, probeOrca) })
}

// callerSelectsOrca is the Orca decision of the session running the verb (its cwd's
// checkout): a load of ANOTHER project passes it, so that project's own LETS_LAUNCHER
// can neither switch Orca on for a session that opted out nor off for one that opted in.
func callerSelectsOrca(cwd string, probe bool) func(string) bool {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	sel := probe
	if root := gitutil.ProjectRoot(cwd, 2*time.Second); root != "" {
		sel = orcaSelected(root, probe)
	}
	return func(string) bool { return sel }
}

// loadRepoFor is loadRepo with the Orca decision supplied: useOrca gets the loaded
// checkout's root.
func loadRepoFor(ctx context.Context, cwd string, useOrca func(root string) bool) (*repoContext, error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	root := gitutil.ProjectRoot(cwd, 2*time.Second)
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if root == "" || mainRoot == "" {
		return nil, &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	rc := &repoContext{root: root, mainRoot: mainRoot, degraded: []Degraded{}}
	var regDegraded []Degraded
	rc.entries, rc.rootOf, rc.roots, rc.snap, regDegraded = registryPeers(ctx, mainRoot)
	rc.degraded = append(rc.degraded, regDegraded...)
	var invalid []string
	rc.roles, invalid = loadRoles(root)
	for _, name := range invalid {
		rc.degraded = append(rc.degraded, Degraded{Source: "roles", Reason: "role_file_invalid", Detail: name})
	}
	rc.moves = reconcileRoles(rc.roles, rc.snap)
	applyMoves(rc.roles, rc.moves)
	if useOrca(root) {
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

// otherRepo resolves --repo / --repo-index to another project's main checkout;
// given=false when the caller named neither (its own repo is meant).
func otherRepo(ctx context.Context, repo string, idx *int) (path string, given bool, e *Error) {
	if repo == "" && idx == nil {
		return "", false, nil
	}
	if repo == "" {
		p, f := orcacmd.RepoByIndex(ctx, *idx)
		if f != nil {
			return "", true, &Error{Code: ExitNotInRepo, Kind: "repo_invalid", Message: f.Error()}
		}
		repo = p
	}
	if fi, err := os.Stat(repo); err != nil || !fi.IsDir() {
		return "", true, &Error{Code: ExitNotInRepo, Kind: "repo_invalid", Message: "--repo is not a directory"}
	}
	if inWt, main := gitutil.DetectInsideWorktreeAt(repo); inWt || main == "" || !fsutil.SameDir(main, repo) {
		return "", true, &Error{Code: ExitNotInRepo, Kind: "repo_invalid", Message: "--repo is not a main checkout"}
	}
	return repo, true, nil
}

// peerRepo is the repo a verb looks its target up in: rc (this checkout's), or the
// project --repo / --repo-index names - a hub addressing another project's session.
// A name matches only inside the repo it belongs to, so the target is never looked
// up by name here; Orca is consulted there exactly when this session selects it.
func peerRepo(ctx context.Context, rc *repoContext, repo string, idx *int, probe bool) (*repoContext, *Error) {
	path, given, e := otherRepo(ctx, repo, idx)
	if e != nil {
		return nil, e
	}
	if !given {
		return rc, nil
	}
	prc, err := loadRepoFor(ctx, path, callerSelectsOrca(rc.root, probe))
	if err != nil {
		return nil, err.(*Error)
	}
	return prc, nil
}

// peers merges the sources. A registry session and an Orca terminal are one peer
// only when the terminal handle equals the `orca_terminal:` the session reported
// about itself in its role file AND the terminal's worktree is the session's
// worktree root; the pair must be unique on both sides. Titles are never identity.
//
// The result is cached: it costs a git subprocess and a transcript stat per registry
// row, and a resolution tests one candidate after another. Any mutation of rc.roles
// (Who's prune) must therefore happen BEFORE the first call - it does.
func (rc *repoContext) peers(ctx context.Context) []Peer {
	if rc.peersCache != nil {
		return rc.peersCache
	}
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
	// `claude -r <sid>` beside a still-running original: one session, two live pids.
	// Counted over the machine-wide registry - the second pid may sit in another
	// directory or repo under another name, and rc.entries is this repo's rows only.
	perSession := map[string]int{}
	for _, e := range rc.snap.Entries {
		perSession[e.SessionID]++
	}
	seenSession := map[string]bool{}
	out := []Peer{}
	for _, e := range rc.entries {
		if seenSession[e.SessionID] {
			continue // one peer per session: the first row stands for it, flagged below
		}
		seenSession[e.SessionID] = true
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
		case perSession[e.SessionID] > 1:
			p.Send, p.Reason = "none", "session_duplicated"
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
	rc.peersCache = out
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
	if o.OrcaRepos {
		return whoOrcaRepos(ctx, o)
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	useOrca := func(root string) bool { return orcaSelected(root, o.ProbeOrca) }
	if o.Repo != "" {
		if fi, err := os.Stat(o.Repo); err != nil || !fi.IsDir() {
			return repoInvalid(res)
		}
		if inWt, main := gitutil.DetectInsideWorktreeAt(o.Repo); inWt || main == "" || !fsutil.SameDir(main, o.Repo) {
			return repoInvalid(res)
		}
		useOrca = callerSelectsOrca(o.Cwd, o.ProbeOrca)
		o.Cwd, o.Prune = o.Repo, false // another project is read, never written
	}
	rc, err := loadRepoFor(ctx, o.Cwd, useOrca)
	if err != nil {
		e, _ := err.(*Error)
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, err
	}
	res.Degraded = rc.degraded
	if o.Session != "" && o.Repo == "" {
		res.Degraded = append(res.Degraded, HealSelf(rc, o.Session, rc.root, branchOf(ctx, o.Cwd))...)
	}
	if o.Prune {
		if unlock, err := lockPeers(rc.root, max(time.Until(deadlineOf(ctx)), 50*time.Millisecond)); err == nil {
			// Decide from what is on disk now, under the lock - never from the view
			// loaded before it (a role written since would be overwritten).
			files, _, snap, err := reconcileLocked(rc.root)
			if err != nil {
				res.Degraded = append(res.Degraded, Degraded{Source: "roles", Reason: "role_write_failed", Detail: err.Error()})
			}
			pruneRoles(files, snap, "")
			rc.roles, rc.moves = files, nil
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
	if o.Repo != "" {
		res.LastOrchestrators = lastOrchestrators(rc)
	}
	res.OK = true
	return res, nil
}

func repoInvalid(res *WhoResult) (*WhoResult, error) {
	e := &Error{Code: ExitNotInRepo, Kind: "repo_invalid", Message: "--repo is not a main checkout"}
	res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
	return res, e
}

// lastOrchestrators reads last-seen files and the dead orchestrator role files not
// yet pruned. Read-only.
func lastOrchestrators(rc *repoContext) []LastOrchestrator {
	out := []LastOrchestrator{}
	peers := peersDir(rc.root)
	files, _ := filepath.Glob(filepath.Join(lastDir(peers), "*.last"))
	sort.Strings(files)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		fields := map[string]string{}
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(line, ": "); ok {
				fields[k] = v
			}
		}
		if !ccregistry.ValidName(fields["name"]) {
			continue
		}
		out = append(out, lastOrch(fields["name"], fields["scope"], fields["session"], fields["pid"], "last_seen"))
	}
	for sid, f := range rc.roles {
		if f.Role == "orchestrator" && rc.snap.Liveness(sid, f.Pid) == ccregistry.Dead {
			out = append(out, lastOrch(f.Name, f.Scope, sid, strconv.Itoa(f.Pid), "role_file"))
		}
	}
	return out
}

func lastOrch(name, scope, sid, pid, source string) LastOrchestrator {
	lo := LastOrchestrator{Name: name, Scope: redact.Control(scope), Source: source}
	var notes []string
	if ccregistry.ValidSession(sid) {
		lo.Session, lo.Session6 = sid, session6(sid)
	} else {
		notes = append(notes, "session invalid")
	}
	if n, err := strconv.Atoi(pid); err == nil && n >= 0 {
		lo.Pid = &n
	} else {
		notes = append(notes, "pid invalid")
	}
	lo.Note = strings.Join(notes, "; ")
	return lo
}

// whoOrcaRepos runs who over every repo Orca knows (validated in Go) and tags each
// row with its repo_index, so the hub never types an Orca path into a shell.
func whoOrcaRepos(ctx context.Context, o WhoOptions) (*WhoResult, error) {
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}, LastOrchestrators: []LastOrchestrator{}}
	info, f := orcacmd.ListRepos(ctx)
	res.OK = true
	if f != nil {
		res.Degraded = append(res.Degraded, Degraded{Source: "orca", Reason: nonEmpty(info.Reason, f.Reason), Detail: f.Detail})
		return res, nil
	}
	res.Repos = info.Repos
	for _, name := range info.Dropped {
		res.Degraded = append(res.Degraded, Degraded{Source: "orca", Reason: "repo_not_a_checkout", Detail: name})
	}
	for _, r := range info.Repos {
		idx := r.Index
		sub, err := Who(ctx, WhoOptions{Cwd: o.Cwd, Repo: r.Path, Role: o.Role, ProbeOrca: o.ProbeOrca, Timeout: o.Timeout})
		if err != nil {
			res.Degraded = append(res.Degraded, Degraded{Source: "repo", Reason: "repo_invalid", Detail: r.Name})
			continue
		}
		for _, p := range sub.Peers {
			p.RepoIndex = &idx
			res.Peers = append(res.Peers, p)
		}
		for _, lo := range sub.LastOrchestrators {
			lo.RepoIndex = &idx
			res.LastOrchestrators = append(res.LastOrchestrators, lo)
		}
		for _, d := range sub.Degraded {
			d.Detail = strings.TrimSpace(r.Name + " " + d.Detail)
			res.Degraded = append(res.Degraded, d)
		}
	}
	return res, nil
}

func deadlineOf(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(time.Second)
}
