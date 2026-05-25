//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestRemove_Clean(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res.OK)
	}
	if _, err := os.Stat(filepath.Join(repo, ".worktrees", "foo")); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists")
	}
}

func TestRemove_DirtyBlocks(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	if err := os.WriteFile(filepath.Join(cr.Worktree.Path, "new.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitDirtyWorktree {
		t.Errorf("want ExitDirtyWorktree, got %v", err)
	}
}

func TestRemove_DirtyForce(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	if err := os.WriteFile(filepath.Join(cr.Worktree.Path, "new.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo", Force: true})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res.OK)
	}
}

func TestRemove_DeleteBranchMerged(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo", DeleteBranch: true})
	if err != nil || !res.OK {
		t.Fatal(err)
	}
	if !res.Removed.BranchDeleted {
		t.Error("expected branch_deleted=true")
	}
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "worktree-foo").Output()
	if strings.Contains(string(out), "worktree-foo") {
		t.Errorf("branch survived: %s", out)
	}
}

func TestRemove_DeleteBranchUnmerged(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	runIn(t, cr.Worktree.Path, "git", "commit", "--allow-empty", "-m", "wt commit")
	_, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{
		Name: "foo", Force: true, DeleteBranch: true,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchUnmerged {
		t.Errorf("want ExitBranchUnmerged, got %v", err)
	}
}

// R3 single-trip flow: remove worktree first, then call --branch-only follow-up.
func TestRemove_BranchOnly_AfterWorktreeRemoved(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	branch := cr.Worktree.Branch
	if _, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"}); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{
		Name: "foo", BranchOnly: true, Branch: branch, DeleteBranch: true,
	})
	if err != nil || !res.OK || res.Removed == nil || !res.Removed.BranchDeleted {
		t.Errorf("R3 branch-only failed: err=%v ok=%v removed=%+v", err, res.OK, res.Removed)
	}
}

// Legacy state: bd-worktree's stale .beads/redirect must not block removal.
func TestRemove_MigrationFromLegacyState(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".worktrees", "legacy")
	runIn(t, repo, "git", "worktree", "add", "-b", "worktree-legacy", wtPath, "main")
	if err := os.MkdirAll(filepath.Join(wtPath, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, ".beads", "redirect"), []byte("../../.beads"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "legacy", Force: true})
	if err != nil || !res.OK {
		t.Errorf("legacy worktree removal failed: err=%v ok=%v", err, res.OK)
	}
}
