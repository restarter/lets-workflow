//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// TestTail_AddressedTurnKeepsMoreThan2K: a reply turn well over 2 KiB survives whole
// when read with --since-message (the message-cap path - see Task 6's textCapMessage),
// while the identical transcript read plain is cut at the default 2 KiB cap.
func TestTail_AddressedTurnKeepsMoreThan2K(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	big := strings.Repeat("x", 6<<10) // 6 KiB: over the 2 KiB default, under the 16 KiB message cap
	writeTranscript(t, home, repo, sidWork,
		userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq"),
		assistantText("2026-09-15T10:00:01Z", big),
		turnEnd("2026-09-15T10:00:02Z"))
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil || len(res.Turns) != 1 || res.Turns[0].Text != big || res.Turns[0].TruncatedBytes != 0 || res.TruncatedBytes != 0 {
		t.Fatalf("--since-message keeps a reply whole up to 16 KiB: %+v %v", res, err)
	}
	res, err = Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork})
	if err != nil || len(res.Turns) == 0 {
		t.Fatalf("plain tail: %+v %v", res, err)
	}
	last := res.Turns[len(res.Turns)-1]
	if last.TruncatedBytes == 0 || len(last.Text) >= len(big) || res.TruncatedBytes == 0 {
		t.Errorf("plain tail must cut the same turn at the default 2 KiB cap: %+v total=%d", last, res.TruncatedBytes)
	}
}

// TestTail_TruncatedBytesCountsSourceBytes: the counter is exact source bytes lost -
// not the marker's own length - and a turn carries exactly one truncation marker.
func TestTail_TruncatedBytesCountsSourceBytes(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}})
	const size = 5000 // over the 2 KiB default cap
	writeTranscript(t, home, repo, sidMain, assistantText("2026-09-15T10:00:00Z", strings.Repeat("y", size)), turnEnd("2026-09-15T10:00:01Z"))
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidMain})
	if err != nil || len(res.Turns) != 1 {
		t.Fatalf("tail: %+v %v", res, err)
	}
	turn := res.Turns[0]
	if turn.TruncatedBytes != size-textCapDefault {
		t.Errorf("truncated_bytes must be exact source bytes lost: got %d, want %d", turn.TruncatedBytes, size-textCapDefault)
	}
	if got := strings.Count(turn.Text, "…[truncated"); got != 1 {
		t.Errorf("exactly one truncation marker, got %d: %q", got, turn.Text)
	}
	if res.TruncatedBytes != turn.TruncatedBytes {
		t.Errorf("the result total must equal the one turn's cut: %d vs %d", res.TruncatedBytes, turn.TruncatedBytes)
	}
}

// TestTail_CallCeiling: ten 12 KiB turns (each under the 16 KiB per-turn cap, so the
// per-turn cap alone would let all ten through) must still be trimmed by the 64 KiB
// call ceiling - the turn that breaks the budget is dropped, not kept.
func TestTail_CallCeiling(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq")}
	turnText := strings.Repeat("z", 12<<10)
	for i := 0; i < 10; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+1), turnText))
	}
	writeTranscript(t, home, repo, sidWork, append(lines, turnEnd("2026-09-15T10:00:20Z"))...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	var sum int
	for _, tn := range res.Turns {
		sum += len(tn.Text)
	}
	if sum > callCapBytes {
		t.Errorf("the returned turns must sum to at most the call ceiling: %d > %d", sum, callCapBytes)
	}
	if res.Omitted == 0 || res.TruncatedBytes == 0 {
		t.Errorf("the call ceiling must drop turns and count them: omitted=%d truncated=%d", res.Omitted, res.TruncatedBytes)
	}
	if len(res.Turns) == 0 || res.Turns[len(res.Turns)-1].Text != turnText {
		t.Error("the newest turn must never be dropped by the ceiling")
	}
}

// TestTail_CallCeilingCountsDroppedTurnsOwnTruncation: a turn the call ceiling drops
// may have ALSO lost bytes to the per-turn cap before it ever got there - the counter
// must report the turn's EXACT original source size, neither under (missing the
// per-turn cut, FIX B) nor over (adding redact.Cap's own marker length on top of an
// already-capped turn, FIX E).
func TestTail_CallCeilingCountsDroppedTurnsOwnTruncation(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq")}
	const oldestSize = 40 << 10 // well over the 16 KiB per-turn message cap
	lines = append(lines, assistantText("2026-09-15T10:00:01Z", strings.Repeat("o", oldestSize)))
	for i := 0; i < 4; i++ {
		// under the per-turn cap: these four alone (64000 B) plus the oldest turn's
		// capped remainder push the running sum past the 64 KiB ceiling.
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+2), strings.Repeat("n", 16000)))
	}
	writeTranscript(t, home, repo, sidWork, append(lines, turnEnd("2026-09-15T10:00:10Z"))...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if res.Omitted == 0 {
		t.Fatalf("the oldest, largest turn must be dropped by the call ceiling: %+v", res)
	}
	for _, tn := range res.Turns {
		if strings.HasPrefix(tn.Text, "oooo") {
			t.Fatalf("the dropped turn must not be among those returned: %+v", res.Turns)
		}
	}
	if res.TruncatedBytes != oldestSize {
		t.Errorf("a dropped turn's own per-turn truncation must be counted EXACTLY - not its kept remainder alone (undercounts), not that plus redact.Cap's own marker (overcounts): truncated_bytes=%d, want exactly %d", res.TruncatedBytes, oldestSize)
	}
}

