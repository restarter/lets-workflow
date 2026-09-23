//go:build unix

package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
	"github.com/restarter/lets-workflow/cli/internal/notifycmd"
)

func TestWorktreeRelease_AlarmNotifies(t *testing.T) {
	repo, _ := filepath.EvalSymlinks(t.TempDir()) // macOS: /var -> /private/var, git resolves it
	// main checkout with one commit, a linked worktree on lets-al1-x, its task-state file,
	// no snapshot -> snapshot=missing -> alarm.
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init"}, {"config", "user.email", "t@e"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "one"}, {"worktree", "add", "-q", "-b", "lets-al1-x", filepath.Join(repo, "wt")},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	_ = os.MkdirAll(filepath.Join(repo, ".lets", "sessions"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, ".lets", "sessions", ".task-lets-al1-x"), []byte("task: lets-al1\n"), 0o600)

	var got []notifycmd.Options
	t.Cleanup(cli.SetReleaseNotify(func(_ context.Context, o notifycmd.Options) (*notifycmd.Result, error) {
		got = append(got, o)
		return &notifycmd.Result{}, nil
	}))

	cmd := cli.NewWorktreeCmd()
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"release", "--quiet", "--dir", filepath.Join(repo, "wt")})
	_ = cmd.Execute()
	if len(got) != 1 || !strings.Contains(got[0].Body, "snapshot=missing") {
		t.Fatalf("notify = %+v", got)
	}
	if !strings.Contains(stderr.String(), "lets: lets-al1") {
		t.Errorf("stderr must carry the alarm even with --quiet: %q", stderr.String())
	}
}
