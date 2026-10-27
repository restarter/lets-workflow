//go:build unix

package worktreecmd_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestBase_ReportsRemoteBaseAndCutsNothing(t *testing.T) {
	repo := initRepo(t)
	origin := withOrigin(t, repo)
	tip := advanceOrigin(t, origin)
	branchesBefore := gitOutput(t, repo, "for-each-ref", "refs/heads/")
	res, err := worktreecmd.Base(context.Background(), repo, worktreecmd.BaseOptions{})
	if err != nil || res.BaseRef != "origin/main" || res.BaseResolution.Sha != tip {
		t.Fatalf("base: %+v, %v", res, err)
	}
	if n := strings.Count(gitOutput(t, repo, "worktree", "list", "--porcelain"), "worktree "); n != 1 {
		t.Errorf("base created a worktree: %d listed", n)
	}
	if gitOutput(t, repo, "for-each-ref", "refs/heads/") != branchesBefore {
		t.Error("base created a branch")
	}
}

func TestBase_AmbiguousRemoteRefuses(t *testing.T) {
	repo := initRepo(t)
	for _, name := range []string{"a", "b"} {
		bare := filepath.Join(realTempDir(t), name+".git")
		runIn(t, repo, "git", "init", "-q", "--bare", "-b", "main", bare)
		runIn(t, repo, "git", "remote", "add", name, bare)
	}
	_, err := worktreecmd.Base(context.Background(), repo, worktreecmd.BaseOptions{})
	var e *worktreecmd.Error
	if !errors.As(err, &e) || e.Code != worktreecmd.ExitNoRemoteBase || e.Kind != "remote_ambiguous" {
		t.Fatalf("err=%v, want 31 remote_ambiguous", err)
	}
}
