package cli_test

import (
	"os"
	"testing"
)

// TestMain sandboxes HOME for the ENTIRE cli package: the cobra wrappers call
// os.UserHomeDir(), and an unsandboxed run on a machine with a real user-scope
// install would READ the developer's ~/.lets/.env (machine-dependent flaky
// assertions) or WRITE ~/.claude/rules/lets-rules.md (clobbering it with test
// fixtures). This floor also covers init_json_test.go and any FUTURE test in
// the package - per-test t.Setenv("HOME", ...) still overrides it wherever a
// POPULATED fake home is needed. CLAUDE_CONFIG_DIR and CLAUDE_CODE_SESSION_ID are
// isolated the same way (t.Setenv overrides them per test).
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "lets-cli-test-home-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	// The session guard reads the Claude Code session registry and the caller's own
	// session id: a test run inside a Claude session must see neither, so every test
	// starts from no session and an empty registry dir (no sessions/ - registry_absent).
	_ = os.Unsetenv("CLAUDE_CODE_SESSION_ID")
	cfg, err := os.MkdirTemp("", "lets-cli-test-claude-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("CLAUDE_CONFIG_DIR", cfg)
	code := m.Run()
	_ = os.RemoveAll(home)
	_ = os.RemoveAll(cfg)
	os.Exit(code)
}
