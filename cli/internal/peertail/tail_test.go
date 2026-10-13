package peertail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tailCwd = "/fixture/repo"

func writeTranscript(t *testing.T, home, cwd, sid string, lines ...map[string]any) {
	t.Helper()
	dir := filepath.Join(home, "projects", transcriptSlug(cwd))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func userText(ts, text string) map[string]any {
	return map[string]any{"type": "user", "timestamp": ts, "message": map[string]any{"role": "user", "content": text}}
}

func assistantText(ts, text string) map[string]any {
	return map[string]any{"type": "assistant", "timestamp": ts, "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}}}
}

func turnEnd(ts string) map[string]any {
	return map[string]any{"type": "system", "subtype": "turn_duration", "timestamp": ts}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestRead_TranscriptNotFound(t *testing.T) {
	res := Read(Options{ClaudeDir: t.TempDir(), Cwd: tailCwd, Session: sidA})
	if len(res.Turns) != 0 || len(res.Degraded) != 1 || res.Degraded[0].Reason != "transcript_not_found" {
		t.Errorf("no transcript: %+v", res)
	}
	if res.Turns == nil {
		t.Error("Turns must be an empty slice, never nil")
	}
}

// A reply read with Since is relayed whole: the default five turns would cut its
// beginning off, and whatever a limit leaves out is counted, never silent.
func TestRead_SinceReadsTheWholeReply(t *testing.T) {
	home := t.TempDir()
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", "q")}
	for i := 0; i < 7; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+1), fmt.Sprintf("part %d", i+1)))
	}
	writeTranscript(t, home, tailCwd, sidB, append(lines, turnEnd("2026-09-15T10:00:09Z"))...)
	o := Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, "2026-09-15T10:00:01Z")}
	res := Read(o)
	if len(res.Turns) != 7 || res.Omitted != 0 || res.Turns[0].Text != "part 1" {
		t.Errorf("Since without Last returns the whole reply: %+v", res)
	}
	o.Last = 3
	res = Read(o)
	if len(res.Turns) != 3 || res.Omitted != 4 || res.Turns[2].Text != "part 7" {
		t.Errorf("an explicit Last keeps the newest and counts the rest: %+v", res)
	}
	res = Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB})
	if len(res.Turns) != 5 || res.Omitted != 3 {
		t.Errorf("plain read: five turns, the older ones counted: %+v", res)
	}
}

// Since compares instants, never strings: fractional seconds and offsets break
// lexical order, and a record with no or a malformed timestamp is kept only without
// Since.
func TestRead_SinceComparesInstants(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, tailCwd, sidB,
		assistantText("2026-10-12T11:59:59.9Z", "before"),
		assistantText("2026-10-12T13:59:59+02:00", "before, offset"), // 11:59:59Z, lexically after 12:00:00Z
		assistantText("not-a-time", "malformed"),
		assistantText("", "no timestamp"),
		assistantText("2026-10-12T12:00:00.5Z", "fractional"),
		assistantText("2026-10-12T14:00:00.5+02:00", "offset"),
	)
	for _, bound := range []string{"2026-10-12T12:00:00Z", "2026-10-12T14:00:00+02:00"} {
		res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, bound)})
		var got []string
		for _, tr := range res.Turns {
			got = append(got, tr.Text)
		}
		if strings.Join(got, "|") != "fractional|offset" {
			t.Errorf("bound %s: got %q", bound, got)
		}
	}
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Last: 10})
	if len(res.Turns) != 6 {
		t.Errorf("without Since every turn is kept, malformed timestamps included: %+v", res.Turns)
	}
}

