//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

const (
	sidA = "11111111-1111-4111-8111-111111111111"
	sidB = "22222222-2222-4222-8222-222222222222"
	sidC = "33333333-3333-4333-8333-333333333333"
)

// startedRow is a registry row with its process start time (Snapshot.Rotated reads it).
type startedRow struct {
	pid     int
	sid     string
	started time.Time
}

// registryWithStart points the registry seams at rows that carry startedAt.
func registryWithStart(t *testing.T, cwd string, rows ...startedRow) {
	t.Helper()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	alive := map[int]bool{}
	for _, r := range rows {
		b, _ := json.Marshal(map[string]any{"sessionId": r.sid, "name": "S", "cwd": cwd, "peerProtocol": 1, "status": "idle", "startedAt": r.started.UnixMilli()})
		_ = os.WriteFile(filepath.Join(dir, "sessions", itoa(r.pid)+".json"), b, 0o600)
		alive[r.pid] = true
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return alive[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
}

func hasProof(root, sid string) bool {
	_, _, ok := RoleProof(root, sid)
	return ok
}

func TestProof_WriteAndRead(t *testing.T) {
	root := t.TempDir()
	set := time.Now().UTC().Truncate(time.Second)
	if err := writeProof(root, sidA, 42, set); err != nil {
		t.Fatal(err)
	}
	pid, got, ok := RoleProof(root, sidA)
	if !ok || pid != 42 || !got.Equal(set) {
		t.Errorf("RoleProof = %d %v %v", pid, got, ok)
	}
	if fi, _ := os.Stat(peersDir(root)); fi == nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("peers dir must be 0700: %v", fi)
	}
	if _, _, ok := RoleProof(root, "../x"); ok {
		t.Error("an invalid session id has no proof")
	}
}

// A proof moves only on a same-process re-mint, never across processes, and is
// never created from nothing.
func TestProof_CarryOnlySameProcess(t *testing.T) {
	root := t.TempDir()
	start := time.Now().Add(-time.Hour)
	set := time.Now().Add(-time.Minute)
	if err := writeProof(root, sidA, 100, set); err != nil {
		t.Fatal(err)
	}
	// another process holds the new id: nothing moves
	registryWithStart(t, root, startedRow{200, sidB, start})
	CarryProof(context.Background(), root)
	if !hasProof(root, sidA) || hasProof(root, sidB) {
		t.Fatal("a new id in another process must not take the proof")
	}
	// the same process (pid 100, started before set) now runs sidB: /clear
	registryWithStart(t, root, startedRow{100, sidB, start})
	CarryProof(context.Background(), root)
	if hasProof(root, sidA) || !hasProof(root, sidB) {
		t.Fatal("a same-process re-mint carries the proof to the new id")
	}
	// a live session without a proof never gets one
	registryWithStart(t, root, startedRow{100, sidB, start}, startedRow{300, sidC, start})
	CarryProof(context.Background(), root)
	if hasProof(root, sidC) {
		t.Error("CarryProof never creates a proof from nothing")
	}
	// no peers dir: nothing is created at all
	bare := t.TempDir()
	CarryProof(context.Background(), bare)
	if _, err := os.Stat(filepath.Join(bare, ".lets")); !os.IsNotExist(err) {
		t.Error("CarryProof must not create .lets")
	}
}

// `claude -r`: the same session under a new pid refreshes pid and set, so a later
// /clear in THAT process is provable.
func TestProof_ResumeThenClear(t *testing.T) {
	root := t.TempDir()
	if err := writeProof(root, sidA, 100, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	resumed := time.Now().Add(-time.Minute)
	registryWithStart(t, root, startedRow{300, sidA, resumed}) // pid 100 is gone
	CarryProof(context.Background(), root)
	pid, set, ok := RoleProof(root, sidA)
	if !ok || pid != 300 || set.Before(resumed) {
		t.Fatalf("resume refresh: pid=%d set=%v ok=%v", pid, set, ok)
	}
	registryWithStart(t, root, startedRow{300, sidB, resumed}) // /clear in the resumed process
	CarryProof(context.Background(), root)
	if !hasProof(root, sidB) || hasProof(root, sidA) {
		t.Error("the /clear after a resume is provable and carried")
	}
}

// Two /clears with the first carry losing the lock: the lazy carry in the next `who`
// recovers it before the second /clear, which is then provable too.
func TestProof_LazyCarryRecoversAMissedOne(t *testing.T) {
	old := proofLockWait
	proofLockWait = 100 * time.Millisecond
	t.Cleanup(func() { proofLockWait = old })
	repo := repoWithLets(t, "")
	start := time.Now().Add(-time.Hour)
	if err := writeProof(repo, sidA, 100, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	registryWithStart(t, repo, startedRow{100, sidB, start}) // first /clear
	unlock, err := lockPeers(repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	CarryProof(context.Background(), repo) // the hook's carry: lock busy, silent
	unlock()
	if !hasProof(repo, sidA) || hasProof(repo, sidB) {
		t.Fatal("a busy lock must leave the proof where it was")
	}
	if _, err := Who(context.Background(), WhoOptions{Cwd: repo}); err != nil {
		t.Fatal(err)
	}
	if !hasProof(repo, sidB) {
		t.Fatal("the lazy carry in who must recover the missed carry")
	}
	registryWithStart(t, repo, startedRow{100, sidC, start}) // second /clear
	pid, set, ok := RoleProof(repo, sidB)
	if !ok {
		t.Fatal("the middle id must be provable before the second carry")
	}
	if snapTo, rotated := ccregistry.Read(ccregistry.HomeDir()).Rotated(sidB, pid, set); !rotated || snapTo != sidC {
		t.Errorf("the guard proves the second /clear: %q %v", snapTo, rotated)
	}
	CarryProof(context.Background(), repo)
	if !hasProof(repo, sidC) {
		t.Error("the second carry moves it on")
	}
}

// An older binary's role file (role:, task:, name: ... lines) is a proof too.
func TestProof_ReadsOldRoleFiles(t *testing.T) {
	root := t.TempDir()
	set := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	_ = os.MkdirAll(peersDir(root), 0o700)
	_ = os.WriteFile(filepath.Join(peersDir(root), sidA+".role"), []byte("role: worker\ntask: lets-abc\nname: W\npid: 100\nset: "+set+"\n"), 0o600)
	registryWithStart(t, root, startedRow{100, sidB, time.Now().Add(-time.Hour)})
	CarryProof(context.Background(), root)
	if !hasProof(root, sidB) || hasProof(root, sidA) {
		t.Error("an old role file is carried like a proof")
	}
}
