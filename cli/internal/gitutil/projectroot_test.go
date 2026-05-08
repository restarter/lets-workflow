package gitutil_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
)

func TestProjectRoot_OutsideGit_ReturnsEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir() // NOT a git repo
	got := gitutil.ProjectRoot(tmp, time.Second)
	if got != "" {
		t.Errorf("ProjectRoot(non-git) = %q, want empty string (bash parity: no os.Getwd fallback)", got)
	}
}

func TestProjectRoot_InsideGit_ReturnsToplevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)

	// On macOS, t.TempDir() lives under /var/folders/... which is symlinked
	// from /private/var/folders/... - git resolves to the latter, so use Eval
	// to compare apples to apples.
	wantResolved, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	got := gitutil.ProjectRoot(tmp, time.Second)
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(got=%q): %v", got, err)
	}
	if gotResolved != wantResolved {
		t.Errorf("ProjectRoot(git repo) = %q (resolved %q), want %q", got, gotResolved, wantResolved)
	}
}

func TestProjectRoot_NoTimeout_StillWorks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	gitInit(t, tmp)

	// Pass timeout=0 - no context bound. Should still resolve quickly for
	// a local git command (no network involved).
	got := gitutil.ProjectRoot(tmp, 0)
	if got == "" {
		t.Error("ProjectRoot(... timeout=0) returned empty for valid git repo")
	}
}

// gitInit runs `git init` in dir. Helper kept private to this test file.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in %s: %v\n%s", dir, err, out)
	}
	// Ensure git allows operations (some sandboxed CIs need this; harmless on dev).
	_ = os.Setenv("GIT_AUTHOR_NAME", "test")
	_ = os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	_ = os.Setenv("GIT_COMMITTER_NAME", "test")
	_ = os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}
