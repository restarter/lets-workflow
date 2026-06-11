package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// initUserResult is the subset of the initcmd.Result envelope these tests
// assert on (consumed by name, not position - same as init.md does).
type initUserResult struct {
	SchemaVersion int    `json:"schema_version"`
	OK            bool   `json:"ok"`
	Error         string `json:"error,omitempty"`
	ProjectRoot   string `json:"project_root"`
	Steps         []struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"steps"`
}

func TestInitUser_JSONEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginRoot := makeFakePluginRoot(t)

	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--user", "--json", "--plugin-root=" + pluginRoot, "--language=Ukrainian"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, buf.String())
	}

	var r initUserResult
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("invalid JSON envelope: %v\n%s", err, buf.String())
	}
	if !r.OK {
		t.Errorf("ok=false: %s", r.Error)
	}
	// project_root carries the scope root (home dir) for a --user run.
	if r.ProjectRoot != home {
		t.Errorf("project_root: got %q want home %q", r.ProjectRoot, home)
	}
	if len(r.Steps) == 0 {
		t.Error("no steps in envelope")
	}

	for _, want := range []string{
		filepath.Join(home, ".claude", "rules", "lets-rules.md"),
		filepath.Join(home, ".lets", ".env"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing %s after --user install", want)
		}
	}
}

func TestInitUser_MissingPluginRoot_ErrorEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")

	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--user", "--json"})
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err == nil {
		t.Fatal("expected error without plugin root")
	}
	var r initUserResult
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("error path must still emit a JSON envelope: %v\n%s", err, buf.String())
	}
	if r.OK {
		t.Error("ok must be false")
	}
	if !strings.Contains(r.Error, "plugin") {
		t.Errorf("error should mention plugin root: %q", r.Error)
	}
}

func TestInitUser_ProjectFlagsWarnAndIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginRoot := makeFakePluginRoot(t)

	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--user", "--json", "--plugin-root=" + pluginRoot, "--merge-branch=develop"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errBuf.String(), "ignored with --user") {
		t.Errorf("expected project-flag warning on stderr, got: %q", errBuf.String())
	}
	env, _ := os.ReadFile(filepath.Join(home, ".lets", ".env"))
	if strings.Contains(string(env), "LETS_MERGE_BRANCH=develop") {
		t.Errorf("--merge-branch must be ignored with --user:\n%s", env)
	}
}
