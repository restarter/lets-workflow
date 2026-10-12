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

// WhoOptions configures Who.
type WhoOptions struct {
	Cwd            string
	Orc            string // filter: sessions whose branch is bound to this orchestrator name
	ExcludeSession string
	Session        string        // self lookup
	OrcaRepos      bool          // every repo Orca knows, validated in Go; orchestrator rows carry repo_index
	Timeout        time.Duration // total budget (default 2500ms)
}

// listOrcaRepos is orcacmd.ListRepos, a seam so tests supply Orca's repo list.
var listOrcaRepos = orcacmd.ListRepos

// repoByIndex resolves an index through the same seam `who --orca-repos` numbered with.
func repoByIndex(ctx context.Context, idx int) (string, *orcacmd.Failure) {
	info, f := listOrcaRepos(ctx)
	if f != nil {
		return "", f
	}
	for _, r := range info.Repos {
		if r.Index == idx {
			return r.Path, nil
		}
	}
	return "", &orcacmd.Failure{Reason: "repo_index_unknown", Verb: "repo list"}
}

// mainRootOf is the main checkout of cwd (the current directory when empty), or a
// not_in_repo error.
func mainRootOf(cwd string) (string, *Error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(cwd)
	if gitutil.ProjectRoot(cwd, 2*time.Second) == "" || mainRoot == "" {
		return "", &Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"}
	}
	return mainRoot, nil
}

func nonEmpty(s, def string) string {
	if s != "" {
		return s
	}
	return def
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
	if o.OrcaRepos {
		return whoOrcaRepos(ctx, o)
	}
	res := &WhoResult{Envelope: newEnvelope("who"), Peers: []Peer{}}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	mainRoot, e := mainRootOf(o.Cwd)
	if e != nil {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	CarryProof(ctx, mainRoot) // the lazy proof carry: recovers one the hook missed
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
	rows, pids := whoRows(ctx, mainRoot, merge, entries, snap, recs, true)
	return rows, pids, degraded
}

// whoRows builds one row per session of entries (the first registry row stands for
// it). Duplicate is judged over the machine-wide snapshot: SendMessage names are
// machine-wide.
//
// enrich adds what only THIS repo may read: each session's branch, its branch
// task-state (task, orc) and its transcript mtime. The hub (whoOrcaRepos) passes
// false: in another repo it classifies orchestrators from the registry and the
// merge-branch records alone (the hub carve-out, protocol/peers.md).
func whoRows(ctx context.Context, mainRoot, merge string, entries []ccregistry.Entry, snap ccregistry.Snapshot, recs []taskstate.Orchestrator, enrich bool) ([]Peer, map[string]int) {
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
		if !enrich {
			rows = append(rows, p)
			continue
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
// last-seen files (the recorded names, or every file when none is recorded),
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
		rows, _ := whoRows(rctx, r.Path, merge, entries, snap, recs, false) // classification only: no branch task-state, no transcript
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
		for _, lo := range readLastSeen(peersDir(r.Path)) {
			// A repo with records lists its recorded names; one with none (an
			// orchestrator from before names were recorded) lists every last-seen file.
			ours := recorded[lo.Name] || len(recs) == 0
			if live[lo.Name] || !ours {
				continue
			}
			lo.RepoIndex = &idx
			res.LastOrchestrators = append(res.LastOrchestrators, lo)
		}
	}
	return res, nil
}
