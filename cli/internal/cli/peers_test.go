//go:build unix

package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

func TestPeersTail_AcceptsPositionalName(t *testing.T) {
	cmd := cli.NewPeersCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"tail", "SOMEONE", "--cwd", t.TempDir()}) // not a repo: tail itself answers
	err := cmd.Execute()
	if err != nil && strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("a positional name must reach tail: %v", err)
	}
	// not_in_repo is only reachable once target validation passed - i.e. once the
	// positional name was copied into TailOptions.Name; a dropped name is `usage`.
	var env struct {
		Subcommand string `json:"subcommand"`
		Error      *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &env); jerr != nil || env.Subcommand != "tail" || env.Error == nil || env.Error.Kind != "not_in_repo" {
		t.Errorf("the name must pass validation and reach repository loading: %s", out.String())
	}
}

// runPeers runs `lets peers <args>` and returns the envelope and the error.
func runPeers(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	cmd := cli.NewPeersCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	var env map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &env); jerr != nil {
		t.Fatalf("%v: no envelope (err=%v): %q", args, err, out.String())
	}
	return env, err
}

func errKind(env map[string]any) string {
	e, _ := env["error"].(map[string]any)
	k, _ := e["kind"].(string)
	return k
}

// A v0.10.0 caller's old flags never fail parsing: every peers subcommand answers
// with an envelope.
func TestPeers_OldFlagsParse(t *testing.T) {
	dir := t.TempDir() // not a repo: each verb answers past flag parsing
	for _, args := range [][]string{
		{"who", "--cwd", dir, "--probe-orca", "--role", "orchestrator", "--prune", "--repo", "/x", "--json"},
		{"tell", "--cwd", dir, "--to-session", "aaaaaaaa-0000-4000-8000-000000000001", "--msgid", "0123456789abcdef", "--repo-index", "0", "--probe-orca", "--json"},
		{"frame", "--cwd", dir, "--session", "aaaaaaaa-0000-4000-8000-000000000001", "--to-session", "aaaaaaaa-0000-4000-8000-000000000002", "--kind", "ask", "--probe-orca", "--json"},
		{"wait", "--cwd", dir, "--to-session", "x", "--since-message", "y", "--sent-at", "z", "--timeout-ms", "5", "--probe-orca", "--json"},
		{"orchestrator", "--cwd", dir, "--session", "x", "--probe-orca", "--json"},
		{"role", "set", "orchestrator", "--cwd", dir, "--session", "aaaaaaaa-0000-4000-8000-000000000001", "--takeover", "--scope", "x", "--json"},
	} {
		env, err := runPeers(t, args...)
		if err != nil && strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("%v: %v", args, err)
		}
		if env["subcommand"] == nil {
			t.Errorf("%v: %v", args, env)
		}
	}
	env, _ := runPeers(t, "wait", "--probe-orca", "--json")
	if env["reason"] != "completion_unverifiable" || env["deprecated"] == nil {
		t.Errorf("wait stub: %v", env)
	}
}

// tail: --repo-index answers the retirement, the other removed flags answer usage
// naming --since.
func TestPeersTail_RemovedFlags(t *testing.T) {
	env, _ := runPeers(t, "tail", "W1", "--cwd", t.TempDir(), "--repo-index", "0")
	if errKind(env) != "cross_repo_read_retired" {
		t.Errorf("--repo-index: %v", env)
	}
	env, _ = runPeers(t, "tail", "--cwd", t.TempDir(), "--to-session", "aaaaaaaa-0000-4000-8000-000000000001", "--addressed-to-session", "aaaaaaaa-0000-4000-8000-000000000002", "--count-only", "--json")
	e, _ := env["error"].(map[string]any)
	if errKind(env) != "usage" || !strings.Contains(e["message"].(string), "--since") || !strings.Contains(e["message"].(string), "--count-only") {
		t.Errorf("removed flags: %v", env)
	}
}
