//go:build !windows

package statusline

import (
	"os/exec"
	"testing"
)

// TestDetachProcessGroup_SetsSetpgid locks the contract that detached
// background fetchers run in their own process group.
//
// Without this, a SIGHUP/SIGTERM to the parent shell would propagate via
// shared PG and kill the in-flight HTTP request - cache never updates,
// statusline goes stale silently. Bash's `&` job control sets setpgid;
// our Go re-exec must mirror that.
//
// Closes S19 from the 2026-05-08 review.
func TestDetachProcessGroup_SetsSetpgid(t *testing.T) {
	cmd := exec.Command("/bin/true")
	detachProcessGroup(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr unset after detachProcessGroup")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Errorf("Setpgid = false, want true (parent SIGHUP would kill child without it)")
	}
}
