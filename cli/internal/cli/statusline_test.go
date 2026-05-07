package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestStatusline_E2E pipes minimal JSON in and verifies basic structure of
// the rendered output. Project root detection runs via git rev-parse - the
// test inherits the lets-workflow git context.
func TestStatusline_E2E(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"statusline"})

	root.SetIn(strings.NewReader(`{"model":{"display_name":"Opus 4.7"}}`))

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "LETS Workflow") {
		t.Errorf("missing LETS branding: %q", out)
	}
	if !strings.Contains(out, "Opus 4.7") {
		t.Errorf("missing model: %q", out)
	}
}

// TestStatusline_FetchOnlyDoesNotPanic verifies that --fetch-usage-only mode
// returns gracefully when called without a token. Real fetch behavior would
// hit the live API so we only check the no-panic / no-token error path here.
func TestStatusline_FetchOnlyDoesNotPanic(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"statusline", "--fetch-usage-only", "--cache-dir", t.TempDir()})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// Either errors with no-token (typical CI) or succeeds (dev machine with
	// keychain creds). Either way: no panic, no infinite loop.
	_ = root.Execute()
}
