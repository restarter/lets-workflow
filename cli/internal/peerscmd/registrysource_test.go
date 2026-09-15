//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
)

// fakeRegistry writes <dir>/sessions/<pid>.json entries and points the registry
// seams at them (every listed pid is alive).
func fakeRegistry(t *testing.T, entries map[int]map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	alive := map[int]bool{}
	for pid, e := range entries {
		b, _ := json.Marshal(e)
		if err := os.WriteFile(filepath.Join(dir, "sessions", itoa(pid)+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		alive[pid] = true
	}
	oldHome, oldAlive := ccregistry.HomeDir, ccregistry.ProcAlive
	ccregistry.HomeDir = func() string { return dir }
	ccregistry.ProcAlive = func(pid int) bool { return alive[pid] }
	t.Cleanup(func() { ccregistry.HomeDir, ccregistry.ProcAlive = oldHome, oldAlive })
	return dir
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func gitRepo(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	repo := filepath.Join(dir, "repo")
	_ = os.Mkdir(repo, 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@e", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return repo
}

func TestRegistry_RepoFilter(t *testing.T) {
	repo := gitRepo(t)
	sub := filepath.Join(repo, "cli")
	_ = os.Mkdir(sub, 0o755)
	outside := t.TempDir()
	fakeRegistry(t, map[int]map[string]any{
		2001: {"sessionId": "11111111-1111-4111-8111-111111111111", "cwd": repo, "peerProtocol": 1, "name": "MAIN"},
		2002: {"sessionId": "22222222-2222-4222-8222-222222222222", "cwd": sub, "peerProtocol": 1, "name": "SUB"},
		2003: {"sessionId": "33333333-3333-4333-8333-333333333333", "cwd": outside, "peerProtocol": 1, "name": "ELSEWHERE"},
	})
	kept, rootOf, _, d := registryPeers(context.Background(), repo)
	if d != nil || len(kept) != 2 {
		t.Fatalf("kept %d (%+v), degraded %+v", len(kept), kept, d)
	}
	if rootOf["22222222-2222-4222-8222-222222222222"] != repo {
		t.Errorf("a session in a subdirectory must map to the worktree root: %q", rootOf["22222222-2222-4222-8222-222222222222"])
	}
}
