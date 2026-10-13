//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The v0.10.0 orc skill's path against the stubs: frame, write the handoff, tell
// (which hands the text back for SendMessage), the native delivery of that text, and
// the correlated reply read.
func TestCompat_V010SkillPath(t *testing.T) {
	ctx := context.Background()
	repo := repoWithLets(t, "")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})

	fr, err := Frame(ctx, FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidWork, Kind: "ask"})
	if err != nil || !fr.OK || fr.Deprecated == "" || !strings.HasPrefix(fr.Header, "[lets-peer id="+fr.MsgID) || !strings.Contains(fr.Header, `to="W1"`) {
		t.Fatalf("frame: %+v %v", fr, err)
	}
	dir := filepath.Dir(fr.HandoffPath)
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("handoff dir must be 0700: %v %v", fi, err)
	}
	if err := os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nis X done?"), 0o600); err != nil {
		t.Fatal(err)
	}

	tr, err := Tell(ctx, TellOptions{Cwd: repo, ToSession: sidWork, MsgID: fr.MsgID})
	if err != nil || !tr.OK || tr.Delivered || tr.Route != "claude" || tr.Reason != "claude_transport_model_send" || tr.Deprecated == "" {
		t.Fatalf("tell: %+v %v", tr, err)
	}
	if tr.Text != fr.Header+"\nis X done?" {
		t.Errorf("tell returns the FULL handoff, header included: %q", tr.Text)
	}
	if _, err := os.Stat(fr.HandoffPath); !os.IsNotExist(err) {
		t.Error("tell consumes the handoff")
	}
	again, err := Tell(ctx, TellOptions{Cwd: repo, ToSession: sidWork, MsgID: fr.MsgID})
	if err == nil || again.Error == nil || again.Error.Kind != "handoff_missing" || again.Text != "" {
		t.Errorf("a second tell on the same msgid is never a resend: %+v %v", again, err)
	}

	// Not delivered yet: the peer only did unrelated work after sent_at.
	sent, _ := time.Parse(time.RFC3339Nano, fr.SentAt)
	at := func(d time.Duration) string { return sent.Add(d).UTC().Format(time.RFC3339Nano) }
	unrelated := assistantText(at(time.Second), "unrelated work")
	writeTranscript(t, home, repo, sidWork, unrelated)
	read := func() *TailResult {
		t.Helper()
		res, err := Tail(ctx, TailOptions{Cwd: repo, Name: "W1", SinceMessage: fr.MsgID, SentAt: fr.SentAt})
		if err != nil || !res.OK || res.Deprecated == "" {
			t.Fatalf("tail --since-message: %+v %v", res, err)
		}
		return res
	}
	if res := read(); len(res.Turns) != 0 || res.Note != "message not seen yet" {
		t.Errorf("not delivered yet: no turns, never a time-only fallback: %+v", res)
	}

	// Delivered: the native-wrapped inbound built FROM the tell response, then the reply.
	writeTranscript(t, home, repo, sidWork,
		unrelated,
		userText(at(2*time.Second), nativeDelivery("MAIN", tr.Text)),
		assistantText(at(3*time.Second), "X is done."),
		turnEnd(at(4*time.Second)))
	if res := read(); len(res.Turns) != 1 || res.Turns[0].Text != "X is done." {
		t.Errorf("only the reply: %+v", res)
	}

	w, err := Wait(ctx, WaitOptions{Cwd: repo, ToSession: sidWork, SinceMessage: fr.MsgID, SentAt: fr.SentAt})
	if err != nil || !w.OK || w.Reason != "completion_unverifiable" || w.Satisfied || w.Deprecated == "" {
		t.Errorf("wait: %+v %v", w, err)
	}
	if res, err := Tail(ctx, TailOptions{Cwd: repo, ToSession: sidWork, RepoIndex: &[]int{0}[0]}); err == nil || res.OK || res.Error.Kind != "cross_repo_read_retired" || !strings.Contains(res.Error.Message, "/lets:update") {
		t.Errorf("--repo-index: %+v %v", res, err)
	}
}

// A handoff whose header addresses another session or msgid is refused and consumed.
func TestCompat_TellRefusesForeignHeader(t *testing.T) {
	ctx := context.Background()
	repo := repoWithLets(t, "")
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "W1", repo}})
	fr, err := Frame(ctx, FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidWork, Kind: "tell"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nfyi"), 0o600)
	if res, err := Tell(ctx, TellOptions{Cwd: repo, ToSession: sidFable, MsgID: fr.MsgID}); err == nil || res.Error.Kind != "handoff_refused" || res.Text != "" {
		t.Errorf("another session: %+v %v", res, err)
	}
	if _, err := os.Stat(fr.HandoffPath); !os.IsNotExist(err) {
		t.Error("a refused handoff is consumed")
	}
	if _, err := Frame(ctx, FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidMain, Kind: "ask"}); err == nil {
		t.Error("a session cannot frame to itself")
	}
}

// role set records the /clear proof from the registry (no role semantics); a session
// the registry does not list gets ok=false and no file; role clear removes it.
// Every role envelope carries deprecated.
func TestCompat_RoleSetWritesProofOnly(t *testing.T) {
	ctx := context.Background()
	repo := repoWithLets(t, "")
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {102, sidFable, "MAIN", repo}})
	for _, sid := range []string{sidMain, sidFable} { // two sessions, one name: no name_held any more
		res, err := RoleSet(ctx, repo, RoleOptions{Session: sid, Role: "orchestrator", Takeover: true, Scope: "x"})
		if err != nil || !res.OK || res.Role == nil || !res.Role.Granted || res.Deprecated == "" {
			t.Fatalf("role set %s: %+v %v", sid, res, err)
		}
	}
	if pid, set, ok := RoleProof(repo, sidMain); !ok || pid != 101 || set.IsZero() {
		t.Errorf("proof: %d %v %v", pid, set, ok)
	}
	data, _ := os.ReadFile(filepath.Join(peersDir(repo), sidMain+".role"))
	if strings.Contains(string(data), "role:") || !strings.Contains(string(data), "session: "+sidMain) {
		t.Errorf("a proof file holds session / pid / set only:\n%s", data)
	}
	res, err := RoleSet(ctx, repo, RoleOptions{Session: sidWork, Role: "worker", Task: "lets-abc"})
	if err != nil || res.OK || res.Error == nil || res.Error.Kind != "session_not_in_registry" || res.Role.Reason != "session_not_in_registry" {
		t.Errorf("unknown session: %+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(peersDir(repo), sidWork+".role")); !os.IsNotExist(err) {
		t.Error("no proof for a session the registry does not list")
	}
	clr, err := RoleClear(ctx, repo, sidMain)
	if err != nil || !clr.OK || clr.Deprecated == "" {
		t.Errorf("role clear: %+v %v", clr, err)
	}
	if _, _, ok := RoleProof(repo, sidMain); ok {
		t.Error("role clear removes the proof")
	}
}
