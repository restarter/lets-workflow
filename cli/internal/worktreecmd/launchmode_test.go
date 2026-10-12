//go:build unix

package worktreecmd_test

import (
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/worktreecmd"
)

func TestLaunchCommand(t *testing.T) {
	for _, c := range []struct{ mode, base, want string }{
		{"", "claude", "claude"},
		{"default", "claude", "claude"},
		{"auto", "claude", "claude --permission-mode auto"},
		{"all-in", "claude", "claude --permission-mode bypassPermissions"},
		{"auto", "docker compose run --rm dev claude --model opus", "docker compose run --rm dev claude --model opus --permission-mode auto"},
	} {
		got, err := worktreecmd.LaunchCommand(c.mode, c.base)
		if err != nil || got != c.want {
			t.Errorf("LaunchCommand(%q, %q) = %q, %v; want %q", c.mode, c.base, got, err, c.want)
		}
	}
	for _, bad := range []string{"bypassPermissions", "All-in", "plan", "yolo"} {
		if got, err := worktreecmd.LaunchCommand(bad, "claude"); err == nil {
			t.Errorf("LaunchCommand(%q) = %q, want an error - an unknown mode is never guessed", bad, got)
		}
	}
}
