package letsconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
)

func TestValidLaunchMode(t *testing.T) {
	for _, m := range []string{"default", "auto", "all-in"} {
		if !letsconfig.ValidLaunchMode(m) {
			t.Errorf("%q must be valid", m)
		}
	}
	for _, m := range []string{"", "bypassPermissions", "All-in", "plan", "auto "} {
		if letsconfig.ValidLaunchMode(m) {
			t.Errorf("%q must be refused", m)
		}
	}
}

// all-in is never the default: a fresh .env and every fallback start in the
// non-bypass class.
func TestLaunchModeDefaultIsNotAllIn(t *testing.T) {
	if d := letsconfig.Defaults()["LETS_LAUNCH_MODE"]; d != "default" {
		t.Errorf("LETS_LAUNCH_MODE default = %q, want default", d)
	}
}

func TestLaunchModeSetting(t *testing.T) {
	write := func(t *testing.T, dir, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, ".lets"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".lets", ".env"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name, project, user, wantValue, wantSource string
	}{
		{"project wins", "LETS_LAUNCH_MODE=auto\n", "LETS_LAUNCH_MODE=all-in\n", "auto", "project"},
		{"empty project falls through", "LETS_LAUNCH_MODE=\n", "LETS_LAUNCH_MODE=all-in\n", "all-in", "user"},
		{"user only", "", "LETS_LAUNCH_MODE=auto\n", "auto", "user"},
		{"unset", "LETS_LANGUAGE=English\n", "", "", "unset"},
		{"invalid kept as written", "LETS_LAUNCH_MODE=bypass\n", "", "bypass", "project"},
	} {
		t.Run(c.name, func(t *testing.T) {
			project, home := t.TempDir(), t.TempDir()
			if c.project != "" {
				write(t, project, c.project)
			}
			if c.user != "" {
				write(t, home, c.user)
			}
			v, src := letsconfig.LaunchModeSetting(project, home)
			if v != c.wantValue || src != c.wantSource {
				t.Errorf("LaunchModeSetting = %q, %q; want %q, %q", v, src, c.wantValue, c.wantSource)
			}
		})
	}
}
