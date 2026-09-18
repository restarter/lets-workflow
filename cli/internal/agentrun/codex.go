//go:build unix

package agentrun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// Seams (tests replace them).
var (
	lookCodex = func() (string, bool) {
		p, err := exec.LookPath("codex")
		return p, err == nil
	}
	codexHome = func() string {
		if h := os.Getenv("CODEX_HOME"); h != "" {
			return h
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".codex")
	}
	now   = time.Now
	sleep = time.Sleep
)

// stderrTailCap bounds the stderr tail a Result carries; stderrScan is how much of
// the end of stderr is redacted before the tail is cut from it.
const (
	stderrTailCap = 2 << 10
	stderrScan    = 64 << 10
)

var threadIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type codex struct{}

func (codex) Name() string { return "codex" }

// Run starts `codex exec` in a read-only sandbox with the prompt file on stdin
// (argv, never a shell). The report is the -o file: codex fills it from the root
// turn's last agent message (codex-rs/exec final_message_from_turn_items), so a
// subagent's message never lands there; the rollout's task_complete is the second
// source when -o comes back empty. When Run returns, nothing it started is left
// running: the whole process group is killed, including a tool that ignored
// SIGTERM or one Codex left behind.
func (codex) Run(ctx context.Context, r Request) Result {
	report, events, stderr := Outputs(r.OutBase)
	res := Result{Provider: "codex", ReportPath: report, EventsPath: events, StderrPath: stderr, Warnings: []string{}}
	ioFail := func(err error) Result {
		res.Reason, res.StderrTail = ReasonIO, clip(err.Error())
		return res
	}
	bin, ok := lookCodex()
	if !ok {
		res.Reason = "codex_not_found"
		return res
	}
	for _, p := range []string{report, events, stderr} {
		if _, err := os.Lstat(p); err == nil {
			res.Reason = ReasonReportExists
			return res
		}
	}
	prompt, err := os.Open(r.PromptFile)
	if err != nil {
		return ioFail(err)
	}
	defer func() { _ = prompt.Close() }()
	outF, err := os.OpenFile(events, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ioFail(err)
	}
	errF, err := os.OpenFile(stderr, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = outF.Close()
		return ioFail(err)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(rctx, bin, "exec", "--sandbox", "read-only", "--cd", r.Dir, "--json", "--color", "never", "--output-last-message", report, "-")
	cmd.Dir = r.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = prompt, outF, errF
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 10 * time.Second
	before := Fingerprint(r.Dir)
	start := now()
	res.Ran = true
	runErr := cmd.Run()
	if cmd.Process != nil {
		// WaitDelay kills only the leader (os/exec); end the group, ESRCH when empty.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	res.DurationMS = now().Sub(start).Milliseconds()
	_ = outF.Close()
	_ = errF.Close()
	if Fingerprint(r.Dir) != before {
		res.WorkspaceChanged = true
		res.Warnings = append(res.Warnings, "the working tree changed during a read-only run")
	}
	res.StderrTail = tailOf(stderr)
	ev := readEvents(events)
	res.Warnings = append(res.Warnings, ev.errors...)
	if threadIDRe.MatchString(ev.threadID) {
		res.SessionID = ev.threadID
		res.RolloutPath = rolloutByThread(ev.threadID)
	}
	var exitErr *exec.ExitError
	isExit := errors.As(runErr, &exitErr)
	if isExit {
		res.ExitCode = exitErr.ExitCode()
	}
	switch {
	case errors.Is(rctx.Err(), context.DeadlineExceeded):
		res.Reason = ReasonTimeout
		return res
	case ctx.Err() != nil:
		res.Reason = ReasonCanceled
		return res
	case ev.failure != "":
		res.Reason = ReasonTurnFailed
		res.StderrTail = strings.TrimSpace(clip(ev.failure) + "\n" + res.StderrTail)
		return res
	case runErr != nil && !isExit:
		return ioFail(runErr)
	case runErr != nil:
		res.Reason = ReasonExitNonZero
		return res
	}
	text, _ := os.ReadFile(report)
	fromRollout := ""
	if res.RolloutPath != "" {
		if s := scanRollout(res.RolloutPath, nil, time.Time{}); s.state == stateComplete {
			fromRollout = s.message
		}
	}
	switch {
	case len(bytes.TrimSpace(text)) == 0 && strings.TrimSpace(fromRollout) != "":
		text = []byte(fromRollout)
		res.Warnings = append(res.Warnings, "the -o file was empty; the report is the rollout's task_complete message")
	case fromRollout != "" && strings.TrimSpace(fromRollout) != strings.TrimSpace(string(text)):
		res.Warnings = append(res.Warnings, "the -o file differs from the rollout's task_complete message; the -o file is kept")
	}
	if len(bytes.TrimSpace(text)) == 0 {
		res.Reason = ReasonReportEmpty
		return res
	}
	if err := replace(report, clean(string(text))); err != nil {
		return ioFail(err)
	}
	res.Complete = true
	return res
}

type execEvents struct {
	threadID string
	failure  string   // turn.failed: the turn ended in an error
	errors   []string // error events: often transient (a reconnect), kept as warnings
}

// readEvents reads the `codex exec --json` stream (codex-cli 0.154: thread.started,
// turn.started, item.*, turn.completed | turn.failed, error). Lines are read whole:
// a command's aggregated output can exceed any Scanner buffer.
func readEvents(path string) execEvents {
	var ev execEvents
	f, err := os.Open(path)
	if err != nil {
		return ev
	}
	defer func() { _ = f.Close() }()
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadBytes('\n')
		var e struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Message  string `json:"message"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if len(bytes.TrimSpace(line)) > 0 && json.Unmarshal(line, &e) == nil {
			switch e.Type {
			case "thread.started":
				ev.threadID = e.ThreadID
			case "turn.failed":
				ev.failure = "turn failed"
				if e.Error != nil && e.Error.Message != "" {
					ev.failure = e.Error.Message
				}
			case "error":
				if len(ev.errors) < 5 {
					ev.errors = append(ev.errors, "codex error event: "+clip(e.Message))
				}
			}
		}
		if err != nil {
			return ev
		}
	}
}

// rolloutByThread finds a headless run's rollout by its thread id - the file name
// ends in it; the id passed threadIDRe before it reaches a glob.
func rolloutByThread(id string) string {
	m, _ := filepath.Glob(filepath.Join(codexHome(), "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
	if len(m) == 1 {
		return m[0]
	}
	return ""
}

// tailOf returns the end of a file, redacted and capped. It redacts a wide window
// first and cuts the tail from the result: cutting first could split a secret from
// its key or prefix (`password=`, `ghp_`), and a recognizer never sees the rest.
func tailOf(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > stderrScan {
		b = b[len(b)-stderrScan:]
	}
	s := redact.Control(redact.Text(redact.Creds(string(b))))
	if len(s) > stderrTailCap {
		s = s[len(s)-stderrTailCap:]
		for len(s) > 0 && !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
	}
	return strings.TrimSpace(s)
}

// clip redacts and caps one message.
func clip(s string) string {
	return redact.Cap(redact.Control(redact.Text(redact.Creds(s))), stderrTailCap)
}
