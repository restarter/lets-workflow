//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestResolveBranch_Attach_Existing(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "existing")
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "existing", worktreecmd.BranchAttach, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "attached" || plan.Branch != "existing" {
		t.Errorf("got %+v", plan)
	}
}

func TestResolveBranch_Attach_Missing(t *testing.T) {
	repo := initRepo(t)
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "missing", worktreecmd.BranchAttach, "main")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v, want ExitBranchConflict", err)
	}
}

func TestResolveBranch_NewBranch_Conflict(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "foo")
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "foo", worktreecmd.BranchNewBranch, "main")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v", err)
	}
}

func TestResolveBranch_Auto_PrefersAttach(t *testing.T) {
	repo := initRepo(t)
	runIn(t, repo, "git", "branch", "feat-x")
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "feat-x", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "attached" {
		t.Errorf("got %s", plan.Mode)
	}
}

func TestResolveBranch_Auto_FallsBackToCreate(t *testing.T) {
	repo := initRepo(t)
	plan, err := worktreecmd.ResolveBranch(context.Background(), repo, "newfeat", worktreecmd.BranchAuto, "main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "created" || plan.Branch != "worktree-newfeat" {
		t.Errorf("got %+v", plan)
	}
}

func TestResolveBranch_BaseRefMissing(t *testing.T) {
	repo := initRepo(t)
	_, err := worktreecmd.ResolveBranch(context.Background(), repo, "newfeat", worktreecmd.BranchAuto, "nonexistent")
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitBranchConflict {
		t.Errorf("got %v", err)
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
