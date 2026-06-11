package gitutil

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// gitRun executes a git command in dir, failing the test on error.
//
// Identity-sensitive commands (commit) must pass hermetic `-c user.name=...`
// `-c user.email=...` flags: CI runners have no global git config, and this
// file sorts BEFORE projectroot_test.go, so its env-leaking gitInit
// (os.Setenv GIT_AUTHOR_*) has not run yet.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDefaultBranch_NoRemote_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "master")
	if got := DefaultBranch(dir, 2*time.Second); got != "" {
		t.Fatalf("want empty for repo without origin/HEAD, got %q", got)
	}
}

func TestDefaultBranch_OriginHead_StripsPrefix(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "master")
	// Simulate a cloned repo: create the symbolic ref by hand.
	gitRun(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-q", "-m", "x")
	gitRun(t, dir, "update-ref", "refs/remotes/origin/master", "HEAD")
	gitRun(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/master")
	if got := DefaultBranch(dir, 2*time.Second); got != "master" {
		t.Fatalf("want master (origin/ prefix stripped), got %q", got)
	}
}

func TestDefaultBranch_NotARepo_ReturnsEmpty(t *testing.T) {
	// Existing-but-empty dir: the interesting case - git walks UP from it.
	// Assumes t.TempDir() is outside any enclosing repo (true on CI and
	// typical dev machines).
	if got := DefaultBranch(t.TempDir(), 2*time.Second); got != "" {
		t.Fatalf("want empty for non-repo dir, got %q", got)
	}
	// Nonexistent dir: git -C fails outright.
	if got := DefaultBranch(filepath.Join(t.TempDir(), "missing"), 2*time.Second); got != "" {
		t.Fatalf("want empty for nonexistent dir, got %q", got)
	}
}
