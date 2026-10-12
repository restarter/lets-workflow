//go:build unix

package worktreecmd

import (
	"fmt"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

// LaunchCommand renders base (the agent command, e.g. `claude`) for a launch mode
// (lets-d3wu3) - the one renderer of the three levels: default -> base as is,
// auto -> base + ` --permission-mode auto`, all-in -> base + ` --permission-mode
// bypassPermissions`. "" is default. An unknown mode is an error, never a guess:
// all-in is selected only by name.
func LaunchCommand(mode, base string) (string, error) {
	switch mode {
	case "", "default":
		return base, nil
	case "auto":
		return base + " --permission-mode auto", nil
	case "all-in":
		return base + " --permission-mode bypassPermissions", nil
	}
	return "", fmt.Errorf("launch mode %q is not one of %v", mode, letsconfig.LaunchModes)
}
