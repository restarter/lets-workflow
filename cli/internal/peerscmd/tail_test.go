//go:build unix

package peerscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func tailErrKind(t *testing.T, o TailOptions) string {
	t.Helper()
	res, err := Tail(context.Background(), o)
	if err == nil || res.OK || res.Error == nil {
		t.Fatalf("%+v: want a failed envelope, got %+v %v", o, res, err)
	}
	return res.Error.Kind
}

// A name or a session id resolves among this repo's live rows only.
func TestTail_ResolvesInThisRepo(t *testing.T) {
	repo := repoWithLets(t, "")
	elsewhere := gitRepo(t)
	home := claudeHome(t, []regRow{{1, sidMain, "MAIN", repo}, {2, sidFable, "DUP", repo}, {3, sidWork, "DUP", repo}, {4, sidW, "FAR", elsewhere}})
	writeTranscript(t, home, repo, sidMain, userText("2026-09-22T10:00:00Z", "hi"), assistantText("2026-09-22T10:00:01Z", "hello"))
	writeTranscript(t, home, elsewhere, sidW, assistantText("2026-09-22T10:00:01Z", "secret elsewhere"))

	for _, o := range []TailOptions{{Cwd: repo, Name: "MAIN"}, {Cwd: repo, ToSession: sidMain}} {
		res, err := Tail(context.Background(), o)
		if err != nil || !res.OK || res.Session != sidMain || len(res.Turns) != 2 || res.Turns[1].Text != "hello" {
			t.Errorf("%+v: %+v %v", o, res, err)
		}
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "NOPE"}); k != "peer_not_found" {
		t.Errorf("unknown name: %s", k)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "DUP"}); k != "peer_ambiguous" {
		t.Errorf("a shared name: %s", k)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, ToSession: sidW}); k != "peer_not_found" {
		t.Errorf("a session of another repo is never read: %s", k)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "FAR"}); k != "peer_not_found" {
		t.Errorf("a name of another repo is never read: %s", k)
	}
	for _, o := range []TailOptions{{Cwd: repo}, {Cwd: repo, Name: "MAIN", ToSession: sidMain}, {Cwd: repo, Name: "x --auto"}, {Cwd: repo, ToSession: "../x"}} {
		if k := tailErrKind(t, o); k != "usage" {
			t.Errorf("%+v: %s, want usage", o, k)
		}
	}
	if k := tailErrKind(t, TailOptions{Cwd: t.TempDir(), Name: "MAIN"}); k != "not_in_repo" {
		t.Errorf("outside a repo: %s", k)
	}
}

// --since keeps only later turns and reads a reply whole (the message cap); a plain
// read cuts at the default cap and keeps five turns.
func TestTail_Since(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{3, sidWork, "W1", repo}})
	big := strings.Repeat("x", 6<<10)
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", "q")}
	for i := 0; i < 6; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+1), fmt.Sprintf("part %d", i+1)))
	}
	lines = append(lines, assistantText("2026-09-15T10:00:07.5Z", big), turnEnd("2026-09-15T10:00:09Z"))
	writeTranscript(t, home, repo, sidWork, lines...)

	res, err := Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", Since: "2026-09-15T12:00:01+02:00"})
	if err != nil || len(res.Turns) != 7 || res.Turns[0].Text != "part 1" || res.Turns[6].Text != big || res.TruncatedBytes != 0 || res.Deprecated != "" {
		t.Fatalf("--since: %+v %v", res, err)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1"})
	if len(res.Turns) != 5 || res.Omitted != 3 || res.TruncatedBytes != len(big)-textCapDefault {
		t.Errorf("plain read: %d turns, omitted %d, truncated %d", len(res.Turns), res.Omitted, res.TruncatedBytes)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "W1", Since: "yesterday"}); k != "usage" {
		t.Errorf("a malformed --since: %s", k)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "W1", Since: "2026-09-15T10:00:00Z", SinceMessage: msg1, SentAt: "2026-09-15T10:00:00Z"}); k != "usage" {
		t.Errorf("--since with --since-message: %s", k)
	}
}

