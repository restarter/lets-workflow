//go:build unix

package orcacmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mainCheckout(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}

func TestOpen_ArgvAndResult(t *testing.T) {
	repo := mainCheckout(t)
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/lets-abc-x","path":"/o/lets-abc-x","branch":"refs/heads/lets-abc-x"}}}`, "", false
	})
	res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x", Prompt: "/lets:start lets-abc"})
	if err != nil || !res.Launch.Launched || res.Launch.Branch != "lets-abc-x" || res.Launch.OrcaWorktreeID != "r::/o/lets-abc-x" {
		t.Fatalf("err=%v launch=%+v", err, res.Launch)
	}
	want := []string{"worktree", "create", "--repo", "path:" + repo, "--name", "lets-abc-x", "--no-parent", "--agent", "claude", "--prompt", "/lets:start lets-abc", "--json"}
	if last := (*calls)[len(*calls)-1].args; !slices.Equal(last, want) {
		t.Errorf("argv = %q\nwant  %q", last, want)
	}
}

func TestOpen_DupGuardAndFallbacks(t *testing.T) {
	repo := mainCheckout(t)
	useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		return `{"result":{"worktrees":[{"worktreeId":"r::/o/x","path":"/o/x","branch":"refs/heads/lets-abc-x","displayName":"lets-abc-x"}]}}`, "", false
	})
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "lets-abc-x"}); res.Launch.Launched || res.Launch.Reason != "already_open" {
		t.Errorf("dup guard: %+v", res.Launch)
	}

	useFakeOrca(t, func([]string) (string, string, bool) {
		return `{"ok":true,"result":{"app":{"running":false}}}`, "", false
	})
	if res, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); err != nil || res.Launch.Launched || res.Launch.Reason != ReasonAppNotRunning || !strings.Contains(res.Launch.FallbackCommand, "--no-orca") {
		t.Errorf("app not running: err=%v %+v", err, res.Launch)
	}

	useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		if strings.HasPrefix(joined(args), "worktree ps") {
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":false,"error":{"code":"invalid_argument","message":"Unknown flag --agent for command: worktree create"}}`, "", true
	})
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); res.Launch.Launched || res.Launch.Reason != ReasonCapabilityMissing {
		t.Errorf("capability missing: %+v", res.Launch)
	}

	oldLook := lookOrca
	lookOrca = func() (string, bool) { return "", false }
	defer func() { lookOrca = oldLook }()
	if res, _ := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); res.Launch.Reason != ReasonNotFound {
		t.Errorf("not found: %+v", res.Launch)
	}
}

func TestOpen_RepoInvalidIsHard(t *testing.T) {
	if _, err := Open(context.Background(), OpenOptions{Repo: filepath.Join(t.TempDir(), "nope"), Name: "n"}); err == nil {
		t.Error("missing repo dir must be a hard error")
	}
	plain := t.TempDir()
	if err := os.MkdirAll(filepath.Join(plain, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), OpenOptions{Repo: plain, Name: "n"}); err == nil {
		t.Error("a non-repo dir must be a hard error")
	}
}

func TestNotify_TargetFromEnvOrCwd(t *testing.T) {
	cwdTarget := t.TempDir()
	t.Setenv("ORCA_WORKTREE_ID", "")
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "worktree ps --json" {
			return `{"result":{"worktrees":[{"worktreeId":"r::cwd","path":"` + cwdTarget + `"}]}}`, "", false
		}
		return `{"ok":true}`, "", false
	})
	res, err := Notify(context.Background(), NotifyOptions{Title: "Plan ready", Body: "token=ghp_abcdefghijklmnopqrstuvwxyz0123", Cwd: cwdTarget})
	if err != nil || !res.Notify.Notified || res.Notify.Target != "r::cwd" {
		t.Fatalf("cwd target: err=%v %+v", err, res.Notify)
	}
	last := joined((*calls)[len(*calls)-1].args)
	if !strings.Contains(last, "--worktree id:r::cwd") || strings.Contains(last, "ghp_") {
		t.Errorf("set argv = %q (must target the id and redact the body)", last)
	}
	if res, _ := Notify(context.Background(), NotifyOptions{Title: "t", Cwd: t.TempDir()}); res.Notify.Notified || res.Notify.Reason != ReasonWorktreeNotFound {
		t.Errorf("no match: %+v", res.Notify)
	}
	if _, err := Notify(context.Background(), NotifyOptions{}); err == nil {
		t.Error("a missing title is a hard error")
	}
}

func TestTruncate_RuneBoundary(t *testing.T) {
	s := strings.Repeat("ж", 300)
	got := Truncate(s, 280)
	if len([]rune(got)) != 280 || !strings.HasPrefix(s, got) {
		t.Errorf("truncate cut mid-rune or wrong length: %d", len([]rune(got)))
	}
}
