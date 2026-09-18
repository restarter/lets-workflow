//go:build unix

package handoffcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/fsutil"
	"github.com/restarter/lets-workflow/cli/internal/orcacmd"
)

type fakeOps struct {
	terms     []orcacmd.Terminal
	relist    []orcacmd.Terminal // returned from the second Terminals call on, when set
	listed    int
	source    string
	screen    []string
	draft     string
	receipt   orcacmd.Receipt
	sendFails []*orcacmd.Failure // consumed one per SendText call
	up        bool
	calls     []string
	sent      string
	sentTo    []string
}

func (f *fakeOps) Terminals(context.Context) ([]orcacmd.Terminal, *orcacmd.Failure) {
	f.calls = append(f.calls, "terminals")
	f.listed++
	if f.listed > 1 && f.relist != nil {
		return f.relist, nil
	}
	return f.terms, nil
}

func (f *fakeOps) Screen(context.Context, string) (orcacmd.Frame, *orcacmd.Failure) {
	f.calls = append(f.calls, "screen")
	return orcacmd.Frame{Source: f.source, Lines: f.screen, Draft: f.draft}, nil
}

func (f *fakeOps) SendText(_ context.Context, h string, text string) (orcacmd.Receipt, *orcacmd.Failure) {
	f.calls = append(f.calls, "send")
	f.sent, f.sentTo = text, append(f.sentTo, h)
	if len(f.sendFails) > 0 {
		fail := f.sendFails[0]
		f.sendFails = f.sendFails[1:]
		if fail != nil {
			return orcacmd.Receipt{}, fail
		}
	}
	return f.receipt, nil
}

func (f *fakeOps) CreateTerminal(_ context.Context, _, _, command string) (string, *orcacmd.Failure) {
	f.calls = append(f.calls, "create "+command)
	return "term_new", nil
}

func (f *fakeOps) WaitStartup(_ context.Context, h, _, _ string) (string, bool, *orcacmd.Failure) {
	f.calls = append(f.calls, "wait")
	return h, f.up, nil
}

// useFake installs ops as the Orca surface and returns a checkout with a brief.
func useFake(t *testing.T, ops *fakeOps) (root, brief string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "w")
	brief = filepath.Join(handoffsDir(t, root), "2026-09-18-1600-lets-w5tm5-handoff.md")
	if err := os.WriteFile(brief, []byte("# brief\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ops.source == "" {
		ops.source = "screen"
	}
	oldOps, oldLock, oldFP := newOps, sendLockDir, fingerprint
	newOps = func(context.Context) (orcaOps, *orcacmd.Failure) { return ops, nil }
	lockDir := t.TempDir()
	sendLockDir = func() string { return lockDir }
	fingerprint = func(string) string { return "fp" }
	t.Cleanup(func() { newOps, sendLockDir, fingerprint = oldOps, oldLock, oldFP })
	return root, brief
}

func term(root, handle, agent, state string) orcacmd.Terminal {
	return orcacmd.Terminal{Handle: handle, Path: root, Title: handle + " tab", AgentType: agent, State: state, PaneKey: "tab:" + handle, Writable: true, Connected: true}
}

var (
	proven   = orcacmd.Receipt{InputAccepted: true, TurnStarted: true}
	accepted = orcacmd.Receipt{InputAccepted: true}
)

