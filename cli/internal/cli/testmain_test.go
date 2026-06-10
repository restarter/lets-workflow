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
// POPULATED fake home is needed.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "lets-cli-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
