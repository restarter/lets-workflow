package peerscmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	sidA = "11111111-1111-4111-8111-111111111111"
	sidB = "22222222-2222-4222-8222-222222222222"
	sidC = "33333333-3333-4333-8333-333333333333"
)

func fixturePath(name string) string { return filepath.Join("testdata", "transcripts", name) }

func TestTranscript_Kinds(t *testing.T) {
	recs, d := readAll(fixturePath("basic.jsonl"))
	if d != nil {
		t.Fatal(d)
	}
	var kinds []string
	for _, r := range recs {
		kinds = append(kinds, r.turn.Kind)
	}
	want := "TEXT TEXT TOOL RESULT TOOL TOOL END TEXT INBOUND TEXT END"
	if got := strings.Join(kinds, " "); got != want {
		t.Fatalf("kinds:\n got %s\nwant %s", got, want)
	}
	if recs[3].turn.Tool != "Bash" || !strings.Contains(recs[3].turn.Text, "all tests passed") {
		t.Errorf("result: %+v", recs[3].turn)
	}
	if recs[8].header == nil || recs[8].header.ID != "dddddddddddddddd" {
		t.Errorf("inbound header after a cross-session wrapper: %+v", recs[8].header)
	}
}

func TestTranscript_SinceMessage(t *testing.T) {
	recs, _ := readAll(fixturePath("basic.jsonl"))
	after, ok := SinceMessage(recs, "dddddddddddddddd", "2026-09-15T10:00:09Z")
	if !ok || len(after) != 2 || after[0].turn.Text != "Checked X: fine." || !after[1].end {
		t.Fatalf("since: ok=%v %+v", ok, after)
	}
	if _, ok := SinceMessage(recs, "dddddddddddddddd", "2026-09-15T10:00:11Z"); ok {
		t.Error("a message older than sentAt must not anchor")
	}
	if _, ok := SinceMessage(recs, "cccccccccccccccc", "2026-09-15T10:00:00Z"); ok {
		t.Error("a header quoted inside text must not anchor")
	}
}

func TestTranscript_AddressedTo(t *testing.T) {
	recs, _ := readAll(fixturePath("basic.jsonl"))
	got := AddressedTo(recs, sidB)
	if len(got) != 1 || got[0].header.ID != "aaaaaaaaaaaaaaaa" {
		t.Errorf("to MAIN-PWA must not match the message to MAIN-PWA-LIC: %+v", got)
	}
}

func TestTranscript_Secrets(t *testing.T) {
	recs, d := readAll(fixturePath("secrets.jsonl"))
	if d != nil {
		t.Fatal(d)
	}
	all, _ := json.Marshal(turnsOf(recs, textCapDefault))
	for _, leak := range []string{"hunter2", "ghp_abcdefghijklmnopqrstuvwxyz0123", "MIIE"} {
		if strings.Contains(string(all), leak) {
			t.Errorf("leaked %q: %s", leak, all)
		}
	}
	if !strings.Contains(string(all), "[redacted:env-like source]") {
		t.Errorf("cat .env result not withheld: %s", all)
	}
}

func TestTranscript_UnknownShape(t *testing.T) {
	if _, d := readAll(fixturePath("unknown-shape.jsonl")); d == nil || d.Reason != "transcript_format_unknown" {
		t.Errorf("unknown shape: %+v", d)
	}
}

func TestTranscript_LocateDirectBeforeGlob(t *testing.T) {
	claude := t.TempDir()
	cwd := "/Users/x/my repo.v2"
	dir := filepath.Join(claude, "projects", "-Users-x-my-repo-v2")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, sidA+".jsonl"), []byte("{}\n"), 0o600)
	globbed := false
	old := globTranscripts
	globTranscripts = func(p string) ([]string, error) { globbed = true; return old(p) }
	t.Cleanup(func() { globTranscripts = old })
	if p, d := LocateTranscript(claude, cwd, sidA); d != nil || !strings.HasSuffix(p, sidA+".jsonl") || globbed {
		t.Errorf("direct: %q %+v globbed=%v", p, d, globbed)
	}
	other := filepath.Join(claude, "projects", "-elsewhere")
	_ = os.MkdirAll(other, 0o700)
	_ = os.WriteFile(filepath.Join(other, sidB+".jsonl"), []byte("{}\n"), 0o600)
	if p, d := LocateTranscript(claude, cwd, sidB); d != nil || !strings.Contains(p, "-elsewhere") || !globbed {
		t.Errorf("glob fallback: %q %+v", p, d)
	}
	if _, d := LocateTranscript(claude, cwd, "../../etc"); d == nil {
		t.Error("an invalid sid must be refused")
	}
}

