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

// The hub addresses another project's session by id AND repo: looked up in the hub's
// own repo the target is not a peer there, and a name would match the wrong session.
func TestTell_ForeignRepoTarget(t *testing.T) {
	ctx := context.Background()
	hub := repoWithLets(t, "")
	foreign := gitRepo(t)
	claudeHome(t, []regRow{{101, sidMain, "HUB", hub}, {103, sidWork, "MAIN-PWA", foreign}})
	frame := func() *FrameResult {
		t.Helper()
		fr, err := Frame(ctx, FrameOptions{Cwd: hub, Session: sidMain, ToSession: sidWork, Kind: "tell"})
		if err != nil {
			t.Fatalf("frame a foreign live session: %v", err)
		}
		_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nplease check"), 0o600)
		return fr
	}
	if res, err := Tell(ctx, TellOptions{Cwd: hub, ToSession: sidWork, MsgID: frame().MsgID}); err != nil || res.Reason != "peer_unreachable" {
		t.Errorf("without a repo the foreign session is no peer of the hub's repo: %+v %v", res, err)
	}
	fr := frame()
	res, err := Tell(ctx, TellOptions{Cwd: hub, ToSession: sidWork, MsgID: fr.MsgID, Repo: foreign})
	if err != nil || res.Route != "claude" || res.Reason != "claude_transport_model_send" || !strings.HasSuffix(res.Text, "please check") {
		t.Errorf("with --repo the target is looked up in its own repo: %+v %v", res, err)
	}
	if _, err := os.Stat(fr.HandoffPath); !os.IsNotExist(err) {
		t.Error("the handoff in the hub's checkout is consumed")
	}
	fr = frame()
	if _, err := Tell(ctx, TellOptions{Cwd: hub, ToSession: sidWork, MsgID: fr.MsgID, Repo: filepath.Join(foreign, "missing")}); err == nil || err.(*Error).Kind != "repo_invalid" {
		t.Errorf("an invalid repo is refused: %v", err)
	}
	if _, err := os.Stat(fr.HandoffPath); err != nil {
		t.Error("a refused target leaves the handoff unread")
	}
}

// Orca is the calling session's opt-in: reading or messaging another project consults
// Orca exactly when THIS session selects it, whatever that project's LETS_LAUNCHER says.
func TestForeignRepo_CallerDecidesOrca(t *testing.T) {
	ctx := context.Background()
	hub := repoWithLets(t, "terminal")
	foreign := gitRepo(t)
	setLauncher := func(repo, launcher string) {
		_ = os.MkdirAll(filepath.Join(repo, ".lets"), 0o755)
		_ = os.WriteFile(filepath.Join(repo, ".lets", ".env"), []byte("LETS_LAUNCHER="+launcher+"\n"), 0o644)
	}
	setLauncher(foreign, "orca")
	claudeHome(t, []regRow{{101, sidMain, "HUB", hub}, {103, sidWork, "MAIN-PWA", foreign}})
	calls := useOrca(t, &fakeOps{})

	fr, err := Frame(ctx, FrameOptions{Cwd: hub, Session: sidMain, ToSession: sidWork, Kind: "tell"})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(fr.HandoffPath, []byte(fr.Header+"\nq"), 0o600)
	if res, err := Tell(ctx, TellOptions{Cwd: hub, ToSession: sidWork, MsgID: fr.MsgID, Repo: foreign}); err != nil || res.Route != "claude" {
		t.Errorf("an opted-out caller routes a foreign peer over Claude: %+v %v", res, err)
	}
	if _, err := Tail(ctx, TailOptions{Cwd: hub, ToSession: sidWork, Repo: foreign}); err != nil {
		t.Errorf("tail --repo: %v", err)
	}
	if _, err := Who(ctx, WhoOptions{Cwd: hub, Repo: foreign}); err != nil {
		t.Errorf("who --repo: %v", err)
	}
	if *calls != 0 {
		t.Errorf("a foreign LETS_LAUNCHER=orca must not switch Orca on for a terminal caller (%d lookups)", *calls)
	}

	setLauncher(hub, "orca")
	setLauncher(foreign, "terminal")
	if _, err := Who(ctx, WhoOptions{Cwd: hub, Repo: foreign}); err != nil {
		t.Fatal(err)
	}
	if *calls == 0 {
		t.Error("an Orca caller consults Orca for a project whose own launcher is terminal")
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
