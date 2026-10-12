//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/peername"
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
	return resolveOrchestratorLegacy(context.Background(), rc, ResolveOptions{Session: session, Cwd: root})
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
	if r := resolveOrchestratorLegacy(cancelled, rc, ResolveOptions{Session: sidB, Cwd: root, Branch: "feature/x"}); r.Source != "bound" || r.Target == nil || r.Target.Session != sidA {
		t.Fatalf("a supplied Branch must resolve the binding even under an exhausted ctx: %+v", r)
	}

	// No Branch supplied: ResolveOrchestrator must read it itself under the SAME
	// exhausted ctx, fail, and degrade loudly - never silently fall through to the
	// unbound path and pick the other live orchestrator, ORC-B.
	rc2, err := loadRepo(cancelled, root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	r2 := resolveOrchestratorLegacy(cancelled, rc2, ResolveOptions{Session: sidB, Cwd: root})
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
	r := resolveOrchestratorLegacy(cancelled, rc, ResolveOptions{Session: sidB, Cwd: root, Branch: "feature/y"})
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
	if res.Refused[0].Detail != "orca_not_selected" {
		t.Errorf("without the Orca launcher the refusal must say so: %+v", res.Refused[0])
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
	for reason := range legacyRefusalDetails {
		if remedy(reason, "", "X", false, false) == "" || remedy(reason, "", "X", true, false) == "" {
			t.Errorf("%s has no remedy", reason)
		}
	}
}

// An unsendable target's remedy follows its reason: idling fixes none of these.
func TestRemedy_UnsendableFollowsDetail(t *testing.T) {
	for detail, want := range map[string]string{
		"session_duplicated": "close one of them",
		"name_not_unique":    "/rename all but one",
		"no_valid_name":      "/rename it",
		"peer_ambiguous":     "close the stale pane",
		"liveness_unknown":   "/lets:orc who",
	} {
		got := remedy("target_unsendable", detail, "X", false, false)
		if !strings.Contains(got, want) || strings.Contains(got, "idle") {
			t.Errorf("%s: %q, want it to say %q", detail, got, want)
		}
	}
}

// fakeRepos makes Orca list exactly these main checkouts, indexed in this order.
func fakeRepos(t *testing.T, paths ...string) {
	t.Helper()
	old := listOrcaRepos
	listOrcaRepos = func(context.Context) (*orcacmd.ReposInfo, *orcacmd.Failure) {
		info := &orcacmd.ReposInfo{Repos: []orcacmd.RepoInfo{}}
		for i, p := range paths {
			info.Repos = append(info.Repos, orcacmd.RepoInfo{Index: i, Name: filepath.Base(filepath.Dir(p)), Path: p})
		}
		return info, nil
	}
	t.Cleanup(func() { listOrcaRepos = old })
}

// siblingRepo is a second git main checkout with a .lets dir, as Orca would list it.
func siblingRepo(t *testing.T) string {
	t.Helper()
	repo := gitRepo(t)
	_ = os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755)
	return repo
}

// boundWorker: a worker in root (launcher orca, faked Orca) bound to name.
func boundWorker(t *testing.T, launcher, name string) string {
	t.Helper()
	root := repoWithLets(t, launcher)
	useOrca(t, &fakeOps{})
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", name)
	return root
}

func idxOf(p *Peer) int {
	if p == nil || p.RepoIndex == nil {
		return -1
	}
	return *p.RepoIndex
}

// Shape B: the orchestrator registered in its own repo; this repo has no role file.
func TestResolveOrchestrator_BoundSiblingShapeB(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, sib, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Source != "bound" || r.Target == nil || r.Target.Session != sidA || idxOf(r.Target) != 1 || r.Target.Send != "claude" {
		t.Fatalf("a bound sibling orchestrator must resolve with its repo_index: %+v", r)
	}
	if tr := tellFromIdx(t, root, sidW, sidA, r.Target.RepoIndex); tr.Reason != "claude_transport_model_send" {
		t.Errorf("tell with the returned index must reach it: %+v", tr)
	}
	if tr := tellFrom(t, root, sidW, sidA); tr.Reason != "peer_unreachable" {
		t.Errorf("without the index tell looks in this repo and must not reach it: %+v", tr)
	}
}

