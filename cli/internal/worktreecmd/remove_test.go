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

// Unpushed-commits guard: parity with pre-rewrite markdown Step R2. A
// worktree with local commits ahead of upstream must block remove without
// --force. The original safety net was lost when the markdown rewrote into
// a thin dispatcher; this re-adds it as a typed error in the Go subcommand.
func TestRemove_UnpushedCommitsBlocks(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	// Stand up an upstream by registering the main repo itself as a remote,
	// pointing the worktree's branch upstream at remote/main, then committing
	// onto the worktree branch so log @{u}.. has output.
	runIn(t, repo, "git", "remote", "add", "origin", repo)
	runIn(t, repo, "git", "fetch", "origin", "--quiet")
	runIn(t, cr.Worktree.Path, "git", "branch", "--set-upstream-to=origin/main")
	runIn(t, cr.Worktree.Path, "git", "commit", "--allow-empty", "-m", "wt commit")

	_, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitUnpushedCommits || e.Kind != "unpushed_commits" {
		t.Errorf("want ExitUnpushedCommits/unpushed_commits, got %v (kind=%q)", err, func() string {
			if e == nil {
				return ""
			}
			return e.Kind
		}())
	}
}

// --force bypasses the unpushed-commits guard the same way it bypasses the
// dirty-worktree guard.
func TestRemove_UnpushedCommitsForce(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, _ := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	runIn(t, repo, "git", "remote", "add", "origin", repo)
	runIn(t, repo, "git", "fetch", "origin", "--quiet")
	runIn(t, cr.Worktree.Path, "git", "branch", "--set-upstream-to=origin/main")
	runIn(t, cr.Worktree.Path, "git", "commit", "--allow-empty", "-m", "wt commit")

	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo", Force: true})
	if err != nil || !res.OK {
		t.Fatalf("--force should bypass unpushed-commits guard: err=%v ok=%v", err, res.OK)
	}
}

