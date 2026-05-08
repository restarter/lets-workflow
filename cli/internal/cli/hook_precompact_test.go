package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestHookPreCompact_E2E mirrors TestHookSessionStart_E2E since the two
// commands currently share output behavior. Pinning a separate test here
// guards against regression if PreCompact ever diverges (e.g. future
// context snapshotting).
func TestHookPreCompact_E2E(t *testing.T) {
	dir := t.TempDir()
	rulesPath := filepath.Join(dir, "rules.md")
	if err := os.WriteFile(rulesPath, []byte("RULES BODY\n"), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "precompact", "--rules=" + rulesPath})

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
	if !strings.Contains(out, "## LETS Config") {
		t.Errorf("expected LETS Config block, got: %q", out)
	}
}

func TestHookPreCompact_RulesFlagRequired(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"hook", "precompact"})

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
