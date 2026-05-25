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

// Refuses --switch-main-if-needed when main is mid-revert (S-12 — REVERT_HEAD
// was missing from the marker list pre-fix).
func TestCreate_SwitchMainIfNeeded_MidRevertRefuses(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repo, ".git")
	// Fake mid-revert by touching REVERT_HEAD.
	if err := os.WriteFile(filepath.Join(gitDir, "REVERT_HEAD"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitDirtyWorktree {
		t.Fatalf("got %v, want ExitDirtyWorktree (mid-revert)", err)
	}
	if !strings.Contains(e.Kind, "mid_op") {
		t.Errorf("error kind=%s, expected to contain mid_op", e.Kind)
	}
}

// Refuses --switch-main-if-needed when main is mid-auto-merge (S-12 — git
// 2.41+ AUTO_MERGE marker was missing pre-fix).
func TestCreate_SwitchMainIfNeeded_MidAutoMergeRefuses(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(repo, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, "AUTO_MERGE"), []byte("conflict\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitDirtyWorktree {
		t.Fatalf("got %v, want ExitDirtyWorktree (mid-auto-merge)", err)
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

// Verifies the post-S-1-followup fix: when Step 5 auto-switches main and a
// LATER pre-rollback step (Step 6 EnsureGitignore) fails, the helper
// restores main to the pre-switch branch. Without the restore, fail() would
// leave the user's main repo on $LETS_MERGE_BRANCH while reporting an error
// — a partial state change the user didn't ask for.
//
// Reproduction strategy: induce a Step 6 EnsureGitignore failure by
// pre-creating `.lets/locks` as a regular FILE (not a directory). When
// EnsureGitignore tries `os.MkdirAll(...) .lets/locks/...`, the FS rejects
// because the parent path is a file. The Step 5 auto-switch has already
// happened by then; the new restoreMainIfSwitched helper must reverse it.
func TestCreate_SwitchMainIfNeeded_RestoresMainOnLaterFailure(t *testing.T) {
	repo := initRepo(t)
	// Pre-commit .gitignore so ensureCleanTree passes after we write .lets/.env.
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".lets/\n.worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runIn(t, repo, "git", "add", ".gitignore")
	runIn(t, repo, "git", "commit", "-m", "ignore .lets/.worktrees")
	runIn(t, repo, "git", "branch", "develop")
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_MERGE_BRANCH=develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Block EnsureGitignore's MkdirAll(`.lets/locks`) by placing a regular
	// file there: subsequent MkdirAll fails with "not a directory".
	if err := os.WriteFile(filepath.Join(repo, ".lets", "locks"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Main is currently on `main`. Attach mode + name=main → Step 5 sees
	// cur == plan.Branch, auto-switches main → develop, then Step 6 fails.
	res, err := worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "main", Mode: worktreecmd.BranchAttach, SwitchMainIfNeeded: true,
	})
	if err == nil || res.OK {
		t.Fatalf("expected gitignore-update failure, got ok=%v err=%v", res.OK, err)
	}
	// The fix: main must be back on its original branch.
	out, _ := exec.Command("git", "-C", repo, "branch", "--show-current").Output()
	if got := strings.TrimSpace(string(out)); got != "main" {
		t.Errorf("main was not restored: now on %q, want %q", got, "main")
	}
	// Surface check: a "restored main" step should be visible in the envelope.
	sawRestore := false
	for _, s := range res.Steps {
		if strings.Contains(s.Message, "restored main") {
			sawRestore = true
		}
	}
	if !sawRestore {
		t.Errorf("expected a 'restored main' step, got steps: %+v", res.Steps)
	}
}

// Credential-redaction sanity. Covers all four cred shapes git can emit
// plus the bare-SSH no-op case. Scheme is PRESERVED in the redacted output
// (per security review B-2 — scheme-flip masks transport-security bugs).
func TestRedactCreds(t *testing.T) {
	cases := map[string]string{
		// classic user:password
		"https://alice:s3cret@github.com/repo.git": "https://<redacted>@github.com/repo.git",
		// http stays http (scheme preserved)
		"http://x:y@example/path": "http://<redacted>@example/path",
		// token-only (gh auth setup-git / x-access-token form) - WAS LEAKED PRE-B-2
		"https://:ghp_abc123@github.com/x/y": "https://<redacted>@github.com/x/y",
		// single-token without colon - WAS LEAKED PRE-B-2
		"https://ghp_singletoken@github.com/x/y": "https://<redacted>@github.com/x/y",
		// password contains @ - WAS PARTIAL-LEAKED PRE-B-2 ("ssword" tail).
		// RE2's greedy [^\s/]* consumes up to the LAST `@` before `/` or
		// whitespace, so the whole "user:p@ssword" disappears (best case).
		"https://user:p@ssword@host/path": "https://<redacted>@host/path",
		// bare SSH form - no scheme, untouched
		"git@github.com:user/repo.git": "git@github.com:user/repo.git",
		// plain URL, no creds - untouched
		"plain https://github.com/repo no creds": "plain https://github.com/repo no creds",
		// mid-text URL inside a longer error message
		"fatal: Authentication failed for 'https://x:tok@github.com/foo/bar.git'": "fatal: Authentication failed for 'https://<redacted>@github.com/foo/bar.git'",
	}
	for in, want := range cases {
		if got := worktreecmd.RedactCredsForTesting(in); got != want {
			t.Errorf("redactCreds(%q):\n  got  %q\n  want %q", in, got, want)
		}
	}
}
