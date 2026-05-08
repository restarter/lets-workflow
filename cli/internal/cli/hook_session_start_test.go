package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestHookSessionStart_E2E exercises the cobra → sessionstart pipeline with
// the --rules flag. Project root falls through to whatever DetectProjectRoot
// finds at test time - typically the lets-workflow repo root since tests
// run from cli/internal/cli/. We deliberately don't sandbox the project
// root here: assertions only check Contains() so the test tolerates
// whatever .env happens to live at the detected root. Fully hermetic test
// would require chdir into a temp dir with `git init`; the brittleness
// risk is accepted for now (test passes in any git-aware environment).
func TestHookSessionStart_E2E(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	if err := os.WriteFile(rulesPath, []byte("RULES BODY\n"), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "session-start", "--rules=" + rulesPath})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "RULES BODY\n") {
		preview := out
		if len(preview) > 50 {
			preview = preview[:50]
		}
		t.Errorf("output should start with rules body, got: %q", preview)
	}
	// Project root will be detected (we're in a git repo during tests), so
	// LETS Config block is expected.
	if !strings.Contains(out, "## LETS Config") {
		t.Errorf("expected LETS Config block (test runs inside git repo), got: %q", out)
	}
	if !strings.Contains(out, "LETS_PROJECT_ROOT=") {
		t.Errorf("expected LETS_PROJECT_ROOT line, got: %q", out)
	}
}

func TestHookSessionStart_RulesFlagRequired(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "session-start"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when --rules is missing")
	}
	if !strings.Contains(err.Error(), "rules") {
		t.Errorf("error should mention rules flag, got: %v", err)
	}
}