// Shape A: the role file is here, the live session in the sibling.
func TestResolveOrchestrator_BoundSiblingShapeA(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Target == nil || r.Target.Session != sidA || idxOf(r.Target) != 1 {
		t.Fatalf("shape A must resolve to the sibling session: %+v", r)
	}
}

// A local role registered as LIC whose session now runs as OTHER loses to a live LIC.
func TestResolveOrchestrator_SiblingRenamedLocalRole(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	plantRole(t, sib, sidB, "role: orchestrator\nname: LIC\npid: 102\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "OTHER", sib}, {102, sidB, "LIC", sib}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Target == nil || r.Target.Session != sidB {
		t.Fatalf("the live holder of the name must win, as in-repo pass 1: %+v", r)
	}
}

// Two siblings each hold a live LIC: ambiguous, whatever order Orca lists them in.
func TestResolveOrchestrator_SiblingAmbiguousAnyOrder(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	s1, s2 := siblingRepo(t), siblingRepo(t)
	plantRole(t, s1, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	plantRole(t, s2, sidB, "role: orchestrator\nname: LIC\npid: 102\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", s1}, {102, sidB, "LIC", s2}, {3, sidW, "W", root}})
	for _, order := range [][]string{{root, s1, s2}, {root, s2, s1}} {
		fakeRepos(t, order...)
		r := resolveIn(t, root, sidW)
		if r.Target != nil || r.Reason != "bound_ambiguous" || len(r.Refused) != 1 || r.Refused[0].Detail != "sibling_repos" {
			t.Errorf("order %v: several live LIC across siblings must be ambiguous: %+v", order, r)
		}
	}
}

// The same session's role file here AND in the sibling is one candidate.
func TestResolveOrchestrator_SiblingDuplicateRoleFiles(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	plantRole(t, sib, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Target == nil || r.Target.Session != sidA {
		t.Fatalf("one session counted twice must still resolve: %+v", r)
	}
}

// A sibling role file whose session lives in a repo Orca does not list is no candidate.
func TestResolveOrchestrator_SiblingRoleFileSessionElsewhere(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib, third := siblingRepo(t), siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, sib, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", third}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Target != nil || r.Reason != "orchestrator_not_registered" {
		t.Fatalf("a session outside every Orca repo must not be picked: %+v", r)
	}
}

// Without the Orca launcher the rule never applies: refused, and says why.
func TestResolveOrchestrator_SiblingNeedsOrca(t *testing.T) {
	root := boundWorker(t, "", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	r := resolveIn(t, root, sidW)
	if r.Target != nil || r.Reason != "target_in_other_repo" || len(r.Refused) != 1 || r.Refused[0].Detail != "orca_not_selected" || !r.Refused[0].Sibling {
		t.Fatalf("no Orca launcher: refused with orca_not_selected: %+v", r)
	}
}

// Orca does not list the repo the session lives in.
func TestResolveOrchestrator_SiblingNotInOrca(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Target != nil || len(r.Refused) != 1 || r.Refused[0].Detail != "repo_not_in_orca" {
		t.Fatalf("a repo Orca does not list: refused with repo_not_in_orca: %+v", r)
	}
}

// errAfter is a context whose Err turns Canceled on its n-th call - a deadline that
// expires at a chosen stage of the lookup, deterministically.
type errAfter struct {
	context.Context
	n, calls int
}

func (c *errAfter) Err() error {
	c.calls++
	if c.calls >= c.n {
		return context.Canceled
	}
	return nil
}

// A spent budget is never reported as a configuration verdict.
func TestResolveOrchestrator_SiblingBudget(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	plantRole(t, root, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	budgeted := func(r *Resolution) bool {
		for _, d := range r.Degraded {
			if d.Source == "context" && d.Reason == "deadline_exceeded" {
				return r.Reason == "budget_exhausted" && r.Target == nil && len(r.Refused) == 0
			}
		}
		return false
	}
	// (The verb no longer walks Orca repos: `lets peers orchestrator` resolves by name.)
	// Expiry after the repo list but during the shape-A owner lookup.
	fakeRepos(t, root)
	rc, lerr := loadRepo(context.Background(), root, false)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if r := resolveOrchestratorLegacy(&errAfter{Context: context.Background(), n: 2}, rc, ResolveOptions{Session: sidW, Cwd: root}); !budgeted(r) {
		t.Errorf("expiry at the shape-A lookup must be budget_exhausted, not repo_not_in_orca: %+v", r)
	}
	// A context already spent before the lookup starts.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := resolveOrchestratorLegacy(ctx, rc, ResolveOptions{Session: sidW, Cwd: root, Branch: "feature/x"}); !budgeted(r) {
		t.Errorf("a cancelled context: %+v", r)
	}
}

// A sibling role whose session was re-minted (/clear) resolves under the new id,
// and the file on disk is left exactly as it was.
func TestResolveOrchestrator_SiblingRotatedRole(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, sib, sidM, "role: orchestrator\nname: LIC\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "LIC", Cwd: sib, Started: setT.Add(-time.Hour)}, regAt{Pid: 3, Sid: sidW, Name: "W", Cwd: root})
	before, _ := os.ReadFile(filepath.Join(peersDir(sib), sidM+".role"))
	r := resolveIn(t, root, sidW)
	if r.Target == nil || r.Target.Session != sidN || idxOf(r.Target) != 1 {
		t.Fatalf("a rotated sibling role must resolve under its live id: %+v", r)
	}
	after, err := os.ReadFile(filepath.Join(peersDir(sib), sidM+".role"))
	if err != nil || string(after) != string(before) || fileExists(sib, sidN) {
		t.Errorf("the sibling role file must be untouched: err=%v newFile=%v", err, fileExists(sib, sidN))
	}
}

// An unbound worker never adopts another repo's orchestrator.
func TestResolveOrchestrator_UnboundNeverCrosses(t *testing.T) {
	root := repoWithLets(t, "orca")
	useOrca(t, &fakeOps{})
	withBranch(t, "feature/x")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, sib, sidA, "role: orchestrator\nname: LIC\npid: 101\nset: x\n")
	claudeHome(t, []regRow{{101, sidA, "LIC", sib}, {3, sidW, "W", root}})
	if r := resolveIn(t, root, sidW); r.Source != "none" || r.Target != nil {
		t.Fatalf("unbound resolution must stay in this repo: %+v", r)
	}
}

