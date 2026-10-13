package peertail

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/peername"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

const (
	chunkSize    = 64 << 10
	maxTailBytes = 8 << 20
	// textCapDefault, textCapMessage and callCapBytes together bound one tail call:
	// a per-turn cap (which one depends on why the caller is reading), and a ceiling
	// on the sum of the turns returned. textCapMessage must stay well under
	// callCapBytes so the newest turn alone can never overflow the ceiling - see
	// tail.go's byte-ceiling trim, which relies on that relationship.
	textCapDefault = 2 << 10  // a glance at what a peer is doing
	textCapMessage = 16 << 10 // an addressed message or a relayed reply: the payload IS the point
	callCapBytes   = 64 << 10 // a whole tail call, so N large turns cannot flood a reader
	toolInputCap   = 200
	toolResultCap  = 400
)

// record is one recognized transcript line, before it becomes a Turn.
type record struct {
	turn   Turn
	toolID string         // tool_use id / tool_result tool_use_id
	input  map[string]any // tool_use input
	header *Header        // INBOUND header, or a header at the start of a TOOL input string
	end    bool           // system turn_duration: the assistant turn is over
}

// transcriptSlug is Claude Code's project directory name for cwd: every byte
// outside [A-Za-z0-9] becomes '-'.
func transcriptSlug(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

// globTranscripts is the fallback lookup (a seam so tests can see it is not used
// when the direct path exists).
var globTranscripts = filepath.Glob

// LocateTranscript finds <claudeDir>/projects/<slug(cwd)>/<sid>.jsonl, falling back
// to one Glob across projects. The resolved file must stay under <claudeDir>/projects.
func LocateTranscript(claudeDir, cwd, sid string) (string, *Degraded) {
	if !ccregistry.ValidSession(sid) {
		return "", &Degraded{Source: "transcript", Reason: "session_invalid"}
	}
	projects := filepath.Join(claudeDir, "projects")
	path := filepath.Join(projects, transcriptSlug(cwd), sid+".jsonl")
	if _, err := os.Lstat(path); err != nil {
		matches, _ := globTranscripts(filepath.Join(projects, "*", sid+".jsonl"))
		if len(matches) == 0 {
			return "", &Degraded{Source: "transcript", Reason: "transcript_not_found"}
		}
		path = matches[0]
	}
	real, err := filepath.EvalSymlinks(path)
	realProjects, perr := filepath.EvalSymlinks(projects)
	if err != nil || perr != nil || !strings.HasPrefix(real, realProjects+string(filepath.Separator)) {
		return "", &Degraded{Source: "transcript", Reason: "transcript_outside_projects"}
	}
	return real, nil
}

// bytesRead counts what readBackward pulled from disk (a test seam for the cap).
var bytesRead atomic.Int64

// readBackward reads path from EOF in 64 KiB chunks and returns recognized records in
// chronological order, stopping once stop reports true for the records collected so
// far (newest first internally) or maxTailBytes were read. The first partial line of
// a chunk that did not start at offset 0 is dropped.
func readBackward(path string, stop func(newestFirst []record) bool) ([]record, int, *Degraded) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, &Degraded{Source: "transcript", Reason: "transcript_unreadable"}
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, &Degraded{Source: "transcript", Reason: "transcript_unreadable"}
	}
	var (
		newest    []record
		carry     []byte
		lines     int
		skipped   int
		read      int64
		offset    = fi.Size()
		toolInput = map[string]map[string]any{}
	)
	pending := []record{}
	for offset > 0 && read < maxTailBytes {
		n := int64(chunkSize)
		if n > offset {
			n = offset
		}
		offset -= n
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, lines, &Degraded{Source: "transcript", Reason: "transcript_unreadable"}
		}
		read += n
		bytesRead.Add(n)
		data := append(buf, carry...)
		parts := bytes.Split(data, []byte("\n"))
		if offset > 0 {
			carry = parts[0]
			parts = parts[1:]
		} else {
			carry = nil
		}
		for i := len(parts) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(parts[i])
			if len(line) == 0 {
				continue
			}
			lines++
			recs, ok := parseLine(line)
			if !ok {
				skipped++
				continue
			}
			for j := len(recs) - 1; j >= 0; j-- {
				pending = append(pending, recs[j])
			}
		}
		newest = append(newest, pending...)
		pending = pending[:0]
		if stop != nil && stop(newest) {
			break
		}
	}
	// chronological order, then resolve RESULT redaction against its tool_use input
	out := make([]record, len(newest))
	for i, r := range newest {
		out[len(newest)-1-i] = r
	}
	for i := range out {
		if out[i].turn.Kind == "TOOL" && out[i].toolID != "" {
			toolInput[out[i].toolID] = out[i].input
		}
	}
	for i := range out {
		if out[i].turn.Kind == "RESULT" {
			in := toolInput[out[i].toolID]
			out[i].turn.Tool = toolNameFor(out, out[i].toolID)
			out[i].turn.Text = redact.ToolResult(out[i].turn.Tool, in, out[i].turn.Text, toolResultCap)
		}
	}
	if len(out) == 0 && lines > 0 {
		return nil, lines, &Degraded{Source: "transcript", Reason: "transcript_format_unknown", Detail: itoaInt(skipped) + " lines skipped"}
	}
	return out, lines, nil
}

