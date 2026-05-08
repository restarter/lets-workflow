package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
	for _, want := range []string{"--language", "--merge-branch", "--pr-flow", "--plugin-root", "--skip-beads", "deprecated"} {
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

// makeFakePluginRoot creates a minimal plugin tree that DetectPluginRoot
// will accept (.claude-plugin/plugin.json marker) and Run() can read from
// (rules/lets-rules.md). Note: no config-template.env — `.env.example` is
// generated from letsconfig.Keys defaults post lets-8ilsl.
func makeFakePluginRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(".claude-plugin/plugin.json", `{"name":"lets","version":"0.4.0"}`)
	mustWrite("rules/lets-rules.md", "---\nversion: 0.4.0\n---\n# test rules\n")
	return root
}

// chdirTo changes cwd for the test's lifetime. Go 1.22 lacks t.Chdir() (added
// in 1.24), so we restore manually via t.Cleanup. Tests in this package run
// serially by default - safe.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// TestInit_GithubAndPRFlowConflict_Errors covers B13 from the 2026-05-08
// review. The conflict check fires AFTER plugin-root resolution but BEFORE
// any file mutation, so we can run it from the lets-workflow repo's own
// cwd without side effects.
func TestInit_GithubAndPRFlowConflict_Errors(t *testing.T) {
	plugin := makeFakePluginRoot(t)

	root := cli.NewRootCmd()
	root.SetArgs([]string{
		"init",
		"--plugin-root=" + plugin,
		"--github",
		"--pr-flow=local",
		"--skip-beads",
	})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "--github conflicts") {
		t.Errorf("error should mention conflict, got: %v", err)
	}
}

// TestInit_NotInGit_ReturnsError covers B13. Run from a fresh tempdir with
// no git repo - DetectProjectRoot returns "" and init bails before mutation.
func TestInit_NotInGit_ReturnsError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	plugin := makeFakePluginRoot(t)
	nonGit := t.TempDir()
	chdirTo(t, nonGit)

	root := cli.NewRootCmd()
	root.SetArgs([]string{
		"init",
		"--plugin-root=" + plugin,
		"--language=English",
		"--merge-branch=main",
		"--pr-flow=local",
		"--skip-beads",
	})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected 'not in a git repository' error, got nil")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error should mention git, got: %v", err)
	}
}

// TestInit_InvalidPluginRoot_ReturnsError covers the S9 marker check
// (Cleanup 4): malicious or wrong --plugin-root values must be rejected
// before any rules content is copied into <project>/.claude/rules/.
func TestInit_InvalidPluginRoot_ReturnsError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// chdir into a real git repo so the projectRoot check passes; we want
	// the failure to be specifically the plugin-root marker check.
	gitDir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = gitDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	chdirTo(t, gitDir)

	bogusPlugin := t.TempDir() // no .claude-plugin/plugin.json

	root := cli.NewRootCmd()
	root.SetArgs([]string{
		"init",
		"--plugin-root=" + bogusPlugin,
		"--skip-beads",
	})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected plugin-root marker error, got nil")
	}
	if !strings.Contains(err.Error(), "plugin install") && !strings.Contains(err.Error(), "plugin.json") {
		t.Errorf("error should mention plugin install or plugin.json, got: %v", err)
	}
}
