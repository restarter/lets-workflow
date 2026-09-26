//go:build unix

package worktreecmd_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// The envelope carries the state RemoteContainsAny decided; every state is ok.
func TestPushed_States(t *testing.T) {
	root := realTempDir(t)
	bare := filepath.Join(root, "remote.git")
	clone := filepath.Join(root, "a")
	runIn(t, root, "git", "init", "-q", "--bare", "-b", "main", bare)
	runIn(t, root, "git", "clone", "-q", bare, clone)
	runIn(t, clone, "git", "config", "user.email", "t@e")
	runIn(t, clone, "git", "config", "user.name", "t")
	runIn(t, clone, "git", "commit", "-q", "--allow-empty", "-m", "base")
	runIn(t, clone, "git", "push", "-q", "-u", "origin", "main")
	pushed := strings.TrimSpace(gitOutput(t, clone, "rev-parse", "HEAD"))
	runIn(t, clone, "git", "commit", "-q", "--allow-empty", "-m", "local")
	local := strings.TrimSpace(gitOutput(t, clone, "rev-parse", "HEAD"))

	for _, tc := range []struct{ sha, want, head string }{{pushed, "pushed", "main"}, {local, "not_pushed", ""}} {
		res, err := worktreecmd.Pushed(context.Background(), clone, worktreecmd.PushedOptions{Commit: tc.sha, Branch: "main"})
		if err != nil || !res.OK || res.State != tc.want || res.Head != tc.head {
			t.Errorf("%s: %+v, %v - want %s", tc.sha, res, err, tc.want)
		}
	}
	runIn(t, clone, "git", "remote", "set-url", "origin", filepath.Join(root, "gone.git"))
	res, err := worktreecmd.Pushed(context.Background(), clone, worktreecmd.PushedOptions{Commit: local})
	if err != nil || res.State != "unverified" || res.Reason != "git_error" {
		t.Errorf("unreachable remote: %+v, %v - want unverified git_error", res, err)
	}
	if _, err := worktreecmd.Pushed(context.Background(), clone, worktreecmd.PushedOptions{Commit: "not-a-sha"}); worktreecmd.ExitCode(err) != worktreecmd.ExitUsage {
		t.Errorf("a bad --commit must be a usage error, got %v", err)
	}
}
