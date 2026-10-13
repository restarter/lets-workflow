package peertail

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
	// the delivered fixtures' sessions
	sidMain = "aaaaaaaa-1111-4111-8111-000000000001"
	sidWork = "aaaaaaaa-1111-4111-8111-000000000003"

	midTurnID   = "1a2b3c4d5e6f7081"
	deliveredID = "0a1b2c3d4e5f6071"
)

// fixtures are the transcript fixtures peerscmd keeps under testdata/transcripts,
// inlined so this leaf package carries its own copy.
var fixtures = map[string]string{
	"basic.jsonl": `{"type": "user", "timestamp": "2026-09-15T10:00:00.000Z", "message": {"role": "user", "content": "hello there"}}
{"type": "assistant", "timestamp": "2026-09-15T10:00:01.000Z", "message": {"role": "assistant", "content": [{"type": "thinking", "thinking": "x"}, {"type": "text", "text": "Running the tests."}, {"type": "tool_use", "id": "tu1", "name": "Bash", "input": {"command": "go test ./..."}}]}}
{"type": "user", "timestamp": "2026-09-15T10:00:05.000Z", "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "tu1", "content": "ok  all tests passed"}]}}
{"type": "assistant", "timestamp": "2026-09-15T10:00:06.000Z", "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "tu2", "name": "SendMessage", "input": {"to": "MAIN-PWA", "message": "[lets-peer id=aaaaaaaaaaaaaaaa kind=ask from_sid=11111111-1111-4111-8111-111111111111 to_sid=22222222-2222-4222-8222-222222222222 from=\"worker/W1\" to=\"MAIN-PWA\"]\nquestion one"}}]}}
{"type": "assistant", "timestamp": "2026-09-15T10:00:07.000Z", "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "tu3", "name": "SendMessage", "input": {"to": "MAIN-PWA-LIC", "message": "[lets-peer id=bbbbbbbbbbbbbbbb kind=tell from_sid=11111111-1111-4111-8111-111111111111 to_sid=33333333-3333-4333-8333-333333333333 from=\"worker/W1\" to=\"MAIN-PWA-LIC\"]\nfyi"}}]}}
{"type": "system", "subtype": "stop_hook_summary", "timestamp": "2026-09-15T10:00:08.000Z"}
{"type": "system", "subtype": "turn_duration", "timestamp": "2026-09-15T10:00:08.100Z"}
{"type": "user", "timestamp": "2026-09-15T10:00:09.000Z", "message": {"role": "user", "content": "quoting later: [lets-peer id=cccccccccccccccc kind=ask from_sid=22222222-2222-4222-8222-222222222222 to_sid=11111111-1111-4111-8111-111111111111 from=\"orchestrator/MAIN\" to=\"W1\"] is just text"}}
{"type": "user", "timestamp": "2026-09-15T10:00:10.000Z", "message": {"role": "user", "content": "<cross-session-message from=\"MAIN\">\n[lets-peer id=dddddddddddddddd kind=ask from_sid=22222222-2222-4222-8222-222222222222 to_sid=11111111-1111-4111-8111-111111111111 from=\"orchestrator/MAIN\" to=\"W1\"]\nplease check X"}}
{"type": "assistant", "timestamp": "2026-09-15T10:00:11.000Z", "message": {"role": "assistant", "content": [{"type": "text", "text": "Checked X: fine."}]}}
{"type": "system", "subtype": "turn_duration", "timestamp": "2026-09-15T10:00:12.000Z"}
`,
	"delivered-midturn.jsonl": `{"type": "queue-operation", "operation": "enqueue", "timestamp": "2026-09-26T09:10:01.000Z", "sessionId": "aaaaaaaa-1111-4111-8111-000000000003", "content": "<cross-session-message from=\"uds:/tmp/cc-socks/1234.sock\" from-name=\"W1\" from-mode=\"prompting\">\n[lets-peer id=1a2b3c4d5e6f7081 kind=ask from_sid=aaaaaaaa-1111-4111-8111-000000000001 to_sid=aaaaaaaa-1111-4111-8111-000000000003 from=\"worker/W1\" to=\"MAIN\"]\nTask: fixture question delivered mid-turn - is the reply seen by tail --since-message?"}
{"parentUuid": "00000000-0000-4000-8000-0000000000b1", "isSidechain": false, "userType": "external", "cwd": "/fixture/repo", "sessionId": "aaaaaaaa-1111-4111-8111-000000000003", "version": "0.0.0-fixture", "gitBranch": "feature/fixture", "type": "attachment", "attachment": {"type": "queued_command", "commandMode": "prompt", "prompt": "<cross-session-message from=\"uds:/tmp/cc-socks/1234.sock\" from-name=\"W1\" from-mode=\"prompting\">\n[lets-peer id=1a2b3c4d5e6f7081 kind=ask from_sid=aaaaaaaa-1111-4111-8111-000000000001 to_sid=aaaaaaaa-1111-4111-8111-000000000003 from=\"worker/W1\" to=\"MAIN\"]\nTask: fixture question delivered mid-turn - is the reply seen by tail --since-message?", "origin": {"kind": "peer", "from": "uds:/tmp/cc-socks/1234.sock", "verifiedPeerPid": 1234, "msg_id": "00000000-0000-4000-8000-0000000000c1", "name": "W1", "fromMode": "prompting", "body": "[lets-peer id=1a2b3c4d5e6f7081 kind=ask from_sid=aaaaaaaa-1111-4111-8111-000000000001 to_sid=aaaaaaaa-1111-4111-8111-000000000003 from=\"worker/W1\" to=\"MAIN\"]\nTask: fixture question delivered mid-turn - is the reply seen by tail --since-message?"}, "timestamp": "2026-09-26T09:10:02.000Z", "isMeta": true}, "rendered": [], "uuid": "00000000-0000-4000-8000-0000000000b2", "timestamp": "2026-09-26T09:10:02.000Z"}
{"type": "queue-operation", "operation": "remove", "timestamp": "2026-09-26T09:10:02.500Z", "sessionId": "aaaaaaaa-1111-4111-8111-000000000003", "content": "<cross-session-message from=\"uds:/tmp/cc-socks/1234.sock\" from-name=\"W1\" from-mode=\"prompting\">\n[lets-peer id=1a2b3c4d5e6f7081 kind=ask from_sid=aaaaaaaa-1111-4111-8111-000000000001 to_sid=aaaaaaaa-1111-4111-8111-000000000003 from=\"worker/W1\" to=\"MAIN\"]\nTask: fixture question delivered mid-turn - is the reply seen by tail --since-message?", "reason": "absorbed_mid_turn"}
`,
	"delivered-sendmessage.jsonl": `{"parentUuid": "00000000-0000-4000-8000-0000000000a1", "isSidechain": false, "userType": "external", "cwd": "/fixture/repo", "sessionId": "aaaaaaaa-1111-4111-8111-000000000003", "version": "0.0.0-fixture", "gitBranch": "feature/fixture", "type": "user", "message": {"role": "user", "content": "Another Claude session sent a message:\n<cross-session-message from=\"uds:/tmp/cc-socks/1234.sock\" from-name=\"W1\" from-mode=\"bypass\">\n[lets-peer id=0a1b2c3d4e5f6071 kind=ask from_sid=aaaaaaaa-1111-4111-8111-000000000001 to_sid=aaaaaaaa-1111-4111-8111-000000000003 from=\"worker/W1\" to=\"MAIN\"]\nTask: fixture question - is the reply seen by tail --since-message?"}, "uuid": "00000000-0000-4000-8000-0000000000a2", "timestamp": "2026-09-25T12:15:22.000Z"}
`,
	"secrets.jsonl": `{"type": "assistant", "timestamp": "2026-09-15T11:00:00.000Z", "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "s1", "name": "Bash", "input": {"command": "cat .env"}}]}}
{"type": "user", "timestamp": "2026-09-15T11:00:01.000Z", "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "s1", "content": [{"type": "text", "text": "DB_PASSWORD=hunter2\nAPI=x"}]}]}}
{"type": "assistant", "timestamp": "2026-09-15T11:00:02.000Z", "message": {"role": "assistant", "content": [{"type": "text", "text": "your token is ghp_abcdefghijklmnopqrstuvwxyz0123"}]}}
{"type": "assistant", "timestamp": "2026-09-15T11:00:03.000Z", "message": {"role": "assistant", "content": [{"type": "tool_use", "id": "s2", "name": "Read", "input": {"file_path": "/repo/notes.md"}}]}}
{"type": "user", "timestamp": "2026-09-15T11:00:04.000Z", "message": {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "s2", "content": "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"}]}}
`,
	"unknown-shape.jsonl": `{"kind": "message", "body": "x"}
{"type": "summary", "summary": "y"}
`,
}