func TestTargets_AgentsOnly(t *testing.T) {
	ops := &fakeOps{}
	root, _ := useFake(t, ops)
	other := t.TempDir()
	old, newer := term(root, "term_old", "codex", "done"), term(root, "term_new", "antigravity", "")
	old.LastOutputAt, newer.LastOutputAt = 10, 20
	peer := term(root, "term_peer", "claude", "done")
	peer.Title = "✳ Peer-Messaging"
	ro, off, shell, elsewhere := term(root, "term_ro", "claude", ""), term(root, "term_off", "claude", ""), term(root, "term_sh", "", ""), term(other, "term_other", "codex", "")
	ro.Writable, off.Connected = false, false
	ops.terms = []orcacmd.Terminal{old, term(root, "term_self", "claude", ""), elsewhere, ro, off, shell, newer, peer}
	res, err := Targets(context.Background(), TargetsOptions{Root: root, Self: "term_self"})
	if err != nil || !res.OK || !res.Targets.Available {
		t.Fatalf("targets: %+v %v", res, err)
	}
	var got []string
	for _, tg := range res.Targets.Terminals {
		got = append(got, tg.Handle)
	}
	if strings.Join(got, " ") != "term_new term_old term_peer" {
		t.Errorf("targets %v", got)
	}
	if res, _ := Targets(context.Background(), TargetsOptions{Root: root, Match: "term_old"}); len(res.Targets.Terminals) != 1 || res.Targets.Terminals[0].Handle != "term_old" {
		t.Errorf("handle match: %+v", res.Targets.Terminals)
	}
	if res, _ := Targets(context.Background(), TargetsOptions{Root: root, Match: "peer-messaging"}); len(res.Targets.Terminals) != 1 || res.Targets.Terminals[0].Handle != "term_peer" {
		t.Errorf("title match: %+v", res.Targets.Terminals)
	}
	// An agent name matches the agent, not the title: live tab titles do not carry it.
	for q, want := range map[string]string{"antigravity": "term_new", "Codex": "term_old"} {
		if res, _ := Targets(context.Background(), TargetsOptions{Root: root, Match: q}); len(res.Targets.Terminals) != 1 || res.Targets.Terminals[0].Handle != want {
			t.Errorf("agent match %q: %+v", q, res.Targets.Terminals)
		}
	}
}

func TestTargets_OrcaAbsent(t *testing.T) {
	useFake(t, &fakeOps{})
	newOps = func(context.Context) (orcaOps, *orcacmd.Failure) {
		return nil, &orcacmd.Failure{Reason: orcacmd.ReasonNotFound, Verb: "lookup"}
	}
	res, err := Targets(context.Background(), TargetsOptions{Root: "/w"})
	if err != nil || !res.OK || res.Targets.Available || res.Targets.Reason != orcacmd.ReasonNotFound {
		t.Errorf("targets: %+v %v", res.Targets, err)
	}
}

func TestSend_ReadyCodex(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "done")}, frame(t, "codex-empty")
	res, err := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if err != nil || res.Send.Delivery != DeliveryProven || res.Send.InputLine != lineReady || res.Send.Fingerprint != "fp" || res.Send.SentAt == "" {
		t.Fatalf("send: %+v %v", res.Send, err)
	}
	if strings.Join(ops.calls, " ") != "terminals screen send" || ops.sent != Pointer(brief) || res.Send.Agent != "codex" {
		t.Errorf("calls %v sent %q", ops.calls, ops.sent)
	}
}

func TestSend_NotClearNothingTyped(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "")}, frame(t, "codex-typed")
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Delivery != DeliverySkipped || res.Send.Reason != "input_line_not_clear" || len(ops.sentTo) != 0 {
		t.Errorf("send: %+v calls %v", res.Send, ops.calls)
	}
}

func TestSend_DraftRefused(t *testing.T) {
	// Claude keeps typed text in Orca's draft; the screen shows an empty `❯`. A send
	// would submit draft and pointer together (T0 probe), so the draft refuses -
	// for any agent, Antigravity included.
	for _, agent := range []string{"claude", "antigravity"} {
		ops := &fakeOps{receipt: proven, draft: "half typed"}
		root, brief := useFake(t, ops)
		ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", agent, "done")}, frame(t, "claude-typed")
		res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
		if res.Send.Delivery != DeliverySkipped || res.Send.Reason != "input_line_not_clear" || res.Send.InputLine != lineNotClear || len(ops.sentTo) != 0 {
			t.Errorf("%s: %+v", agent, res.Send)
		}
	}
}

