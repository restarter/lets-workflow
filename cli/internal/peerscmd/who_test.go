//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/peername"
)

const (
	sidMain  = "aaaaaaaa-1111-4111-8111-000000000001"
	sidFable = "aaaaaaaa-1111-4111-8111-000000000002"
	sidWork  = "aaaaaaaa-1111-4111-8111-000000000003"
)

func peerBySession(ps []Peer, sid string) *Peer {
	for i := range ps {
		if ps[i].Session == sid {
			return &ps[i]
		}
	}
	return nil
}

// linkedWorktree adds a git worktree of repo whose .lets is a symlink to the main
// checkout's - the shape /lets:worktree create and `lets worktree adopt` produce.
func linkedWorktree(t *testing.T, repo, branch string) string {
	t.Helper()
	wt := filepath.Join(filepath.Dir(repo), "wt-"+strings.ReplaceAll(branch, "/", "-"))
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", branch, wt).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v %s", err, out)
	}
	if err := os.Symlink(filepath.Join(repo, ".lets"), filepath.Join(wt, ".lets")); err != nil {
		t.Fatal(err)
	}
	return wt
}

// taskState writes the task-state file of branch in repo's .lets.
func taskState(t *testing.T, repo, branch, content string) {
	t.Helper()
	p := filepath.Join(repo, ".lets", "sessions", ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func who(t *testing.T, o WhoOptions) *WhoResult {
	t.Helper()
	res, err := Who(context.Background(), o)
	if err != nil || !res.OK {
		t.Fatalf("who: %+v %v", res, err)
	}
	return res
}

// One row per session of this repo (worktrees included, other repos left out), with
// the branch's task and orc binding - never the merge-branch file's - and none of the
// old role / Orca fields.
func TestWho_RowsFromRegistry(t *testing.T) {
	repo := repoWithLets(t, "orca") // the launcher no longer matters: who never consults Orca
	wt := linkedWorktree(t, repo, "feature/w1")
	elsewhere := gitRepo(t)
	taskState(t, repo, "feature/w1", "task: lets-abc\norc: MAIN-LETS\n")
	taskState(t, repo, "main", "task: lets-trunk\norc: SOMEONE\n")
	claudeHome(t, []regRow{{101, sidMain, "MAIN-LETS", repo}, {102, sidWork, "W1", wt}, {103, sidWork, "W1", wt}, {104, sidFable, "OTHER", elsewhere}})

	res := who(t, WhoOptions{Cwd: wt})
	if len(res.Peers) != 2 || res.RepoSlug != peername.RepoSlug(repo) || res.RepoSlug == "" {
		t.Fatalf("rows: %+v slug=%q", res.Peers, res.RepoSlug)
	}
	w := peerBySession(res.Peers, sidWork)
	if w == nil || w.Name != "W1" || w.Branch != "feature/w1" || w.Task != "lets-abc" || w.Orc != "MAIN-LETS" || w.Alive != "alive" || w.Session6 != sidWork[:6] {
		t.Errorf("worker row: %+v", w)
	}
	if m := peerBySession(res.Peers, sidMain); m == nil || m.Branch != "main" || m.Task != "" || m.Orc != "" {
		t.Errorf("the merge-branch file is never a row's binding: %+v", m)
	}
	b, _ := json.Marshal(res.Peers)
	for _, gone := range []string{`"role"`, `"send"`, `"via"`, `"terminal_id"`, `"reason"`, `"repo"`} {
		if strings.Contains(string(b), gone) {
			t.Errorf("who must not fill %s any more: %s", gone, b)
		}
	}
}

func TestWho_SelfExcludeAndOrc(t *testing.T) {
	repo := repoWithLets(t, "")
	wt := linkedWorktree(t, repo, "feature/w1")
	taskState(t, repo, "feature/w1", "task: lets-abc\norc: MAIN-LIC\n")
	claudeHome(t, []regRow{{101, sidMain, "MAIN-PWA", repo}, {102, sidFable, "MAIN-LIC", repo}, {103, sidWork, "W1", wt}})
	res := who(t, WhoOptions{Cwd: repo, Orc: "MAIN-LIC"})
	if len(res.Peers) != 1 || res.Peers[0].Session != sidWork {
		t.Errorf("--orc keeps the sessions bound to it: %+v", res.Peers)
	}
	res = who(t, WhoOptions{Cwd: repo, Session: sidWork, ExcludeSession: sidWork})
	if res.Self == nil || res.Self.Session != sidWork || peerBySession(res.Peers, sidWork) != nil || len(res.Peers) != 2 {
		t.Errorf("self lookup + exclude: %+v", res)
	}
}

// A recorded name is the orchestrator, with its record's scope and standing; while a
// recorded name is live the <repo>-orc convention counts for nothing.
func TestWho_RecordedOrchestrators(t *testing.T) {
	repo := repoWithLets(t, "")
	taskState(t, repo, "main", "orchestrator: MAIN-LETS scope=docs standing=on\norchestrator: MAIN-ETH standing=off\norchestrator: MAIN-GONE\n")
	conv := peername.Orchestrator(peername.RepoSlug(repo), "")
	claudeHome(t, []regRow{{101, sidMain, "MAIN-LETS", repo}, {102, sidFable, "MAIN-ETH", repo}, {103, sidWork, conv, repo}})
	res := who(t, WhoOptions{Cwd: repo})
	m, e, c := peerBySession(res.Peers, sidMain), peerBySession(res.Peers, sidFable), peerBySession(res.Peers, sidWork)
	if m == nil || !m.Orchestrator || m.Scope != "docs" || !m.Standing {
		t.Errorf("recorded with standing on: %+v", m)
	}
	if e == nil || !e.Orchestrator || e.Standing {
		t.Errorf("recorded with standing off: %+v", e)
	}
	if c == nil || c.Orchestrator {
		t.Errorf("the convention must not count while a recorded name is live: %+v", c)
	}
	if !res.Peers[0].Orchestrator || !res.Peers[1].Orchestrator || res.Peers[2].Orchestrator {
		t.Errorf("orchestrators sort first: %+v", res.Peers)
	}
}

// With no recorded name live, <repo>-orc[-scope] is an orchestrator (scope from the
// name, standing off); a name that only starts like it is not.
func TestWho_ConventionFallback(t *testing.T) {
	repo := repoWithLets(t, "")
	taskState(t, repo, "main", "orchestrator: MAIN-GONE standing=on\n")
	slug := peername.RepoSlug(repo)
	claudeHome(t, []regRow{{101, sidMain, peername.Orchestrator(slug, "docs"), repo}, {102, sidFable, slug + "-orchestra", repo}})
	res := who(t, WhoOptions{Cwd: repo})
	if m := peerBySession(res.Peers, sidMain); m == nil || !m.Orchestrator || m.Scope != "docs" || m.Standing {
		t.Errorf("convention row: %+v", m)
	}
	if f := peerBySession(res.Peers, sidFable); f == nil || f.Orchestrator {
		t.Errorf("not a convention name: %+v", f)
	}
}

// A name another live session carries anywhere on the machine is a duplicate; one
// session under two pids is not.
func TestWho_DuplicateNameMachineWide(t *testing.T) {
	repo := repoWithLets(t, "")
	elsewhere := gitRepo(t)
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {102, sidFable, "MAIN", elsewhere}, {103, sidWork, "W1", repo}, {104, sidWork, "W1", repo}})
	res := who(t, WhoOptions{Cwd: repo})
	if m := peerBySession(res.Peers, sidMain); m == nil || !m.Duplicate {
		t.Errorf("MAIN is carried in another repo too: %+v", m)
	}
	if w := peerBySession(res.Peers, sidWork); w == nil || w.Duplicate {
		t.Errorf("one session under two pids is not a duplicate: %+v", w)
	}
}

