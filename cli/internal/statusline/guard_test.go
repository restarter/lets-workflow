package statusline

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// spies replaces the call-site seams so no Keychain, HTTPS or bd call is reached,
// and counts what the statusline tried to do.
type spies struct {
	usageFetch, taskFetch, placeholder, usageSpawn, taskSpawn int
}

func installSpies(t *testing.T) *spies {
	t.Helper()
	s := &spies{}
	oldUF, oldTF, oldWP, oldSU, oldST := usageFetcher, taskFetcher, writePlaceholder, spawnUsage, spawnTask
	usageFetcher = func(string) error { s.usageFetch++; return nil }
	taskFetcher = func(string, string) error { s.taskFetch++; return nil }
	writePlaceholder = func(string, string) error { s.placeholder++; return nil }
	spawnUsage = func(string) { s.usageSpawn++ }
	spawnTask = func(string, string) { s.taskSpawn++ }
	t.Cleanup(func() {
		usageFetcher, taskFetcher, writePlaceholder, spawnUsage, spawnTask = oldUF, oldTF, oldWP, oldSU, oldST
	})
	return s
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// newRepo makes a main checkout on mainBranch with one commit.
func newRepo(t *testing.T, mainBranch string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "init", "-q", "-b", mainBranch)
	gitRun(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func renderAt(t *testing.T, dir string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"workspace": map[string]string{"current_dir": dir}})
	var out bytes.Buffer
	if err := Render(bytes.NewReader(payload), &out, false, false, false, false, true); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

// TestStatuslineGuard_WorktreeWithoutLets: in a linked worktree whose .lets is not
// bootstrapped yet (Orca just made it), the statusline neither creates .lets nor
// starts a fetch; in a main checkout the same branch shape still fetches.
func TestStatuslineGuard_WorktreeWithoutLets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := newRepo(t, "feature/lets-abc-y") // the main checkout is off the merge-branch too
	wt := filepath.Join(filepath.Dir(repo), "wt")
	gitRun(t, repo, "worktree", "add", "-q", "-b", "feature/lets-abc-x", wt)

	t.Run("linked worktree, no .lets", func(t *testing.T) {
		s := installSpies(t)
		renderAt(t, wt)
		if err := RunFetchOnly(filepath.Join(wt, ".lets", "cache")); err != nil {
			t.Fatalf("RunFetchOnly: %v", err)
		}
		if err := RunFetchTaskOnly(filepath.Join(wt, ".lets", "cache"), "lets-abc"); err != nil {
			t.Fatalf("RunFetchTaskOnly: %v", err)
		}
		if *s != (spies{}) {
			t.Errorf("worktree without .lets must reach no fetch or cache write, got %+v", *s)
		}
		if _, err := os.Lstat(filepath.Join(wt, ".lets")); !os.IsNotExist(err) {
			t.Errorf("statusline created %s/.lets (err=%v)", wt, err)
		}
	})

	t.Run("main checkout", func(t *testing.T) {
		s := installSpies(t)
		renderAt(t, repo)
		if err := RunFetchOnly(filepath.Join(repo, ".lets", "cache")); err != nil {
			t.Fatalf("RunFetchOnly: %v", err)
		}
		if err := RunFetchTaskOnly(filepath.Join(repo, ".lets", "cache"), "lets-abc"); err != nil {
			t.Fatalf("RunFetchTaskOnly: %v", err)
		}
		want := spies{usageFetch: 1, taskFetch: 1, placeholder: 1, usageSpawn: 1, taskSpawn: 1}
		if *s != want {
			t.Errorf("main checkout spies = %+v, want %+v", *s, want)
		}
	})

	t.Run("linked worktree after bootstrap", func(t *testing.T) {
		if err := os.Symlink(filepath.Join(repo, ".lets"), filepath.Join(wt, ".lets")); err != nil {
			t.Fatal(err)
		}
		s := installSpies(t)
		renderAt(t, wt)
		if s.usageSpawn != 1 || s.taskSpawn != 1 {
			t.Errorf("bootstrapped worktree must refresh as before, got %+v", *s)
		}
	})
}