func TestSend_BusyByState(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	ops.terms = []orcacmd.Terminal{term(root, "term_a", "claude", "working")}
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Reason != "target_busy" || strings.Join(ops.calls, " ") != "terminals" {
		t.Errorf("send: %+v calls %v", res.Send, ops.calls)
	}
}

func TestSend_BusyByScreen(t *testing.T) {
	for agent, fixture := range map[string]string{"codex": "codex-working", "claude": "claude-working"} {
		ops := &fakeOps{receipt: proven}
		root, brief := useFake(t, ops)
		ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", agent, "")}, frame(t, fixture)
		res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
		if res.Send.Reason != "target_busy" || len(ops.sentTo) != 0 {
			t.Errorf("%s: %+v", agent, res.Send)
		}
	}
}

func TestSend_AntigravitySentUnknown(t *testing.T) {
	ops := &fakeOps{receipt: accepted}
	root, brief := useFake(t, ops)
	ops.terms = []orcacmd.Terminal{term(root, "term_a", "antigravity", "")}
	ops.screen = []string{"> /plan ", "  plan · Gemini 3.8 Flash · high"}
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if len(ops.sentTo) != 1 || res.Send.InputLine != lineUnknown || res.Send.Delivery != DeliveryUnproven || len(res.Send.ScreenTail) == 0 {
		t.Errorf("send: %+v calls %v", res.Send, ops.calls)
	}
}

func TestSend_ScreenUnavailable(t *testing.T) {
	ops := &fakeOps{receipt: proven, source: "stream"}
	root, brief := useFake(t, ops)
	ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "")}, frame(t, "codex-typed")
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if len(ops.sentTo) != 1 || res.Send.InputLine != lineUnknown || res.Send.Delivery != DeliveryProven {
		t.Errorf("send: %+v", res.Send)
	}
}

func TestSend_NotAnAgent(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	ops.terms = []orcacmd.Terminal{term(root, "term_a", "", "")}
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Reason != "not_an_agent" || strings.Join(ops.calls, " ") != "terminals" {
		t.Errorf("send: %+v calls %v", res.Send, ops.calls)
	}
}

func TestSend_UnprovenReadsBackOnce(t *testing.T) {
	ops := &fakeOps{receipt: accepted}
	root, brief := useFake(t, ops)
	token := "ghp" + "_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	ops.terms = []orcacmd.Terminal{term(root, "term_a", "codex", "")}
	ops.screen = append(frame(t, "codex-empty"), "fetch https://u:p@h/x", "token "+token)
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	tail := strings.Join(res.Send.ScreenTail, "\n")
	if res.Send.Delivery != DeliveryUnproven || res.Send.Reason != "delivery_unconfirmed" || len(ops.sentTo) != 1 {
		t.Fatalf("send: %+v", res.Send)
	}
	if strings.Contains(tail, "u:p@") || strings.Contains(tail, token) || len(res.Send.ScreenTail) > screenLines {
		t.Errorf("read-back not redacted or capped:\n%s", tail)
	}
}

func TestSend_StaleHandleRejoinsOnce(t *testing.T) {
	stale := &orcacmd.Failure{Reason: orcacmd.ReasonHandleStale, Verb: "terminal send"}
	ops := &fakeOps{receipt: proven, sendFails: []*orcacmd.Failure{stale}}
	root, brief := useFake(t, ops)
	a := term(root, "term_a", "codex", "")
	b := a
	b.Handle = "term_b"
	ops.terms, ops.relist, ops.screen = []orcacmd.Terminal{a}, []orcacmd.Terminal{b}, frame(t, "codex-empty")
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if strings.Join(ops.sentTo, " ") != "term_a term_b" || res.Send.Delivery != DeliveryProven || res.Send.Handle != "term_b" {
		t.Fatalf("send: %+v sentTo %v", res.Send, ops.sentTo)
	}
	if len(res.Steps) != 1 || res.Steps[0].Status != StepWarn || !strings.Contains(res.Steps[0].Message, "term_a") || !strings.Contains(res.Steps[0].Message, "term_b") {
		t.Errorf("steps: %+v", res.Steps)
	}
}

