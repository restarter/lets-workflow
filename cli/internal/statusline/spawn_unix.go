//go:build !windows

package statusline

import (
	"os/exec"
	"syscall"
)

// detachProcessGroup puts the child in its own process group on Unix.
// Without Setpgid, a SIGHUP/SIGTERM to the parent shell propagates to the
// child via the shared PG and kills the in-flight HTTP request - cache never
// updates. Equivalent to bash's `&` job control (which uses setpgid).
func detachProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
