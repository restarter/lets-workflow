//go:build unix

package peerscmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
