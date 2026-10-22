//go:build unix

package gitutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A remote helper that never answers and leaves a descendant holding git's stderr
// (the ssh / remote-helper case; the ext:: transport runs `sh -c "sleep 30"`, which
// inherits it). Only the process-group kill closes that pipe at the deadline: with
// git alone killed, Run would wait the whole WaitDelay (Codex round 1). A fetch that
// finishes is never followed by a group kill - read fetchBounded, there is no kill
// on its success path.
func TestFetchBounded_KillsTheProcessGroup(t *testing.T) {
	_, clone := remoteSetup(t)
	start := time.Now()
	err := fetchBounded(context.Background(), clone, time.Second, "-c", "protocol.ext.allow=always", "fetch", "--no-tags", "ext::sh -c sleep% 30", "main")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if d := time.Since(start); d >= time.Second+fetchWaitDelay {
		t.Errorf("returned after %s: a descendant kept stderr open until WaitDelay - the group was not killed", d)
	}
}

// A failing fetch whose stderr carries a credential-bearing URL: the error text -
// which every envelope's stale warning and refusal message embed - is redacted
// (Codex round 2). The ext:: helper prints the URL to git's stderr and fails.
func TestFetchBounded_RedactsCredentials(t *testing.T) {
	_, clone := remoteSetup(t)
	err := fetchBounded(context.Background(), clone, 5*time.Second, "-c", "protocol.ext.allow=always", "fetch", "--no-tags", "ext::sh -c echo% https://u:s3cret@host/x% >&2;% exit% 1", "main")
	if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("err = %v, want the credential redacted", err)
	}
}

// fetchInterruptHelperEnv marks the re-run test binary as the interrupt helper; its
// value is the clone the helper fetches in.
const fetchInterruptHelperEnv = "LETS_TEST_FETCH_INTERRUPT_HELPER"

// TestFetchBounded_InterruptHelper is the helper process of
// TestFetchBounded_InterruptEndsTheFetch, not a test of its own: it runs a fetch that
// hangs on the same ext:: remote as the kill test, with a deadline far beyond the
// test's wait, so only the interrupt can end it.
func TestFetchBounded_InterruptHelper(t *testing.T) {
	clone := os.Getenv(fetchInterruptHelperEnv)
	if clone == "" {
		t.Skip("the helper process of TestFetchBounded_InterruptEndsTheFetch")
	}
	err := fetchBounded(context.Background(), clone, time.Minute, "-c", "protocol.ext.allow=always", "fetch", "--no-tags", "ext::sh -c sleep% 30", "main")
	// The re-delivered signal ends this process; a helper still here was not ended by it.
	time.Sleep(5 * time.Second)
	t.Fatalf("still alive 5s after fetchBounded returned (%v): the interrupt was not re-delivered", err)
}

// fetchChild finds the helper's one child - the fetch's git - and its process group.
func fetchChild(t *testing.T, parent int) (pid, pgid int) {
	t.Helper()
	out, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "pgid=").Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) != 3 {
			continue
		}
		if pp, _ := strconv.Atoi(f[1]); pp == parent {
			pid, _ = strconv.Atoi(f[0])
			pgid, _ = strconv.Atoi(f[2])
			return pid, pgid
		}
	}
	return 0, 0
}

// groupRuns reports whether a process of group pgid runs the program name (`ps`'s
// comm: a full path on macOS, the bare name on Linux - compared by base name).
func groupRuns(t *testing.T, pgid int, name string) bool {
	t.Helper()
	out, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "pgid=", "-o", "comm=").Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		if g, _ := strconv.Atoi(f[1]); g == pgid && filepath.Base(strings.Join(f[2:], " ")) == name {
			return true
		}
	}
	return false
}

// Ctrl-C reaches lets but not the fetch, which Setpgid moved out of the terminal's
// foreground group (c2-F2): the interrupt must end the whole fetch group AND still
// end lets by that signal - never a hanging remote orphaned, never an interrupt
// turned into a stale resolution. Without fetchBounded's signal handling the helper
// dies by SIGINT at once, but the fetch group (the 30 s sleep) outlives it.
func TestFetchBounded_InterruptEndsTheFetch(t *testing.T) {
	if signal.Ignored(os.Interrupt) {
		t.Skip("SIGINT is ignored in this process tree; the helper would inherit it")
	}
	_, clone := remoteSetup(t)
	helper := exec.Command(os.Args[0], "-test.run=^TestFetchBounded_InterruptHelper$")
	helper.Env = append(os.Environ(), fetchInterruptHelperEnv+"="+clone)
	var out bytes.Buffer
	helper.Stdout, helper.Stderr = &out, &out
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = helper.Wait(); close(exited) }()
	stopHelper := func() {
		_ = helper.Process.Kill()
		<-exited
	}

	// Go sets the child's group only in the child, between fork and exec: until then
	// ps shows git in the helper's group, which is this test's (and go test's). So
	// poll until git leads its own group, and arm the group kill only after that -
	// never on a group this test runs in.
	pid, pgid := 0, 0
	for deadline := time.Now().Add(15 * time.Second); pid == 0 || pgid != pid; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			stopHelper()
			if pid != 0 {
				t.Fatalf("the fetch's git %d is in group %d, not leading its own", pid, pgid)
			}
			t.Fatalf("the helper never started its fetch:\n%s", out.String())
		}
		pid, pgid = fetchChild(t, helper.Process.Pid)
	}
	if pgid > 0 {
		t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) }) // never leave the sleep behind
	}
	// Signal only once the descendant that holds stderr exists, so a cancellation that
	// killed git alone cannot pass for want of a process to orphan (Codex round 1 on
	// the branch). The group is git, then `git remote-ext`, then `sleep` (the `sh -c
	// sleep 30` remote-ext runs, exec'd by sh) - a probe of the group showed all
	// three. A member count is not enough: two members can be git + remote-ext before
	// the sleep exists, and remote-ext starts its command only on git's connect and
	// exits on EOF before it, so a git-only kill then would leave nothing behind.
	for deadline := time.Now().Add(15 * time.Second); !groupRuns(t, pgid, "sleep"); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			stopHelper()
			t.Fatalf("no sleep in the fetch's group %d - its remote helper never started the command:\n%s", pgid, out.String())
		}
	}

	if err := helper.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		stopHelper()
		t.Fatalf("the helper outlived the interrupt by 10s (the remote sleeps 30s):\n%s", out.String())
	}
	if ws, ok := helper.ProcessState.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Errorf("the helper ended with %v, want death by the re-delivered SIGINT:\n%s", helper.ProcessState, out.String())
	}
	for deadline := time.Now().Add(3 * time.Second); syscall.Kill(-pgid, 0) == nil; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the fetch's process group %d outlived the interrupt: the hanging remote was orphaned", pgid)
		}
	}
}
