//go:build unix

package worktreecmd

import (
	"os"
	"testing"
)

// TestMain isolates the package from the Claude session running the tests: no
// session id, and an empty Claude config dir (no sessions/ - registry absent), so
// a registry-reading verb (info's team_lead, the session guard) never sees the
// developer's live sessions. A test that needs either sets it with t.Setenv.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	cfg, err := os.MkdirTemp("", "lets-worktreecmd-test-claude-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("CLAUDE_CONFIG_DIR", cfg)
	code := m.Run()
	_ = os.RemoveAll(cfg)
	os.Exit(code)
}
