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

const sidW = "dddddddd-0000-4000-8000-00000000000d"

func TestResolveOrchestrator_Self(t *testing.T) {
	root := rolesRoot(t)
	registry(t, map[int][2]string{1: {sidM, "MAIN"}}, nil, 1)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "OTHER")
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidM}); r.Source != "self" || r.Target.Name != "MAIN" {
		t.Errorf("self: %+v", r)
	}
}

func TestResolveOrchestrator_Bound(t *testing.T) {
	root := rolesRoot(t)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN-PWA\npid: 1\nset: x\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\npid: 2\nset: x\n")
	withBranch(t, "feature/x")
	bindBranch(t, root, "feature/x", "MAIN-PWA")

	registry(t, map[int][2]string{1: {sidM, "MAIN-PWA"}, 2: {sidN, "MAIN-LIC"}, 3: {sidW, "W"}}, nil, 1, 2, 3)
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source != "bound" || r.Target.Session != sidM || r.Target.Alive != "alive" {
		t.Errorf("bound alive: %+v %+v", r, r.Target)
	}
	registry(t, map[int][2]string{2: {sidN, "MAIN-LIC"}, 3: {sidW, "W"}}, nil, 2, 3) // MAIN-PWA's pid died
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source != "bound" || r.Target.Session != sidM || r.Target.Alive != "dead" {
		t.Errorf("bound dead must be reported, never re-routed: %+v %+v", r, r.Target)
	}
	// the holder ran /rename MAIN-PWA2 and the branch is bound to the new name
	registry(t, map[int][2]string{1: {sidM, "MAIN-PWA2"}, 2: {sidN, "MAIN-LIC"}}, nil, 1, 2)
	bindBranch(t, root, "feature/x", "MAIN-PWA2")
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Target == nil || r.Target.Session != sidM {
		t.Errorf("bound by live name: %+v", r)
	}
}

func TestResolveOrchestrator_Unbound(t *testing.T) {
	root := rolesRoot(t)
	withBranch(t, "feature/x")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN-PWA\nscope: pwa\npid: 1\nset: x\n")
	registry(t, map[int][2]string{1: {sidM, "MAIN-PWA"}}, nil, 1)
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source != "single" || r.Target.Session != sidM {
		t.Errorf("single: %+v", r)
	}
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\nscope: lic\nset: x\n") // pid-less: unknown
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source != "ambiguous" || len(r.Candidates) != 2 {
		t.Errorf("one live plus one unknown must be ambiguous: %+v", r)
	}
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-LIC\nscope: lic\npid: 2\nset: x\n")
	registry(t, map[int][2]string{1: {sidM, "MAIN-PWA"}, 2: {sidN, "MAIN-LIC"}}, nil, 1, 2)
	r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW})
	if r.Source != "ambiguous" || len(r.Candidates) != 2 || r.Candidates[0].Scope == "" || r.Candidates[1].Scope == "" {
		t.Errorf("two live: %+v", r)
	}
	bindBranch(t, root, "feature/x", "$(x)")
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source == "bound" {
		t.Errorf("an invalid orc: line is unbound: %+v", r)
	}
	withBranch(t, "main")
	bindBranch(t, root, "main", "MAIN-PWA")
	if r := ResolveOrchestrator(context.Background(), root, ResolveOptions{Session: sidW}); r.Source == "bound" {
		t.Errorf("orc: on the merge-branch is ignored: %+v", r)
	}
	if r := ResolveOrchestrator(context.Background(), rolesRoot(t), ResolveOptions{Session: sidW}); r.Source != "none" {
		t.Errorf("none: %+v", r)
	}
}