// Every who refreshes the last-seen file of each orchestrator row it lists; a busy
// lock is silent, and a repo without .lets is never written.
func TestWho_RefreshesLastSeen(t *testing.T) {
	repo := repoWithLets(t, "")
	taskState(t, repo, "main", "orchestrator: MAIN-LETS scope=docs\n")
	claudeHome(t, []regRow{{101, sidMain, "MAIN-LETS", repo}, {103, sidWork, "W1", repo}})
	peers := peersDir(repo)

	unlock, err := lockPeers(repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	who(t, WhoOptions{Cwd: repo})
	unlock()
	if _, err := os.Stat(lastSeenFile(peers, "MAIN-LETS")); !os.IsNotExist(err) || time.Since(start) > 2*time.Second {
		t.Errorf("a busy lock skips the write, briefly: err=%v after %v", err, time.Since(start))
	}

	who(t, WhoOptions{Cwd: repo})
	got := readLastSeen(peers)
	if len(got) != 1 || got[0].Name != "MAIN-LETS" || got[0].Session != sidMain || got[0].Scope != "docs" || got[0].Pid == nil || *got[0].Pid != 101 {
		t.Errorf("last-seen: %+v", got)
	}
	if _, err := os.Stat(lastSeenFile(peers, "W1")); !os.IsNotExist(err) {
		t.Error("only orchestrator rows get a last-seen file")
	}

	bare := gitRepo(t)
	claudeHome(t, []regRow{{101, sidMain, peername.Orchestrator(peername.RepoSlug(bare), ""), bare}})
	who(t, WhoOptions{Cwd: bare})
	if _, err := os.Stat(filepath.Join(bare, ".lets")); !os.IsNotExist(err) {
		t.Error("who must not create .lets in a repo without one")
	}
}

func TestWho_DegradedKeepsRows(t *testing.T) {
	repo := repoWithLets(t, "")
	taskState(t, repo, "main", "orchestrator: MAIN standing=maybe\n")
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}}, 199)
	res := who(t, WhoOptions{Cwd: repo})
	reasons := map[string]bool{}
	for _, d := range res.Degraded {
		reasons[d.Reason] = true
	}
	if len(res.Peers) != 1 || !reasons["registry_protocol_unknown"] || !reasons["orchestrator_line_invalid"] {
		t.Errorf("rows kept, both sources named: %+v", res)
	}
	if res.Peers[0].Orchestrator {
		t.Errorf("a malformed record names nobody: %+v", res.Peers[0])
	}
	if _, err := Who(context.Background(), WhoOptions{Cwd: t.TempDir()}); err == nil || err.(*Error).Kind != "not_in_repo" {
		t.Errorf("outside a repo: %v", err)
	}
}