// treeState is every file under dir: mode, mtime and content.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		s := fi.Mode().String() + " " + fi.ModTime().String()
		if !fi.IsDir() {
			b, _ := os.ReadFile(p)
			s += " " + string(b)
		}
		out[p] = s
		return nil
	})
	return out
}

// Resolving across repos writes nothing into the sibling - with a stale role that
// reconcile moves in memory, through both shapes.
func TestResolveOrchestrator_SiblingReadOnly(t *testing.T) {
	root := boundWorker(t, "orca", "LIC")
	sib := siblingRepo(t)
	fakeRepos(t, root, sib)
	plantRole(t, sib, sidM, "role: orchestrator\nname: LIC\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "LIC", Cwd: sib, Started: setT.Add(-time.Hour)}, regAt{Pid: 3, Sid: sidW, Name: "W", Cwd: root})
	before := treeState(t, filepath.Join(sib, ".lets"))
	if r := resolveIn(t, root, sidW); r.Target == nil { // shape B
		t.Fatalf("shape B: %+v", r)
	}
	plantRole(t, root, sidN, "role: orchestrator\nname: LIC\npid: 101\n"+setLine)
	if r := resolveIn(t, root, sidW); r.Target == nil { // shape A
		t.Fatalf("shape A: %+v", r)
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

// ---- name resolution (ResolveOrchestrator, `lets peers orchestrator`) ----

// recordOrchestrators writes the merge-branch (main) task-state of root.
func recordOrchestrators(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".lets", "sessions", ".task-main"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// orcEnv runs the verb and returns its result and the emitted JSON envelope.
func orcEnv(t *testing.T, root, session string) (*OrchestratorResult, map[string]any) {
	t.Helper()
	res, err := Orchestrator(context.Background(), root, session)
	if err != nil || !res.OK {
		t.Fatalf("orchestrator: %+v %v", res, err)
	}
	b, _ := json.Marshal(res)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	return res, env
}

// skillStanding is the orc skill's condition for a standing grant: target.standing
// is the JSON true.
func skillStanding(env map[string]any) bool {
	tg, _ := env["target"].(map[string]any)
	v, present := tg["standing"]
	return present && v == true
}

// selfStanding is self_record.standing as the skill reads it; ok=false when
// self_record is null or absent.
func selfStanding(env map[string]any) (string, bool) {
	sr, present := env["self_record"]
	if !present {
		panic("self_record must always be in the envelope")
	}
	m, ok := sr.(map[string]any)
	if !ok {
		return "", false
	}
	st, _ := m["standing"].(string)
	return st, true
}

func TestResolveByName_Self(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS scope=docs standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}})
	res, env := orcEnv(t, root, sidM)
	if res.Source != "self" || res.Target == nil || res.Target.Session != sidM || !skillStanding(env) {
		t.Errorf("self: %+v", res)
	}
	if res.SelfRecord == nil || *res.SelfRecord != (SelfRecord{Name: "MAIN-LETS", Scope: "docs", Standing: "on"}) {
		t.Errorf("self_record: %+v", res.SelfRecord)
	}
}

// bound wins first, even for a session that is itself a recorded orchestrator;
// self_record still says it is one.
func TestResolveByName_BoundFirst(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "OTHER")
	recordOrchestrators(t, root, "orchestrator: MAIN-LETS\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, "OTHER", root}})
	res, env := orcEnv(t, root, sidM)
	if res.Source != "bound" || res.Target == nil || res.Target.Session != sidN {
		t.Errorf("bound first: %+v", res)
	}
	if st, ok := selfStanding(env); !ok || st != "" {
		t.Errorf("self_record keeps the unset standing: %q %v", st, ok)
	}
}

// target.standing is true only for a record's standing=on; self_record.standing
// keeps the string. Checked on the emitted envelope, as the skill reads it.
func TestResolveByName_Standing(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN-PWA")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {3, sidW, "W", root}})
	for line, want := range map[string]struct {
		target bool
		self   string
	}{
		"orchestrator: MAIN-PWA standing=on\n":  {true, "on"},
		"orchestrator: MAIN-PWA standing=off\n": {false, "off"},
		"orchestrator: MAIN-PWA\n":              {false, ""},
	} {
		recordOrchestrators(t, root, line)
		res, env := orcEnv(t, root, sidW)
		tg, _ := env["target"].(map[string]any)
		if _, present := tg["standing"]; res.Source != "bound" || skillStanding(env) != want.target || !present {
			t.Errorf("%q: worker sees target.standing=%v (present=%v), want %v", line, tg["standing"], present, want.target)
		}
		if st, ok := selfStanding(env); ok {
			t.Errorf("%q: the worker has no record of its own: %q", line, st)
		}
		_, env = orcEnv(t, root, sidM)
		if st, ok := selfStanding(env); !ok || st != want.self || skillStanding(env) != want.target {
			t.Errorf("%q: the orchestrator sees self_record.standing=%q (ok=%v), target.standing=%v", line, st, ok, skillStanding(env))
		}
	}
}

// A bound name whose only live session runs in another directory is the target -
// by name, with its cwd, no repo_index, and never with standing (cross-repo stays
// per-message).
func TestResolveByName_BoundElsewhere(t *testing.T) {
	root := repoWithLets(t, "")
	far := gitRepo(t)
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "FAR")
	recordOrchestrators(t, root, "orchestrator: FAR standing=on\n")
	claudeHome(t, []regRow{{1, sidM, "FAR", far}, {3, sidW, "W", root}})
	res, env := orcEnv(t, root, sidW)
	if res.Source != "bound" || res.Target == nil || res.Target.Session != sidM || res.Target.Cwd != far || res.Target.RepoIndex != nil || res.Reason != "" || len(res.Refused) != 0 {
		t.Fatalf("cross-repo bound target: %+v", res)
	}
	if skillStanding(env) {
		t.Errorf("a cross-repo target never carries standing: %+v", env["target"])
	}
}