func toolNameFor(recs []record, id string) string {
	for _, r := range recs {
		if r.turn.Kind == "TOOL" && r.toolID == id {
			return r.turn.Tool
		}
	}
	return ""
}

func itoaInt(i int) string { b, _ := json.Marshal(i); return string(b) }

type rawLine struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	// Attachment is read only for a peer message delivered mid-turn (queued_command
	// with origin.kind peer); every other attachment is ignored.
	Attachment *struct {
		Type   string `json:"type"`
		Prompt string `json:"prompt"`
		Origin *struct {
			Kind string `json:"kind"`
			Body string `json:"body"`
		} `json:"origin"`
	} `json:"attachment"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// parseLine recognizes user / assistant records (text, tool_use, tool_result), the
// system turn_duration record, and a peer message delivered mid-turn - an
// `attachment` of type `queued_command` whose origin.kind is `peer`, read as the
// user turn it is (text = origin.body, else prompt; ts = the record's timestamp).
// Its queue-operation enqueue / remove records are bookkeeping and stay unrecognized
// (counting them would double the message). Anything else is not recognized.
func parseLine(line []byte) ([]record, bool) {
	var l rawLine
	if json.Unmarshal(line, &l) != nil {
		return nil, false
	}
	if l.Type == "system" && l.Subtype == "turn_duration" {
		return []record{{turn: Turn{TS: l.Timestamp, Kind: "END"}, end: true}}, true
	}
	if l.Type == "attachment" {
		a := l.Attachment
		if a == nil || a.Type != "queued_command" || a.Origin == nil || a.Origin.Kind != "peer" {
			return nil, false
		}
		text := a.Origin.Body
		if text == "" {
			text = a.Prompt
		}
		if text == "" {
			return nil, false
		}
		return []record{textRecord("user", l.Timestamp, text)}, true
	}
	if (l.Type != "user" && l.Type != "assistant") || l.Message == nil {
		return nil, false
	}
	var recs []record
	var s string
	if json.Unmarshal(l.Message.Content, &s) == nil {
		recs = append(recs, textRecord(l.Type, l.Timestamp, s))
		return recs, true
	}
	var blocks []rawBlock
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return nil, false
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			recs = append(recs, textRecord(l.Type, l.Timestamp, b.Text))
		case "tool_use":
			in, _ := json.Marshal(b.Input)
			r := record{turn: Turn{TS: l.Timestamp, Kind: "TOOL", Role: l.Type, Tool: redact.Control(b.Name), Text: redact.Cap(redact.Control(redact.Text(string(in))), toolInputCap)}, toolID: b.ID, input: b.Input}
			for _, v := range b.Input {
				if str, ok := v.(string); ok {
					if h, ok := leadingHeader(str); ok {
						r.header = &h
						break
					}
				}
			}
			recs = append(recs, r)
		case "tool_result":
			recs = append(recs, record{turn: Turn{TS: l.Timestamp, Kind: "RESULT", Role: l.Type, Text: resultText(b.Content)}, toolID: b.ToolUseID})
		}
	}
	if len(recs) == 0 {
		return nil, false
	}
	return recs, true
}

// textRecord redacts but does NOT cap: a TEXT/INBOUND turn is capped exactly once,
// in turnsOf, which is also where its TruncatedBytes is counted. Capping here too
// would let redact.Cap cut its own marker and measure the count against the wrong
// baseline.
//
// A user text is INBOUND when it opens with a legacy `[lets-peer ...]` header (at
// the start, or first inside a `<cross-session-message>` wrapper) - its header is
// kept on the record - or when it is a native cross-session delivery (see
// nativeInbound), whose from-name becomes the turn's From.
func textRecord(typ, ts, text string) record {
	r := record{turn: Turn{TS: ts, Kind: "TEXT", Role: typ}}
	if typ == "user" {
		if h, ok := leadingHeader(text); ok {
			r.turn.Kind = "INBOUND"
			r.header = &h
		}
		if from, ok := nativeInbound(text); ok {
			r.turn.Kind = "INBOUND"
			r.turn.From = from
		}
	}
	r.turn.Text = redact.Control(redact.Text(text))
	return r
}

// nativePrefix is the one line Claude Code puts before a delivered SendMessage.
const nativePrefix = "Another Claude session sent a message:"

// fromNameRe reads the from-name attribute of the wrapper's opening tag. The closing
// quote must be followed by whitespace or the tag's end, so a name carrying a quote
// (`from-name="a"b"`) matches nothing instead of its prefix.
var fromNameRe = regexp.MustCompile(`from-name="([^"]{1,64})"[\s>]`)

