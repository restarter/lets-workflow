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
// whatever .env happens to live at the detected root.
//
// Phase 4b: rules emission was removed. Output is now just the LETS Config
// block (+ optional drift notice when installed rules differ from plugin).
func TestHookSessionStart_E2E(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	// Plugin rules with frontmatter so driftCheck has something to compare.
	if err := os.WriteFile(rulesPath, []byte("---\nversion: 0.4.0\n---\nRULES BODY\n"), 0o644); err != nil {
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
	// Rules body should NOT be in output (Phase 4b: rules moved to project's
	// .claude/rules/lets-rules.md, hook only emits Config + drift notice).
	if strings.Contains(out, "RULES BODY") {
		t.Errorf("rules body should not be emitted in Phase 4b, got:\n%s", out)
	}
	if !strings.Contains(out, "## LETS Config") {
		t.Errorf("expected LETS Config block (test runs inside git repo), got: %q", out)
	}
	if !strings.Contains(out, "LETS_PROJECT_ROOT=") {
		t.Errorf("expected LETS_PROJECT_ROOT line, got: %q", out)
	}
}

// TestHookSessionStart_PreCompact_OutputParity locks the contract that the
// two hook subcommands produce byte-identical output today (they share
// runHookSessionPipeline). Future PreCompact-specific divergence should
// be intentional - this test forces the change to be visible by failing,
// rather than letting one subcommand silently drift while the other
// stays correct. Closes S15 from the 2026-05-08 review.
func TestHookSessionStart_PreCompact_OutputParity(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	if err := os.WriteFile(rulesPath, []byte("---\nversion: 0.4.0\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, sub string) string {
		t.Helper()
		root := cli.NewRootCmd()
		root.SetArgs([]string{"hook", sub, "--rules=" + rulesPath})
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute %s: %v", sub, err)
		}
		return buf.String()
	}

	got1 := run(t, "session-start")
	got2 := run(t, "precompact")

	if got1 != got2 {
		t.Errorf("session-start vs precompact output diverged.\nsession-start:\n%s\nprecompact:\n%s", got1, got2)
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
