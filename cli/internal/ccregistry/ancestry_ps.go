//go:build unix && !linux

package ccregistry

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// parentOf asks /bin/ps for the ppid of pid (darwin and the BSDs have no /proc).
// An exited pid makes ps fail, which is an error here.
func parentOf(pid int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
