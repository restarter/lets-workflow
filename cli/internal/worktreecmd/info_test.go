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

func TestInfo_InMain(t *testing.T) {
	repo := initRepo(t)
	res, err := worktreecmd.Info(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if res.InWorktree {
		t.Error("expected in_worktree=false")
	}
	if res.MainRoot != repo {
		t.Errorf("MainRoot=%s, want %s", res.MainRoot, repo)
	}
}

func TestInfo_InWorktree(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "x", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	res, _ := worktreecmd.Info(context.Background(), cr.Worktree.Path)
	if !res.InWorktree {
		t.Error("expected in_worktree=true")
	}
	if res.MainRoot != repo {
		t.Errorf("MainRoot=%s, want %s", res.MainRoot, repo)
	}
	if res.Worktree == nil || res.Worktree.Branch != "worktree-x" {
		t.Errorf("expected branch worktree-x, got %+v", res.Worktree)
	}
}

// Info called from a subdirectory of the worktree must resolve `dir` to the
// worktree root before probing for the .lets / .beads/.env symlinks; otherwise
// the symlink probes look in the wrong place and report LetsSymlinked=false on
// a perfectly valid worktree.
func TestInfo_FromSubdir(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "x", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(cr.Worktree.Path, "deep", "nest")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := worktreecmd.Info(context.Background(), sub)
	if err != nil || !res.OK {
		t.Fatalf("Info from subdir failed: err=%v ok=%v", err, res.OK)
	}
	if !res.InWorktree {
		t.Error("expected in_worktree=true")
	}
	if res.Worktree == nil || res.Worktree.Path != cr.Worktree.Path {
		t.Errorf("Worktree.Path = %v, want %s (subdir must resolve to worktree root)", res.Worktree, cr.Worktree.Path)
	}
	if res.Worktree == nil || !res.Worktree.LetsSymlinked {
		t.Errorf("LetsSymlinked = false on a valid worktree (subdir resolution probably broken)")
	}
}

func TestInfo_OutsideRepo(t *testing.T) {
	// Use a temp dir that's NOT in a git repo (its parent isn't either).
	tmp := t.TempDir()
	_, err := worktreecmd.Info(context.Background(), tmp)
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitNotInRepo {
		t.Errorf("want ExitNotInRepo, got %v", err)
	}
}
