//go:build !unix

package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// fetchBounded: as fetch_unix.go without a process group (no Setpgid here);
// WaitDelay still unblocks Run once the deadline killed git. stderr is redacted
// the same way.
func fetchBounded(ctx context.Context, dir string, timeout time.Duration, args ...string) error {
	fctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(fctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = fetchWaitDelay
	if err := cmd.Run(); err != nil {
		if fctx.Err() != nil {
			return fmt.Errorf("timed out after %s", timeout)
		}
		return fmt.Errorf("%w: %s", err, redact.Creds(strings.TrimSpace(stderr.String())))
	}
	return nil
}
