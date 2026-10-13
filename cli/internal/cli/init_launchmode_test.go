package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `lets init --launch-mode` is validated like --launcher (lets-d3wu3): an unknown
// value is refused before anything is written; a valid one lands in .lets/.env.
func TestInit_LaunchModeFlag(t *testing.T) {
	pluginRoot := makeFakePluginRoot(t)
	base := []string{"--json", "--plugin-root=" + pluginRoot, "--language=English", "--merge-branch=main", "--pr-flow=local", "--skip-beads"}

	dir := initGitRepo(t)
	bad := runInitJSON(t, dir, append(base, "--launch-mode=bogus")...)
	if bad.OK || !strings.Contains(bad.Error, "--launch-mode must be one of") {
		t.Errorf("--launch-mode=bogus: ok=%v error=%q, want a refusal naming the values", bad.OK, bad.Error)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lets", ".env")); !os.IsNotExist(err) {
		t.Errorf("a refused --launch-mode wrote .lets/.env (err %v)", err)
	}

	dir = initGitRepo(t)
	good := runInitJSON(t, dir, append(base, "--launch-mode=all-in")...)
	if !good.OK {
		t.Fatalf("--launch-mode=all-in: %+v", good)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".lets", ".env"))
	if err != nil || !strings.Contains(string(b), "\nLETS_LAUNCH_MODE=all-in\n") {
		t.Errorf("--launch-mode=all-in not written (err %v):\n%s", err, b)
	}
}
