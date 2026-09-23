//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withBranch(t *testing.T, branch string) {
	t.Helper()
	old := branchOf
	branchOf = func(context.Context, string) string { return branch }
	t.Cleanup(func() { branchOf = old })
}

func bindBranch(t *testing.T, root, branch, orc string) {
	t.Helper()
	p := filepath.Join(root, ".lets", "sessions", ".task-"+strings.ReplaceAll(branch, "/", "-"))
	if err := os.WriteFile(p, []byte("task: lets-abc\norc: "+orc+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resolveIn loads root as a repoContext and resolves session's orchestrator - the
// same two calls every peers verb makes; a fresh call gets a fresh peers() cache.
func resolveIn(t *testing.T, root, session string) *Resolution {
	t.Helper()
	rc, err := loadRepo(context.Background(), root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	return ResolveOrchestrator(context.Background(), rc, ResolveOptions{Session: session, Cwd: root})
}

const sidW = "dddddddd-0000-4000-8000-00000000000d"

func TestResolveOrchestrator_Self(t *testing.T) {
	root := repoWithLets(t, "")
	claudeHome(t, []regRow{{1, sidM, "MAIN", root}})
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "OTHER")
	if r := resolveIn(t, root, sidM); r.Source != "self" || r.Target.Name != "MAIN" {
		t.Errorf("self: %+v", r)
	}
}

func TestResolveOrchestrator_Bound(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN-PWA\npid: 1\nset: x\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\npid: 2\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN-PWA")

	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", root}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Source != "bound" || r.Target == nil || r.Target.Session != sidM || r.Target.Alive != "alive" || r.Target.Send == "" {
		t.Errorf("bound alive with a computed send: %+v %+v", r, r.Target)
	}
	claudeHome(t, []regRow{{2, sidN, "MAIN-LIC", root}, {3, sidW, "W", root}}) // MAIN-PWA's pid died
	if r := resolveIn(t, root, sidW); r.Source != "bound" || r.Target != nil || r.Reason != "target_not_alive" || len(r.Refused) != 1 || r.Refused[0].Name != "MAIN-PWA" {
		t.Errorf("bound dead must be refused by name, never re-routed: %+v", r)
	}
	// the holder ran /rename MAIN-PWA2 and the branch is bound to the new name
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA2", root}, {2, sidN, "MAIN-LIC", root}})
	bindBranch(t, root, "feature/x", "MAIN-PWA2")
	if r := resolveIn(t, root, sidW); r.Target == nil || r.Target.Session != sidM {
		t.Errorf("bound by live name: %+v", r)
	}
}

func TestResolveOrchestrator_Unbound(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN-PWA\nscope: pwa\npid: 1\nset: x\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}})
	if r := resolveIn(t, root, sidW); r.Source != "single" || r.Target.Session != sidM {
		t.Errorf("single: %+v", r)
	}
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\nscope: lic\nset: x\n") // pid-less: liveness unknown, so unaddressable
	if r := resolveIn(t, root, sidW); r.Source != "ambiguous" || len(r.Candidates) != 1 || r.Candidates[0].Session != sidM || len(r.Refused) != 1 || r.Refused[0].Name != "MAIN-LIC" || r.Refused[0].Reason != "target_unsendable" {
		t.Errorf("one live plus one unknown-liveness orchestrator must stay ambiguous, not silently single: %+v", r)
	}
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\nscope: lic\npid: 2\nset: x\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", root}})
	r := resolveIn(t, root, sidW)
	if r.Source != "ambiguous" || len(r.Candidates) != 2 || r.Candidates[0].Scope == "" || r.Candidates[1].Scope == "" {
		t.Errorf("two live: %+v", r)
	}
	bindBranch(t, root, "feature/x", "$(x)")
	if r := resolveIn(t, root, sidW); r.Source == "bound" {
		t.Errorf("an invalid orc: line is unbound: %+v", r)
	}
	withBranch(t, "main")
	bindBranch(t, root, "main", "MAIN-PWA")
	if r := resolveIn(t, root, sidW); r.Source == "bound" {
		t.Errorf("orc: on the merge-branch is ignored: %+v", r)
	}
	if r := resolveIn(t, repoWithLets(t, ""), sidW); r.Source != "none" {
		t.Errorf("none: %+v", r)
	}
}

