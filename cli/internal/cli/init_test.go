package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

func TestInit_Help(t *testing.T) {
	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--help"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"--language", "--merge-branch", "--pr-flow", "--non-interactive", "deprecated"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in help: %s", want, out)
		}
	}
}

func TestInit_GithubDeprecationFlagPresent(t *testing.T) {
	// Verify --github flag exists (full execution requires fixture project)
	root := cli.NewRootCmd()
	cmd, _, err := root.Find([]string{"init"})
	if err != nil {
		t.Fatal(err)
	}
	if f := cmd.Flags().Lookup("github"); f == nil {
		t.Errorf("--github flag missing")
	}
}