func TestSend_StaleHandlePaneGone(t *testing.T) {
	stale := &orcacmd.Failure{Reason: orcacmd.ReasonHandleStale, Verb: "terminal send"}
	ops := &fakeOps{receipt: proven, sendFails: []*orcacmd.Failure{stale}}
	root, brief := useFake(t, ops)
	ops.terms, ops.relist, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "")}, []orcacmd.Terminal{term(root, "term_x", "codex", "")}, frame(t, "codex-empty")
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Delivery != DeliveryFailed || res.Send.Reason != orcacmd.ReasonHandleStale || strings.Join(ops.sentTo, " ") != "term_a" {
		t.Errorf("send: %+v sentTo %v", res.Send, ops.sentTo)
	}
}

func TestSend_OtherFailureNoRetry(t *testing.T) {
	ops := &fakeOps{receipt: proven, sendFails: []*orcacmd.Failure{{Reason: orcacmd.ReasonError, Verb: "terminal send"}}}
	root, brief := useFake(t, ops)
	ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "")}, frame(t, "codex-empty")
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Delivery != DeliveryFailed || res.Send.Reason != orcacmd.ReasonError || len(ops.sentTo) != 1 {
		t.Errorf("send: %+v sentTo %v", res.Send, ops.sentTo)
	}
}

func TestSend_NewCodex(t *testing.T) {
	ops := &fakeOps{receipt: proven, up: true}
	root, brief := useFake(t, ops)
	ops.screen = frame(t, "codex-empty")
	ops.terms = []orcacmd.Terminal{term(root, "term_new", "codex", "")}
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, New: "codex"})
	if got := strings.Join(ops.calls, ", "); got != "create codex --sandbox read-only, wait, terminals, screen, send" {
		t.Errorf("calls: %s", got)
	}
	if !res.Send.Created || res.Send.Delivery != DeliveryProven || res.Send.Agent != "codex" {
		t.Errorf("send: %+v", res.Send)
	}
	ops2 := &fakeOps{receipt: proven, up: false}
	root, brief = useFake(t, ops2)
	res, _ = Send(context.Background(), SendOptions{Root: root, Brief: brief, New: "codex"})
	if res.Send.Reason != "startup_not_idle" || len(ops2.sentTo) != 0 || !res.Send.Created {
		t.Errorf("not idle: %+v", res.Send)
	}
}

func TestSend_LockBusy(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	ops.terms, ops.screen = []orcacmd.Terminal{term(root, "term_a", "codex", "")}, frame(t, "codex-empty")
	lf, err := os.OpenFile(filepath.Join(sendLockDir(), "handoff-send-term_a.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lf.Close() }()
	if err := fsutil.LockFile(lf); err != nil {
		t.Fatal(err)
	}
	old := sendLockWait
	sendLockWait = 50 * time.Millisecond
	t.Cleanup(func() { sendLockWait = old })
	res, _ := Send(context.Background(), SendOptions{Root: root, Brief: brief, Terminal: "term_a"})
	if res.Send.Reason != "another_send_in_progress" || len(ops.sentTo) != 0 {
		t.Errorf("send: %+v", res.Send)
	}
}

func TestSend_Usage(t *testing.T) {
	ops := &fakeOps{receipt: proven}
	root, brief := useFake(t, ops)
	for _, o := range []SendOptions{
		{Terminal: "term_a", New: "codex"},
		{},
		{New: "claude"},
		{Terminal: "bad handle"},
	} {
		o.Root, o.Brief = root, brief
		res, err := Send(context.Background(), o)
		var he *Error
		if res.OK || !errors.As(err, &he) || he.ExitCode() != ExitUsage || res.Error == nil || res.Error.Kind != "usage" {
			t.Errorf("%+v: ok=%v err=%v", o, res.OK, err)
		}
	}
	if len(ops.calls) != 0 {
		t.Errorf("usage errors must not reach Orca: %v", ops.calls)
	}
}
