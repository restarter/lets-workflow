//go:build unix

package orcacmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// selfRepo is a git checkout the test runs in, listed by the fake `worktree ps`.
func selfRepo(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	t.Chdir(dir)
	return dir
}

func psWith(path string) string {
	return `{"ok":true,"result":{"worktrees":[{"worktreeId":"r::` + path + `","path":"` + path + `","agents":[]}]}}`
}

func TestCard_Gates(t *testing.T) {
	calls := useFakeOrca(t, func([]string) (string, string, bool) { return "{}", "", false })
	t.Setenv("ORCA_WORKTREE_ID", "r::/somewhere")
	if res, _ := Card(context.Background(), CardOptions{Phase: "start", Launcher: "terminal"}); res.Card.Reason != ReasonNotEnabled || len(*calls) != 0 {
		t.Errorf("LETS_LAUNCHER=terminal inside an Orca terminal: %+v calls=%d", res.Card, len(*calls))
	}
	t.Setenv("ORCA_WORKTREE_ID", "")
	if res, _ := Card(context.Background(), CardOptions{Phase: "start", Launcher: "orca"}); res.Card.Reason != ReasonEnvAbsent || len(*calls) != 0 {
		t.Errorf("env absent: %+v calls=%d", res.Card, len(*calls))
	}
	if _, err := Card(context.Background(), CardOptions{Phase: "deploy", Launcher: "orca"}); err == nil || err.(*Error).Code != ExitUsage {
		t.Errorf("unknown phase: %v", err)
	}
}

func TestCard_ArgvRedactionAndReasons(t *testing.T) {
	dir := selfRepo(t)
	other := t.TempDir()
	t.Setenv("ORCA_WORKTREE_ID", "r::"+dir)
	var set []string
	mode := "ok"
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch {
		case args[0] == "worktree" && args[1] == "ps":
			if mode == "mismatch" {
				return psWith(other), "", false
			}
			return psWith(dir), "", false
		case args[0] == "worktree" && args[1] == "set":
			set = args
			if mode == "capability" {
				return `{"ok":false,"error":{"code":"invalid_argument","message":"Unknown flag --workspace-status"}}`, "", true
			}
			return `{"ok":true}`, "", false
		}
		return "{}", "", false
	})
	for phase, status := range map[string]string{"start": "in-progress", "pr": "in-review", "closed": "completed", "end": "", "blocked": "", "gate": ""} {
		set = nil
		res, err := Card(context.Background(), CardOptions{Phase: phase, Comment: "c", Launcher: "orca"})
		if err != nil || !res.Card.Updated {
			t.Fatalf("%s: %+v %v", phase, res.Card, err)
		}
		got := joined(set)
		if status != "" && !strings.Contains(got, "--workspace-status "+status) || status == "" && strings.Contains(got, "--workspace-status") {
			t.Errorf("%s argv: %s", phase, got)
		}
	}
	set = nil
	_, _ = Card(context.Background(), CardOptions{Phase: "end", Comment: "token ghp_abcdefghijklmnopqrstuvwxyz0123 " + strings.Repeat("ж", 300), Launcher: "orca"})
	comment := set[5]
	if strings.Contains(comment, "ghp_") || !utf8.ValidString(comment) || utf8.RuneCountInString(comment) > commentCap {
		t.Errorf("comment not redacted / cut on a rune boundary: %d runes", utf8.RuneCountInString(comment))
	}
	mode = "capability"
	if res, _ := Card(context.Background(), CardOptions{Phase: "start", Launcher: "orca"}); res.Card.Reason != ReasonCapabilityMissing {
		t.Errorf("capability missing: %+v", res.Card)
	}
	mode = "mismatch"
	if res, _ := Card(context.Background(), CardOptions{Phase: "start", Launcher: "orca"}); res.Card.Reason != ReasonEnvMismatch {
		t.Errorf("env id of another worktree: %+v", res.Card)
	}
}

func TestCard_AppNotRunning(t *testing.T) {
	selfRepo(t)
	t.Setenv("ORCA_WORKTREE_ID", "r::/x")
	useFakeOrca(t, func(args []string) (string, string, bool) {
		return "", "Orca is not running", true
	})
	if res, _ := Card(context.Background(), CardOptions{Phase: "start", Launcher: "orca"}); res.Card.Reason == "" || res.Card.Updated {
		t.Errorf("app down degrades by name: %+v", res.Card)
	}
}
