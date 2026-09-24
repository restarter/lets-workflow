//go:build unix

package peerscmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
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

func TestWho_RegistryOnlyAndLauncherGate(t *testing.T) {
	repo := repoWithLets(t, "terminal")
	t.Setenv("ORCA_WORKTREE_ID", "repo::"+repo)
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}})
	calls := useOrca(t, &fakeOps{})
	res, err := Who(context.Background(), WhoOptions{Cwd: repo})
	if err != nil || len(res.Peers) != 1 || res.Peers[0].Send != "claude" || res.Peers[0].Via[0] != "claude" {
		t.Fatalf("registry only: %+v %v", res, err)
	}
	if *calls != 0 {
		t.Errorf("LETS_LAUNCHER=terminal must never look up Orca, even with ORCA_WORKTREE_ID set (%d calls)", *calls)
	}
}

func TestWho_UserLevelLauncherSelectsOrca(t *testing.T) {
	repo := repoWithLets(t, "")
	home := os.Getenv("HOME")
	_ = os.MkdirAll(filepath.Join(home, ".lets"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".lets", ".env"), []byte("LETS_LAUNCHER=orca\n"), 0o644)
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}})
	calls := useOrca(t, &fakeOps{})
	if _, err := Who(context.Background(), WhoOptions{Cwd: repo}); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Errorf("a user-level LETS_LAUNCHER=orca selects the Orca source (%d calls)", *calls)
	}
}

func TestWho_JoinRule(t *testing.T) {
	repo := repoWithLets(t, "orca")
	other := filepath.Join(filepath.Dir(repo), "wt2") // another worktree of the SAME repo
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", "wt2", other).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v %s", err, out)
	}
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {102, sidFable, "MAIN-FABLE", repo}})
	plantRole(t, repo, sidMain, "role: peer\npid: 101\norca_terminal: term_main\nset: x\n")
	plantRole(t, repo, sidFable, "role: peer\npid: 102\norca_terminal: term_fable\nset: x\n")
	ops := &fakeOps{terms: []orcaTerm{
		{Handle: "term_fable", Path: repo, Title: "MAIN", AgentType: "claude", State: "done"},    // right id, spoofed title
		{Handle: "term_spoof", Path: repo, Title: "MAIN-FABLE", AgentType: "claude"},             // right title, no id
		{Handle: "term_main", Path: other, Title: "MAIN", AgentType: "claude"},                   // right id, other worktree
		{Handle: "term_codex", Path: repo, Title: "codex", AgentType: "codex", State: "working"}, // non-Claude agent
	}}
	useOrca(t, ops)
	res, err := Who(context.Background(), WhoOptions{Cwd: repo})
	if err != nil {
		t.Fatal(err)
	}
	fable, main := peerBySession(res.Peers, sidFable), peerBySession(res.Peers, sidMain)
	if fable == nil || fable.Send != "orca" || fable.TerminalID != "term_fable" || len(fable.Via) != 2 {
		t.Errorf("MAIN-FABLE joins by its self-reported id: %+v", fable)
	}
	if main == nil || main.Send != "claude" || main.TerminalID != "" {
		t.Errorf("MAIN: the id in another worktree joins nothing: %+v", main)
	}
	codex := false
	for _, p := range res.Peers {
		if p.TerminalID == "term_spoof" {
			t.Errorf("a title match must not produce a joined or listed Claude row: %+v", p)
		}
		if p.TerminalID == "term_codex" {
			codex = p.Send == "none" && p.Reason == "non_claude_send_unsupported_v1"
		}
	}
	if !codex {
		t.Errorf("the Codex pane is read-only in v1: %+v", res.Peers)
	}
}

