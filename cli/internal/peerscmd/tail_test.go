//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTail_Sources(t *testing.T) {
	repo := repoWithLets(t, "orca")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {102, sidFable, "MAIN-FABLE", repo}})
	writeTranscript(t, home, repo, sidFable, userText("2026-09-15T10:00:00Z", "q"), assistantText("2026-09-15T10:00:01Z", "answer"), turnEnd("2026-09-15T10:00:02Z"))
	plantRole(t, repo, sidFable, "role: peer\npid: 102\norca_terminal: term_fable\nset: x\n")
	ops := &fakeOps{terms: []orcaTerm{{Handle: "term_fable", Path: repo, AgentType: "claude"}, {Handle: "term_codex", Path: repo, AgentType: "codex"}}, screen: idleScreen}
	useOrca(t, ops)

	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidFable})
	if err != nil || len(res.Turns) != 2 || len(res.Screen) == 0 || res.TerminalID != "term_fable" {
		t.Errorf("jsonl + screen: %+v %v", res, err)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, ToTerminal: "term_codex"})
	if len(res.Turns) != 0 || len(res.Screen) == 0 || !strings.Contains(res.Note, "no transcript") {
		t.Errorf("screen only: %+v", res)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidMain})
	if len(res.Screen) != 0 || len(res.Degraded) == 0 || res.Degraded[0].Reason != "transcript_not_found" {
		t.Errorf("no transcript, no terminal: %+v", res)
	}
	if _, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: "bbbbbbbb-1111-4111-8111-000000000009"}); err == nil || err.(*Error).Kind != "peer_not_found" {
		t.Errorf("unknown session: %v", err)
	}
}

func TestTail_TranscriptOnlyAndCountOnly(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	tool := map[string]any{"type": "assistant", "timestamp": "2026-09-15T10:00:05Z", "message": map[string]any{"role": "assistant", "content": []any{
		map[string]any{"type": "tool_use", "id": "s1", "name": "SendMessage", "input": map[string]any{"to": "W1", "message": header(msg1, sidWork) + "\nsecret-ish body"}},
	}}}
	writeTranscript(t, home, repo, sidMain, assistantText("2026-09-15T10:00:01Z", "hi"), tool, turnEnd("2026-09-15T10:00:06Z"))
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidMain, AddressedToSession: sidWork, CountOnly: true})
	if err != nil || res.AddressedToMe == nil || res.AddressedToMe.Count != 1 {
		t.Fatalf("count only: %+v %v", res, err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "secret-ish") || len(res.Turns) != 0 {
		t.Errorf("--count-only must carry no turn text: %s", b)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidMain})
	if len(res.Turns) != 2 || res.Screen != nil {
		t.Errorf("transcript only: %+v", res)
	}
}

// A reply read after a message is relayed whole: the default five turns would cut its
// beginning off, and whatever a limit leaves out is counted, never silent.
func TestTail_SinceMessageReadsTheWholeReply(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq")}
	for i := 0; i < 7; i++ {
		lines = append(lines, assistantText("2026-09-15T10:00:0"+string(rune('1'+i))+"Z", "part "+string(rune('1'+i))))
	}
	writeTranscript(t, home, repo, sidWork, append(lines, turnEnd("2026-09-15T10:00:09Z"))...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil || len(res.Turns) != 7 || res.Omitted != 0 || res.Turns[0].Text != "part 1" {
		t.Errorf("--since-message without --last returns the whole reply: %+v %v", res, err)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z", Last: 3})
	if len(res.Turns) != 3 || res.Omitted != 4 || res.Turns[2].Text != "part 7" {
		t.Errorf("an explicit --last keeps the newest and counts the rest: %+v", res)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork})
	if len(res.Turns) != 5 || res.Omitted != 3 {
		t.Errorf("plain tail: five turns, the older ones counted: %+v", res)
	}
}
