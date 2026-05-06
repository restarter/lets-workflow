package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// TestVersionCmd asserts strict output for the `lets version` subcommand.
// We control this format - any drift is a regression.
func TestVersionCmd(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"version"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if got, want := buf.String(), "lets version 0.4.0-dev\n"; got != want {
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
	if !strings.Contains(out, "0.4.0-dev") {
		t.Errorf("expected output to contain version, got %q", out)
	}
}
