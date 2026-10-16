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

// TestHookPreCompact_SilentNoOp pins the deprecated stub (lets-v59n8): an older
// plugin's hooks.json still runs `lets hook precompact --rules=<path>`, and it
// must exit 0 and print nothing - with the flag, without it, and with the
// PreCompact payload on stdin.
func TestHookPreCompact_SilentNoOp(t *testing.T) {
	for _, args := range [][]string{
		{"hook", "precompact", "--rules=/nonexistent/lets-rules.md"},
		{"hook", "precompact"},
	} {
		root := cli.NewRootCmd()
		root.SetArgs(args)
		root.SetIn(strings.NewReader(`{"hook_event_name":"PreCompact","trigger":"manual","custom_instructions":null}`))
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out.Len() != 0 || errOut.Len() != 0 {
			t.Errorf("%v: want no output, got stdout %q stderr %q", args, out.String(), errOut.String())
		}
	}
}

// TestHooksJSON_SessionStartOnly pins the plugin side of lets-v59n8: the plugin
// wires SessionStart (every source, compact included) and no PreCompact hook.
func TestHooksJSON_SessionStartOnly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "lets", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hooks) != 1 || len(cfg.Hooks["SessionStart"]) != 1 {
		t.Fatalf("hooks.json must wire SessionStart only, got events %v", cfg.Hooks)
	}
	ss := cfg.Hooks["SessionStart"][0]
	if ss.Matcher != "startup|resume|clear|compact" {
		t.Errorf("SessionStart matcher = %q, want startup|resume|clear|compact", ss.Matcher)
	}
	const want = "lets hook session-start --rules=${CLAUDE_PLUGIN_ROOT}/rules/lets-rules.md"
	if len(ss.Hooks) != 1 || ss.Hooks[0].Type != "command" || ss.Hooks[0].Command != want {
		t.Errorf("SessionStart must run one command hook %q, got %+v", want, ss.Hooks)
	}
}
