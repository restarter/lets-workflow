//go:build windows

package statusline

import "os/exec"

// detachProcessGroup is a no-op on Windows. Native Windows support (with
// CREATE_NEW_PROCESS_GROUP / DETACHED_PROCESS / CREATE_NO_WINDOW) is
// deferred to lets-ds6bc (Statusline 2.0).
func detachProcessGroup(cmd *exec.Cmd) {
	_ = cmd
}