// TestResolveOrchestrator_ExhaustedBudgetNeverRebinds is the regression for the
// silent re-route FIX A closes: Orchestrator()'s resolution budget spending itself
// before the branch is read must degrade loudly, never fall through to the unbound
// path and pick a DIFFERENT live orchestrator than the one this branch is bound to.
func TestResolveOrchestrator_ExhaustedBudgetNeverRebinds(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidA, "role: orchestrator\nname: ORC-A\npid: 101\nset: x\n")
	plantRole(t, root, sidO, "role: orchestrator\nname: ORC-B\npid: 102\nset: x\n")
	bindBranch(t, root, "feature/x", "ORC-A")
	claudeHome(t, []regRow{{101, sidA, "ORC-A", root}, {102, sidO, "ORC-B", root}})

	cancelled, cancel := context.WithCancel(context.Background())
	cancel() // exactly what Orchestrator()'s 2500ms budget leaves behind once exhausted

	// The branch is supplied (read outside the budget, as Orchestrator() now does):
	// the binding to ORC-A must survive an already-exhausted ctx.
	rc, err := loadRepo(cancelled, root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	if r := ResolveOrchestrator(cancelled, rc, ResolveOptions{Session: sidB, Cwd: root, Branch: "feature/x"}); r.Source != "bound" || r.Target == nil || r.Target.Session != sidA {
		t.Fatalf("a supplied Branch must resolve the binding even under an exhausted ctx: %+v", r)
	}

	// No Branch supplied: ResolveOrchestrator must read it itself under the SAME
	// exhausted ctx, fail, and degrade loudly - never silently fall through to the
	// unbound path and pick the other live orchestrator, ORC-B.
	rc2, err := loadRepo(cancelled, root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	r2 := ResolveOrchestrator(cancelled, rc2, ResolveOptions{Session: sidB, Cwd: root})
	if r2.Source != "none" || r2.Reason != "branch_unreadable" || r2.Target != nil {
		t.Fatalf("an exhausted budget with no Branch must degrade loudly, not silently resolve to ORC-B: %+v", r2)
	}
	found := false
	for _, d := range r2.Degraded {
		if d.Source == "git" && d.Reason == "branch_unreadable" {
			found = true
		}
	}
	if !found {
		t.Errorf("a git-degraded entry must be recorded: %+v", r2.Degraded)
	}
}

// TestResolveOrchestrator_ExhaustedBudgetOnNamedBranchNeverResolvesSingle is FIX C:
// the guard must fire even when the branch itself is perfectly readable and simply
// has no binding - an exhausted ctx by the time resolution runs must still degrade
// loudly rather than let the unbound path resolve a guess.
func TestResolveOrchestrator_ExhaustedBudgetOnNamedBranchNeverResolvesSingle(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidA, "role: orchestrator\nname: ORC-A\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "ORC-A", root}})
	// no bindBranch: "feature/y" is a genuinely readable, genuinely unbound branch

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	rc, err := loadRepo(cancelled, root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	r := ResolveOrchestrator(cancelled, rc, ResolveOptions{Session: sidB, Cwd: root, Branch: "feature/y"})
	if r.Source != "none" || r.Reason != "budget_exhausted" || r.Target != nil {
		t.Fatalf("an exhausted budget on a readable-but-unbound branch must degrade loudly, never resolve single: %+v", r)
	}
	found := false
	for _, d := range r.Degraded {
		if d.Source == "context" && d.Reason == "deadline_exceeded" {
			found = true
		}
	}
	if !found {
		t.Errorf("a context-degraded entry must be recorded: %+v", r.Degraded)
	}
}

// TestResolveOrchestrator_BoundSameRepoIsAddressable is the regression case that
// must NOT break: a bound orchestrator of this same repo resolves with a computed
// send, never the uncomputed "" the live bug reported (see the plan's Context).
func TestResolveOrchestrator_BoundSameRepoIsAddressable(t *testing.T) {
	root := repoWithLets(t, "terminal")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN")
	plantRole(t, root, sidA, "role: orchestrator\nname: MAIN\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "MAIN", root}, {102, sidB, "WORKER", root}})
	res := resolveIn(t, root, sidB)
	if res.Target == nil || res.Target.Send == "" {
		t.Fatalf("same-repo orchestrator must resolve with a computed send: %+v", res)
	}
}

// TestResolveOrchestrator_CrossRepoRefused: a bound orchestrator's role file lives in
// this repo's peers dir, but its session currently registers a cwd outside it (the
// stale-role shape) - refused by name, never returned as an unreachable target.
func TestResolveOrchestrator_CrossRepoRefused(t *testing.T) {
	root := repoWithLets(t, "")
	foreign := gitRepo(t)
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "FOREIGN")
	plantRole(t, root, sidA, "role: orchestrator\nname: FOREIGN\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "FOREIGN", foreign}})
	res := resolveIn(t, root, sidB)
	if res.Target != nil || res.Reason != "target_in_other_repo" || len(res.Refused) != 1 || res.Refused[0].Name != "FOREIGN" {
		t.Fatalf("cross-repo orchestrator must be refused, not returned: %+v", res)
	}
	if res.Refused[0].Session6 == "" {
		t.Errorf("a refusal names the target without a full session id: %+v", res.Refused[0])
	}
}

// TestResolveOrchestrator_DeadRoleFileRefused: a bound orchestrator whose session is
// not in the registry at all (its pid is not alive) is refused, not returned.
func TestResolveOrchestrator_DeadRoleFileRefused(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "GONE")
	plantRole(t, root, sidA, "role: orchestrator\nname: GONE\npid: 101\nset: x\n")
	claudeHome(t, nil) // a present, empty registry: pid 101 reads as not alive, not unknown
	res := resolveIn(t, root, sidB)
	if res.Target != nil || res.Reason != "target_not_alive" || len(res.Refused) != 1 {
		t.Errorf("a role file with no live session must be refused, not returned: %+v", res)
	}
}

// TestResolveOrchestrator_UnsendableNameCollisionRefused: the row is present and
// alive, but who computes Send=none (a name shared by two live sessions) - a third
// refusal reason beyond cross-repo and dead.
func TestResolveOrchestrator_UnsendableNameCollisionRefused(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "DUP")
	plantRole(t, root, sidA, "role: orchestrator\nname: DUP\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "DUP", root}, {102, sidO, "DUP", root}})
	res := resolveIn(t, root, sidB)
	if res.Target != nil || res.Reason != "target_unsendable" || len(res.Refused) != 1 || res.Refused[0].Detail != "name_not_unique" {
		t.Errorf("a present-but-unsendable orchestrator must be refused with its reason: %+v", res)
	}
}

// A dead end carries the line the user acts on.
func TestResolve_EveryReasonHasRemediation(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "GONE")
	claudeHome(t, []regRow{{3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Reason != "orchestrator_not_registered" || !strings.Contains(r.Remediation, "/lets:start") || !strings.HasPrefix(r.Remediation, "GONE") {
		t.Errorf("a dead end must carry its remedy: %+v", r)
	}
	for _, reason := range []string{"orchestrator_not_registered", "target_not_alive", "target_in_other_repo", "target_unsendable", "bound_ambiguous", "branch_unreadable", "budget_exhausted", "orchestrator_needs_name"} {
		if remedy(reason, "X", false) == "" || remedy(reason, "X", true) == "" {
			t.Errorf("%s has no remedy", reason)
		}
	}
}
