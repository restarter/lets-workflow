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

func TestCreate_HappyDefault(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "foo", Mode: worktreecmd.BranchAuto,
	})
	if err != nil || !res.OK {
		t.Fatalf("Create: err=%v ok=%v", err, res.OK)
	}
	if res.Worktree.Branch != "worktree-foo" {
		t.Errorf("branch %s", res.Worktree.Branch)
	}
	if res.Worktree.BranchMode != "created" {
		t.Errorf("mode %s", res.Worktree.BranchMode)
	}

	// Symlink assertions: Lstat + Readlink-relative + round-trip-write.
	wtLets := filepath.Join(res.Worktree.Path, ".lets")
	fi, err := os.Lstat(wtLets)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".lets is not a symlink: %v", err)
	}
	target, _ := os.Readlink(wtLets)
	if filepath.IsAbs(target) {
		t.Errorf("symlink target absolute %q, want relative", target)
	}
	if err := os.WriteFile(filepath.Join(wtLets, "probe.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".lets", "probe.txt"))
	if string(b) != "hi" {
		t.Errorf("round-trip via symlink failed: got %q", b)
	}
}

func TestCreate_AttachExistingBranch(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "branch", "myfeature")
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "myfeature", Mode: worktreecmd.BranchAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Worktree.Branch != "myfeature" || res.Worktree.BranchMode != "attached" {
		t.Errorf("got %s (%s)", res.Worktree.Branch, res.Worktree.BranchMode)
	}
	// Attached branch's HEAD matches main repo's branch HEAD.
	hwt, _ := exec.Command("git", "-C", res.Worktree.Path, "rev-parse", "HEAD").Output()
	hm, _ := exec.Command("git", "-C", repo, "rev-parse", "refs/heads/myfeature").Output()
	if string(hwt) != string(hm) {
		t.Errorf("HEADs differ: worktree=%s main=%s", hwt, hm)
	}
}

func TestCreate_NoBeadsDir(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "foo", Mode: worktreecmd.BranchAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Worktree.BeadsSymlinked {
		t.Error("expected beads_symlinked=false")
	}
}

func TestCreate_NoSymlinkFlags(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".beads", ".env"), []byte("X=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "foo", Mode: worktreecmd.BranchAuto,
		NoSymlinkLets: true, NoSymlinkBeads: true,
	})
	if err != nil || !res.OK {
		t.Fatal(err)
	}
	if res.Worktree.LetsSymlinked || res.Worktree.BeadsSymlinked {
		t.Errorf("expected both symlinks skipped, got lets=%v beads=%v",
			res.Worktree.LetsSymlinked, res.Worktree.BeadsSymlinked)
	}
}

func TestCreate_LETSMergeBranchOther(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "develop")
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_MERGE_BRANCH=develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "feat", Mode: worktreecmd.BranchAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Worktree.BaseRef != "develop" {
		t.Errorf("base_ref=%s, want develop", res.Worktree.BaseRef)
	}
}

func TestCreate_NoOp_Idempotency(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _ = worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitWorktreeExists {
		t.Errorf("got %v, want ExitWorktreeExists", err)
	}
}

// QA F2: pre-existing real .lets/ dir gets replaced by symlink.
func TestCreate_PreExistingDotLets_GetsReplaced(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Direct exercise of CreateRelativeSymlink after RemoveAll — full path
	// (Create + git race) requires intercepting between git worktree add
	// and symlink, which the public API doesn't expose.
	wt := filepath.Join(repo, ".worktrees", "test")
	if err := os.MkdirAll(filepath.Join(wt, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	wtLets := filepath.Join(wt, ".lets")
	if err := os.RemoveAll(wtLets); err != nil {
		t.Fatal(err)
	}
	if err := worktreecmd.CreateRelativeSymlink(wtLets, filepath.Join(repo, ".lets"), repo); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(wtLets)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink after RemoveAll+CreateRelativeSymlink")
	}
}

// QA F1: re-create after rollback must succeed.
func TestCreate_Rollback_InvariantsAndReCreate(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".worktrees", "test-rollback")
	runIn(t, repo, "git", "worktree", "add", "-b", "worktree-test-rollback", wtPath, "main")

	plan := worktreecmd.BranchPlan{Mode: "created", Branch: "worktree-test-rollback"}
	res := &worktreecmd.CreateResult{Envelope: worktreecmd.Envelope{SchemaVersion: 1, Subcommand: "create"}}
	_, _ = worktreecmd.PerformRollbackForTesting(context.Background(), res, repo, wtPath, plan, "", "induced", errors.New("test"))

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("wtPath still exists after rollback")
	}
	out, _ := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if strings.Contains(string(out), "test-rollback") {
		t.Errorf("git worktree list still has stale entry:\n%s", out)
	}
	out, _ = exec.Command("git", "-C", repo, "branch", "--list", "worktree-test-rollback").Output()
	if strings.Contains(string(out), "worktree-test-rollback") {
		t.Errorf("branch survived rollback: %s", out)
	}
	if res.Rollback == nil || !res.Rollback.Attempted || !res.Rollback.Succeeded {
		t.Errorf("Rollback info: %+v", res.Rollback)
	}

	// Re-create after rollback succeeds.
	res2, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "test-rollback", Mode: worktreecmd.BranchAuto,
	})
	if err != nil || !res2.OK {
		t.Errorf("re-create after rollback failed: err=%v ok=%v", err, res2.OK)
	}
}

// Review item 61: --switch-main-if-needed restores prev branch on rollback.
func TestCreate_SwitchMainIfNeeded_RollbackRestoresPrev(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "develop")
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	wtPath := filepath.Join(repo, ".worktrees", "test-restore")
	runIn(t, repo, "git", "worktree", "add", "-b", "worktree-test-restore", wtPath, "main")
	// Switch main to develop (simulates the --switch-main-if-needed effect).
	runIn(t, repo, "git", "switch", "develop")

	plan := worktreecmd.BranchPlan{Mode: "created", Branch: "worktree-test-restore"}
	res := &worktreecmd.CreateResult{Envelope: worktreecmd.Envelope{SchemaVersion: 1, Subcommand: "create"}}
	_, _ = worktreecmd.PerformRollbackForTesting(context.Background(), res, repo, wtPath, plan, "main", "induced", errors.New("test"))

	out, _ := exec.Command("git", "-C", repo, "branch", "--show-current").Output()
	if strings.TrimSpace(string(out)) != "main" {
		t.Errorf("main not restored to main, currently on: %s", out)
	}
}

func TestRollback_RefusesPathEscape(t *testing.T) {
	repo := initRepo(t)
	plan := worktreecmd.BranchPlan{Mode: "created", Branch: "worktree-escape"}
	res := &worktreecmd.CreateResult{Envelope: worktreecmd.Envelope{SchemaVersion: 1}}
	_, _ = worktreecmd.PerformRollbackForTesting(context.Background(), res, repo, "/tmp/evil", plan, "", "test", errors.New("x"))
	if res.Rollback == nil || res.Rollback.Succeeded {
		t.Errorf("rollback should have refused: %+v", res.Rollback)
	}
}
