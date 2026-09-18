//go:build unix

package agentrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const testThread = "01a0b4a9-c0a3-7fc0-a476-3b53bb522ee5"

// fakeCodex installs a fake `codex` shell script: it records its argv and stdin in
// dir, sets $out to the --output-last-message path and runs body. git and the
// Codex home are stubbed too.
func fakeCodex(t *testing.T, body string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	script := "#!/bin/sh\ndir='" + dir + "'\nout=''; prev=''\nfor a in \"$@\"; do [ \"$prev\" = --output-last-message ] && out=\"$a\"; prev=\"$a\"; done\n" +
		"printf '%s\\n' \"$@\" > \"$dir/argv\"\ncat > \"$dir/stdin\"\n" + body
	p := filepath.Join(dir, "codex")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	oldLook, oldGit, oldHome := lookCodex, gitOut, codexHome
	lookCodex = func() (string, bool) { return p, true }
	gitOut = func(string, ...string) ([]byte, error) { return nil, nil }
	home := t.TempDir()
	codexHome = func() string { return home }
	t.Cleanup(func() { lookCodex, gitOut, codexHome = oldLook, oldGit, oldHome })
	return dir
}

// runRequest writes a brief and returns a request whose outputs sit next to it.
func runRequest(t *testing.T) Request {
	t.Helper()
	dir := t.TempDir()
	brief := filepath.Join(dir, "b.md")
	write(t, brief, "# the brief\n")
	return Request{PromptFile: brief, Dir: dir, OutBase: filepath.Join(dir, "b")}
}

func line(v string) string { return "printf '%s\\n' '" + v + "'\n" }

func TestRun_Complete(t *testing.T) {
	dir := fakeCodex(t, line(`{"type":"thread.started","thread_id":"`+testThread+`"}`)+
		"printf 'ROOT FINAL' > \"$out\"\n"+line(`{"type":"turn.completed"}`))
	req := runRequest(t)
	res := codex{}.Run(context.Background(), req)
	if !res.Ran || !res.Complete || res.Reason != "" || res.SessionID != testThread {
		t.Fatalf("result: %+v", res)
	}
	if b, _ := os.ReadFile(res.ReportPath); string(b) != "ROOT FINAL" {
		t.Errorf("report %q", b)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	for _, want := range []string{"exec", "--sandbox", "read-only", "--cd", req.Dir, "--json", "-"} {
		if !strings.Contains("\n"+string(argv), "\n"+want+"\n") {
			t.Errorf("argv lacks %q:\n%s", want, argv)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "stdin")); string(b) != "# the brief\n" {
		t.Errorf("stdin %q", b)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings: %v", res.Warnings)
	}
}

func TestRun_NotFound(t *testing.T) {
	fakeCodex(t, "")
	lookCodex = func() (string, bool) { return "", false }
	req := runRequest(t)
	res := codex{}.Run(context.Background(), req)
	if res.Ran || res.Reason != "codex_not_found" {
		t.Errorf("result: %+v", res)
	}
	for _, p := range []string{res.ReportPath, res.EventsPath, res.StderrPath} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s must not exist", p)
		}
	}
}

func TestRun_RefusesExistingOutputs(t *testing.T) {
	dir := fakeCodex(t, "")
	req := runRequest(t)
	report, _, _ := Outputs(req.OutBase)
	write(t, report, "old")
	res := codex{}.Run(context.Background(), req)
	if res.Ran || res.Reason != ReasonReportExists {
		t.Errorf("result: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "argv")); err == nil {
		t.Error("codex must not have run")
	}
	if b, _ := os.ReadFile(report); string(b) != "old" {
		t.Errorf("report overwritten: %q", b)
	}
}

func TestRun_ExitNonZero(t *testing.T) {
	fakeCodex(t, "echo boom >&2\nexit 3\n")
	res := codex{}.Run(context.Background(), runRequest(t))
	if res.Complete || res.Reason != ReasonExitNonZero || res.ExitCode != 3 || !strings.Contains(res.StderrTail, "boom") {
		t.Errorf("result: %+v", res)
	}
}

func TestRun_TurnFailed(t *testing.T) {
	fakeCodex(t, line(`{"type":"turn.failed","error":{"message":"quota exceeded"}}`)+"exit 1\n")
	res := codex{}.Run(context.Background(), runRequest(t))
	if res.Reason != ReasonTurnFailed || !strings.Contains(res.StderrTail, "quota exceeded") {
		t.Errorf("result: %+v", res)
	}
}

func TestRun_ErrorEventIsWarning(t *testing.T) {
	fakeCodex(t, line(`{"type":"error","message":"reconnecting 1/5"}`)+line(`{"type":"thread.started","thread_id":"`+testThread+`"}`)+
		"printf 'DONE' > \"$out\"\n"+line(`{"type":"turn.completed"}`))
	res := codex{}.Run(context.Background(), runRequest(t))
	if !res.Complete || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "reconnecting") {
		t.Errorf("result: %+v", res)
	}
}

