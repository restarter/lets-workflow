package letsconfig_test

import (
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
