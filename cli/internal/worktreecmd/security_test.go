//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// Default behavior: refuse to attach the branch that's checked out in main.
func TestCreate_BranchCheckedOutInMain_RefusesByDefault(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v, want ExitBranchConflict", err)
	}
}

// Opt-in: --switch-main-if-needed on a clean tree auto-switches main.
func TestCreate_SwitchMainIfNeeded_CleanTree(t *testing.T) {
	repo := initRepo(t)
	// Pre-ignore .lets/.worktrees and commit so ensureCleanTree sees a clean
	// tree once the test writes .lets/.env below. Without this, the .env
	// file is untracked and ensureCleanTree refuses to auto-switch.
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".lets/\n.worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "add", ".gitignore")
	runIn(t, repo, "git", "commit", "-m", "ignore .lets/.worktrees")
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "branch", "develop")
	if err := os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_MERGE_BRANCH=develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res.OK)
	}
	sawWarn := false
	for _, s := range res.Steps {
		if s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, "auto-switched main") {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Errorf("expected StepWarn for auto-switch, got steps %+v", res.Steps)
	}
}

// Refuses --switch-main-if-needed on dirty tree.
func TestCreate_SwitchMainIfNeeded_DirtyTreeRefuses(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitDirtyWorktree {
		t.Errorf("got %v, want ExitDirtyWorktree", err)
	}
}

// Refuses --switch-main-if-needed when main is mid-rebase.
func TestCreate_SwitchMainIfNeeded_MidRebaseRefuses(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitDirtyWorktree {
		t.Fatalf("got %v, want ExitDirtyWorktree (mid-op)", err)
	}
	if !strings.Contains(e.Kind, "mid_op") {
		t.Errorf("error kind=%s, expected to contain mid_op", e.Kind)
	}
}

// Credential-redaction sanity.
func TestRedactCreds(t *testing.T) {
	cases := map[string]string{
		"https://alice:s3cret@github.com/repo.git": "https://***:***@github.com/repo.git",
		"http://x:y@example/path":                  "https://***:***@example/path",
		"plain https://github.com/repo no creds":   "plain https://github.com/repo no creds",
	}
	for in, want := range cases {
		if got := worktreecmd.RedactCredsForTesting(in); got != want {
			t.Errorf("redactCreds(%q) = %q, want %q", in, got, want)
		}
	}
}
