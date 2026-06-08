//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestResolveBranch_Attach_Existing(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "existing")
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "existing", "", worktreecmd.BranchAttach, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "attached" || plan.Branch != "existing" {
		t.Errorf("got %+v", plan)
	}
}

func TestResolveBranch_Attach_Missing(t *testing.T) {
	repo := initRepo(t)
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "missing", "", worktreecmd.BranchAttach, "main")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v, want ExitBranchConflict", err)
	}
}

func TestResolveBranch_NewBranch_Conflict(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "foo")
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "foo", "", worktreecmd.BranchNewBranch, "main")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v", err)
	}
}

func TestResolveBranch_Auto_PrefersAttach(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "feat-x")
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "feat-x", "", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "attached" {
		t.Errorf("got %s", plan.Mode)
	}
}

func TestResolveBranch_Auto_FallsBackToCreate(t *testing.T) {
	repo := initRepo(t)
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "newfeat", "", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "created" || plan.Branch != "worktree-newfeat" {
		t.Errorf("got %+v", plan)
	}
}

func TestResolveBranch_BaseRefMissing(t *testing.T) {
	repo := initRepo(t)
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "newfeat", "", worktreecmd.BranchAuto, "nonexistent")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v", err)
	}
}

// --- Explicit --branch ref (lets-x5ucf): decouple dir NAME from branch REF. ---

// Attach to an existing slash-bearing branch via an explicit ref while the
// worktree dir name stays slash-free. This is the core lets-x5ucf scenario.
func TestResolveBranch_ExplicitBranch_AttachSlashRef(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "feature/pwa-46696")
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "pwa-46696", "feature/pwa-46696", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "attached" || plan.Branch != "feature/pwa-46696" {
		t.Errorf("got %+v, want attached feature/pwa-46696", plan)
	}
}

// Create a NEW slash-bearing branch verbatim (no worktree- prefix) off base.
func TestResolveBranch_ExplicitBranch_CreatesVerbatim(t *testing.T) {
	repo := initRepo(t)
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "pwa-46696", "feature/pwa-46696", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "created" || plan.Branch != "feature/pwa-46696" {
		t.Errorf("got %+v, want created feature/pwa-46696 (no worktree- prefix)", plan)
	}
}

// --attach with an explicit ref that doesn't exist surfaces branch_missing
// against the REF (not the dir name).
func TestResolveBranch_ExplicitBranch_AttachMissing(t *testing.T) {
	repo := initRepo(t)
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "pwa", "feature/nope", worktreecmd.BranchAttach, "main")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Fatalf("got %v, want ExitBranchConflict", err)
	}
	if !strings.Contains(e.Message, "feature/nope") {
		t.Errorf("message %q should name the ref", e.Message)
	}
}

// An invalid explicit ref is rejected by the ref-grammar validator.
func TestResolveBranch_ExplicitBranch_InvalidRefRejected(t *testing.T) {
	repo := initRepo(t)
	cases := []string{"feature/..x", "feature/x..", "has space", "trailing/", "-leadingdash", "ctrl\x01char"}
	for _, c := range cases {
		_, err := worktreecmd.ResolveBranch(context.Background(), repo, "ok-name", c, worktreecmd.BranchAuto, "main")
		var e *worktreecmd.Error
		if !errors.As(err, &e) || e.Code != worktreecmd.ExitUsage {
			t.Errorf("ResolveBranch branch=%q: got %v, want ExitUsage", c, err)
		}
	}
}

func TestValidateName_AcceptsValid(t *testing.T) {
	cases := []string{"foo", "foo-bar", "foo.bar", "foo_bar", "lets-hpi.3-worktree-start"}
	for _, c := range cases {
		if err := worktreecmd.ValidateName(context.Background(), c); err != nil {
			t.Errorf("ValidateName(%q): %v", c, err)
		}
	}
}

func TestValidateName_RejectsInvalid(t *testing.T) {
	cases := []string{
		"",           // empty
		"FOO",        // uppercase
		"foo bar",    // space
		"foo/bar",    // slash
		".foo",       // leading dot
		"-foo",       // leading dash
		"foo..bar",   // double dot
		"foo.lock",   // .lock suffix
		"foo:bar",    // colon (git reject)
		"foo@{x}",    // @{
		"foo~1",      // tilde
		"foo^",       // caret
		"foo?",       // question mark
		"foo*",       // asterisk
		"foo[bar",    // bracket
		"foo\x00bar", // NUL byte
		"this-name-is-way-too-long-to-be-allowed-as-a-worktree-identifier-here", // >64 chars
	}
	for _, c := range cases {
		if err := worktreecmd.ValidateName(context.Background(), c); err == nil {
			t.Errorf("ValidateName(%q) should reject", c)
		}
	}
}
