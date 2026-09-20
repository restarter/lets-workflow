//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// orcaTerm is one Orca terminal joined to its `worktree ps` agent row by pane key
// (`<tabId>:<leafId>`).
type orcaTerm struct {
	Handle, Path, Title, AgentType, State string
}

// orcaOps is the Orca surface the transport uses (a seam: tests fake it).
type orcaOps interface {
	Terminals(ctx context.Context) ([]orcaTerm, *orcacmd.Failure)
	Screen(ctx context.Context, handle string) (source string, lines []string, f *orcacmd.Failure)
	Send(ctx context.Context, handle, text string) (orcacmd.Receipt, *orcacmd.Failure)
}

type clientOps struct{ c *orcacmd.Client }

func (o clientOps) Terminals(ctx context.Context) ([]orcaTerm, *orcacmd.Failure) {
	out, f := o.c.Run(ctx, "terminal list", "terminal", "list", "--json")
	if f != nil {
		return nil, f
	}
	var env struct {
		Result *struct {
			Terminals []struct {
				Handle        string `json:"handle"`
				WorktreePath  string `json:"worktreePath"`
				Title         string `json:"title"`
				TabID         string `json:"tabId"`
				LeafID        string `json:"leafId"`
				AgentIdentity string `json:"agentIdentity"`
			} `json:"terminals"`
		} `json:"result"`
	}
	if f := orcacmd.DecodeJSON("terminal list", out, &env); f != nil {
		return nil, f
	}
	if env.Result == nil {
		return nil, &orcacmd.Failure{Reason: orcacmd.ReasonOutputUnrecognized, Verb: "terminal list"}
	}
	rows, f := o.c.Ps(ctx)
	if f != nil {
		return nil, f
	}
	state := map[string]orcacmd.PsAgent{}
	for _, r := range rows {
		for _, a := range r.Agents {
			state[a.PaneKey] = a
		}
	}
	var terms []orcaTerm
	for _, t := range env.Result.Terminals {
		if !orcacmd.ValidHandle(t.Handle) {
			continue
		}
		a := state[t.TabID+":"+t.LeafID]
		agent := a.AgentType
		if agent == "" {
			agent = t.AgentIdentity
		}
		terms = append(terms, orcaTerm{Handle: t.Handle, Path: t.WorktreePath, Title: redact.Control(t.Title), AgentType: agent, State: a.State})
	}
	return terms, nil
}