// A turn well over 2 KiB survives whole when read with Since (the message cap),
// while the identical transcript read plain is cut at the default 2 KiB cap.
func TestRead_SinceTurnKeepsMoreThan2K(t *testing.T) {
	home := t.TempDir()
	big := strings.Repeat("x", 6<<10) // over the 2 KiB default, under the 16 KiB message cap
	writeTranscript(t, home, tailCwd, sidB,
		userText("2026-09-15T10:00:00Z", "q"),
		assistantText("2026-09-15T10:00:01Z", big),
		turnEnd("2026-09-15T10:00:02Z"))
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, "2026-09-15T10:00:01Z")})
	if len(res.Turns) != 1 || res.Turns[0].Text != big || res.Turns[0].TruncatedBytes != 0 || res.TruncatedBytes != 0 {
		t.Fatalf("Since keeps a reply whole up to 16 KiB: %+v", res)
	}
	res = Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB})
	if len(res.Turns) == 0 {
		t.Fatalf("plain read: %+v", res)
	}
	last := res.Turns[len(res.Turns)-1]
	if last.TruncatedBytes == 0 || len(last.Text) >= len(big) || res.TruncatedBytes == 0 {
		t.Errorf("plain read must cut the same turn at the default 2 KiB cap: %+v total=%d", last, res.TruncatedBytes)
	}
}

// The counter is exact source bytes lost - not the marker's own length - and a turn
// carries exactly one truncation marker.
func TestRead_TruncatedBytesCountsSourceBytes(t *testing.T) {
	home := t.TempDir()
	const size = 5000 // over the 2 KiB default cap
	writeTranscript(t, home, tailCwd, sidA, assistantText("2026-09-15T10:00:00Z", strings.Repeat("y", size)), turnEnd("2026-09-15T10:00:01Z"))
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidA})
	if len(res.Turns) != 1 {
		t.Fatalf("read: %+v", res)
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

// Ten 12 KiB turns (each under the 16 KiB per-turn cap) must still be trimmed by the
// 64 KiB call ceiling - the turn that breaks the budget is dropped, not kept.
func TestRead_CallCeiling(t *testing.T) {
	home := t.TempDir()
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", "q")}
	turnText := strings.Repeat("z", 12<<10)
	for i := 0; i < 10; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+1), turnText))
	}
	writeTranscript(t, home, tailCwd, sidB, append(lines, turnEnd("2026-09-15T10:00:20Z"))...)
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, "2026-09-15T09:59:59Z")})
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

// A turn the call ceiling drops may ALSO have lost bytes to the per-turn cap: the
// counter reports the turn's EXACT original source size (srcLen), neither under (the
// kept remainder alone) nor over (plus redact.Cap's own marker).
func TestRead_CallCeilingCountsDroppedTurnsSrcLen(t *testing.T) {
	home := t.TempDir()
	const oldestSize = 40 << 10 // well over the 16 KiB per-turn message cap
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", "q"), assistantText("2026-09-15T10:00:01Z", strings.Repeat("o", oldestSize))}
	for i := 0; i < 4; i++ {
		// under the per-turn cap: these four alone (64000 B) plus the oldest turn's
		// capped remainder push the running sum past the 64 KiB ceiling.
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+2), strings.Repeat("n", 16000)))
	}
	writeTranscript(t, home, tailCwd, sidB, append(lines, turnEnd("2026-09-15T10:00:10Z"))...)
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, "2026-09-15T10:00:01Z")})
	if res.Omitted == 0 {
		t.Fatalf("the oldest, largest turn must be dropped by the call ceiling: %+v", res)
	}
	for _, tn := range res.Turns {
		if strings.HasPrefix(tn.Text, "oooo") {
			t.Fatalf("the dropped turn must not be among those returned: %+v", res.Turns)
		}
	}
	if res.TruncatedBytes != oldestSize {
		t.Errorf("a dropped turn counts its exact source size: truncated_bytes=%d, want %d", res.TruncatedBytes, oldestSize)
	}
}