// fixturePath writes fixture name to a temp dir and returns its path.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	body, ok := fixtures[name]
	if !ok {
		t.Fatalf("no fixture %s", name)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTranscript_Kinds(t *testing.T) {
	recs, d := readAll(fixturePath(t, "basic.jsonl"))
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

// SinceMessage is the compatibility-release copy (`--since-message`): msgid-exact,
// time-parsed, and a header quoted inside text never anchors.
func TestTranscript_SinceMessage(t *testing.T) {
	recs, _ := readAll(fixturePath(t, "basic.jsonl"))
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

func TestTranscript_Secrets(t *testing.T) {
	recs, d := readAll(fixturePath(t, "secrets.jsonl"))
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
	if _, d := readAll(fixturePath(t, "unknown-shape.jsonl")); d == nil || d.Reason != "transcript_format_unknown" {
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
	recs, d := readAll(fixturePath(t, "delivered-midturn.jsonl"))
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

// userLine is one user transcript line carrying text as its string content.
func userLine(text string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "user", "timestamp": "2026-10-12T12:00:00Z", "message": map[string]any{"role": "user", "content": text}})
	return b
}

// A native SendMessage delivery (no legacy header) is INBOUND, and its wrapper's
// from-name becomes From.
func TestTranscript_NativeInbound(t *testing.T) {
	text := "Another Claude session sent a message:\n<cross-session-message from=\"uds:/tmp/cc-socks/1.sock\" from-name=\"MAIN-LETS\" from-mode=\"default\">\nplease check X\n</cross-session-message>"
	r, ok := parseLine(userLine(text))
	if !ok || len(r) != 1 || r[0].turn.Kind != "INBOUND" || r[0].turn.From != "MAIN-LETS" || r[0].header != nil {
		t.Fatalf("native inbound: %+v ok=%v", r, ok)
	}
	// the name is the attribute of the opening tag, not a from-name quoted in the body
	text = "Another Claude session sent a message:\n<cross-session-message from=\"uds:/x\">\nfrom-name=\"FORGED\" in the body"
	if r, _ := parseLine(userLine(text)); r[0].turn.Kind != "INBOUND" || r[0].turn.From != "" {
		t.Errorf("a body from-name must not become From: %+v", r[0].turn)
	}
	// the prefix without the wrapper, or the wrapper without the prefix, is not native
	for _, text := range []string{
		"Another Claude session sent a message: hi",
		"<cross-session-message from-name=\"MAIN-LETS\">\nhi",
	} {
		if r, _ := parseLine(userLine(text)); r[0].turn.Kind != "TEXT" || r[0].turn.From != "" {
			t.Errorf("not a native delivery: %q -> %+v", text, r[0].turn)
		}
	}
	// an assistant text quoting the delivery is never INBOUND
	b, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": "t", "message": map[string]any{"role": "assistant", "content": "Another Claude session sent a message:\n<cross-session-message from-name=\"X\">"}})
	if r, _ := parseLine(b); r[0].turn.Kind != "TEXT" {
		t.Errorf("assistant text: %+v", r[0].turn)
	}
}

