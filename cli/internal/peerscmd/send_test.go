//go:build unix

package peerscmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrame_MsgIDEntropy(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := newMsgID()
		if err != nil || !msgIDRe.MatchString(id) || seen[id] {
			t.Fatalf("msgid %q err=%v dup=%v", id, err, seen[id])
		}
		seen[id] = true
	}
}

func TestParseHeader(t *testing.T) {
	h, ok := ParseHeader(`[lets-peer id=0123456789abcdef kind=tell from_sid=` + sidMain + ` to_sid=` + sidWork + ` from="orchestrator/MAIN PWA" to="W 1"] body`)
	if !ok || h.From != "orchestrator/MAIN PWA" || h.To != "W 1" || h.ToSID != sidWork {
		t.Errorf("quoted names with spaces: %+v %v", h, ok)
	}
	if h.To == "MAIN-PWA" {
		t.Error("exact")
	}
	for _, bad := range []string{
		"x [lets-peer id=0123456789abcdef kind=ask from_sid=" + sidMain + " to_sid=" + sidWork + "]",
		"[lets-peer id=XYZ kind=ask from_sid=" + sidMain + " to_sid=" + sidWork + "]",
		"[lets-peer id=0123456789abcdef kind=rm from_sid=" + sidMain + " to_sid=" + sidWork + "]",
		"[lets-peer id=0123456789abcdef kind=ask from_sid=" + sidMain + " to_sid=" + sidWork + " to_sid=" + sidMain + "]",
		"[lets-peer id=0123456789abcdef kind=ask from_sid=" + sidMain + " to_sid=nope]",
	} {
		if _, ok := ParseHeader(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func frameFor(t *testing.T, repo string) *FrameResult {
	t.Helper()
	res, err := Frame(context.Background(), FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidWork, Kind: "ask"})
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	return res
}

func TestFrameAndTell_Handoff(t *testing.T) {
	repo := repoWithLets(t, "")
	claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {103, sidWork, "MAIN-PWA", repo}})
	fr := frameFor(t, repo)
	if !strings.HasPrefix(fr.Header, "[lets-peer id="+fr.MsgID) || !strings.Contains(fr.Header, `to="MAIN-PWA"`) || !strings.Contains(fr.Header, `from="peer/MAIN"`) {
		t.Errorf("header: %s", fr.Header)
	}
	if fi, err := os.Lstat(filepath.Dir(fr.HandoffPath)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("peer-msg dir: %v %v", fi, err)
	}
	// claude route: the framed text comes back for SendMessage, Go sends nothing
	_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nplease check"), 0o600)
	res, err := Tell(context.Background(), TellOptions{Cwd: repo, ToSession: sidWork, MsgID: fr.MsgID})
	if err != nil || res.Route != "claude" || res.Reason != "claude_transport_model_send" || !strings.HasSuffix(res.Text, "please check") || res.Delivered {
		t.Errorf("claude route: %+v %v", res, err)
	}
	if _, err := os.Stat(fr.HandoffPath); !os.IsNotExist(err) {
		t.Error("tell must delete the handoff")
	}

	refuse := func(name string, prep func(fr *FrameResult)) {
		fr := frameFor(t, repo)
		prep(fr)
		if _, err := Tell(context.Background(), TellOptions{Cwd: repo, ToSession: sidWork, MsgID: fr.MsgID}); err == nil || err.(*Error).Kind != "handoff_refused" {
			t.Errorf("%s: %v", name, err)
		}
	}
	refuse("symlink", func(fr *FrameResult) {
		target := filepath.Join(t.TempDir(), "x")
		_ = os.WriteFile(target, []byte(fr.Header+"\nx"), 0o600)
		_ = os.Symlink(target, fr.HandoffPath)
	})
	refuse("over 8 KiB", func(fr *FrameResult) {
		_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+strings.Repeat("x", 9000)), 0o600)
	})
	refuse("without the issued header", func(fr *FrameResult) {
		_ = os.WriteFile(fr.HandoffPath, []byte("no header here"), 0o600)
	})
	refuse("header for another message", func(fr *FrameResult) {
		_ = os.WriteFile(fr.HandoffPath, []byte(strings.Replace(fr.Header, fr.MsgID, "ffffffffffffffff", 1)), 0o600)
	})
	// a planted symlink in place of peer-msg/ is refused at frame time
	_ = os.RemoveAll(filepath.Join(repo, ".lets", "cache", "peer-msg"))
	_ = os.Symlink(t.TempDir(), filepath.Join(repo, ".lets", "cache", "peer-msg"))
	if _, err := Frame(context.Background(), FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidWork, Kind: "ask"}); err == nil {
		t.Error("a planted peer-msg symlink must be refused")
	}
}

func TestTell_OrcaNotReadyPassthrough(t *testing.T) {
	fastLoops(t)
	repo := repoWithLets(t, "orca")
	home := claudeHome(t, []regRow{{101, sidMain, "MAIN", repo}, {102, sidFable, "MAIN-FABLE", repo}})
	writeTranscript(t, home, repo, sidFable, userText("2026-09-15T10:00:00Z", "go"), toolUse("2026-09-15T10:00:01Z", "t1", "Bash"))
	plantRole(t, repo, sidFable, "role: peer\npid: 102\norca_terminal: term_fable\nset: x\n")
	ops := &fakeOps{terms: []orcaTerm{{Handle: "term_fable", Path: repo, AgentType: "claude", State: "working"}}, screen: idleScreen}
	useOrca(t, ops)
	fr, err := Frame(context.Background(), FrameOptions{Cwd: repo, Session: sidMain, ToSession: sidFable, Kind: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nq"), 0o600)
	res, err := Tell(context.Background(), TellOptions{Cwd: repo, ToSession: sidFable, MsgID: fr.MsgID})
	if err != nil || !res.OK || res.Delivered || res.Route != "orca" || res.Reason != "peer_not_ready" || res.State != "tool_running" || len(ops.sends) != 0 || !res.ClaudeFallbackAllowed {
		t.Errorf("not ready: %+v %v", res, err)
	}
}

func TestWait_Usage(t *testing.T) {
	for _, o := range []WaitOptions{{ToSession: sidMain, SentAt: "2026-09-15T10:00:00Z"}, {ToSession: sidMain, SinceMessage: msg1}} {
		if _, err := Wait(context.Background(), o); err == nil || err.(*Error).Code != ExitUsage {
			t.Errorf("wait %+v: %v", o, err)
		}
	}
}