// The hub's view: each Orca repo's live orchestrator rows with repo_index, and its
// stopped recorded orchestrators from last-seen files minus the live ones - without
// writing anything in any repo.
func TestWho_OrcaReposReadOnly(t *testing.T) {
	_ = repoWithLets(t, "orca")
	a, b := siblingRepo(t), siblingRepo(t)
	gone := siblingRepo(t)
	fakeRepos(t, a, gone, b)
	_ = os.RemoveAll(gone)
	taskState(t, a, "main", "orchestrator: LIVE-A standing=on\norchestrator: STOPPED-A\n")
	convB := peername.Orchestrator(peername.RepoSlug(b), "")
	claudeHome(t, []regRow{{101, sidMain, "LIVE-A", a}, {102, sidWork, "WORKER-A", a}, {103, sidFable, convB, b}})
	for _, name := range []string{"LIVE-A", "STOPPED-A", "UNRECORDED-A"} {
		_ = writeLastSeen(peersDir(a), lastSeen{Session: sidFable, Pid: 9, Name: name})
	}
	_ = writeLastSeen(peersDir(b), lastSeen{Session: sidMain, Pid: 8, Name: peername.Orchestrator(peername.RepoSlug(b), "old")})
	_ = writeLastSeen(peersDir(b), lastSeen{Session: sidWork, Pid: 7, Name: "MAIN-OLD"}) // a v0.10.0-era name: listed while b records none
	before := map[string]map[string]string{a: treeState(t, filepath.Join(a, ".lets")), b: treeState(t, filepath.Join(b, ".lets"))}

	res := who(t, WhoOptions{OrcaRepos: true})
	if len(res.Peers) != 2 {
		t.Fatalf("only orchestrator rows: %+v", res.Peers)
	}
	if p := peerBySession(res.Peers, sidMain); p == nil || p.RepoIndex == nil || *p.RepoIndex != 0 || !p.Standing {
		t.Errorf("repo a: %+v", p)
	}
	if p := peerBySession(res.Peers, sidFable); p == nil || p.RepoIndex == nil || *p.RepoIndex != 2 {
		t.Errorf("repo b (convention): %+v", p)
	}
	var stopped []string
	for _, lo := range res.LastOrchestrators {
		stopped = append(stopped, lo.Name)
		if lo.RepoIndex == nil {
			t.Errorf("a stopped orchestrator carries its repo_index: %+v", lo)
		}
	}
	if strings.Join(stopped, ",") != "STOPPED-A,MAIN-OLD,"+peername.Orchestrator(peername.RepoSlug(b), "old") {
		t.Errorf("stopped = %v", stopped)
	}
	named := false
	for _, d := range res.Degraded {
		named = named || (d.Source == "repo" && d.Reason == "repo_invalid" && d.Detail == filepath.Base(filepath.Dir(gone)))
	}
	if !named || len(res.Repos) != 3 {
		t.Errorf("an unreadable repo degrades by name: %+v", res.Degraded)
	}
	for dir, st := range before {
		after := treeState(t, filepath.Join(dir, ".lets"))
		if len(after) != len(st) {
			t.Errorf("%s changed: %d -> %d entries", dir, len(st), len(after))
		}
		for p, s := range st {
			if after[p] != s {
				t.Errorf("%s changed", p)
			}
		}
	}

	old := listOrcaRepos
	listOrcaRepos = func(context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		return &orcacmd.ReposInfo{Reason: "orca_unavailable"}, &orcacmd.Failure{Reason: "orca_unavailable"}
	}
	t.Cleanup(func() { listOrcaRepos = old })
	if res := who(t, WhoOptions{OrcaRepos: true}); len(res.Degraded) != 1 || res.Degraded[0].Reason != "orca_unavailable" {
		t.Errorf("Orca unavailable: %+v", res.Degraded)
	}
}
