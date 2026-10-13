//go:build unix

package worktreecmd_test

import (
	"slices"
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

func TestClassifyLaunch(t *testing.T) {
	for _, c := range []struct {
		cmd, class string
		warnings   []string
	}{
		{"claude", "default", nil},
		{"", "default", nil},
		{"claude --model opus", "default", nil},
		{"claude --permission-mode plan", "default", nil},
		{"claude --permission-mode auto", "auto", nil},
		{"claude --permission-mode=auto", "auto", nil},
		{"claude --permission-mode bypassPermissions", "bypass", nil},
		{"claude --permission-mode=bypassPermissions", "bypass", nil},
		{"claude --dangerously-skip-permissions", "bypass", nil},
		{"claude --allow-dangerously-skip-permissions", "bypass", nil},
		{"claude --permission-mode auto --dangerously-skip-permissions", "bypass", nil},
		{`claude --settings '{"permissions":{"defaultMode":"bypassPermissions"}}'`, "bypass", []string{"settings_override"}},
		{`claude --settings='{"permissions": {"defaultMode": "bypassPermissions"}}'`, "bypass", []string{"settings_override"}},
		{"claude --settings ./team-settings.json", "default", []string{"settings_override"}},
		{`claude --settings '{"crossSessionInbound":"accept"}'`, "default", []string{"settings_override", "cross_session_inbound"}},
		{`claude --permission-mode auto --settings "{\"crossSessionInbound\": \"accept\"}"`, "auto", []string{"settings_override", "cross_session_inbound"}},
		{`docker compose run --rm dev sh -c 'claude --permission-mode bypassPermissions'`, "bypass", nil},
		{`docker compose run --rm dev sh -c "claude --permission-mode=auto"`, "auto", nil},
		{"claude --name bypassPermissions", "default", nil},
		{"(cd /w; claude --dangerously-skip-permissions)", "bypass", nil},
		{"claude --dangerously-skip-permissions; true", "bypass", nil},
		{"claude --permission-mode=bypassPermissions&&x", "bypass", nil},
		{"claude --permission-mode auto|tee log", "auto", nil},
		{"`claude --dangerously-skip-permissions`", "bypass", nil},
		{"sh -c \"`claude --permission-mode auto`\"", "auto", nil},
		{`claude --name 'a;--dangerously-skip-permissions'`, "default", nil},
	} {
		class, warnings := worktreecmd.ClassifyLaunch(c.cmd)
		if class != c.class || !slices.Equal(warnings, c.warnings) {
			t.Errorf("ClassifyLaunch(%q) = %q, %v; want %q, %v", c.cmd, class, warnings, c.class, c.warnings)
		}
	}
}

// Every command LaunchCommand renders classifies back to its own mode's class.
func TestClassifyLaunch_RoundTripsLaunchCommand(t *testing.T) {
	for mode, want := range map[string]string{"default": "default", "auto": "auto", "all-in": "bypass"} {
		cmd, err := worktreecmd.LaunchCommand(mode, "claude")
		if err != nil {
			t.Fatal(err)
		}
		if class, _ := worktreecmd.ClassifyLaunch(cmd); class != want {
			t.Errorf("ClassifyLaunch(LaunchCommand(%q)) = %q, want %q", mode, class, want)
		}
	}
}