// TestTail_OmittedStillCountsDroppedTurns: the pre-existing whole-turn --last
// behaviour is unchanged by the byte-ceiling addition, and short turns under both
// caps report no truncated_bytes.
func TestTail_OmittedStillCountsDroppedTurns(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}})
	var recs []map[string]any
	for i := 0; i < 5; i++ {
		recs = append(recs, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i), fmt.Sprintf("turn %d", i)))
	}
	writeTranscript(t, home, repo, sidMain, append(recs, turnEnd("2026-09-15T10:00:10Z"))...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidMain, Last: 2})
	if err != nil || len(res.Turns) != 2 || res.Omitted != 3 || res.Turns[1].Text != "turn 4" {
		t.Fatalf("a --last trim must still count what it left out: %+v %v", res, err)
	}
	if res.TruncatedBytes != 0 {
		t.Errorf("short turns under both caps must not report truncated_bytes: %d", res.TruncatedBytes)
	}
}

func TestTail_ByName(t *testing.T) {
	root := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{1, sidM, "MAIN", root}, {2, sidN, "DUP", root}, {3, sidW, "DUP", root}})
	writeTranscript(t, home, root, sidM, userText("2026-09-22T10:00:00Z", "hi"), assistantText("2026-09-22T10:00:01Z", "hello"))
	if res, err := Tail(context.Background(), TailOptions{Cwd: root, Name: "MAIN"}); err != nil || !res.OK || len(res.Turns) == 0 {
		t.Fatalf("tail MAIN: err=%v %+v", err, res)
	}
	if _, err := Tail(context.Background(), TailOptions{Cwd: root, Name: "NOPE"}); err == nil {
		t.Error("an unknown name must be refused")
	}
	if res, _ := Tail(context.Background(), TailOptions{Cwd: root, Name: "DUP"}); res.Error == nil || res.Error.Kind != "peer_ambiguous" {
		t.Errorf("a shared name must be ambiguous: %+v", res)
	}
	if _, err := Tail(context.Background(), TailOptions{Cwd: root, Name: "MAIN", ToSession: sidM}); err == nil {
		t.Error("a name and a session id together must be refused")
	}
}

// A reply to a message delivered through SendMessage: the receiver's transcript
// holds Claude Code's delivered record (prefix line, wrapper, header), and
// --since-message finds it - before lets-rry3c it stayed "not seen yet".
func TestTail_SinceMessageSeesDeliveredSendMessage(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	writeTranscript(t, home, repo, sidWork,
		deliveredLine(t),
		assistantText("2026-09-25T12:15:30Z", "the answer"),
		turnEnd("2026-09-25T12:15:31Z"))
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: deliveredID, SentAt: "2026-09-25T12:15:21Z"})
	if err != nil || len(res.Turns) != 1 || res.Turns[0].Text != "the answer" {
		t.Errorf("--since-message must see the delivered message and return the reply: %+v %v", res, err)
	}
}

// A reply to a message delivered mid-turn: the queued_command attachment, wrapped in
// its queue-operation enqueue / remove records, anchors --since-message exactly once.
func TestTail_SinceMessageSeesMidTurnDelivery(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	b, err := os.ReadFile(fixturePath("delivered-midturn.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, m)
	}
	lines = append(lines, assistantText("2026-09-26T09:10:05Z", "the mid-turn answer"), turnEnd("2026-09-26T09:10:06Z"))
	writeTranscript(t, home, repo, sidWork, lines...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, SinceMessage: midTurnID, SentAt: "2026-09-26T09:10:00Z"})
	if err != nil || len(res.Turns) != 1 || res.Turns[0].Text != "the mid-turn answer" {
		t.Fatalf("--since-message must see the mid-turn delivery: %+v %v", res, err)
	}
	all, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, Last: 20})
	if err != nil {
		t.Fatal(err)
	}
	inbound := 0
	for _, tr := range all.Turns {
		if tr.Kind == "INBOUND" {
			inbound++
		}
	}
	if inbound != 1 {
		t.Errorf("the queue-operation records must not double the message: %d inbound turns", inbound)
	}
}