// No upstream configured (common for attach-mode worktrees on local-only
// branches): the check is skipped with a warn step rather than blocking.
func TestRemove_NoUpstreamSkipsCheck(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	// No remote, no upstream — `git log @{u}..` fails. Should warn-and-continue.
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	if err != nil || !res.OK {
		t.Fatalf("no-upstream worktree should remove cleanly: err=%v ok=%v", err, res.OK)
	}
	hasWarn := false
	for _, s := range res.Steps {
		if s.Status == worktreecmd.StepWarn && strings.Contains(s.Message, "no upstream") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected a warn step about missing upstream; got steps=%+v", res.Steps)
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

// Remove deletes the per-branch .task state file (keyed by the re-derived
// branch) along with the legacy session ref - lets-dsdmp.
func TestRemove_DeletesTaskStateFile(t *testing.T) {
	repo := initRepo(t)
	sessions := filepath.Join(repo, ".lets", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	// take-task would have written these for branch worktree-foo.
	taskFile := filepath.Join(sessions, ".task-worktree-foo")
	legacyFile := filepath.Join(sessions, ".session-start-ref-worktree-foo")
	for _, f := range []string{taskFile, legacyFile} {
		if err := os.WriteFile(f, []byte("task: lets-x\nstart: abc\nsession: s sid\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res.OK)
	}
	for _, f := range []string{taskFile, legacyFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("state file %s not cleaned up after remove: %v", filepath.Base(f), err)
		}
	}
}

// Attach-mode: the worktree dir name differs from the branch, so the state file
// is keyed by the RE-DERIVED branch. Deriving the slug from the dir name would
// miss it - this locks the re-derivation in remove.go (lets-dsdmp, review S4).
func TestRemove_DeletesTaskStateFile_AttachMode(t *testing.T) {
	repo := initRepo(t)
	sessions := filepath.Join(repo, ".lets", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	// Existing branch attached to a worktree whose dir name ("myattach") != branch.
	runIn(t, repo, "git", "branch", "feature/bar")
	wtPath := filepath.Join(repo, ".worktrees", "myattach")
	runIn(t, repo, "git", "worktree", "add", wtPath, "feature/bar")
	// take-task keys by the attached branch (feature/bar -> feature-bar), NOT the dir.
	taskFile := filepath.Join(sessions, ".task-feature-bar")
	if err := os.WriteFile(taskFile, []byte("task: lets-x\nstart: abc\nsession: s sid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "myattach", Force: true})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res.OK)
	}
	if _, err := os.Stat(taskFile); !os.IsNotExist(err) {
		t.Errorf("attach-mode .task-feature-bar not cleaned (dir-name slug would miss it): %v", err)
	}
}

func TestRemove_DeleteBranchMergedUpstreamWhileLocalMainLags(t *testing.T) {
	repo := initRepoWithOrigin(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, cr.Worktree.Path, "git", "commit", "-q", "--allow-empty", "-m", "work")
	runIn(t, repo, "git", "push", "-q", "origin", "worktree-foo:main") // merged upstream; local main lags
	runIn(t, repo, "git", "fetch", "-q", "origin")

	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo", DeleteBranch: true})
	if err != nil || !res.OK {
		t.Fatalf("err=%v ok=%v", err, res != nil && res.OK)
	}
	if !res.Removed.BranchDeleted {
		t.Error("expected branch_deleted=true for a branch merged into origin/main")
	}
	var sawMergeBase bool
	for _, s := range res.Steps {
		if strings.Contains(s.Message, "merged into origin/main") {
			sawMergeBase = true
		}
	}
	if !sawMergeBase {
		t.Errorf("steps do not name the merge-base decision: %+v", res.Steps)
	}
}

func TestRemove_AlreadyGoneFinishesBranchStep(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "gone", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "worktree", "remove", "--force", cr.Worktree.Path)
	runIn(t, repo, "git", "worktree", "prune")

	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "gone", DeleteBranch: true})
	if err != nil || !res.OK {
		t.Fatalf("err=%v", err)
	}
	if !res.Removed.AlreadyGone || !res.Removed.BranchDeleted || res.Removed.Branch != "worktree-gone" {
		t.Errorf("removed = %+v, want already_gone + branch_deleted for worktree-gone", res.Removed)
	}
	if out, _ := exec.Command("git", "-C", repo, "branch", "--list", "worktree-gone").Output(); len(out) != 0 {
		t.Errorf("branch still exists: %s", out)
	}

	// A second run has nothing left to finish: worktree_not_found.
	_, err = worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "gone", DeleteBranch: true})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Kind != "worktree_not_found" {
		t.Errorf("second run: want worktree_not_found, got %v", err)
	}
}

func TestRemove_ExternalWorktreeRefused(t *testing.T) {
	repo := initRepo(t)
	ext := filepath.Join(realTempDir(t), "outside")
	if err := os.MkdirAll(ext, 0o755); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "worktree", "add", "-q", "-b", "bar-branch", filepath.Join(ext, "bar"))

	_, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "bar", DeleteBranch: true})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Kind != "worktree_external" {
		t.Fatalf("want worktree_external, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(ext, "bar")); statErr != nil {
		t.Errorf("external worktree must be left alone: %v", statErr)
	}
}

func TestRemove_LetsManagedEntriesAreNotDirty(t *testing.T) {
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
	installAdapter(t, repo, "beads", "links: `.beads/.env` (0600).")
	cr, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto})
	if err != nil {
		t.Fatal(err)
	}
	// adopt's moved-aside cache dir is LETS-managed too
	if err := os.MkdirAll(filepath.Join(cr.Worktree.Path, ".lets.pre-adopt", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cr.Worktree.Path, ".lets.pre-adopt", "cache", "usage"), []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.Remove(context.Background(), repo, worktreecmd.RemoveOptions{Name: "foo"})
	if err != nil || !res.OK || res.Removed.HadUncommittedChanges {
		t.Fatalf("a worktree holding only LETS-managed entries must not be dirty: err=%v removed=%+v", err, res.Removed)
	}
}

func TestList_StoreLinksFromAdapter(t *testing.T) {
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
	installAdapter(t, repo, "beads", "links: `.beads/.env` (0600).")
	if _, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{Name: "foo", Mode: worktreecmd.BranchAuto}); err != nil {
		t.Fatal(err)
	}
	res, err := worktreecmd.List(context.Background(), repo)
	if err != nil || len(res.Worktrees) != 1 {
		t.Fatalf("err=%v worktrees=%+v", err, res.Worktrees)
	}
	w := res.Worktrees[0]
	if !w.StoreLinked || !w.BeadsSymlinked || len(w.StoreLinks) != 1 || !w.StoreLinks[0].Linked {
		t.Errorf("store links = %+v linked=%v", w.StoreLinks, w.StoreLinked)
	}
	var b strings.Builder
	worktreecmd.RenderList(&b, res)
	if !strings.Contains(b.String(), "STORE") || strings.Contains(b.String(), "BEADS") {
		t.Errorf("list table must show STORE, not BEADS:\n%s", b.String())
	}
}
