//go:build unix

package peerscmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// regAt is a registry row with the fields reconcile and heal read.
type regAt struct {
	Pid       int
	Sid, Name string
	Cwd       string
	Source    string    // nameSource; "" = derived
	Started   time.Time // zero = startedAt absent
}

// registryAt writes live registry rows (every pid alive) and points the seams at them.
func registryAt(t *testing.T, rows ...regAt) {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	alive := map[int]bool{}
	for _, r := range rows {
		m := map[string]any{"sessionId": r.Sid, "name": r.Name, "cwd": r.Cwd, "peerProtocol": 1, "status": "idle", "nameSource": r.Source}
		if !r.Started.IsZero() {
			m["startedAt"] = r.Started.UnixMilli()
		}
		b, _ := json.Marshal(m)
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(r.Pid)+".json"), b, 0o600)
		alive[r.Pid] = true
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return alive[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
}

var (
	setT    = time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	setLine = "set: 2026-09-23T10:00:00Z\n"
)

func fileExists(root, sid string) bool {
	_, err := os.Stat(filepath.Join(peersDir(root), sid+".role"))
	return err == nil
}

// /clear: same pid, new id - the orchestrator's file follows it, and a third
// session's call is enough to move it.
func TestReconcile_ClearCarriesRoleToNewID(t *testing.T) {
	root := rolesRoot(t)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\nscope: api\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "MAIN", Source: "user", Started: setT.Add(-time.Hour)},
		regAt{Pid: 102, Sid: sidO, Name: "W"})
	if _, err := SetRole(root, RoleOptions{Session: sidO, Role: "peer"}); err != nil {
		t.Fatal(err)
	}
	files, _ := loadRoles(root)
	if fileExists(root, sidM) || files[sidN].Role != "orchestrator" || files[sidN].Scope != "api" || files[sidN].Pid != 101 {
		t.Errorf("role not carried to the new id: %+v", files)
	}
	if _, err := os.Stat(lastDir(peersDir(root))); err == nil {
		t.Error("a moved orchestrator must not get a last-seen record")
	}
}

// A reused pid started after the role was written: pruned as before, not taken over.
func TestReconcile_ReusedPidStillPruned(t *testing.T) {
	root := rolesRoot(t)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "OTHER", Started: setT.Add(time.Hour)}, regAt{Pid: 102, Sid: sidO, Name: "W"})
	info, _ := SetRole(root, RoleOptions{Session: sidO, Role: "peer"})
	if info.Pruned != 1 || fileExists(root, sidM) || fileExists(root, sidN) {
		t.Errorf("a reused pid must prune, not move: %+v", info)
	}
}

// The new id already registered itself (/lets:start --main after /clear): the old file goes.
func TestReconcile_NewIDAlreadyHasFile(t *testing.T) {
	root := rolesRoot(t)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 101\n"+setLine)
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN\nscope: new\npid: 101\nset: 2026-09-23T11:00:00Z\n")
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "MAIN", Started: setT.Add(-time.Hour)})
	files, _ := loadRoles(root)
	moves := reconcileRoles(files, ccregistry.Read(ccregistry.HomeDir()))
	applyMoves(files, moves)
	if err := persistMoves(root, moves); err != nil {
		t.Fatal(err)
	}
	after, _ := loadRoles(root)
	if fileExists(root, sidM) || after[sidN].Scope != "new" {
		t.Errorf("the newer file must win: %+v", after)
	}
}

// `claude -r`: same id under a new pid - pid and set refreshed, so a later /clear
// in the new process still carries the file.
func TestReconcile_ResumeRefreshesPid(t *testing.T) {
	root := rolesRoot(t)
	old := now
	now = func() time.Time { return setT.Add(2 * time.Hour) }
	t.Cleanup(func() { now = old })
	plantRole(t, root, sidM, "role: worker\ntask: lets-abc\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 205, Sid: sidM, Name: "W", Started: setT.Add(time.Hour)}, regAt{Pid: 300, Sid: sidN, Name: "P"})
	if _, err := SetRole(root, RoleOptions{Session: sidN, Role: "peer"}); err != nil {
		t.Fatal(err)
	}
	files, _ := loadRoles(root)
	if files[sidM].Pid != 205 || files[sidM].Set != "2026-09-23T12:00:00Z" {
		t.Errorf("pid/set not refreshed: %+v", files[sidM])
	}
}

// A load corrects the view in memory and never writes by itself.
func TestReconcile_LoadIsInMemory(t *testing.T) {
	root := repoWithLets(t, "")
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 101\n"+setLine)
	registryAt(t, regAt{Pid: 101, Sid: sidN, Name: "MAIN", Cwd: root, Started: setT.Add(-time.Hour)})
	rc, err := loadRepo(t.Context(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if rc.roles[sidN].Role != "orchestrator" || !fileExists(root, sidM) {
		t.Errorf("memory must be corrected, disk untouched: %+v", rc.roles)
	}
}
