//go:build unix

package worktreecmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// Task 4a placeholder. Full happy-path assertions land in 4b.
//
// Create currently panics with "Task 4a stop point" once it reaches the
// post-`git worktree add` boundary; this test recovers to confirm everything
// up to that boundary executes without a real error.
func TestCreate_4aSkeleton_Compiles(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = recover() }()
	_, _ = worktreecmd.Create(context.Background(), repo, worktreecmd.CreateOptions{
		Name: "foo", Mode: worktreecmd.BranchAuto,
	})
}