func TestWho_DegradedAndFilters(t *testing.T) {
	repo := repoWithLets(t, "")
	claudeHome(t, []regRow{
		{101, sidMain, "MAIN-PWA", repo}, {102, sidFable, "MAIN-LIC", repo}, {103, sidWork, "W1", repo},
	}, 199)
	plantRole(t, repo, sidMain, "role: orchestrator\nscope: pwa\npid: 101\nset: x\n")
	plantRole(t, repo, sidFable, "role: orchestrator\nscope: lic\npid: 102\nset: x\n")
	plantRole(t, repo, sidWork, "role: worker\ntask: lets-abc\npid: 103\nset: x\n")
	old := branchOf
	branchOf = func(_ context.Context, cwd string) string { return "feature/w1" }
	t.Cleanup(func() { branchOf = old })
	_ = os.WriteFile(filepath.Join(repo, ".lets", "sessions", ".task-feature-w1"), []byte("task: lets-abc\norc: MAIN-LIC\n"), 0o644)

	res, _ := Who(context.Background(), WhoOptions{Cwd: repo, Role: "orchestrator"})
	if len(res.Peers) != 2 || res.Peers[0].Scope == "" || res.Peers[1].Scope == "" {
		t.Errorf("two orchestrators with scopes: %+v", res.Peers)
	}
	if len(res.Degraded) != 1 || res.Degraded[0].Reason != "registry_protocol_unknown" {
		t.Errorf("mixed registry keeps rows and names the unknown entry: %+v", res.Degraded)
	}
	res, _ = Who(context.Background(), WhoOptions{Cwd: repo, Orc: "MAIN-LIC"})
	if len(res.Peers) != 1 || res.Peers[0].Session != sidWork || res.Peers[0].Orc != "MAIN-LIC" {
		t.Errorf("--orc lists the workers bound to it: %+v", res.Peers)
	}
	res, _ = Who(context.Background(), WhoOptions{Cwd: repo, Session: sidWork, ExcludeSession: sidWork})
	if res.Self == nil || res.Self.Session != sidWork || peerBySession(res.Peers, sidWork) != nil {
		t.Errorf("self lookup + exclude: %+v", res)
	}
}

func TestWho_BothSourcesDegraded(t *testing.T) {
	repo := repoWithLets(t, "orca")
	oldHome := claudeHome(t, nil)
	_ = os.RemoveAll(filepath.Join(oldHome, "sessions"))
	old := newOrcaOps
	newOrcaOps = func() (orcaOps, *orcacmd.Failure) {
		return nil, &orcacmd.Failure{Reason: "orca_not_found", Verb: "lookup"}
	}
	t.Cleanup(func() { newOrcaOps = old })
	res, err := Who(context.Background(), WhoOptions{Cwd: repo})
	if err != nil || len(res.Degraded) != 2 {
		t.Errorf("two degraded reasons: %+v %v", res.Degraded, err)
	}
}

func TestWho_ForeignRepoReadOnly(t *testing.T) {
	_ = repoWithLets(t, "")
	foreign := gitRepo(t)
	peers := filepath.Join(foreign, ".lets", "sessions", "peers")
	_ = os.MkdirAll(filepath.Join(peers, "last"), 0o700)
	claudeHome(t, nil) // nobody alive
	_ = os.WriteFile(lastSeenFile(peers, "MAIN-PWA"), []byte("session: "+sidMain+"\npid: 11\nname: MAIN-PWA\nscope: pwa\n"), 0o600)
	_ = os.WriteFile(lastSeenFile(peers, "MAIN-LIC"), []byte("session: "+sidFable+"\npid: nope\nname: MAIN-LIC\n"), 0o600)
	_ = os.WriteFile(filepath.Join(peers, sidWork+".role"), []byte("role: orchestrator\nname: MAIN-OLD\npid: 12\nset: x\n"), 0o600)
	before, _ := os.ReadDir(peers)

	res, err := Who(context.Background(), WhoOptions{Repo: foreign, Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]LastOrchestrator{}
	for _, lo := range res.LastOrchestrators {
		got[lo.Name] = lo
	}
	if lo := got["MAIN-PWA"]; lo.Source != "last_seen" || lo.Session != sidMain || lo.Pid == nil || *lo.Pid != 11 {
		t.Errorf("MAIN-PWA: %+v", lo)
	}
	if lo := got["MAIN-LIC"]; lo.Pid != nil || lo.Note == "" {
		t.Errorf("a malformed pid is omitted with a note: %+v", lo)
	}
	if lo := got["MAIN-OLD"]; lo.Source != "role_file" || lo.Session != sidWork {
		t.Errorf("dead unpruned role file: %+v", lo)
	}
	after, _ := os.ReadDir(peers)
	if len(after) != len(before) {
		t.Errorf("a foreign repo must not be written (--prune ignored): %d -> %d entries", len(before), len(after))
	}
	if _, err := os.Stat(filepath.Join(peers, sidWork+".role")); err != nil {
		t.Error("the dead role file of a foreign repo must not be pruned")
	}
	if _, err := Who(context.Background(), WhoOptions{Repo: t.TempDir()}); err == nil || err.(*Error).Kind != "repo_invalid" {
		t.Errorf("a non-checkout repo: %v", err)
	}
}

// orcWithSiblingWorker: orchestrator MAIN in root (launcher orca), one worker in a
// sibling repo whose branch is bound to bindTo.
func orcWithSiblingWorker(t *testing.T, bindTo string) (root, sib string) {
	t.Helper()
	root = repoWithLets(t, "orca")
	useOrca(t, &fakeOps{})
	withBranch(t, "feature/x")
	sib = siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	plantRole(t, sib, sidW, "role: worker\ntask: t-1\npid: 3\nset: x\n")
	bindBranch(t, sib, "feature/x", bindTo)
	return root, sib
}

func TestWho_OrcListsSiblingWorkers(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}})
	res, err := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	w := peerBySession(res.Peers, sidW)
	if err != nil || w == nil || w.RepoIndex == nil || *w.RepoIndex != 1 || w.Repo != filepath.Base(filepath.Dir(sib)) || w.Send != "claude" {
		t.Fatalf("a bound sibling worker must be listed, marked with its repo: %+v %v", res, err)
	}
	if tr := tellFromIdx(t, root, sidM, sidW, w.RepoIndex); tr.Reason != "claude_transport_model_send" {
		t.Errorf("tell with the row's index must reach the sibling worker: %+v", tr)
	}
	if tr := tellFrom(t, root, sidM, sidW); tr.Reason != "peer_unreachable" {
		t.Errorf("without the index tell must not reach it: %+v", tr)
	}
}

