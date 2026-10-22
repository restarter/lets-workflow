//go:build unix

package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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
	if err := cmd.Run(); err != nil {
		if fctx.Err() != nil {
			return fmt.Errorf("timed out after %s", timeout)
		}
		return fmt.Errorf("%w: %s", err, redact.Creds(strings.TrimSpace(stderr.String())))
	}
	return nil
}