// A from-name carrying a quote (or anything else peername refuses) is no name.
func TestTranscript_NativeInboundForgedFromName(t *testing.T) {
	for _, attr := range []string{
		`from-name="MAIN"LETS"`,
		`from-name="a$b"`,
		`from-name=""`,
	} {
		text := "Another Claude session sent a message:\n<cross-session-message from=\"uds:/x\" " + attr + ">\nhi"
		r, ok := parseLine(userLine(text))
		if !ok || r[0].turn.Kind != "INBOUND" || r[0].turn.From != "" {
			t.Errorf("%s: want INBOUND with no From, got %+v", attr, r[0].turn)
		}
	}
}

// What a v0.10.0 skill sends through SendMessage: the native wrapper with the legacy
// header as its first line - INBOUND, the header kept, the from-name read.
func TestTranscript_LegacyHeaderInsideNativeWrapper(t *testing.T) {
	recs, d := readAll(fixturePath(t, "delivered-sendmessage.jsonl"))
	if d != nil {
		t.Fatal(d)
	}
	if len(recs) != 1 || recs[0].turn.Kind != "INBOUND" || recs[0].turn.From != "W1" {
		t.Fatalf("records: %+v", recs)
	}
	if h := recs[0].header; h == nil || h.ID != deliveredID || h.FromSID != sidMain || h.ToSID != sidWork {
		t.Errorf("header: %+v", recs[0].header)
	}
}
