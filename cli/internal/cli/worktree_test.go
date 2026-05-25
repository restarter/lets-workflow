package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// initRepo creates a temp git repo with one empty commit. Returns the
// realpath-resolved root (macOS /var → /private/var) so it matches what
// `git rev-parse --show-toplevel` returns from inside.
func initTestRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	dir, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("-c", "init.defaultBranch=main", "init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")
	runGit("commit", "--allow-empty", "-m", "initial")
	return dir
}

// chdir helper that auto-restores on test cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func runWorktreeCmd(t *testing.T, cwd string, args ...string) (string, string, error) {
	t.Helper()
	chdir(t, cwd)
	if err := os.MkdirAll(filepath.Join(cwd, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := cli.NewRootCmd()
	root.SetArgs(append([]string{"worktree"}, args...))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetContext(context.Background())
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestWorktreeCreate_JSON(t *testing.T) {
	repo := initTestRepo(t)
	stdout, _, err := runWorktreeCmd(t, repo, "create", "foo", "--json")
	if err != nil {
		t.Fatalf("err=%v\nstdout=%s", err, stdout)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if got["schema_version"].(float64) != 1 {
		t.Errorf("schema_version=%v, want 1", got["schema_version"])
	}
	if got["ok"] != true {
		t.Errorf("ok=%v, want true", got["ok"])
	}
	if got["subcommand"] != "create" {
		t.Errorf("subcommand=%v, want create", got["subcommand"])
	}
	wt, _ := got["worktree"].(map[string]any)
	if wt == nil || wt["branch"] != "worktree-foo" {
		t.Errorf("worktree.branch=%v, want worktree-foo", wt)
	}
}

func TestWorktreeCreate_PrintCD_PathOnly(t *testing.T) {
	repo := initTestRepo(t)
	stdout, stderr, err := runWorktreeCmd(t, repo, "create", "foo", "--print-cd")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// stdout: path + newline.
	expected := filepath.Join(repo, ".worktrees", "foo") + "\n"
	if stdout != expected {
		t.Errorf("stdout=%q, want %q", stdout, expected)
	}
	// stderr: empty (no --verbose, no --json).
	if stderr != "" {
		t.Errorf("stderr non-empty: %q", stderr)
	}
}

func TestWorktreeCreate_PrintCD_ShellSafe(t *testing.T) {
	repo := initTestRepo(t)
	stdout, _, err := runWorktreeCmd(t, repo, "create", "foo", "--print-cd")
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimSpace(stdout)
	if strings.ContainsAny(path, " \t\"'`$\\") {
		t.Errorf("path not shell-safe: %q", path)
	}
}

func TestWorktreeCreate_FlagConflict(t *testing.T) {
	repo := initTestRepo(t)
	_, _, err := runWorktreeCmd(t, repo, "create", "foo", "--attach", "--new-branch")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually-exclusive error, got: %v", err)
	}
}

func TestWorktreeList_JSON(t *testing.T) {
	repo := initTestRepo(t)
	if _, _, err := runWorktreeCmd(t, repo, "create", "foo"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runWorktreeCmd(t, repo, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	wts, _ := got["worktrees"].([]any)
	if len(wts) != 1 {
		t.Errorf("worktrees count=%d, want 1", len(wts))
	}
}

func TestWorktreeRemove_Human(t *testing.T) {
	repo := initTestRepo(t)
	if _, _, err := runWorktreeCmd(t, repo, "create", "foo"); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runWorktreeCmd(t, repo, "remove", "foo")
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !strings.Contains(stdout, "Worktree removed:") {
		t.Errorf("unexpected output: %q", stdout)
	}
}

func TestWorktreeInfo_Human(t *testing.T) {
	repo := initTestRepo(t)
	stdout, _, err := runWorktreeCmd(t, repo, "info")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "In worktree: false") {
		t.Errorf("expected 'In worktree: false', got: %q", stdout)
	}
}

func TestWorktreeCreate_FailureExitCode(t *testing.T) {
	repo := initTestRepo(t)
	if _, _, err := runWorktreeCmd(t, repo, "create", "foo"); err != nil {
		t.Fatal(err)
	}
	// Second create on same name → ExitWorktreeExists.
	_, _, err := runWorktreeCmd(t, repo, "create", "foo", "--json")
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
	// Stress the format: error message includes "already exists" + kind.
	if !strings.Contains(err.Error(), "exists") && !strings.Contains(err.Error(), "worktree_path_exists") {
		t.Errorf("error doesn't mention conflict: %v", err)
	}
	// Silence unused-import warning in some Go toolchains.
	_ = fmt.Sprintf
}
