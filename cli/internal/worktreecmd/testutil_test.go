//go:build unix

package worktreecmd_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// realTempDir wraps t.TempDir() with filepath.EvalSymlinks because macOS
// returns /var/folders/... which is a symlink to /private/var/folders/...
// git resolves these inconsistently. On Linux this is a no-op.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return dir
}

// initRepo creates a temp git repo with an initial empty commit on main.
func initRepo(t *testing.T) string {
	t.Helper()
	repo := realTempDir(t)
	runIn(t, repo, "git", "-c", "init.defaultBranch=main", "init")
	runIn(t, repo, "git", "config", "user.email", "test@example.com")
	runIn(t, repo, "git", "config", "user.name", "test")
	runIn(t, repo, "git", "commit", "--allow-empty", "-m", "initial")
	return repo
}

// runIn runs the given command in dir; calls t.Fatal on failure.
func runIn(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
