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
	if r := resolveIn(t, root, sidW); r.Source != "single" || r.Target.Session != sidM || len(r.Refused) != 1 || r.Refused[0].Reason != "target_unsendable" {
		t.Errorf("an unaddressable unknown-liveness candidate is refused by name, not counted toward ambiguity: %+v", r)
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