// The whole-turn Last trim still counts what it leaves out, and short turns under
// both caps report no truncated_bytes.
func TestRead_OmittedStillCountsDroppedTurns(t *testing.T) {
	home := t.TempDir()
	var recs []map[string]any
	for i := 0; i < 5; i++ {
		recs = append(recs, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i), fmt.Sprintf("turn %d", i)))
	}
	writeTranscript(t, home, tailCwd, sidA, append(recs, turnEnd("2026-09-15T10:00:10Z"))...)
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidA, Last: 2})
	if len(res.Turns) != 2 || res.Omitted != 3 || res.Turns[1].Text != "turn 4" {
		t.Fatalf("a Last trim must still count what it left out: %+v", res)
	}
	if res.TruncatedBytes != 0 {
		t.Errorf("short turns under both caps must not report truncated_bytes: %d", res.TruncatedBytes)
	}
}

// A native delivery and its reply: Since from the send time returns the INBOUND turn
// with its sender's name, then the answer.
func TestRead_SinceSeesNativeDeliveryAndReply(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, home, tailCwd, sidB,
		assistantText("2026-10-12T12:00:00Z", "earlier work"),
		userText("2026-10-12T12:00:05.250Z", "Another Claude session sent a message:\n<cross-session-message from=\"uds:/x\" from-name=\"MAIN-LETS\">\nstatus?\n</cross-session-message>"),
		assistantText("2026-10-12T12:00:06Z", "the answer"),
		turnEnd("2026-10-12T12:00:07Z"))
	res := Read(Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, Since: mustTime(t, "2026-10-12T12:00:05Z")})
	if len(res.Turns) != 2 || res.Turns[0].Kind != "INBOUND" || res.Turns[0].From != "MAIN-LETS" || res.Turns[1].Text != "the answer" {
		t.Fatalf("native delivery + reply: %+v", res)
	}
	b, _ := json.Marshal(res.Turns[0])
	if !strings.Contains(string(b), `"from":"MAIN-LETS"`) {
		t.Errorf("from must be on the JSON surface: %s", b)
	}
}

// The compatibility reply read: only the turns after the message with that msgid, at
// or after SentAt, the whole reply; not seen yet is no turns and a note - never a
// time-only fallback.
func TestRead_SinceMessage(t *testing.T) {
	home := t.TempDir()
	const id = "0123456789abcdef"
	head := "[lets-peer id=" + id + " kind=ask from_sid=" + sidMain + " to_sid=" + sidWork + ` from="peer/MAIN" to="W1"]`
	lines := []map[string]any{
		assistantText("2026-09-15T10:00:00Z", "before the question"),
		userText("2026-09-15T10:00:01Z", "Another Claude session sent a message:\n<cross-session-message from=\"uds:/x\" from-name=\"MAIN\">\n"+head+"\nq\n</cross-session-message>"),
	}
	for i := 0; i < 7; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+2), fmt.Sprintf("part %d", i+1)))
	}
	writeTranscript(t, home, tailCwd, sidB, append(lines, turnEnd("2026-09-15T10:00:20Z"))...)
	o := Options{ClaudeDir: home, Cwd: tailCwd, Session: sidB, SinceMessage: id, SentAt: "2026-09-15T10:00:00Z"}
	res := Read(o)
	if len(res.Turns) != 7 || res.Turns[0].Text != "part 1" || res.Omitted != 0 || res.Note != "" {
		t.Errorf("the whole reply: %+v", res)
	}
	o.Last = 3
	if res := Read(o); len(res.Turns) != 3 || res.Omitted != 4 || res.Turns[2].Text != "part 7" {
		t.Errorf("Last keeps the newest and counts the rest: %+v", res)
	}
	for _, miss := range []Options{
		{ClaudeDir: home, Cwd: tailCwd, Session: sidB, SinceMessage: "fedcba9876543210", SentAt: "2026-09-15T09:00:00Z"},
		{ClaudeDir: home, Cwd: tailCwd, Session: sidB, SinceMessage: id, SentAt: "2026-09-15T10:00:02Z"}, // sent after it was seen
		{ClaudeDir: home, Cwd: tailCwd, Session: sidB, SinceMessage: id, SentAt: "not a time"},
	} {
		if res := Read(miss); len(res.Turns) != 0 || res.Note != "message not seen yet" {
			t.Errorf("%+v: %+v", miss, res)
		}
	}
}
