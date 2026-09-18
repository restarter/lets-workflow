//go:build unix

package orcacmd

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTerminals_JoinAndFallback(t *testing.T) {
	useFakeOrca(t, func(args []string) (string, string, bool) {
		switch joined(args[:2]) {
		case "terminal list":
			return `{"ok":true,"result":{"terminals":[
				{"handle":"bad handle!","tabId":"t0","leafId":"l0"},
				{"handle":"term_claude","worktreePath":"/w","title":"\u001b[31mred","tabId":"t1","leafId":"l1","agentIdentity":"claude","lastOutputAt":1789756557492,"writable":true,"connected":true},
				{"handle":"term_ag","worktreePath":"/w","title":"ag","tabId":"t2","leafId":"l2","agentIdentity":"antigravity","lastOutputAt":5,"writable":true,"connected":false},
				{"handle":"term_shell","worktreePath":"/w","title":"zsh","tabId":"t3","leafId":"l3","agentIdentity":null,"writable":false,"connected":true}]}}`, "", false
		case "worktree ps":
			return `{"ok":true,"result":{"worktrees":[{"path":"/w","agents":[{"paneKey":"t1:l1","state":"done","agentType":"claude"}]}]}}`, "", false
		}
		return "{}", "", false
	})
	c, _ := NewClient()
	terms, f := c.Terminals(context.Background())
	if f != nil || len(terms) != 3 {
		t.Fatalf("terminals: %+v %v", terms, f)
	}
	cl, ag, sh := terms[0], terms[1], terms[2]
	if cl.Handle != "term_claude" || cl.AgentType != "claude" || cl.State != "done" || cl.PaneKey != "t1:l1" || cl.LastOutputAt != 1789756557492 || !cl.Writable || !cl.Connected {
		t.Errorf("claude row: %+v", cl)
	}
	if strings.ContainsRune(cl.Title, '\x1b') || !strings.Contains(cl.Title, "red") {
		t.Errorf("title not control-cleaned: %q", cl.Title)
	}
	if ag.AgentType != "antigravity" || ag.State != "" || ag.PaneKey != "t2:l2" || ag.Connected {
		t.Errorf("agentIdentity fallback: %+v", ag)
	}
	if sh.AgentType != "" || sh.Writable {
		t.Errorf("shell row: %+v", sh)
	}
}

func TestScreen_Draft(t *testing.T) {
	out := `{"ok":true,"result":{"terminal":{"source":"screen","tail":["❯"],"draft":"half typed"}}}`
	useFakeOrca(t, func([]string) (string, string, bool) { return out, "", false })
	c, _ := NewClient()
	fr, f := c.Screen(context.Background(), "term_a")
	if f != nil || fr.Source != "screen" || len(fr.Lines) != 1 || fr.Draft != "half typed" {
		t.Errorf("frame: %+v %v", fr, f)
	}
	out = `{"ok":true,"result":{"terminal":{"source":"stream","tail":["x"]}}}`
	if fr, _ := c.Screen(context.Background(), "term_a"); fr.Draft != "" || fr.Source != "stream" {
		t.Errorf("no draft: %+v", fr)
	}
	out = `{"ok":true,"result":{}}`
	if _, f := c.Screen(context.Background(), "term_a"); f == nil || f.Reason != ReasonOutputUnrecognized {
		t.Errorf("unrecognized: %v", f)
	}
}

func TestCreateTerminal_WorktreeCheck(t *testing.T) {
	path := t.TempDir()
	echo := ""
	useFakeOrca(t, func([]string) (string, string, bool) {
		return `{"ok":true,"result":{"terminal":{"handle":"term_new"` + echo + `}}}`, "", false
	})
	c, _ := NewClient()
	for _, tc := range []struct {
		echo, want, reason string
	}{
		{`,"worktreeId":"r::/other"`, "", ReasonEnvMismatch},
		{`,"worktreeId":"no-separator"`, "", ReasonEnvMismatch},
		{`,"worktreeId":"2b557cc3::` + path + `"`, "term_new", ""},
		{"", "term_new", ""},
	} {
		echo = tc.echo
		h, f := c.CreateTerminal(context.Background(), path, "t", "codex")
		if h != tc.want || (tc.reason == "") != (f == nil) || (f != nil && f.Reason != tc.reason) {
			t.Errorf("echo %q: handle %q failure %v", tc.echo, h, f)
		}
	}
}

func TestWaitStartup_OwnDeadline(t *testing.T) {
	oldLook, oldRun := lookOrca, runOrca
	t.Cleanup(func() { lookOrca, runOrca = oldLook, oldRun })
	lookOrca = func() (string, bool) { return "/fake/orca", true }
	var left time.Duration
	runOrca = func(ctx context.Context, _ string, args ...string) ([]byte, []byte, error) {
		if joined(args[:2]) == "terminal wait" {
			d, _ := ctx.Deadline()
			left = time.Until(d)
		}
		return []byte(`{"ok":true,"result":{"wait":{"satisfied":true}}}`), nil, nil
	}
	c, _ := NewClient()
	h, up, f := c.WaitStartup(context.Background(), "term_a", "/w", "t")
	if h != "term_a" || !up || f != nil {
		t.Fatalf("wait: %q %v %v", h, up, f)
	}
	if left < 100*time.Second {
		t.Errorf("terminal wait runs under a %v deadline; Run's 15 s default would cut the 120 s wait", left)
	}
}

func TestSendText_Argv(t *testing.T) {
	calls := useFakeOrca(t, func([]string) (string, string, bool) {
		return `{"ok":true,"result":{"send":{"accepted":true,"prompt":{"stages":["input_accepted","turn_started"]}}}}`, "", false
	})
	c, _ := NewClient()
	r, f := c.SendText(context.Background(), "term_a", "hello there")
	if f != nil || !r.InputAccepted || !r.TurnStarted {
		t.Fatalf("receipt: %+v %v", r, f)
	}
	want := "terminal send --terminal term_a --text hello there --enter --wait-submit 10 --json"
	if got := joined((*calls)[0].args); got != want {
		t.Errorf("argv:\n got %q\nwant %q", got, want)
	}
}