func TestWho_OrcSkipsUnboundSiblingWorkers(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "OTHER")
	plantRole(t, sib, sidO, "role: worker\ntask: t-2\npid: 4\nset: x\n") // same branch file: also bound to OTHER
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}, {4, sidO, "W2", sib}})
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if peerBySession(res.Peers, sidW) != nil || peerBySession(res.Peers, sidO) != nil {
		t.Fatalf("workers bound elsewhere must not be listed: %+v", res.Peers)
	}
}

func TestWho_OrcSiblingOwnOrchestratorKeepsWorkers(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	plantRole(t, sib, sidN, "role: orchestrator\nname: MAIN\npid: 2\nset: x\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {2, sidN, "MAIN", sib}, {3, sidW, "W", sib}})
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if peerBySession(res.Peers, sidW) != nil {
		t.Fatalf("a sibling with its own live MAIN keeps its workers: %+v", res.Peers)
	}
}

func TestWho_OrcNoSiblingScanWithoutOrca(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	sib := siblingRepo(t)
	plantRole(t, sib, sidW, "role: worker\ntask: t-1\npid: 3\nset: x\n")
	bindBranch(t, sib, "feature/x", "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}})
	calls := 0
	old := listOrcaRepos
	listOrcaRepos = func(context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		calls++
		return &orcacmd.ReposInfo{Repos: []orcacmd.RepoInfo{{Index: 0, Name: "sib", Path: sib}}}, nil
	}
	t.Cleanup(func() { listOrcaRepos = old })
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if calls != 0 || peerBySession(res.Peers, sidW) != nil {
		t.Fatalf("without LETS_LAUNCHER=orca no sibling is read (%d calls): %+v", calls, res.Peers)
	}
	if res2, _ := Who(context.Background(), WhoOptions{Cwd: root}); calls != 0 || len(res2.Peers) == 0 {
		t.Fatalf("plain who never scans siblings (%d calls)", calls)
	}
}

func TestWho_OrcSiblingDegradesByName(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	gone := siblingRepo(t)
	fakeRepos(t, root, gone, sib)
	_ = os.RemoveAll(gone)
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}})
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	named := false
	for _, d := range res.Degraded {
		if d.Source == "repo" && d.Reason == "repo_invalid" && d.Detail == filepath.Base(filepath.Dir(gone)) {
			named = true
		}
	}
	if !named || peerBySession(res.Peers, sidW) == nil {
		t.Fatalf("an unreadable sibling degrades by name and the others still list: %+v", res)
	}
	// Orca answers only once the budget is spent: the walk stops and says so.
	old := listOrcaRepos
	listOrcaRepos = func(ctx context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		<-ctx.Done()
		return &orcacmd.ReposInfo{Repos: []orcacmd.RepoInfo{{Index: 1, Name: "sib", Path: sib}}}, nil
	}
	t.Cleanup(func() { listOrcaRepos = old })
	res, err := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN", Timeout: 300 * time.Millisecond})
	spent := false
	for _, d := range res.Degraded {
		if d.Source == "context" && d.Reason == "deadline_exceeded" {
			spent = true
		}
	}
	if err != nil || !spent || !res.OK {
		t.Fatalf("a spent budget is named, not silent: %+v %v", res, err)
	}
}

