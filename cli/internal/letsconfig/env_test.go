package letsconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
)

func writeEnv(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".lets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lets", ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolvedEnv_DefaultBranchFromOrigin(t *testing.T) {
	// An uninitialized repo whose origin default branch is master resolves master.
	repo := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(repo, "init", "-q", "-b", "master")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")
	run(repo, "remote", "add", "origin", repo)
	run(repo, "fetch", "-q", "origin")
	run(repo, "remote", "set-head", "origin", "master")

	fromOrigin := func(r string) string {
		out, err := exec.Command("git", "-C", r, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output()
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
	}
	if got := ResolvedEnv(repo, "", fromOrigin)["LETS_MERGE_BRANCH"]; got != "master" {
		t.Errorf("LETS_MERGE_BRANCH = %q, want master", got)
	}
	if got := ResolvedEnv(repo, "", nil)["LETS_MERGE_BRANCH"]; got != "main" {
		t.Errorf("nil resolver: LETS_MERGE_BRANCH = %q, want main (no git fork)", got)
	}
}

func TestResolvedEnv_CapAndPrecedence(t *testing.T) {
	project, home := t.TempDir(), t.TempDir()
	long := strings.Repeat("b", 10*1024)
	if got := ResolvedEnv(project, home, func(string) string { return long })["LETS_MERGE_BRANCH"]; len(got) != envfile.MaxValueLen {
		t.Errorf("derived branch length = %d, want capped at %d", len(got), envfile.MaxValueLen)
	}

	writeEnv(t, home, "LETS_LAUNCHER=cmux\nLETS_LANGUAGE=Ukrainian\n")
	writeEnv(t, project, "LETS_LAUNCHER=tmux\nLETS_MERGE_BRANCH=develop\n")
	env := ResolvedEnv(project, home, func(string) string { t.Error("resolver must not run when the key is set"); return "" })
	if env["LETS_LAUNCHER"] != "tmux" || env["LETS_LANGUAGE"] != "Ukrainian" || env["LETS_MERGE_BRANCH"] != "develop" {
		t.Errorf("env = %v", env)
	}
}
