//go:build unix

package orcacmd

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const leadCommand = "claude --name 'snake-lead' '/lets:start'"

func TestTerminal_Argv(t *testing.T) {
	wt := t.TempDir()
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		return `{"ok":true,"result":{"terminal":{"handle":"term_1234","worktreeId":"r::` + wt + `"}}}`, "", false
	})
	res, err := OpenTerminal(context.Background(), TerminalOptions{Worktree: wt, Title: "snake-lead", Command: leadCommand})
	if err != nil || !res.Terminal.Launched || res.Terminal.Handle != "term_1234" || res.Terminal.FallbackCommand != "" {
		t.Fatalf("err=%v terminal=%+v", err, res.Terminal)
	}
	want := []string{"terminal", "create", "--worktree", "path:" + wt, "--title", "snake-lead", "--command", leadCommand, "--json"}
	if last := (*calls)[len(*calls)-1].args; !slices.Equal(last, want) {
		t.Errorf("argv = %q\nwant  %q", last, want)
	}
	for _, c := range *calls {
		if strings.Contains(joined(c.args), "worktree create") {
			t.Errorf("Terminal must never create an Orca worktree: %v", c.args)
		}
	}
}

func TestTerminal_RefusesControlChars(t *testing.T) {
	wt := t.TempDir()
	calls := useFakeOrca(t, func([]string) (string, string, bool) { return statusRunning, "", false })
	for _, o := range []TerminalOptions{
		{Worktree: wt, Title: "snake-lead", Command: "claude\nrm -rf /"},
		{Worktree: wt, Title: "snake\x1b-lead", Command: leadCommand},
		{Worktree: "relative/path", Title: "snake-lead", Command: leadCommand},
		{Worktree: filepath.Join(wt, "missing"), Title: "snake-lead", Command: leadCommand},
		{Worktree: wt, Title: "", Command: leadCommand},
	} {
		if _, err := OpenTerminal(context.Background(), o); err == nil || err.(*Error).Code != ExitUsage {
			t.Errorf("%+v: err=%v, want a usage error", o, err)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("a refused call reached Orca: %v", *calls)
	}
}

func TestTerminal_Degrades(t *testing.T) {
	wt := t.TempDir()
	fallback := "cd '" + wt + "' && " + leadCommand
	check := func(name, reason string) {
		t.Helper()
		res, err := OpenTerminal(context.Background(), TerminalOptions{Worktree: wt, Title: "snake-lead", Command: leadCommand})
		if err != nil || res.Terminal.Launched || res.Terminal.Reason != reason || res.Terminal.FallbackCommand != fallback {
			t.Errorf("%s: err=%v terminal=%+v", name, err, res.Terminal)
		}
	}
	useFakeOrca(t, func([]string) (string, string, bool) {
		return `{"ok":true,"result":{"app":{"running":false}}}`, "", false
	})
	check("app not running", ReasonAppNotRunning)

	useFakeOrca(t, func(args []string) (string, string, bool) {
		if joined(args) == "status --json" {
			return statusRunning, "", false
		}
		return `{"ok":false,"error":{"code":"not_found","message":"worktree not found"}}`, "", true
	})
	res, _ := OpenTerminal(context.Background(), TerminalOptions{Worktree: wt, Title: "snake-lead", Command: leadCommand})
	if res.Terminal.Launched || res.Terminal.Reason == "" || res.Terminal.FallbackCommand != fallback {
		t.Errorf("orca refused: %+v", res.Terminal)
	}

	oldLook := lookOrca
	lookOrca = func() (string, bool) { return "", false }
	defer func() { lookOrca = oldLook }()
	check("not found", ReasonNotFound)
}

// lets orca open stays unchanged by the team flow.
func TestOpen_DefaultArgvUnchanged(t *testing.T) {
	repo := mainCheckout(t)
	calls := useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args) {
		case "status --json":
			return statusRunning, "", false
		case "worktree ps --json":
			return `{"result":{"worktrees":[]}}`, "", false
		}
		return `{"ok":true,"result":{"worktree":{"id":"r::/o/n","path":"/o/n","branch":"refs/heads/n"}}}`, "", false
	})
	if _, err := Open(context.Background(), OpenOptions{Repo: repo, Name: "n"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"worktree", "create", "--repo", "path:" + repo, "--name", "n", "--no-parent", "--agent", "claude", "--json"}
	if last := (*calls)[len(*calls)-1].args; !slices.Equal(last, want) {
		t.Errorf("argv = %q\nwant  %q", last, want)
	}
}