func TestResolveByName_BoundRefusals(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "GONE")
	claudeHome(t, []regRow{{3, sidW, "W", root}})
	res, _ := orcEnv(t, root, sidW)
	if res.Source != "bound" || res.Target != nil || res.Reason != "target_not_alive" || len(res.Refused) != 1 || res.Refused[0].Name != "GONE" ||
		!strings.Contains(res.Remediation, "GONE") || !strings.Contains(res.Remediation, "/lets:start") {
		t.Errorf("bound dead: %+v", res)
	}
	bindBranch(t, root, "feature/x", "DUP")
	claudeHome(t, []regRow{{1, sidM, "DUP", root}, {2, sidN, "DUP", gitRepo(t)}, {3, sidW, "W", root}})
	res, _ = orcEnv(t, root, sidW)
	if res.Target != nil || res.Reason != "target_unsendable" || res.Refused[0].Detail != "name_not_unique" || !strings.Contains(res.Remediation, "/rename") {
		t.Errorf("bound duplicated: %+v", res)
	}
	bindBranch(t, root, "feature/x", "HIDDEN")
	claudeHome(t, []regRow{{3, sidW, "W", root}}, 77)
	res, _ = orcEnv(t, root, sidW)
	if res.Target != nil || res.Reason != "target_unsendable" || res.Refused[0].Detail != "liveness_unknown" {
		t.Errorf("an unreadable registry entry may carry the name: %+v", res)
	}
}

