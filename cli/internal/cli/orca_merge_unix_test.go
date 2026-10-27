//go:build unix

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// team-create run from a subdirectory reads the project's LETS_MERGE_BRANCH, not
// <cwd>/.lets/.env (Codex round 1).
func TestMergeBranchOf_FromSubdirectory(t *testing.T) {
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_MERGE_BRANCH=develop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	if got := mergeBranchOf(sub); got != "develop" {
		t.Errorf("mergeBranchOf(subdirectory) = %q, want the project's develop", got)
	}
}
