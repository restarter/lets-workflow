//go:build unix

package peerscmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

const (
	sidM = "aaaaaaaa-0000-4000-8000-000000000001"
	sidN = "aaaaaaaa-0000-4000-8000-000000000002"
	sidO = "aaaaaaaa-0000-4000-8000-000000000003"
)

// registry writes live registry entries (pid -> {sid, name}) plus unknown-protocol
// entries, and marks exactly alive as running.
func registry(t *testing.T, entries map[int][2]string, unknownProto []int, alive ...int) {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	for pid, e := range entries {
		b, _ := json.Marshal(map[string]any{"sessionId": e[0], "name": e[1], "cwd": "/repo", "peerProtocol": 1})
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(pid)+".json"), b, 0o600)
	}
	for _, pid := range unknownProto {
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(pid)+".json"), []byte(`{"peerProtocol":2}`), 0o600)
	}
	set := map[int]bool{}
	for _, p := range alive {
		set[p] = true
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return set[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
}

func rolesRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(orcaTerminalVar, "")
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".lets", "sessions"), 0o755)
	return root
}

func plantRole(t *testing.T, root, sid, content string) {
	t.Helper()
	_ = os.MkdirAll(peersDir(root), 0o700)
	if err := os.WriteFile(filepath.Join(peersDir(root), sid+".role"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func roleOf(t *testing.T, root, sid string) string {
	files, _ := loadRoles(root)
	return files[sid].Role
}

func TestRole_TwoOrchestratorsUniqueNames(t *testing.T) {
	root := rolesRoot(t)
	registry(t, map[int][2]string{101: {sidM, "MAIN-PWA"}, 102: {sidN, "MAIN-LIC"}, 103: {sidO, "MAIN-PWA"}}, nil, 101, 102, 103)
	for _, sid := range []string{sidM, sidN} {
		if info, err := SetRole(root, RoleOptions{Session: sid, Role: "orchestrator", Scope: "part\n\x1bsecond line"}); err != nil || !info.Granted {
			t.Fatalf("%s: %+v %v", sid, info, err)
		}
	}
	if files, _ := loadRoles(root); files[sidM].Scope != "part" {
		t.Errorf("scope not cleaned: %q", files[sidM].Scope)
	}
	info, _ := SetRole(root, RoleOptions{Session: sidO, Role: "orchestrator"})
	if info.Granted || info.Reason != "name_held" || info.Holder == nil || info.Holder.Session6 != sidM[:6] || info.Holder.Alive != "alive" {
		t.Fatalf("same name: %+v", info)
	}
	info, _ = SetRole(root, RoleOptions{Session: sidO, Role: "orchestrator", Takeover: true})
	if !info.Granted || info.Demoted != sidM[:6] || roleOf(t, root, sidM) != "peer" || roleOf(t, root, sidN) != "orchestrator" {
		t.Errorf("takeover must demote only MAIN-PWA's holder: %+v m=%s n=%s", info, roleOf(t, root, sidM), roleOf(t, root, sidN))
	}
}

func TestRole_NeedsNameAndRegistry(t *testing.T) {
	root := rolesRoot(t)
	registry(t, map[int][2]string{101: {sidM, ""}}, nil, 101)
	if info, _ := SetRole(root, RoleOptions{Session: sidM, Role: "orchestrator"}); info.Reason != "orchestrator_needs_name" || info.Granted {
		t.Errorf("no name: %+v", info)
	}
	if info, _ := SetRole(root, RoleOptions{Session: sidN, Role: "worker", Task: "lets-abc"}); info.Reason != "session_not_in_registry" || info.Granted {
		t.Errorf("unregistered worker: %+v", info)
	}
	if _, err := os.Stat(filepath.Join(peersDir(root), sidN+".role")); !os.IsNotExist(err) {
		t.Error("nothing may be written for an unregistered session")
	}
	if _, err := SetRole(root, RoleOptions{Session: sidM, Role: "worker", Task: "-rf"}); err == nil {
		t.Error("a worker needs a valid task")
	}
}

func TestRole_PruneAndLiveness(t *testing.T) {
	root := rolesRoot(t)
	// 201 dead holder, 202 holder whose pid is an unknown-protocol live entry,
	// 203 holder whose pid now carries another session (reuse)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\npid: 201\nset: 2026-09-15T10:00:00Z\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN-B\npid: 202\nset: 2026-09-15T10:00:00Z\n")
	plantRole(t, root, sidO, "role: orchestrator\nname: MAIN-C\npid: 203\nset: 2026-09-15T10:00:00Z\n")
	const newSid = "bbbbbbbb-0000-4000-8000-000000000009"
	const reuser = "cccccccc-0000-4000-8000-00000000000c"
	registry(t, map[int][2]string{301: {newSid, "MAIN-B"}, 203: {reuser, "OTHER"}}, []int{202}, 301, 202, 203)
	info, _ := SetRole(root, RoleOptions{Session: newSid, Role: "orchestrator"})
	if info.Pruned != 2 {
		t.Errorf("dead and reused holders must be pruned: %+v", info)
	}
	if info.Granted || info.Reason != "name_held" || info.Holder.Alive != "unknown" {
		t.Errorf("a holder on an unrecognized live pid is kept as unknown: %+v", info)
	}
}

func TestRole_PidlessAndRename(t *testing.T) {
	root := rolesRoot(t)
	old := now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	young := now().Add(-time.Hour).UTC().Format(time.RFC3339)
	plantRole(t, root, sidM, "role: orchestrator\nname: MAIN\nset: "+old+"\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN\nset: "+young+"\n")
	registry(t, map[int][2]string{401: {sidO, "MAIN"}}, nil, 401)
	info, _ := SetRole(root, RoleOptions{Session: sidO, Role: "orchestrator"})
	if info.Pruned != 1 || info.Reason != "name_held" || info.Holder.Session6 != sidN[:6] || info.Holder.Alive != "unknown" {
		t.Errorf("legacy pid-less: old pruned, young kept and never treated as dead: %+v", info)
	}
	// the young holder ran /rename: its live name no longer blocks MAIN
	registry(t, map[int][2]string{401: {sidO, "MAIN"}, 402: {sidN, "RENAMED"}}, nil, 401, 402)
	plantRole(t, root, sidN, "role: orchestrator\nname: MAIN\npid: 402\nset: "+young+"\n")
	if info, _ := SetRole(root, RoleOptions{Session: sidO, Role: "orchestrator"}); !info.Granted {
		t.Errorf("a renamed holder must not block its old name: %+v", info)
	}
}

func TestRole_ConcurrentSetRole(t *testing.T) {
	root := rolesRoot(t)
	registry(t, map[int][2]string{501: {sidM, "MAIN"}, 502: {sidN, "MAIN"}}, nil, 501, 502)
	var wg sync.WaitGroup
	granted := make(chan string, 2)
	for _, sid := range []string{sidM, sidN} {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			if info, err := SetRole(root, RoleOptions{Session: sid, Role: "orchestrator"}); err == nil && info.Granted {
				granted <- sid
			}
		}(sid)
	}
	wg.Wait()
	close(granted)
	n := 0
	for range granted {
		n++
	}
	if n != 1 {
		t.Errorf("two sessions with one name: %d granted, want 1", n)
	}
}

func TestRole_WorkerRecordsTerminal(t *testing.T) {
	root := rolesRoot(t)
	t.Setenv(orcaTerminalVar, "term_abc")
	registry(t, map[int][2]string{601: {sidM, "W1"}}, nil, 601)
	if info, _ := SetRole(root, RoleOptions{Session: sidM, Role: "worker", Task: "lets-abc", Cwd: "/wt"}); !info.Granted {
		t.Fatalf("worker: %+v", info)
	}
	data, _ := os.ReadFile(filepath.Join(peersDir(root), sidM+".role"))
	for _, want := range []string{"role: worker", "task: lets-abc", "orca_terminal: term_abc", "pid: 601", "cwd: /wt"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("role file lacks %q:\n%s", want, data)
		}
	}
	if fi, _ := os.Stat(filepath.Join(peersDir(root), sidM+".role")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestRole_LastSeenOnPrune(t *testing.T) {
	root := rolesRoot(t)
	plantRole(t, root, sidM, "role: orchestrator\nname: A B\nscope: pwa\npid: 801\ncwd: /repo\nset: x\n")
	plantRole(t, root, sidN, "role: orchestrator\nname: A_B\npid: 802\nset: x\n")
	plantRole(t, root, sidO, "role: worker\ntask: lets-abc\nname: W\npid: 803\nset: x\n")
	const self = "bbbbbbbb-0000-4000-8000-00000000000b"
	registry(t, map[int][2]string{900: {self, "NEW"}}, nil, 900) // 801-803 are dead
	if info, _ := SetRole(root, RoleOptions{Session: self, Role: "peer"}); info.Pruned != 3 {
		t.Fatalf("prune: %+v", info)
	}
	files, _ := filepath.Glob(filepath.Join(peersDir(root), "last", "*.last"))
	if len(files) != 2 {
		t.Fatalf("two orchestrator names, two last-seen files (workers get none): %v", files)
	}
	a, _ := os.ReadFile(lastSeenFile(peersDir(root), "A B"))
	if !strings.Contains(string(a), "session: "+sidM) || !strings.Contains(string(a), "pid: 801") || !strings.Contains(string(a), "scope: pwa") {
		t.Errorf("A B last-seen:\n%s", a)
	}
	if lastSeenFile(peersDir(root), "A B") == lastSeenFile(peersDir(root), "A_B") {
		t.Error("A B and A_B must not share a file")
	}
	// the same name pruned again overwrites only its own file
	plantRole(t, root, sidN, "role: orchestrator\nname: A_B\npid: 804\nset: y\n")
	_, _ = SetRole(root, RoleOptions{Session: self, Role: "peer"})
	ab, _ := os.ReadFile(lastSeenFile(peersDir(root), "A_B"))
	a2, _ := os.ReadFile(lastSeenFile(peersDir(root), "A B"))
	if !strings.Contains(string(ab), "pid: 804") || string(a2) != string(a) {
		t.Errorf("re-prune: A_B=%q A B changed=%v", ab, string(a2) != string(a))
	}
}
