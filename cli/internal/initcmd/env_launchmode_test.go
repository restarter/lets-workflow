package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/version"
)

// LETS_LAUNCH_MODE is never materialized by a regen (lets-d3wu3): empty means
// `default` and stays "unset", so /lets:start --main can name it and the
// ~/.lets/.env overlay still applies. Only a value the owner gives is written.
func TestRegenerateEnv_LaunchModeStaysUnset(t *testing.T) {
	sixKeys := "LETS_LANGUAGE=English\nLETS_MERGE_BRANCH=main\nLETS_PR_FLOW=local\nLETS_TRACKER=beads\nLETS_LAUNCHER=terminal\nLETS_RULES_SCOPE=project\n"
	write := func(t *testing.T, ver, extra string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(p, []byte("LETS_ENV_VERSION="+ver+"\n"+sixKeys+extra), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	read := func(t *testing.T, p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// at the current version, without the key: nothing to change, nothing written
	p := write(t, version.Version, "")
	before := read(t, p)
	act, err := RegenerateEnv(p, Prefs{})
	if err != nil || act.Kind != EnvSkip || len(act.ChangedKeys) != 0 || read(t, p) != before {
		t.Errorf("a .env without the key must stay as it is: %+v, %v", act, err)
	}

	// an older version regenerates: the key is rendered empty, never `default`
	p = write(t, "0.0.1", "")
	if _, err := RegenerateEnv(p, Prefs{}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); !strings.Contains(got, "\nLETS_LAUNCH_MODE=\n") || strings.Contains(got, "LETS_LAUNCH_MODE=default") || strings.Contains(got, "LETS_LAUNCH_MODE=all-in") {
		t.Errorf("a regen must leave LETS_LAUNCH_MODE empty:\n%s", got)
	}

	// an owner value is kept on regen; an explicit flag is written
	p = write(t, "0.0.1", "LETS_LAUNCH_MODE=auto\n")
	if _, err := RegenerateEnv(p, Prefs{}); err != nil || !strings.Contains(read(t, p), "\nLETS_LAUNCH_MODE=auto\n") {
		t.Errorf("an owner-set mode must survive a regen (err %v):\n%s", err, read(t, p))
	}
	p = write(t, version.Version, "")
	act, err = RegenerateEnv(p, Prefs{LaunchMode: "all-in"})
	if err != nil || !strings.Contains(read(t, p), "\nLETS_LAUNCH_MODE=all-in\n") || len(act.ChangedKeys) != 1 || act.ChangedKeys[0] != "LETS_LAUNCH_MODE" {
		t.Errorf("an explicit --launch-mode is written: %+v, %v\n%s", act, err, read(t, p))
	}
}