func TestTranscript_LargeTailUnderCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	f, _ := os.Create(path)
	filler, _ := json.Marshal(map[string]any{"type": "progress", "data": strings.Repeat("x", 1000)})
	for written := 0; written < 10<<20; written += len(filler) + 1 {
		_, _ = f.Write(filler)
		_, _ = f.Write([]byte("\n"))
	}
	for i := 0; i < 7; i++ {
		b, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": "2026-09-15T12:00:00Z", "message": map[string]any{"role": "assistant", "content": "turn " + itoaInt(i)}})
		_, _ = f.Write(b)
		_, _ = f.Write([]byte("\n"))
	}
	_ = f.Close()
	bytesRead.Store(0)
	turns, d := TailTurns(path, 5)
	if d != nil || len(turns) != 5 || turns[4].Text != "turn 6" || turns[0].Text != "turn 2" {
		t.Fatalf("tail: %+v %+v", turns, d)
	}
	if bytesRead.Load() > maxTailBytes+chunkSize {
		t.Errorf("read %d bytes, cap is %d + one chunk", bytesRead.Load(), maxTailBytes)
	}
}

// A peer message delivered while the receiver is mid-turn is an attachment
// (queued_command, origin.kind peer) between two queue-operation records: it reads
// as exactly ONE inbound turn carrying its header (lead's transcript, smoke §3,
// 2026-09-26; ids and paths are fixture values).
func TestTranscript_MidTurnQueuedCommandIsInbound(t *testing.T) {
	recs, d := readAll(fixturePath("delivered-midturn.jsonl"))
	if d != nil {
		t.Fatal(d)
	}
	if len(recs) != 1 || recs[0].turn.Kind != "INBOUND" || recs[0].turn.Role != "user" || recs[0].turn.TS != "2026-09-26T09:10:02.000Z" {
		t.Fatalf("records: %+v", recs)
	}
	if h := recs[0].header; h == nil || h.ID != midTurnID || h.FromSID != sidMain || h.ToSID != sidWork {
		t.Errorf("header: %+v", recs[0].header)
	}
	// no origin.body: the prompt carries the wrapper, and the header still parses
	line := []byte(`{"type":"attachment","timestamp":"2026-09-26T09:10:02.000Z","attachment":{"type":"queued_command","prompt":"<cross-session-message from=\"uds:/tmp/cc-socks/1.sock\">\n[lets-peer id=` + midTurnID + ` kind=ask from_sid=` + sidMain + ` to_sid=` + sidWork + ` from=\"W1\" to=\"MAIN\"]\nq","origin":{"kind":"peer"}}}`)
	if r, ok := parseLine(line); !ok || len(r) != 1 || r[0].header == nil || r[0].header.ID != midTurnID {
		t.Errorf("prompt fallback: %+v ok=%v", r, ok)
	}
}

func TestTranscript_OtherAttachmentIgnored(t *testing.T) {
	for _, line := range []string{
		`{"type":"attachment","timestamp":"t","attachment":{"type":"file","prompt":"[lets-peer id=` + midTurnID + ` kind=ask]"}}`,
		`{"type":"attachment","timestamp":"t","attachment":{"type":"queued_command","prompt":"[lets-peer id=` + midTurnID + ` kind=ask]","origin":{"kind":"user","body":"[lets-peer id=` + midTurnID + ` kind=ask]"}}}`,
		`{"type":"attachment","timestamp":"t","attachment":{"type":"queued_command","prompt":"hi"}}`,
		`{"type":"queue-operation","operation":"enqueue","timestamp":"t","content":"<cross-session-message>[lets-peer id=` + midTurnID + ` kind=ask]"}`,
	} {
		if r, ok := parseLine([]byte(line)); ok {
			t.Errorf("must be ignored: %s -> %+v", line, r)
		}
	}
}

const midTurnID = "1a2b3c4d5e6f7081"
