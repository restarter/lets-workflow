//go:build unix

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

// runTaskStateSet runs `lets worktree task-state set <args>` in cwd through cobra.
func runTaskStateSet(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	t.Chdir(cwd)
	root := NewRootCmd()
	root.SetArgs(append([]string{"worktree", "task-state", "set"}, args...))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SilenceUsage, root.SilenceErrors = true, true
	err := root.Execute()
	return out.String(), err
}

// ownRegistry writes a registry under a fresh $CLAUDE_CONFIG_DIR listing this test
// process (a live pid) as session sid, started at startedMs.
func ownRegistry(t *testing.T, sid string, startedMs int64) {
	t.Helper()
	cfg := t.TempDir()
	healWrite(t, filepath.Join(cfg, "sessions", fmt.Sprintf("%d.json", os.Getpid())),
		fmt.Sprintf(`{"sessionId":%q,"name":"lead","cwd":"/x","peerProtocol":1,"status":"idle","kind":"interactive","startedAt":%d}`, sid, startedMs))
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
}

// --session-id equal to $CLAUDE_CODE_SESSION_ID records this process's proof;
// another session's id records none (a caller proves only its own process).
func TestTaskStateSetCmd_ProofOnlyForOwnSession(t *testing.T) {
	repo, head, taskFile := guardRepo(t, "task: lets-abc\n")
	ownRegistry(t, gSidLead, gStartedMs)

	t.Setenv("CLAUDE_CODE_SESSION_ID", gSidLead)
	if out, err := runTaskStateSet(t, repo, "--session-sha", head, "--session-id", gSidLead); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}
	want := "task: lets-abc\nsession: " + head + " " + gSidLead + "\n" + proofLine(gSidLead, os.Getpid(), gStartedMs)
	if got := readString(t, taskFile); got != want {
		t.Errorf("own session: want the proof written:\n%s", got)
	}

	t.Setenv("CLAUDE_CODE_SESSION_ID", gSidPane)
	if out, err := runTaskStateSet(t, repo, "--session-sha", head, "--session-id", gSidLead); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}
	if got := readString(t, taskFile); got != "task: lets-abc\nsession: "+head+" "+gSidLead+"\n" {
		t.Errorf("another session's id: no proof may be written:\n%s", got)
	}
}

// A live foreign holder refuses the call: JSON kind session_held, the exit code the
// process ends with (main maps the error's ExitCode) is 35, the file is untouched.
func TestTaskStateSetCmd_LiveHolderExit35(t *testing.T) {
	body := "task: lets-abc\nsession: 1111111 " + gSidLead + "\n"
	repo, head, taskFile := guardRepo(t, body)
	ownRegistry(t, gSidLead, gStartedMs) // the holder is this live process
	t.Setenv("CLAUDE_CODE_SESSION_ID", gSidPane)
	out, err := runTaskStateSet(t, repo, "--session-sha", head, "--session-id", gSidPane, "--json")
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 35 || ec.ExitCode() != worktreecmd.ExitSessionHeld {
		t.Fatalf("err=%v, want exit 35", err)
	}
	if !strings.Contains(out, `"kind": "session_held"`) {
		t.Errorf("JSON kind:\n%s", out)
	}
	if got := readString(t, taskFile); got != body {
		t.Errorf("a refused call wrote:\n%s", got)
	}
}

// The NoProof remedy works: after a /clear that could not carry, `task-state set`
// from this chat records the proof, and the next /clear carries.
func TestTaskStateSetCmd_RemedyFixesNextClear(t *testing.T) {
	w := newGuardWorld(t)
	repo, head, taskFile := guardRepo(t, "task: lets-abc\nsession: 1111111 "+gSidLead+"\n")
	w.putRow(100, interactiveRow(100, gSidNew, repo, gStartedMs))
	out := runSessionStart(t, repo, `{"session_id":"`+gSidNew+`","source":"clear"}`)
	if !strings.Contains(out, "no session_proof recorded its process") {
		t.Fatalf("precondition: a NoProof /clear:\n%s", out)
	}

	t.Setenv("CLAUDE_CODE_SESSION_ID", gSidNew)
	if out, err := runTaskStateSet(t, repo, "--session-sha", head, "--session-id", gSidNew); err != nil {
		t.Fatalf("set: %v\n%s", err, out)
	}
	if got := readString(t, taskFile); !strings.Contains(got, proofLine(gSidNew, 100, gStartedMs)) {
		t.Fatalf("the remedy must record the proof:\n%s", got)
	}

	w.putRow(100, interactiveRow(100, gSidNext, repo, gStartedMs))
	out = runSessionStart(t, repo, `{"session_id":"`+gSidNext+`","source":"clear"}`)
	want := "task: lets-abc\nsession: " + head + " " + gSidNext + "\n" + proofLine(gSidNext, 100, gStartedMs)
	if got := readString(t, taskFile); got != want {
		t.Errorf("the next /clear must carry:\n%s\n%s", got, out)
	}
}
