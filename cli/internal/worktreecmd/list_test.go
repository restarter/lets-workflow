//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestList_Empty(t *testing.T) {
	repo := initRepo(t)
	res, err := worktreecmd.List(context.Background(), repo)
	if err != nil || !res.OK {
		t.Fatal(err)
	}
	if len(res.Worktrees) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(res.Worktrees))
	}
	if res.Main == nil || res.Main.Branch != "main" {
		t.Errorf("Main row missing or wrong branch: %+v", res.Main)
	}
}

func TestList_OneInteractive(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	res, _ := worktreecmd.List(context.Background(), repo)
	if len(res.Worktrees) != 1 {
		t.Fatalf("got %d worktrees", len(res.Worktrees))
	}
	if res.Worktrees[0].Kind != "interactive" {
		t.Errorf("kind=%s, want interactive", res.Worktrees[0].Kind)
	}
	if !res.Worktrees[0].LetsSymlinked {
		t.Error("expected lets_symlinked=true")
	}
}

func TestList_AgentKind(t *testing.T) {
	repo := initRepo(t)
	agentPath := filepath.Join(repo, ".claude", "worktrees", "bar")
	runIn(t, repo, "git", "worktree", "add", "-b", "worktree-bar", agentPath, "main")
	res, _ := worktreecmd.List(context.Background(), repo)
	found := false
	for _, wt := range res.Worktrees {
		if wt.Kind == "agent" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one agent worktree, got: %+v", res.Worktrees)
	}
}

// S-14: TestList_NotInRepo confirms `lets worktree list` fails cleanly with
// ExitGitFailed / kind=git_worktree_list_failed when projectRoot isn't a
// git repo. Without this test, a regression that drops the error wrap and
// returns (nil-or-empty-result, nil) would pass silently.
func TestList_NotInRepo(t *testing.T) {
	tmp := realTempDir(t)
	_, err := worktreecmd.List(context.Background(), tmp)
	var e *worktreecmd.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *worktreecmd.Error, got %T: %v", err, err)
	}
	if e.Code != worktreecmd.ExitGitFailed {
		t.Errorf("Code=%d, want ExitGitFailed (%d)", e.Code, worktreecmd.ExitGitFailed)
	}
	if e.Kind != "git_worktree_list_failed" {
		t.Errorf("Kind=%q, want git_worktree_list_failed", e.Kind)
	}
}

func TestList_LockedFlag(t *testing.T) {
	repo := initRepo(t)
	wtPath := filepath.Join(repo, ".worktrees", "locked")
	runIn(t, repo, "git", "worktree", "add", "-b", "worktree-locked", wtPath, "main")
	runIn(t, repo, "git", "worktree", "lock", wtPath)
	res, _ := worktreecmd.List(context.Background(), repo)
	found := false
	for _, wt := range res.Worktrees {
		if wt.Locked {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one locked worktree, got: %+v", res.Worktrees)
	}
}
