//go:build unix

package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// fetchBounded runs `git -C dir <args>` (a fetch) so a hanging network cannot
// outlast timeout: its own process group, killed whole when the deadline passes
// (an ssh or remote-helper child holding stderr would otherwise keep Run blocked
// after git itself died), WaitDelay as the backstop, GIT_TERMINAL_PROMPT=0 so an
// https credential prompt fails at once. An ssh host-key or passphrase prompt reads
// /dev/tty: in the background group it stops on SIGTTIN and the deadline ends it.
// The group is killed only when the context ends while git runs (the deadline, or
// the caller's cancel) - never after the fetch finished, so a ControlPersist ssh
// master a successful fetch started survives. git's stderr is passed through
// redact.Creds (a leaf package) before it becomes error text: it reaches the stale
// warning and the refusal message of every envelope (Codex round 2).
//
// Setpgid also takes git out of the terminal's foreground group, so a Ctrl-C (or a
// SIGTERM to lets) reaches only this process (c2-F2). While git runs, SIGINT and
// SIGTERM are caught: the fetch's context is cancelled (cmd.Cancel kills the whole
// group), and once Run returned the same signal is re-delivered to this process, so
// its default action ends lets as before - an interrupt never becomes a stale
// resolution or a refusal. The returned error names the interrupt for the moment
// before the process dies. A signal this process inherited as ignored is left
// alone, as before. The handler is stopped and the watcher gone on every path.
func fetchBounded(ctx context.Context, dir string, timeout time.Duration, args ...string) error {
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(fctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = fetchWaitDelay

	var watched []os.Signal
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		if !signal.Ignored(s) {
			watched = append(watched, s)
		}
	}
	sigs := make(chan os.Signal, 1)
	if len(watched) > 0 { // Notify with no signal would relay every signal
		signal.Notify(sigs, watched...)
	}
	defer signal.Stop(sigs)
	done := make(chan struct{})
	caught := make(chan os.Signal, 1) // the watcher's one answer: the signal, or nil
	go func() {
		select {
		case s := <-sigs:
			cancel()
			caught <- s
		case <-done:
			caught <- nil
		}
	}()

	err := cmd.Run()
	close(done)
	sig := <-caught
	signal.Stop(sigs)
	if sig == nil {
		select {
		case sig = <-sigs: // arrived after the watcher left, before Stop
		default:
		}
	}
	if sig != nil {
		if s, ok := sig.(syscall.Signal); ok {
			_ = syscall.Kill(os.Getpid(), s)
		}
		return fmt.Errorf("interrupted by %s", sig)
	}
	if err != nil {
		if fctx.Err() != nil {
			return fmt.Errorf("timed out after %s", timeout)
		}
		return fmt.Errorf("%w: %s", err, redact.Creds(strings.TrimSpace(stderr.String())))
	}
	return nil
}
