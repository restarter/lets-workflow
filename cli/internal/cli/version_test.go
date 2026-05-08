package cli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// TestVersionCmd asserts strict output for the `lets version` subcommand.
// We control this format - any drift is a regression. Reads version.Version
// (not a literal) so the test follows the sentinel without per-bump edits.
func TestVersionCmd(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"version"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	want := fmt.Sprintf("lets version %s\n", version.Version)
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestRootVersionFlag asserts the cobra-default `--version` flag works.
// Uses Contains (not strict equality) since cobra owns the template format
// and may tweak it across minor versions.
func TestRootVersionFlag(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"--version"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, version.Version) {
		t.Errorf("expected output to contain %q, got %q", version.Version, out)
	}
}