// A native SendMessage delivery reads as INBOUND with its sender's name.
func TestTail_NativeInboundFrom(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{3, sidWork, "W1", repo}})
	writeTranscript(t, home, repo, sidWork,
		userText("2026-10-12T12:00:05Z", "Another Claude session sent a message:\n<cross-session-message from=\"uds:/x\" from-name=\"MAIN-LETS\">\nstatus?\n</cross-session-message>"),
		assistantText("2026-10-12T12:00:06Z", "the answer"))
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", Since: "2026-10-12T12:00:05Z"})
	if err != nil || len(res.Turns) != 2 || res.Turns[0].Kind != "INBOUND" || res.Turns[0].From != "MAIN-LETS" {
		t.Fatalf("native inbound: %+v %v", res, err)
	}
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"from":"MAIN-LETS"`) {
		t.Errorf("from on the JSON surface: %s", b)
	}
}

// --since-message --sent-at keep today's meaning for the compatibility release: the
// turns after the message with that msgid, the whole reply, a deprecated line; not
// seen yet is no turns and a note, never a time-only fallback.
func TestTail_SinceMessageCompat(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{3, sidWork, "W1", repo}})
	lines := []map[string]any{assistantText("2026-09-15T09:59:59Z", "before the question"), userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq")}
	for i := 0; i < 7; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+1), fmt.Sprintf("part %d", i+1)))
	}
	writeTranscript(t, home, repo, sidWork, append(lines, turnEnd("2026-09-15T10:00:09Z"))...)

	res, err := Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil || len(res.Turns) != 7 || res.Turns[0].Text != "part 1" || res.Omitted != 0 || res.Deprecated == "" {
		t.Errorf("the whole reply: %+v %v", res, err)
	}
	res, _ = Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z", Last: 3})
	if len(res.Turns) != 3 || res.Omitted != 4 || res.Turns[2].Text != "part 7" {
		t.Errorf("an explicit --last keeps the newest and counts the rest: %+v", res)
	}
	res, err = Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", SinceMessage: "fedcba9876543210", SentAt: "2026-09-15T09:00:00Z"})
	if err != nil || !res.OK || len(res.Turns) != 0 || res.Note != "message not seen yet" {
		t.Errorf("not seen yet: %+v %v", res, err)
	}
	if k := tailErrKind(t, TailOptions{Cwd: repo, Name: "W1", SinceMessage: msg1}); k != "usage" {
		t.Errorf("--since-message without --sent-at: %s", k)
	}
}

// The compatibility read keeps the call ceiling: the oldest turns go once the sum
// passes 64 KiB, counted by their exact source size; the newest always stays.
func TestTail_SinceMessageCallCeiling(t *testing.T) {
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{3, sidWork, "W1", repo}})
	const oldestSize = 40 << 10
	lines := []map[string]any{userText("2026-09-15T10:00:00Z", header(msg1, sidWork)+"\nq"), assistantText("2026-09-15T10:00:01Z", strings.Repeat("o", oldestSize))}
	for i := 0; i < 4; i++ {
		lines = append(lines, assistantText(fmt.Sprintf("2026-09-15T10:00:%02dZ", i+2), strings.Repeat("n", 16000)))
	}
	writeTranscript(t, home, repo, sidWork, append(lines, turnEnd("2026-09-15T10:00:10Z"))...)
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, Name: "W1", SinceMessage: msg1, SentAt: "2026-09-15T09:59:59Z"})
	if err != nil || res.Omitted == 0 || res.TruncatedBytes != oldestSize {
		t.Fatalf("ceiling: omitted=%d truncated=%d %v", res.Omitted, res.TruncatedBytes, err)
	}
	for _, tn := range res.Turns {
		if strings.HasPrefix(tn.Text, "oooo") {
			t.Fatalf("the dropped turn must not be returned")
		}
	}
}

// A v0.10.0 cross-repo read gets one actionable failure, in the message.
func TestTail_RepoIndexRetired(t *testing.T) {
	repo := repoWithLets(t, "")
	claudeHome(t, []regRow{{3, sidWork, "W1", repo}})
	idx := 1
	res, err := Tail(context.Background(), TailOptions{Cwd: repo, ToSession: sidWork, RepoIndex: &idx})
	if err == nil || res.OK || res.Error == nil || res.Error.Kind != "cross_repo_read_retired" ||
		res.Error.Message != "reading another repo's session was removed - update the LETS plugin (/lets:update); the peer can answer with /lets:orc, which arrives here as a cross-session message" {
		t.Errorf("retired: %+v %v", res.Error, err)
	}
}