func TestWho_OrcSiblingReadOnly(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	plantRole(t, sib, sidO, "role: worker\ntask: t-3\npid: 9\n"+setLine) // dead: a normal who would prune it
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}})
	before := treeState(t, filepath.Join(sib, ".lets"))
	if _, err := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN", Prune: true}); err != nil {
		t.Fatal(err)
	}
	after := treeState(t, filepath.Join(sib, ".lets"))
	if len(before) != len(after) {
		t.Fatalf("the sibling tree changed: %d -> %d entries", len(before), len(after))
	}
	for p, s := range before {
		if after[p] != s {
			t.Errorf("%s changed", p)
		}
	}
}

// A sibling orchestrator of that name whose liveness is unknown claims nobody: only a
// known-live one keeps its repo's workers.
func TestWho_OrcSiblingUnknownOrchestratorKeepsNothing(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	plantRole(t, sib, sidN, "role: orchestrator\nname: MAIN\npid: 77\nset: x\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}}, 77) // pid 77: live, unknown protocol
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if peerBySession(res.Peers, sidW) == nil {
		t.Fatalf("an unknown-liveness sibling orchestrator must not hide the worker: %+v", res.Peers)
	}
}

// Orca failing because the budget ran out is reported as the budget, not as Orca.
func TestWho_OrcSiblingListFailsAfterDeadline(t *testing.T) {
	root, sib := orcWithSiblingWorker(t, "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", sib}})
	old := listOrcaRepos
	listOrcaRepos = func(ctx context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		<-ctx.Done()
		return &orcacmd.ReposInfo{Reason: "orca_unavailable"}, &orcacmd.Failure{Reason: "orca_unavailable"}
	}
	t.Cleanup(func() { listOrcaRepos = old })
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN", Timeout: 300 * time.Millisecond})
	for _, d := range res.Degraded {
		if d.Source == "orca" {
			t.Errorf("a spent budget must not read as an Orca failure: %+v", res.Degraded)
		}
	}
	spent := false
	for _, d := range res.Degraded {
		spent = spent || (d.Source == "context" && d.Reason == "deadline_exceeded")
	}
	if !spent {
		t.Errorf("the spent budget must be named: %+v", res.Degraded)
	}
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

// A worker (or a sibling's own orchestrator) running in a linked worktree registers
// through its .lets symlink, i.e. in the main checkout the Orca walk reads.
func TestWho_OrcSiblingWorkerInLinkedWorktree(t *testing.T) {
	root := repoWithLets(t, "orca")
	useOrca(t, &fakeOps{})
	withBranch(t, "feature/x")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	wt := linkedWorktree(t, sib, "feature/x")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	plantRole(t, wt, sidW, "role: worker\ntask: t-1\npid: 3\nset: x\n") // written through wt/.lets
	bindBranch(t, wt, "feature/x", "MAIN")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {3, sidW, "W", wt}})
	res, _ := Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if w := peerBySession(res.Peers, sidW); w == nil || w.RepoIndex == nil || *w.RepoIndex != 1 {
		t.Fatalf("a bound worker in a linked worktree of a sibling must be listed: %+v", res.Peers)
	}
	// The sibling's own live MAIN, itself in another linked worktree, still keeps it.
	wt2 := linkedWorktree(t, sib, "feature/y")
	plantRole(t, wt2, sidN, "role: orchestrator\nname: MAIN\npid: 2\nset: x\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {2, sidN, "MAIN", wt2}, {3, sidW, "W", wt}})
	res, _ = Who(context.Background(), WhoOptions{Cwd: root, Orc: "MAIN"})
	if peerBySession(res.Peers, sidW) != nil {
		t.Fatalf("a worktree-local sibling orchestrator named MAIN keeps its worker: %+v", res.Peers)
	}
}
