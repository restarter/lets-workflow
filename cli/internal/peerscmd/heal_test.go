//go:build unix

package peerscmd

import (
	"os"
	"testing"
	"time"
)

func healIn(t *testing.T, root, sid, branch string) []Degraded {
	t.Helper()
	rc, err := loadRepo(t.Context(), root, false)
	if err != nil {
		t.Fatalf("loadRepo: %v", err)
	}
	return HealSelf(rc, sid, root, branch)
}

// A new agent (new pid, new id) in the worktree of a claimed task is its worker again.
func TestHeal_WorkerFromTaskState(t *testing.T) {
	root := repoWithLets(t, "")
	bindBranch(t, root, "feature/x", "MAIN") // task: lets-abc
	registryAt(t, regAt{Pid: 7, Sid: sidW, Name: "W", Cwd: root})
	healIn(t, root, sidW, "feature/x")
	files, _ := loadRoles(root)
	if files[sidW].Role != "worker" || files[sidW].Task != "lets-abc" || files[sidW].Pid != 7 {
		t.Errorf("worker not restored: %+v", files[sidW])
	}
}

// A new orchestrator session renamed to a dead holder's name takes its role and scope.
func TestHeal_OrchestratorByUserName(t *testing.T) {
	root := repoWithLets(t, "")
	withBranch(t, "main")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\nscope: api\npid: 1\n"+setLine) // pid 1 dead
	registryAt(t, regAt{Pid: 9, Sid: sidN, Name: "MAIN", Cwd: root, Source: "user", Started: setT.Add(time.Hour)})
	healIn(t, root, sidN, "main")
	files, _ := loadRoles(root)
	if files[sidN].Role != "orchestrator" || files[sidN].Scope != "api" || fileExists(root, sidM) {
		t.Errorf("orchestrator not reclaimed: %+v", files)
	}
	if r := resolveIn(t, root, sidN); r.Source != "self" {
		t.Errorf("the reclaimer must resolve as self: %+v", r)
	}
}

// A derived name never reclaims: derived names collide by chance.
func TestHeal_DerivedNameDoesNotReclaim(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: spike-22\npid: 1\n"+setLine)
	registryAt(t, regAt{Pid: 9, Sid: sidN, Name: "spike-22", Cwd: root})
	healIn(t, root, sidN, "main")
	if fileExists(root, sidN) {
		t.Error("a derived name must not reclaim a role")
	}
}

// A live holder of the name blocks reclaim: two live sessions never share a name silently.
func TestHeal_LiveHolderBlocks(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\n"+setLine)
	registryAt(t, regAt{Pid: 1, Sid: sidM, Name: "MAIN", Cwd: root, Source: "user"},
		regAt{Pid: 9, Sid: sidN, Name: "MAIN", Cwd: root, Source: "user"})
	healIn(t, root, sidN, "main")
	if fileExists(root, sidN) {
		t.Error("a live holder must block reclaim")
	}
	if info, _ := SetRole(root, RoleOptions{Session: sidN, Role: "orchestrator"}); info.Reason != "name_held" {
		t.Errorf("name_held must still apply: %+v", info)
	}
}

// Two dead holders under one name: refuse with a named reason, write nothing.
func TestHeal_AmbiguousRefuses(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\n"+setLine)
	plantRole(t, root, sidO, "role: orchestrator\nname: MAIN\npid: 2\n"+setLine)
	registryAt(t, regAt{Pid: 9, Sid: sidN, Name: "MAIN", Cwd: root, Source: "user"})
	d := healIn(t, root, sidN, "main")
	if fileExists(root, sidN) || len(d) == 0 || d[0].Reason != "reclaim_ambiguous" {
		t.Errorf("ambiguous reclaim must refuse: %+v", d)
	}
}

// After a prune, the last-seen record is the anchor; it is consumed.
func TestHeal_OrchestratorFromLastSeen(t *testing.T) {
	root := repoWithLets(t, "")
	_ = os.MkdirAll(peersDir(root), 0o700)
	f := roleFile{Session: sidM, Role: "orchestrator", Name: "MAIN", Scope: "api", Pid: 1, Set: "2026-09-23T10:00:00Z"}
	if err := writeLastSeen(peersDir(root), f); err != nil {
		t.Fatal(err)
	}
	registryAt(t, regAt{Pid: 9, Sid: sidN, Name: "MAIN", Cwd: root, Source: "user"})
	healIn(t, root, sidN, "main")
	files, _ := loadRoles(root)
	if files[sidN].Role != "orchestrator" || files[sidN].Scope != "api" {
		t.Errorf("scope from last-seen: %+v", files[sidN])
	}
	if _, err := os.Stat(lastSeenFile(peersDir(root), "MAIN")); err == nil {
		t.Error("the consumed last-seen record must go")
	}
}

// The merge-branch never makes a worker, even when its .task file names a task.
func TestHeal_NoWorkerOnMergeBranch(t *testing.T) {
	root := repoWithLets(t, "")
	bindBranch(t, root, "main", "MAIN")
	registryAt(t, regAt{Pid: 7, Sid: sidW, Name: "W", Cwd: root})
	healIn(t, root, sidW, "main")
	if fileExists(root, sidW) {
		t.Error("no worker role on the merge-branch")
	}
}

// Two sessions under one user-set name both plan a restore from the same dead
// holder before either writes: the second decides again under the lock and sees
// the first as a live holder - one name never gets two live holders.
func TestHeal_ConcurrentReclaimOneWinner(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 1\n"+setLine)
	registryAt(t, regAt{Pid: 9, Sid: sidN, Name: "MAIN", Cwd: root, Source: "user"},
		regAt{Pid: 10, Sid: sidO, Name: "MAIN", Cwd: root, Source: "user"})
	rcA, _ := loadRepo(t.Context(), root, false)
	rcB, _ := loadRepo(t.Context(), root, false) // both views: MAIN held only by a dead file
	HealSelf(rcA, sidN, root, "main")
	HealSelf(rcB, sidO, root, "main")
	files, _ := loadRoles(root)
	holders := 0
	for _, f := range files {
		if f.Role == "orchestrator" {
			holders++
		}
	}
	if holders != 1 || files[sidN].Role != "orchestrator" || fileExists(root, sidO) {
		t.Errorf("exactly the first healer may hold MAIN: %+v", files)
	}
}

// A tie movesTo refused to break (two old roles of this process, set in the same
// second) must not be broken by a restore either: nothing is written, and the caller
// is told how to register the role it means - in both lexical orders of the old ids.
func TestHeal_TiedCollisionRestoresNothing(t *testing.T) {
	for _, order := range [][2]string{{sidM, sidN}, {sidN, sidM}} {
		root := repoWithLets(t, "")
		plantRole(t, root, order[0], "role: worker\ntask: lets-abc\npid: 101\n"+setLine)
		plantRole(t, root, order[1], "role: orchestrator\nname: MAIN\npid: 101\n"+setLine)
		registryAt(t, regAt{Pid: 101, Sid: sidO, Name: "MAIN", Cwd: root, Source: "user", Started: setT.Add(-time.Hour)})
		d := healIn(t, root, sidO, "main")
		if fileExists(root, sidO) || len(d) == 0 || d[0].Reason != "reclaim_ambiguous" || d[0].Detail == "" {
			t.Errorf("worker=%s orchestrator=%s: a tie must restore nothing and say why: %+v", order[0][len(order[0])-1:], order[1][len(order[1])-1:], d)
		}
	}
}
