//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubClaude writes a fake claude: `--help` prints helpText; a run records its argv,
// env and stdin into dir and prints a JSON result carrying a token.
func stubClaude(t *testing.T, helpText string) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--help" ]; then printf '%s\n' "` + helpText + `"; exit 0; fi
printf '%s\n' "$@" > "` + dir + `/argv"
env > "` + dir + `/env"
cat > "` + dir + `/stdin"
pwd > "` + dir + `/pwd"
printf '{"result":"the answer; token ghp_abcdefghijklmnopqrstuvwxyz0123"}'
`
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := claudeBin
	claudeBin = func() (string, error) { return bin, nil }
	t.Cleanup(func() { claudeBin = old })
	return dir
}

const fullHelp = "--fork-session --permission-mode --tools --disallowedTools --strict-mcp-config --mcp-config"

func askROSetup(t *testing.T) (hub, foreign string, fr *FrameResult) {
	t.Helper()
	hub = repoWithLets(t, "")
	foreign = gitRepo(t)
	claudeHome(t, []regRow{{101, sidMain, "HUB", hub}})
	res, err := Frame(context.Background(), FrameOptions{Cwd: hub, Session: sidMain, ToSession: sidWork, Kind: "ask-ro", ToName: "MAIN-PWA"})
	if err != nil {
		t.Fatalf("frame ask-ro for a stopped session: %v", err)
	}
	_ = os.WriteFile(res.HandoffPath, []byte(res.Header+"\nwhat is in progress?"), 0o600)
	return hub, foreign, res
}

func TestAskRO_RunsNarrowed(t *testing.T) {
	hub, foreign, fr := askROSetup(t)
	dir := stubClaude(t, fullHelp)
	t.Setenv("ORCA_WORKTREE_ID", "r::x")
	t.Setenv("CLAUDE_CODE_SESSION_ID", sidMain)
	t.Setenv("LETS_LAUNCHER", "orca")
	res, err := AskRO(context.Background(), AskROOptions{Cwd: hub, Repo: foreign, Session: sidWork, Pid: 0, MsgID: fr.MsgID})
	// pid 0 + session not in registry = unknown -> refused; a recorded dead pid proceeds
	if err != nil || res.Reason != "liveness_unknown" {
		t.Fatalf("pid-less unknown holder must be refused: %+v %v", res, err)
	}
	res, err = AskRO(context.Background(), AskROOptions{Cwd: hub, Repo: foreign, Session: sidWork, Pid: 4242, MsgID: fr.MsgID})
	if err != nil || !res.Answered || strings.Contains(res.Answer, "ghp_") {
		t.Fatalf("ask-ro: %+v %v", res, err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	want := strings.Join(askROArgs(sidWork, filepath.Join(hub, ".lets", "cache", "ask-ro-mcp-empty.json")), "\n") + "\n"
	if string(argv) != want {
		t.Errorf("argv:\n%s\nwant:\n%s", argv, want)
	}
	for _, must := range []string{"--permission-mode\nplan", "--tools\nRead,Grep,Glob", "--strict-mcp-config", "--fork-session"} {
		if !strings.Contains(string(argv), must) {
			t.Errorf("argv lacks %q", must)
		}
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	for _, leak := range []string{"ORCA_WORKTREE_ID", "CLAUDE_CODE_SESSION_ID", "LETS_LAUNCHER"} {
		if strings.Contains(string(env), leak+"=") {
			t.Errorf("child env carries %s", leak)
		}
	}
	if pwd, _ := os.ReadFile(filepath.Join(dir, "pwd")); !strings.Contains(string(pwd), filepath.Base(foreign)) {
		t.Errorf("child must run in the other project's checkout: %s", pwd)
	}
	if stdin, _ := os.ReadFile(filepath.Join(dir, "stdin")); !strings.HasSuffix(strings.TrimSpace(string(stdin)), "what is in progress?") {
		t.Errorf("stdin: %q", stdin)
	}
	if _, err := os.Stat(fr.HandoffPath); !os.IsNotExist(err) {
		t.Error("the prompt handoff must be consumed")
	}
	mcp, _ := os.Stat(filepath.Join(hub, ".lets", "cache", "ask-ro-mcp-empty.json"))
	if mcp == nil || mcp.Mode().Perm() != 0o600 {
		t.Errorf("empty MCP config must be 0600: %v", mcp)
	}
}

func TestAskRO_Refusals(t *testing.T) {
	hub, foreign, fr := askROSetup(t)
	stubClaude(t, "--fork-session --permission-mode --tools --strict-mcp-config --mcp-config") // no --disallowedTools
	if res, _ := AskRO(context.Background(), AskROOptions{Cwd: hub, Repo: foreign, Session: sidWork, Pid: 4242, MsgID: fr.MsgID}); res.Reason != "headless_readonly_unenforceable" {
		t.Errorf("missing flag: %+v", res)
	}
	if _, err := os.Stat(fr.HandoffPath); err != nil {
		t.Error("nothing may be consumed when ask-ro cannot run")
	}
	claudeHome(t, []regRow{{101, sidMain, "HUB", hub}, {4243, sidWork, "MAIN-PWA", foreign}})
	stubClaude(t, fullHelp)
	if res, _ := AskRO(context.Background(), AskROOptions{Cwd: hub, Repo: foreign, Session: sidWork, Pid: 4243, MsgID: fr.MsgID}); res.Reason != "main_alive" {
		t.Errorf("alive MAIN: %+v", res)
	}
	claudeHome(t, []regRow{{101, sidMain, "HUB", hub}}, 4244)
	if res, _ := AskRO(context.Background(), AskROOptions{Cwd: hub, Repo: foreign, Session: sidWork, Pid: 4244, MsgID: fr.MsgID}); res.Reason != "liveness_unknown" {
		t.Errorf("recorded pid on an unrecognized live entry: %+v", res)
	}
}

func TestAskRO_WideningPanics(t *testing.T) {
	for _, bad := range []string{"--dangerously-skip-permissions", "--allowedTools"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s must panic", bad)
				}
			}()
			askROArgs(bad, "x")
		}()
	}
}