func TestResolveByName_Unbound(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "feature/x")
	conv := peername.Orchestrator(peername.RepoSlug(root), "")
	for _, c := range []struct {
		name    string
		records string
		rows    []regRow
		source  string
		target  string
	}{
		{"single recorded", "orchestrator: MAIN-LETS\n", []regRow{{1, sidM, "MAIN-LETS", root}}, "single", sidM},
		{"two scopes", "orchestrator: MAIN-PWA scope=pwa\norchestrator: MAIN-LIC scope=lic\n", []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", root}}, "ambiguous", ""},
		{"not recorded, no convention", "", []regRow{{1, sidM, "MAIN-LETS", root}}, "none", ""},
		{"recorded not live, convention live", "orchestrator: MAIN-GONE\n", []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, conv, root}}, "single", sidN},
		{"recorded live beats the convention", "orchestrator: MAIN-LETS\n", []regRow{{1, sidM, "MAIN-LETS", root}, {2, sidN, conv, root}}, "single", sidM},
	} {
		recordOrchestrators(t, root, c.records)
		claudeHome(t, append(c.rows, regRow{3, sidW, "W", root}))
		res, env := orcEnv(t, root, sidW)
		got := ""
		if res.Target != nil {
			got = res.Target.Session
		}
		if res.Source != c.source || got != c.target {
			t.Errorf("%s: source=%s target=%s, want %s %s (%+v)", c.name, res.Source, got, c.source, c.target, res)
		}
		if c.source == "ambiguous" && (len(res.Candidates) != 2 || res.Candidates[0].Scope == "" || res.Candidates[1].Scope == "") {
			t.Errorf("%s: candidates %+v", c.name, res.Candidates)
		}
		if sr, present := env["self_record"]; !present || sr != nil {
			t.Errorf("%s: self_record must be null for a worker: %v", c.name, sr)
		}
	}
}

// orc: on the merge-branch file binds nobody.
func TestResolveByName_MergeBranchOrcIgnored(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "main")
	recordOrchestrators(t, root, "orc: MAIN-PWA\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {3, sidW, "W", root}})
	if res, _ := orcEnv(t, root, sidW); res.Source == "bound" {
		t.Errorf("orc: on the merge-branch is ignored: %+v", res)
	}
}

// A spent budget never falls through to another orchestrator; a branch read before
// the budget still resolves its binding.
func TestResolveByName_Budget(t *testing.T) {
	root := repoWithLets(t, "")
	bindBranch(t, root, "feature/x", "MAIN-PWA")
	recordOrchestrators(t, root, "orchestrator: MAIN-PWA\norchestrator: MAIN-LIC\n")
	claudeHome(t, []regRow{{1, sidM, "MAIN-PWA", root}, {2, sidN, "MAIN-LIC", gitRepo(t)}, {3, sidW, "W", root}})
	v := loadNameView(context.Background(), root)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root, Branch: "feature/x"}); r.Source != "bound" || r.Target == nil || r.Target.Session != sidM {
		t.Errorf("a branch read outside the budget keeps its binding: %+v", r)
	}
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root, Branch: "feature/y"}); r.Source != "none" || r.Target != nil || r.Reason != "budget_exhausted" || r.Remediation == "" {
		t.Errorf("a spent budget on an unbound branch must not resolve single: %+v", r)
	}
	old := branchOf
	branchOf = func(context.Context, string) string { return "" }
	t.Cleanup(func() { branchOf = old })
	if r := ResolveOrchestrator(cancelled, v, ResolveOptions{Session: sidW, Cwd: root}); r.Reason != "branch_unreadable" || r.Target != nil {
		t.Errorf("an unreadable branch under a spent budget: %+v", r)
	}
}
