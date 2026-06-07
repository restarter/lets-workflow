package cli_test

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestInteractiveTTY covers the stdin guard that prevents `lets statusline`
// from hanging when run by hand (lets-7frjs).
func TestInteractiveTTY(t *testing.T) {
	// A non-*os.File reader — exactly what the render tests use — must never be
	// treated as interactive, so it falls through to Render.
	if cli.InteractiveTTY(strings.NewReader("{}")) {
		t.Error("strings.Reader should not be interactive")
	}

	// A regular file is not a character device → not interactive.
	f, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer func() { _ = f.Close() }()
	if cli.InteractiveTTY(f) {
		t.Error("regular file should not be interactive")
	}

	// A character device (os.DevNull is one on unix) exercises the positive
	// branch. Faking a real TTY needs a PTY, so /dev/null stands in for the
	// ModeCharDevice check. Windows NUL doesn't reliably report the bit — skip.
	if runtime.GOOS == "windows" {
		t.Skip("char-device probe is unix-only")
	}
	dn, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = dn.Close() }()
	if !cli.InteractiveTTY(dn) {
		t.Errorf("%s is a character device and should hit the interactive branch", os.DevNull)
	}
}

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

// TestStatusline_EmptyStdinDoesNotError verifies that empty stdin (which
// Claude Code occasionally provides during /reload-plugins or initial render)
// produces a graceful zero-value render with exit 0, not a JSON decode error.
func TestStatusline_EmptyStdinDoesNotError(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"statusline"})

	root.SetIn(strings.NewReader(""))

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("empty stdin should NOT error: %v\noutput: %q", err, buf.String())
	}
	if !strings.Contains(buf.String(), "LETS Workflow") {
		t.Errorf("missing LETS branding on empty input: %q", buf.String())
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