// nativeInbound reports whether text is a native cross-session delivery: it starts
// with nativePrefix and holds a `<cross-session-message ` wrapper. from is the
// wrapper's from-name when that is a valid peer name, else "".
func nativeInbound(text string) (from string, ok bool) {
	t := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(t, nativePrefix) {
		return "", false
	}
	i := strings.Index(t, "<cross-session-message ")
	if i < 0 {
		return "", false
	}
	tag := t[i:]
	if j := strings.IndexByte(tag, '>'); j >= 0 {
		tag = tag[:j+1]
	}
	if m := fromNameRe.FindStringSubmatch(tag); m != nil && peername.Valid(m[1]) {
		return m[1], true
	}
	return "", true
}

var (
	msgIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)
	// crossWrapper is Claude Code's wrapper around a delivered cross-session message.
	// A SendMessage delivered to another session lands in its transcript as a user
	// text that opens with ONE exact prefix line before the wrapper (lets-rry3c,
	// recorded live 2026-09-25); the prefix is accepted only when the wrapper follows.
	crossWrapper = regexp.MustCompile(`^\s*(?:Another Claude session sent a message:\s*)?<cross-session-message[^>]*>\s*`)
)

// ValidKind reports whether k is a legacy peer message kind.
func ValidKind(k string) bool {
	switch k {
	case "ask", "ping", "tell", "ask-ro":
		return true
	}
	return false
}

