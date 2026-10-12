//go:build unix

package peerscmd

import (
	"context"
	"errors"
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
	"github.com/restarter/lets-workflow/cli/internal/peername"
	"github.com/restarter/lets-workflow/cli/internal/peertail"
	"github.com/restarter/lets-workflow/cli/internal/redact"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
)

// WhoOptions configures Who. Cwd, Orc, ExcludeSession, Session, OrcaRepos and
// Timeout are the `who` verb; Role, Prune, ProbeOrca and Repo belong to the old role
// / Orca path (whoLegacy), which only the sibling walk still reaches (Repo set) and
// which goes in the next step of lets-0xsw2.
type WhoOptions struct {
	Cwd            string
	Role           string // old path: filter orchestrator | worker | peer
	Orc            string // filter: sessions whose branch is bound to this orchestrator name
	ExcludeSession string
	Session        string        // self lookup
	Prune          bool          // old path
	ProbeOrca      bool          // old path
	Repo           string        // old path: another registered repo's main checkout (read-only)
	OrcaRepos      bool          // every repo Orca knows, validated in Go; orchestrator rows carry repo_index
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
		p, f := repoByIndex(ctx, *idx)
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

// whoLegacy is the old role / Orca `who`, reached only with Repo set (the sibling
// walk of the old orchestrator resolution). It goes with that path.
func whoLegacy(ctx context.Context, o WhoOptions) (*WhoResult, error) {
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}}
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
	// An orchestrator's bound workers in sibling repos (the bound-sibling carve-out).
	// Never from a --repo read: a sibling read does not fan out again.
	if o.Orc != "" && o.Repo == "" && orcaSelected(rc.root, o.ProbeOrca) {
		sib, deg := siblingWorkers(ctx, rc, o)
		res.Peers = append(res.Peers, sib...)
		res.Degraded = append(res.Degraded, deg...)
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
	out := readLastSeen(peersDir(rc.root))
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

func deadlineOf(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(time.Second)
}

// Who lists the live Claude sessions of the repo from the session registry: one row
// per session with its name, branch, the branch's task-state task and orc binding,
// and whether it is an orchestrator - a name recorded in the merge-branch
// task-state, or (only when no recorded name is live) the <repo>-orc[-scope]
// convention. No roles, no Orca. Every call refreshes the last-seen file of each
// orchestrator row it lists.
func Who(ctx context.Context, o WhoOptions) (*WhoResult, error) {
	if o.Timeout <= 0 {
		o.Timeout = 2500 * time.Millisecond
	}
	if o.Repo != "" {
		return whoLegacy(ctx, o)
	}
	if o.OrcaRepos {
		return whoOrcaRepos(ctx, o)
	}
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	cwd := o.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if gitutil.ProjectRoot(cwd, 2*time.Second) == "" || mainRoot == "" {
		e := &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	res.RepoSlug = peername.RepoSlug(mainRoot)
	rows, pids, degraded := repoRows(ctx, mainRoot)
	res.Degraded = append(res.Degraded, degraded...)
	for _, p := range rows {
		if o.Session != "" && p.Session == o.Session {
			self := p
			res.Self = &self
		}
		if (o.ExcludeSession != "" && p.Session == o.ExcludeSession) || (o.Orc != "" && p.Orc != o.Orc) {
			continue
		}
		res.Peers = append(res.Peers, p)
	}
	refreshLastSeen(mainRoot, rows, pids)
	res.OK = true
	return res, nil
}

// mergeBranchOf is the repo's merge-branch, resolved the way the task-state writer
// resolves it (LETS_MERGE_BRANCH, else the origin default branch, else main).
func mergeBranchOf(root string) string {
	home, _ := os.UserHomeDir()
	return letsconfig.ResolvedEnv(root, home, func(r string) string { return gitutil.DefaultBranch(r, 2*time.Second) })["LETS_MERGE_BRANCH"]
}

// orchestratorRecords reads the `orchestrator:` lines of the merge-branch task-state
// of mainRoot. A line that does not parse is named in degraded[]; a missing file is
// no records.
func orchestratorRecords(mainRoot, merge string) ([]taskstate.Orchestrator, []Degraded) {
	slug, ok := taskstate.Slug(merge)
	if !ok {
		return nil, nil
	}
	st, err := taskstate.Read(filepath.Join(mainRoot, ".lets"), slug)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, []Degraded{{Source: "taskstate", Reason: "task_state_unreadable", Detail: err.Error()}}
	}
	recs, bad := taskstate.Orchestrators(st)
	var degraded []Degraded
	for _, b := range bad {
		degraded = append(degraded, Degraded{Source: "taskstate", Reason: "orchestrator_line_invalid", Detail: redact.Cap(redact.Control(b), 120)})
	}
	return recs, degraded
}

// repoRows is the row build of one repo: its registry rows, the merge-branch
// records, and the pid of each row's session (for the last-seen files).
func repoRows(ctx context.Context, mainRoot string) ([]Peer, map[string]int, []Degraded) {
	entries, _, _, snap, degraded := registryPeers(ctx, mainRoot)
	merge := mergeBranchOf(mainRoot)
	recs, rdeg := orchestratorRecords(mainRoot, merge)
	degraded = append(degraded, rdeg...)
	rows, pids := whoRows(ctx, mainRoot, merge, entries, snap, recs)
	return rows, pids, degraded
}

// whoRows builds one row per session of entries (the first registry row stands for
// it). Duplicate is judged over the machine-wide snapshot: SendMessage names are
// machine-wide.
func whoRows(ctx context.Context, mainRoot, merge string, entries []ccregistry.Entry, snap ccregistry.Snapshot, recs []taskstate.Orchestrator) ([]Peer, map[string]int) {
	sessionsOf := map[string]map[string]bool{} // name -> live sessions carrying it
	for _, e := range snap.Entries {
		if e.NameOK {
			if sessionsOf[e.Name] == nil {
				sessionsOf[e.Name] = map[string]bool{}
			}
			sessionsOf[e.Name][e.SessionID] = true
		}
	}
	record := map[string]taskstate.Orchestrator{}
	for _, r := range recs {
		if _, dup := record[r.Name]; !dup {
			record[r.Name] = r
		}
	}
	letsDir := filepath.Join(mainRoot, ".lets")
	seen := map[string]bool{}
	pids := map[string]int{}
	rows := []Peer{}
	recordedLive := false
	for _, e := range entries {
		if seen[e.SessionID] {
			continue
		}
		seen[e.SessionID] = true
		pids[e.SessionID] = e.Pid
		p := Peer{Session: e.SessionID, Session6: session6(e.SessionID), Cwd: e.Cwd, State: e.Status, Alive: "alive"}
		if e.NameOK {
			p.Name = e.Name
			p.Duplicate = len(sessionsOf[e.Name]) > 1
			if r, ok := record[e.Name]; ok {
				p.Orchestrator, p.Scope, p.Standing = true, r.Scope, r.Standing == "on"
				recordedLive = true
			}
		}
		p.Branch = branchOf(ctx, e.Cwd)
		if slug, ok := taskstate.Slug(p.Branch); ok && p.Branch != merge {
			if st, err := taskstate.Read(letsDir, slug); err == nil {
				if taskid.Valid(st.Task) {
					p.Task = st.Task
				}
				if ccregistry.ValidName(st.Orc) {
					p.Orc = st.Orc
				}
			}
		}
		if path, d := peertail.LocateTranscript(ccregistry.HomeDir(), e.Cwd, e.SessionID); d == nil {
			if fi, err := os.Stat(path); err == nil {
				p.LastActivity = fi.ModTime().UTC().Format(time.RFC3339)
			}
		}
		rows = append(rows, p)
	}
	if !recordedLive { // the convention counts only when no recorded name is live
		repo := peername.RepoSlug(mainRoot)
		for i := range rows {
			if scope, ok := peername.ParseOrchestrator(repo, rows[i].Name); ok && rows[i].Name != "" {
				rows[i].Orchestrator, rows[i].Scope = true, scope
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Orchestrator != rows[j].Orchestrator {
			return rows[i].Orchestrator
		}
		return rows[i].Name < rows[j].Name
	})
	return rows, pids
}

// whoOrcaRepos is the hub's view: for every repo Orca knows (validated in Go), its
// live orchestrator rows tagged with repo_index, and the stopped ones from its
// last-seen files (the recorded names, or the convention when none is recorded),
// minus the live ones. Read-only in every repo: no last-seen write there.
func whoOrcaRepos(ctx context.Context, o WhoOptions) (*WhoResult, error) {
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}, LastOrchestrators: []LastOrchestrator{}}
	res.OK = true
	info, f := listOrcaRepos(ctx)
	if f != nil {
		res.Degraded = append(res.Degraded, Degraded{Source: "orca", Reason: nonEmpty(info.Reason, f.Reason), Detail: f.Detail})
		return res, nil
	}
	for _, name := range info.Dropped {
		res.Degraded = append(res.Degraded, Degraded{Source: "orca", Reason: "repo_not_a_checkout", Detail: name})
	}
	res.Repos = info.Repos
	for _, r := range info.Repos {
		idx, name := r.Index, r.Name
		if fi, err := os.Stat(r.Path); err != nil || !fi.IsDir() {
			res.Degraded = append(res.Degraded, Degraded{Source: "repo", Reason: "repo_invalid", Detail: name})
			continue
		}
		if inWt, main := gitutil.DetectInsideWorktreeAt(r.Path); inWt || main == "" || !fsutil.SameDir(main, r.Path) {
			res.Degraded = append(res.Degraded, Degraded{Source: "repo", Reason: "repo_invalid", Detail: name})
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, o.Timeout)
		entries, _, _, snap, degraded := registryPeers(rctx, r.Path)
		merge := mergeBranchOf(r.Path)
		recs, rdeg := orchestratorRecords(r.Path, merge)
		rows, _ := whoRows(rctx, r.Path, merge, entries, snap, recs)
		cancel()
		for _, d := range append(degraded, rdeg...) {
			d.Detail = strings.TrimSpace(name + " " + d.Detail)
			res.Degraded = append(res.Degraded, d)
		}
		live := map[string]bool{}
		for _, p := range rows {
			if !p.Orchestrator {
				continue
			}
			p.RepoIndex = &idx
			res.Peers = append(res.Peers, p)
			live[p.Name] = true
		}
		recorded := map[string]bool{}
		for _, rec := range recs {
			recorded[rec.Name] = true
		}
		repo := peername.RepoSlug(r.Path)
		for _, lo := range readLastSeen(peersDir(r.Path)) {
			_, convention := peername.ParseOrchestrator(repo, lo.Name)
			ours := recorded[lo.Name] || (len(recs) == 0 && convention)
			if live[lo.Name] || !ours {
				continue
			}
			lo.RepoIndex = &idx
			res.LastOrchestrators = append(res.LastOrchestrators, lo)
		}
	}
	return res, nil
}
