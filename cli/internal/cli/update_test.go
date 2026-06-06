package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

// gitInitRepo creates a fresh temp git repo (with user identity configured so
// commits would work) and returns its path. Reused by the `lets update` tests
// because the command bails early when not in a git repository.
func gitInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func runRootUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := cli.NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"update"}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestUpdate_Help(t *testing.T) {
	out, err := runRootUpdate(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--plugin-root", "--json", "--offline", "--refresh-cache"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in help:\n%s", want, out)
		}
	}
}

func TestUpdate_NotGitRepo(t *testing.T) {
	chdirTo(t, t.TempDir()) // not a git repo
	out, err := runRootUpdate(t, "--json", "--offline", "--plugin-root="+makeFakePluginRoot(t))
	if err == nil {
		t.Fatalf("expected error outside a git repo; output:\n%s", out)
	}
	if !strings.Contains(out, "not in a git repository") {
		t.Fatalf("error not surfaced in JSON envelope:\n%s", out)
	}
}

func TestUpdate_MissingPluginRoot(t *testing.T) {
	chdirTo(t, gitInitRepo(t))
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	out, err := runRootUpdate(t, "--json", "--offline")
	if err == nil {
		t.Fatalf("expected error without a plugin root:\n%s", out)
	}
	if !strings.Contains(out, "plugin root not found") && !strings.Contains(out, "plugin-root") {
		t.Fatalf("error not surfaced:\n%s", out)
	}
}

func TestUpdate_JSONEnvelope_Offline(t *testing.T) {
	if version.Version != "dev" {
		t.Skip("test build carries a tagged version - the binary artifact assertion below assumes the 'dev' sentinel")
	}
	chdirTo(t, gitInitRepo(t))
	out, err := runRootUpdate(t, "--json", "--offline", "--plugin-root="+makeFakePluginRoot(t))
	if err != nil {
		t.Fatalf("Execute err = %v\noutput:\n%s", err, out)
	}
	var r struct {
		SchemaVersion int  `json:"schema_version"`
		OK            bool `json:"ok"`
		Artifacts     []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"artifacts"`
	}
	if jerr := json.Unmarshal([]byte(out), &r); jerr != nil {
		t.Fatalf("not valid JSON: %v\n%s", jerr, out)
	}
	if r.SchemaVersion != 2 || !r.OK || len(r.Artifacts) != 4 {
		t.Fatalf("envelope = %+v\n%s", r, out)
	}
	byName := map[string]string{}
	for _, a := range r.Artifacts {
		byName[a.Name] = a.Status
	}
	if byName[".env"] != "not-initialized" {
		t.Errorf(".env status = %q, want not-initialized (bare repo)", byName[".env"])
	}
	// `go test` builds without -ldflags, so version.Version == "dev" -> binary
	// reports "dev" (no release comparison) regardless of --offline. The plugin
	// has a real version in the fake plugin.json, so --offline -> "unknown".
	if byName["binary"] != "dev" {
		t.Errorf("binary status = %q, want dev (untagged test build)", byName["binary"])
	}
	if byName["plugin"] != "unknown" {
		t.Errorf("plugin status = %q, want unknown (offline)", byName["plugin"])
	}
}

func TestUpdate_TextOutput_Offline(t *testing.T) {
	chdirTo(t, gitInitRepo(t))
	out, err := runRootUpdate(t, "--offline", "--plugin-root="+makeFakePluginRoot(t))
	if err != nil {
		t.Fatalf("Execute err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LETS Update Status") {
		t.Fatalf("missing table header:\n%s", out)
	}
}

func TestUpdate_RefusesInsideWorktree(t *testing.T) {
	main := gitInitRepo(t)
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = main
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// A worktree needs a commit to check out.
	if err := os.WriteFile(filepath.Join(main, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "-q", wt)

	chdirTo(t, wt)
	out, err := runRootUpdate(t, "--json", "--offline", "--plugin-root="+makeFakePluginRoot(t))
	if err == nil {
		t.Fatalf("expected an error when run inside a worktree:\n%s", out)
	}
	if !strings.Contains(out, "worktree") {
		t.Fatalf("worktree refusal not surfaced:\n%s", out)
	}
}