// ParseHeader parses a legacy header (a copy of peerscmd's, for the compatibility
// release): line must START with `[lets-peer `, fields are `key=value` /
// `key="value"` tokens up to the closing `]`, and every field is validated. A header
// quoted later inside text is not a header.
func ParseHeader(line string) (Header, bool) {
	rest, ok := strings.CutPrefix(line, "[lets-peer ")
	if !ok {
		return Header{}, false
	}
	fields := map[string]string{}
	for {
		rest = strings.TrimLeft(rest, " ")
		if strings.HasPrefix(rest, "]") {
			break
		}
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			return Header{}, false
		}
		key := rest[:eq]
		if strings.ContainsAny(key, " ]\"") {
			return Header{}, false
		}
		rest = rest[eq+1:]
		var val string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				return Header{}, false
			}
			val, rest = rest[1:1+end], rest[2+end:]
		} else {
			end := strings.IndexAny(rest, " ]")
			if end < 0 {
				return Header{}, false
			}
			val, rest = rest[:end], rest[end:]
		}
		if _, dup := fields[key]; dup {
			return Header{}, false
		}
		fields[key] = val
	}
	h := Header{ID: fields["id"], Kind: fields["kind"], FromSID: fields["from_sid"], ToSID: fields["to_sid"], From: fields["from"], To: fields["to"]}
	if !msgIDRe.MatchString(h.ID) || !ValidKind(h.Kind) || !ccregistry.ValidSession(h.FromSID) || !ccregistry.ValidSession(h.ToSID) {
		return Header{}, false
	}
	return h, true
}

// leadingHeader parses a legacy header at the very start of a message body, after an
// optional Claude Code `<cross-session-message ...>` wrapper - itself optionally
// preceded by Claude Code's delivery prefix "Another Claude session sent a
// message:". Any other text before the header, or the prefix without the wrapper,
// is not a header.
func leadingHeader(text string) (Header, bool) {
	text = crossWrapper.ReplaceAllString(text, "")
	line, _, _ := strings.Cut(strings.TrimLeft(text, " \t\r\n"), "\n")
	return ParseHeader(line)
}

func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// keptLen is how many bytes of s a cap of n leaves, cut on a rune boundary - the
// same cut redact.Cap makes, without its marker. It exists so the marker stays
// redact's to own while the counter below is measured on the text, not on the text
// plus the marker.
func keptLen(s string, n int) int {
	if n <= 0 || len(s) <= n {
		return len(s)
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return cut
}

// turnsOf returns the Turns of recs (END markers are internal), capping each turn's
// text at perTurn and recording how many bytes that cut - a reader must never see a
// truncated answer next to a zero counter.
func turnsOf(recs []record, perTurn int) []Turn {
	out := []Turn{}
	for _, r := range recs {
		if r.end {
			continue
		}
		t := r.turn
		t.srcLen = len(t.Text) // captured before any cap or marker (FIX E)
		if kept := keptLen(t.Text, perTurn); kept < len(t.Text) {
			t.TruncatedBytes = len(t.Text) - kept
			t.Text = redact.Cap(t.Text, perTurn)
		}
		out = append(out, t)
	}
	return out
}

// TailTurns returns the last n turns of a transcript (the glance path: the default
// per-turn cap).
func TailTurns(path string, n int) ([]Turn, *Degraded) {
	recs, _, d := readBackward(path, func(newest []record) bool {
		c := 0
		for _, r := range newest {
			if !r.end {
				c++
			}
		}
		return c >= n
	})
	if d != nil {
		return nil, d
	}
	turns := turnsOf(recs, textCapDefault)
	if len(turns) > n {
		turns = turns[len(turns)-n:]
	}
	return turns, nil
}

// SinceMessage returns the records after the first INBOUND record timestamped at or
// after sentAt whose parsed legacy header id equals msgid exactly; ok=false when
// there is none. Kept for the compatibility release (`--since-message`).
func SinceMessage(recs []record, msgid, sentAt string) ([]record, bool) {
	since, err := time.Parse(time.RFC3339Nano, sentAt)
	if err != nil {
		return nil, false
	}
	for i, r := range recs {
		if r.turn.Kind != "INBOUND" || r.header == nil || r.header.ID != msgid {
			continue
		}
		if ts, err := time.Parse(time.RFC3339Nano, r.turn.TS); err == nil && !ts.Before(since) {
			return recs[i+1:], true
		}
	}
	return nil, false
}

// readAll reads the whole tail window (up to maxTailBytes) in chronological order.
func readAll(path string) ([]record, *Degraded) {
	recs, _, d := readBackward(path, nil)
	return recs, d
}
