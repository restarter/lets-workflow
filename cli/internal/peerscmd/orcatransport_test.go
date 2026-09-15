//go:build unix

package peerscmd

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

const msg1 = "0123456789abcdef"

func settledTranscript(t *testing.T, home, cwd, sid string) string {
	return writeTranscript(t, home, cwd, sid, userText("2026-09-15T10:00:00Z", "hi"), assistantText("2026-09-15T10:00:01Z", "hello"), turnEnd("2026-09-15T10:00:02Z"))
}

func orcaPeer(repo string) Peer {
	return Peer{Session: sidFable, TerminalID: "term_fable", Cwd: repo, Send: "orca"}
}

func header(id, to string) string {
	return "[lets-peer id=" + id + " kind=ask from_sid=" + sidMain + " to_sid=" + to + ` from="peer/MAIN" to="MAIN-FABLE"]`
}

func TestOrcaTell_RefusesWhenNotSendSafe(t *testing.T) {
	fastLoops(t)
	home := t.TempDir()
	repo := t.TempDir()
	cases := []struct {
		name       string
		transcript []map[string]any
		state      string
		screen     []string
		want       string
	}{
		{"tool running", []map[string]any{userText("2026-09-15T10:00:00Z", "go"), toolUse("2026-09-15T10:00:01Z", "t1", "Bash")}, "done", idleScreen, "tool_running"},
		{"no prompt", []map[string]any{assistantText("2026-09-15T10:00:01Z", "x"), turnEnd("2026-09-15T10:00:02Z")}, "done", []string{"✻ Working…"}, "prompt_not_visible"},
		{"approval dialog", []map[string]any{assistantText("2026-09-15T10:00:01Z", "x"), turnEnd("2026-09-15T10:00:02Z")}, "done", []string{"Do you want to proceed?", "❯"}, "approval_prompt"},
		{"agent working", []map[string]any{assistantText("2026-09-15T10:00:01Z", "x"), turnEnd("2026-09-15T10:00:02Z")}, "working", idleScreen, "agent_working"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTranscript(t, home, repo, sidFable, c.transcript...)
			ops := &fakeOps{terms: []orcaTerm{{Handle: "term_fable", Path: repo, State: c.state}}, screen: c.screen}
			out := orcaTell(context.Background(), ops, orcaPeer(repo), path, msg1, header(msg1, sidFable)+"\nq")
			if out.Reason != "peer_not_ready" || out.State != c.want || len(ops.sends) != 0 {
				t.Errorf("%+v sends=%d, want peer_not_ready/%s and nothing typed", out, len(ops.sends), c.want)
			}
		})
	}
}

func TestOrcaTell_SendsObservesAndSerializes(t *testing.T) {
	fastLoops(t)
	home, repo := t.TempDir(), t.TempDir()
	path := settledTranscript(t, home, repo, sidFable)
	ops := &fakeOps{terms: []orcaTerm{{Handle: "term_fable", Path: repo, State: "done"}}, screen: idleScreen}
	var mu sync.Mutex
	ops.onSend = func(text string) { // the target records the inbound message, then starts a turn
		mu.Lock()
		defer mu.Unlock()
		f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		defer f.Close()
		f.WriteString(`{"type":"user","timestamp":"2026-09-15T10:01:00Z","message":{"role":"user","content":` + jsonString(text) + "}}\n")
	}
	out := orcaTell(context.Background(), ops, orcaPeer(repo), path, msg1, header(msg1, sidFable)+"\nq")
	if !out.Delivered || !out.Observed || len(ops.sends) != 1 {
		t.Fatalf("first send: %+v", out)
	}
	// the transcript now ends in a user record, so a second sender must re-check and refuse
	var wg sync.WaitGroup
	results := make([]TellOutcome, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = orcaTell(context.Background(), ops, orcaPeer(repo), path, "fedcba9876543210", header("fedcba9876543210", sidFable)+"\nq")
		}(i)
	}
	wg.Wait()
	if len(ops.sends) != 1 || results[0].Reason != "peer_not_ready" || results[1].Reason != "peer_not_ready" {
		t.Errorf("serialized senders re-check the target: sends=%d %+v", len(ops.sends), results)
	}
}

func TestOrcaTell_StaleHandleRechecksAndNotObserved(t *testing.T) {
	fastLoops(t)
	home, repo := t.TempDir(), t.TempDir()
	path := settledTranscript(t, home, repo, sidFable)
	ops := &fakeOps{terms: []orcaTerm{{Handle: "term_fable", Path: repo, State: "done"}}, screen: idleScreen,
		sendResult: []*orcacmd.Failure{{Reason: orcacmd.ReasonHandleStale, Verb: "terminal send"}}}
	out := orcaTell(context.Background(), ops, orcaPeer(repo), path, msg1, header(msg1, sidFable)+"\nq")
	if !out.Delivered || out.Observed || ops.termsCalls != 2 {
		t.Errorf("stale handle: re-check (terminals listed twice) then one retry; nothing observed: %+v calls=%d", out, ops.termsCalls)
	}
}

func TestScreenTail_RejectsStream(t *testing.T) {
	if _, d := ScreenTail(context.Background(), &fakeOps{source: "stream"}, "term_x"); d == nil || d.Reason != orcacmd.ReasonOutputUnrecognized {
		t.Errorf("source=stream must be refused: %+v", d)
	}
	lines, d := ScreenTail(context.Background(), &fakeOps{screen: []string{"a\x1b[31mb", "token ghp_abcdefghijklmnopqrstuvwxyz0123"}}, "term_x")
	if d != nil || strings.ContainsRune(lines[0], 0x1b) || strings.Contains(lines[1], "ghp_") {
		t.Errorf("screen lines are redacted: %q %+v", lines, d)
	}
}

func TestOrcaWait_SatisfiedOnlyAfterEndOfTurn(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	path := writeTranscript(t, home, repo, sidFable, userText("2026-09-15T10:01:00Z", header(msg1, sidFable)+"\nq"), assistantText("2026-09-15T10:01:05Z", "thinking about it"))
	steps := 0
	stats := 0
	oldSleep, oldStat, oldClock := sleep, statMtime, clock
	fake := time.Date(2026, 9, 15, 10, 2, 0, 0, time.UTC)
	sleep = func(d time.Duration) {
		steps++
		fake = fake.Add(d)
		if steps == 2 {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			f.WriteString(`{"type":"system","subtype":"turn_duration","timestamp":"2026-09-15T10:01:09Z"}` + "\n")
			f.Close()
		}
	}
	statMtime = func(p string) (time.Time, error) { stats++; return fake, nil }
	clock = func() time.Time { return fake }
	t.Cleanup(func() { sleep, statMtime, clock = oldSleep, oldStat, oldClock })
	ok, reason := waitReply(context.Background(), path, msg1, "2026-09-15T10:00:59Z", time.Minute)
	if !ok || reason != "" {
		t.Errorf("satisfied after the end-of-turn record: %v %q", ok, reason)
	}
	if stats > steps+1 {
		t.Errorf("mtime polled %d times over %d backoff steps: busy loop", stats, steps)
	}
	steps = 0
	path2 := writeTranscript(t, home, repo, sidMain, userText("2026-09-15T10:01:00Z", header(msg1, sidMain)+"\nq"))
	if ok, reason := waitReply(context.Background(), path2, msg1, "2026-09-15T10:00:59Z", 3*time.Second); ok || reason != "timeout" {
		t.Errorf("no end of turn: %v %q", ok, reason)
	}
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