func (o clientOps) Screen(ctx context.Context, handle string) (string, []string, *orcacmd.Failure) {
	out, f := o.c.Run(ctx, "terminal read", "terminal", "read", "--terminal", handle, "--screen", "--json")
	if f != nil {
		return "", nil, f
	}
	var env struct {
		Result *struct {
			Terminal *struct {
				Source string   `json:"source"`
				Tail   []string `json:"tail"`
			} `json:"terminal"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &env) != nil || env.Result == nil || env.Result.Terminal == nil {
		return "", nil, &orcacmd.Failure{Reason: orcacmd.ReasonOutputUnrecognized, Verb: "terminal read"}
	}
	return env.Result.Terminal.Source, env.Result.Terminal.Tail, nil
}

func (o clientOps) Send(ctx context.Context, handle, text string) (orcacmd.Receipt, *orcacmd.Failure) {
	out, f := o.c.Run(ctx, "terminal send", "terminal", "send", "--terminal", handle, "--text", text, "--enter", "--wait-submit", "10", "--json")
	if f != nil {
		return orcacmd.Receipt{}, f
	}
	return orcacmd.ParseReceipt(out)
}

// screenLines caps a screen read at 40 lines.
const screenLines = 40

// ScreenTail reads a terminal's current screen. The read must report source=screen
// (the default read is the accumulated stream).
func ScreenTail(ctx context.Context, ops orcaOps, handle string) ([]string, *Degraded) {
	if !orcacmd.ValidHandle(handle) {
		return nil, &Degraded{Source: "orca", Reason: "terminal_invalid"}
	}
	source, lines, f := ops.Screen(ctx, handle)
	if f != nil {
		return nil, &Degraded{Source: "orca", Reason: f.Reason, Detail: f.Detail}
	}
	if source != "screen" {
		return nil, &Degraded{Source: "orca", Reason: orcacmd.ReasonOutputUnrecognized, Detail: "terminal read source=" + redact.Control(source)}
	}
	// Redact the screen as one text, before the cap: a secret spans lines (a private
	// key), and a cut made first could leave a fragment no rule recognizes. Both
	// Text (secrets, high-entropy tokens) and Creds (URL user:password@host) run here
	// - a screen can carry either.
	lines = strings.Split(redact.Creds(redact.Text(strings.Join(lines, "\n"))), "\n")
	if len(lines) > screenLines {
		lines = lines[len(lines)-screenLines:]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = redact.Control(l)
	}
	return out, nil
}

// promptReady reports whether a Claude screen shows the empty input prompt and no
// dialog that a typed Enter would answer (spike 1.0 items 10 and 14).
func promptReady(lines []string) (bool, string) {
	idle := false
	for _, l := range lines {
		s := strings.TrimSpace(l)
		switch {
		case strings.Contains(s, "Do you want to proceed?"), strings.Contains(s, "Do you want to make this edit"):
			return false, "approval_prompt"
		case strings.Contains(s, "trust the files"), strings.Contains(s, "Do you trust"):
			return false, "trust_dialog"
		case strings.Contains(s, "queued messages"):
			return false, "input_queued"
		case s == "❯":
			idle = true
		}
	}
	if !idle {
		return false, "prompt_not_visible"
	}
	return true, ""
}

// transcriptSettled reports whether the transcript's last recognized record ends a
// turn and every tool_use has its result.
func transcriptSettled(recs []record) (bool, string) {
	if len(recs) == 0 {
		return false, "transcript_empty"
	}
	open := map[string]bool{}
	for _, r := range recs {
		switch r.turn.Kind {
		case "TOOL":
			if r.toolID != "" {
				open[r.toolID] = true
			}
		case "RESULT":
			delete(open, r.toolID)
		}
	}
	if len(open) > 0 {
		return false, "tool_running"
	}
	if !recs[len(recs)-1].end {
		return false, "turn_in_progress"
	}
	return true, ""
}

// sendLockDir is where the machine-wide per-target send locks live.
var sendLockDir = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lets", "locks")
}

// Seams for the send loop.
var (
	sendLockWait   = 15 * time.Second
	observeTimeout = 10 * time.Second
	pollEvery      = 500 * time.Millisecond
	sleep          = time.Sleep
)

// TellOutcome is what an Orca send proved.
type TellOutcome struct {
	Delivered bool
	Receipt   orcacmd.Receipt
	Observed  bool
	Reason    string // peer_not_ready | orca_handle_stale | an orca_* failure
	State     string // why not ready
	SentAt    string
	// Attempted is true once ops.Send was called at least once (the first try, or the
	// one retry after a stale handle) - the only thing that separates "refused before
	// typing" (a check() failure: false) from "typed, delivery unproven" (true).
	Attempted bool
}

// orcaTell sends text to target's Orca terminal, under a machine-wide per-target
// lock held from the first safety check through the observation: session ids are
// machine-wide, so every sender (any repo, the hub) contends on one lock. It types
// nothing unless the transcript is settled, the screen shows the idle prompt, and
// Orca reports the agent idle; it never resends.
func orcaTell(ctx context.Context, ops orcaOps, target Peer, transcript, msgid, text string) TellOutcome {
	if err := os.MkdirAll(sendLockDir(), 0o700); err != nil {
		return TellOutcome{Reason: "send_lock_failed", State: err.Error()}
	}
	lf, err := os.OpenFile(filepath.Join(sendLockDir(), "peer-send-"+target.Session+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return TellOutcome{Reason: "send_lock_failed", State: err.Error()}
	}
	defer func() { _ = lf.Close() }()
	if err := fsutil.TryLockFile(lf, time.Now().Add(sendLockWait)); err != nil {
		if errors.Is(err, fsutil.ErrLockBusy) {
			return TellOutcome{Reason: "peer_not_ready", State: "another_send_in_progress"}
		}
		return TellOutcome{Reason: "send_lock_failed", State: err.Error()}
	}
	defer func() { _ = fsutil.UnlockFile(lf) }()

	handle := target.TerminalID
	check := func() (bool, string) {
		recs, d := readAll(transcript)
		if d != nil {
			return false, d.Reason
		}
		if ok, why := transcriptSettled(recs); !ok {
			return false, why
		}
		terms, f := ops.Terminals(ctx)
		if f != nil {
			return false, f.Reason
		}
		found := false
		for _, t := range terms {
			if t.Handle == handle {
				found = true
				if t.State != "done" && t.State != "idle" {
					return false, "agent_" + nonEmpty(t.State, "state_unknown")
				}
			}
		}
		if !found {
			return false, "terminal_not_found"
		}
		lines, d := ScreenTail(ctx, ops, handle)
		if d != nil {
			return false, d.Reason
		}
		return promptReady(lines)
	}
	if ok, why := check(); !ok {
		return TellOutcome{Reason: "peer_not_ready", State: why}
	}
	sentAt := time.Now().UTC().Format(time.RFC3339Nano)
	attempted := true // set immediately before ops.Send: past this point, delivery is unproven, never "nothing was typed"
	rcpt, f := ops.Send(ctx, handle, text)
	if f != nil && f.Reason == orcacmd.ReasonHandleStale {
		// Re-join by terminal id (never by title) and re-check before the one retry.
		if ok, why := check(); !ok {
			return TellOutcome{Reason: "peer_not_ready", State: "after_stale_handle: " + why, Attempted: attempted}
		}
		sentAt = time.Now().UTC().Format(time.RFC3339Nano)
		attempted = true
		rcpt, f = ops.Send(ctx, handle, text)
	}
	if f != nil {
		return TellOutcome{Reason: f.Reason, State: f.Detail, SentAt: sentAt, Attempted: attempted}
	}
	out := TellOutcome{Delivered: rcpt.InputAccepted, Receipt: rcpt, SentAt: sentAt, Attempted: attempted}
	deadline := time.Now().Add(observeTimeout)
	for {
		if recs, d := readAll(transcript); d == nil {
			if hasInbound(recs, msgid) { // msgid is fresh per frame, so any copy is this send
				out.Observed = true
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		sleep(pollEvery)
	}
	return out
}

func hasInbound(recs []record, msgid string) bool {
	for _, r := range recs {
		if r.turn.Kind == "INBOUND" && r.header != nil && r.header.ID == msgid {
			return true
		}
	}
	return false
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Seams for Wait.
var (
	statMtime = func(path string) (time.Time, error) {
		fi, err := os.Stat(path)
		if err != nil {
			return time.Time{}, err
		}
		return fi.ModTime(), nil
	}
	clock = time.Now
)

// waitReply polls the transcript's mtime with a 1-5 s backoff and parses only on a
// change; satisfied needs an end-of-turn record after the INBOUND record of msgid.
func waitReply(ctx context.Context, transcript, msgid, sentAt string, timeout time.Duration) (bool, string) {
	deadline := clock().Add(timeout)
	var last time.Time
	backoff := time.Second
	for {
		if m, err := statMtime(transcript); err == nil && !m.Equal(last) {
			last = m
			if recs, d := readAll(transcript); d == nil {
				if after, ok := SinceMessage(recs, msgid, sentAt); ok {
					for _, r := range after {
						if r.end {
							return true, ""
						}
					}
				}
			}
		}
		if ctx.Err() != nil || !clock().Before(deadline) {
			return false, "timeout"
		}
		sleep(backoff)
		if backoff < 5*time.Second {
			backoff += time.Second
		}
	}
}
