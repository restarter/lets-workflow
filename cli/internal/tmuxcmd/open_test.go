//go:build unix

package tmuxcmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubTmux swaps every exec seam and restores them at test end.
func stubTmux(t *testing.T, found, inside bool, run func(ctx context.Context, bin string, args ...string) ([]byte, error), panes func(ctx context.Context, bin string) ([]byte, error)) {
	t.Helper()
	oLook, oInside, oRun, oPanes := lookTmux, insideTmux, runTmux, listPanesRaw
	t.Cleanup(func() { lookTmux, insideTmux, runTmux, listPanesRaw = oLook, oInside, oRun, oPanes })
	lookTmux = func() (string, bool) { return "/usr/bin/tmux", found }
	insideTmux = func() bool { return inside }
	if run != nil {
		runTmux = run
	}
	if panes != nil {
		listPanesRaw = panes
	}
}

func TestOpen_PathInvalid(t *testing.T) {
	res, err := Open(context.Background(), OpenOptions{Path: ""})
	if err == nil || res.OK {
		t.Fatal("empty path must be a hard error")
	}
	if ExitCode(err) != ExitPathInvalid {
		t.Errorf("ExitCode = %d, want %d", ExitCode(err), ExitPathInvalid)
	}
}

func TestOpen_TmuxNotFound(t *testing.T) {
	stubTmux(t, false, false, nil, nil)
	res, err := Open(context.Background(), OpenOptions{Path: t.TempDir(), Command: "claude '/lets:start x'"})
	if err != nil || !res.OK {
		t.Fatalf("tmux absence must degrade gracefully: err=%v ok=%v", err, res.OK)
	}
	if res.Launch.Launched || res.Launch.Reason != "tmux_not_found" {
		t.Fatalf("launch = %+v", res.Launch)
	}
	if !strings.Contains(res.Launch.FallbackCommand, "claude '/lets:start x'") {
		t.Errorf("fallback must reproduce --command, got %q", res.Launch.FallbackCommand)
	}
}

func TestOpen_AlreadyOpen(t *testing.T) {
	dir := t.TempDir()
	stubTmux(t, true, false, nil, func(_ context.Context, _ string) ([]byte, error) {
		return []byte("work\t2\teditor\t" + dir + "\n"), nil
	})
	res, _ := Open(context.Background(), OpenOptions{Path: dir})
	if res.Launch.Launched || res.Launch.Reason != "already_open" {
		t.Fatalf("launch = %+v", res.Launch)
	}
	if res.Launch.ExistingTarget != "work:2" {
		t.Errorf("ExistingTarget = %q, want work:2", res.Launch.ExistingTarget)
	}
}

func TestOpen_ForceSkipsGuard(t *testing.T) {
	dir := t.TempDir()
	stubTmux(t, true, true,
		func(_ context.Context, _ string, _ ...string) ([]byte, error) { return []byte("work:3\n"), nil },
		func(_ context.Context, _ string) ([]byte, error) { return []byte("work\t2\te\t" + dir + "\n"), nil })
	res, _ := Open(context.Background(), OpenOptions{Path: dir, Force: true})
	if !res.Launch.Launched {
		t.Fatalf("--force must bypass the duplicate guard: %+v", res.Launch)
	}
}

func TestOpen_InsideTmux_NoAttachCommand(t *testing.T) {
	stubTmux(t, true, true,
		func(_ context.Context, _ string, _ ...string) ([]byte, error) { return []byte("cur:7\n"), nil }, nil)
	res, _ := Open(context.Background(), OpenOptions{Path: t.TempDir(), Name: "demo"})
	if !res.Launch.InExistingSession || res.Launch.AttachCommand != "" {
		t.Fatalf("inside $TMUX must not surface an attach command: %+v", res.Launch)
	}
	if res.Launch.Target != "cur:7" {
		t.Errorf("Target = %q, want cur:7", res.Launch.Target)
	}
}

func TestOpen_OutsideTmux_Detached(t *testing.T) {
	stubTmux(t, true, false, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "has-session" {
			return nil, errors.New("can't find session")
		}
		return []byte("demo:0\n"), nil
	}, func(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("no server running") })
	res, _ := Open(context.Background(), OpenOptions{Path: t.TempDir(), Name: "demo"})
	if res.Launch.InExistingSession {
		t.Error("outside $TMUX must not report in_existing_session")
	}
	if res.Launch.AttachCommand != "tmux attach -t 'demo'" {
		t.Errorf("AttachCommand = %q", res.Launch.AttachCommand)
	}
}

func TestOpen_TmuxErrorDegrades(t *testing.T) {
	stubTmux(t, true, true, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("boom"), errors.New("exit 1")
	}, nil)
	res, err := Open(context.Background(), OpenOptions{Path: t.TempDir()})
	if err != nil || !res.OK {
		t.Fatal("a tmux error must never hard-fail")
	}
	if res.Launch.Reason != "tmux_error" {
		t.Errorf("Reason = %q, want tmux_error", res.Launch.Reason)
	}
}

// A long --description must NEVER reach the window name: the tmux window name is
// the status line. It goes to the @lets_task window option instead.
func TestOpen_DescriptionDoesNotBloatWindowName(t *testing.T) {
	const desc = "lets-0np5i · Add tmux worktree launcher (LETS_LAUNCHER=tmux) - mirror cmux"
	var calls [][]string
	stubTmux(t, true, true, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte("cur:1\n"), nil
	}, nil)

	res, _ := Open(context.Background(), OpenOptions{Path: t.TempDir(), Name: "tmux-launcher", Description: desc})
	if !res.Launch.Launched {
		t.Fatalf("launch failed: %+v", res.Launch)
	}

	var sawStamp bool
	for _, args := range calls {
		if args[0] == "new-window" {
			for i, a := range args {
				if a == "-n" && strings.Contains(args[i+1], "Add-tmux-worktree-launcher") {
					t.Fatalf("window name carries the description: %q", args[i+1])
				}
			}
		}
		if args[0] == "set-option" && args[len(args)-2] == "@lets_task" {
			sawStamp = true
			if args[len(args)-1] != desc {
				t.Errorf("@lets_task = %q, want the raw description", args[len(args)-1])
			}
		}
	}
	if !sawStamp {
		t.Error("--description must be stamped into the @lets_task window option")
	}
}
