//go:build unix

package tmuxcmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubNotify controls the seams Notify touches: tmux presence, the active-target
// probe (resolveTarget with no ref/cwd), the client list, and runTmux.
func stubNotify(t *testing.T, found bool, clients []byte, clientsErr error, run func(ctx context.Context, bin string, args ...string) ([]byte, error)) {
	t.Helper()
	oLook, oActive, oClients, oRun := lookTmux, activeTargetRaw, listClientsRaw, runTmux
	t.Cleanup(func() { lookTmux, activeTargetRaw, listClientsRaw, runTmux = oLook, oActive, oClients, oRun })
	lookTmux = func() (string, bool) { return "/usr/bin/tmux", found }
	activeTargetRaw = func(_ context.Context, _ string) ([]byte, error) { return []byte("active:0\n"), nil }
	listClientsRaw = func(_ context.Context, _ string) ([]byte, error) { return clients, clientsErr }
	if run != nil {
		runTmux = run
	}
}

func TestNotify_TitleRequired(t *testing.T) {
	res, err := Notify(context.Background(), NotifyOptions{})
	if err == nil || res.OK {
		t.Fatal("missing --title must be a hard error")
	}
	if ExitCode(err) != ExitUsage {
		t.Errorf("ExitCode = %d, want %d", ExitCode(err), ExitUsage)
	}
}

func TestNotify_TmuxNotFound(t *testing.T) {
	stubNotify(t, false, nil, nil, nil)
	res, err := Notify(context.Background(), NotifyOptions{Title: "gate"})
	if err != nil || !res.OK {
		t.Fatalf("tmux absence must degrade: err=%v ok=%v", err, res.OK)
	}
	if res.Notify.Notified || res.Notify.Reason != "tmux_not_found" {
		t.Fatalf("notify = %+v", res.Notify)
	}
}

// The regression test for the phantom-success bug: a running tmux server with
// zero attached clients must report no_client, NOT Notified=true.
func TestNotify_NoClientIsNotPhantomSuccess(t *testing.T) {
	stubNotify(t, true, []byte(""), nil, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		t.Fatal("display-message must NOT run when no client is attached")
		return nil, nil
	})
	res, err := Notify(context.Background(), NotifyOptions{Title: "gate"})
	if err != nil || !res.OK {
		t.Fatalf("no client must degrade gracefully: err=%v ok=%v", err, res.OK)
	}
	if res.Notify.Notified {
		t.Fatal("Notified=true with zero clients is a phantom success")
	}
	if res.Notify.Reason != "no_client" {
		t.Errorf("Reason = %q, want no_client", res.Notify.Reason)
	}
}

func TestNotify_BroadcastsToAttachedClients(t *testing.T) {
	var calls [][]string
	stubNotify(t, true, []byte("client-a\nclient-b\n"), nil, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	})
	res, err := Notify(context.Background(), NotifyOptions{Title: "gate", Body: "approve?"})
	if err != nil || !res.Notify.Notified {
		t.Fatalf("want Notified=true, got %+v (err=%v)", res.Notify, err)
	}
	if res.Notify.Clients != 2 {
		t.Errorf("Clients = %d, want 2", res.Notify.Clients)
	}
	// Every display-message must be client-scoped (-c) and never session-scoped
	// (-t <session>) - a -t call into a detached session is exactly the bug.
	var displays int
	for _, args := range calls {
		if args[0] != "display-message" {
			continue
		}
		displays++
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-c ") {
			t.Errorf("display-message not client-scoped: %v", args)
		}
		for i, a := range args {
			if a == "-t" {
				t.Errorf("display-message must not be target-scoped, got -t %s", args[i+1])
			}
		}
	}
	if displays != 2 {
		t.Errorf("got %d display-message calls, want 2 (one per client)", displays)
	}
}

func TestNotify_ClientDetachedMidLoop(t *testing.T) {
	first := true
	stubNotify(t, true, []byte("client-a\nclient-b\n"), nil, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "display-message" && first {
			first = false
			return []byte("client not found"), errors.New("exit 1")
		}
		return nil, nil
	})
	res, _ := Notify(context.Background(), NotifyOptions{Title: "gate"})
	if !res.Notify.Notified {
		t.Fatal("one client detaching mid-loop must not sink the whole notify")
	}
	if res.Notify.Clients != 1 {
		t.Errorf("Clients = %d, want 1", res.Notify.Clients)
	}
}

func TestNotify_AllClientsReject(t *testing.T) {
	stubNotify(t, true, []byte("client-a\n"), nil, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("nope"), errors.New("exit 1")
	})
	res, _ := Notify(context.Background(), NotifyOptions{Title: "gate"})
	if res.Notify.Notified || res.Notify.Reason != "tmux_error" {
		t.Fatalf("notify = %+v", res.Notify)
	}
}

func TestComposeMessage(t *testing.T) {
	got := composeMessage(NotifyOptions{Title: "task #123", Subtitle: "GATE 1", Body: "50% done"})
	want := "task ##123 - GATE 1 - 50% done"
	if got != want {
		t.Errorf("composeMessage = %q, want %q", got, want)
	}
}