func TestRun_Timeout(t *testing.T) {
	fakeCodex(t, "sleep 30\n")
	req := runRequest(t)
	req.Timeout = 300 * time.Millisecond
	start := time.Now()
	res := codex{}.Run(context.Background(), req)
	if res.Reason != ReasonTimeout || !res.Ran || res.Complete {
		t.Fatalf("result: %+v", res)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("Run took %v", d)
	}
}

// TestRun_KillsGroup cancels a run only after its SIGTERM-ignoring child has
// written its pid, so a loaded machine cannot end the run before the child
// exists; then the child must be gone (os/exec's WaitDelay kills only the leader).
func TestRun_KillsGroup(t *testing.T) {
	dir := fakeCodex(t, "sh -c 'trap \"\" TERM; echo $$ > \"'\"$dir\"'/child\"; while :; do sleep 1; done' &\nsleep 60\n")
	req := runRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- codex{}.Run(ctx, req) }()
	pidFile := filepath.Join(dir, "child")
	var b []byte
	for deadline := time.Now().Add(20 * time.Second); ; {
		if b, _ = os.ReadFile(pidFile); len(strings.TrimSpace(string(b))) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the child never wrote its pid")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	var res Result
	select {
	case res = <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if res.Reason != ReasonCanceled {
		t.Errorf("result: %+v", res)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	for deadline := time.Now().Add(2 * time.Second); ; {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("the child %d that ignores SIGTERM survived Run (the v1 gap)", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRun_EmptyReportFallsBackToRollout(t *testing.T) {
	fakeCodex(t, line(`{"type":"thread.started","thread_id":"`+testThread+`"}`)+": > \"$out\"\n"+line(`{"type":"turn.completed"}`))
	rollout := filepath.Join(codexHome(), "sessions", "2026", "09", "18", "rollout-2026-09-18T15-16-58-"+testThread+".jsonl")
	write(t, rollout, `{"timestamp":"2026-09-18T15:16:58.000Z","type":"session_meta","payload":{"id":"`+testThread+`","originator":"codex_exec","source":"exec","thread_source":"user"}}
{"timestamp":"2026-09-18T15:16:58.100Z","type":"event_msg","payload":{"type":"task_started","turn_id":"T1"}}
{"timestamp":"2026-09-18T15:17:30.000Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"T1","last_agent_message":"FROM ROLLOUT"}}
`)
	res := codex{}.Run(context.Background(), runRequest(t))
	if !res.Complete || res.RolloutPath != rollout || len(res.Warnings) != 1 {
		t.Fatalf("result: %+v", res)
	}
	if b, _ := os.ReadFile(res.ReportPath); string(b) != "FROM ROLLOUT" {
		t.Errorf("report %q", b)
	}
}

func TestRun_ReportRedacted(t *testing.T) {
	fakeCodex(t, "printf 'leaked %s' '"+fakeToken+"' > \"$out\"\n")
	res := codex{}.Run(context.Background(), runRequest(t))
	b, _ := os.ReadFile(res.ReportPath)
	if !res.Complete || strings.Contains(string(b), fakeToken) || !strings.Contains(string(b), "leaked") {
		t.Errorf("result %+v report %q", res, b)
	}
}

func TestRun_WorkspaceChangedWarns(t *testing.T) {
	fakeCodex(t, "printf 'DONE' > \"$out\"\n")
	n := 0
	gitOut = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "status" {
			n++
			return []byte(fmt.Sprint(n)), nil
		}
		return nil, nil
	}
	res := codex{}.Run(context.Background(), runRequest(t))
	if !res.Complete || !res.WorkspaceChanged || len(res.Warnings) != 1 {
		t.Errorf("result: %+v", res)
	}
}
