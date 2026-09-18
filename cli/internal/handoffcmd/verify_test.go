//go:build unix

package handoffcmd

import (
	"encoding/json"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/agentrun"
)

// TestResult_SchemaContract pins the envelope shapes commands/handoff.md reads by
// name. SchemaVersion is per-package.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump is a breaking change - update commands/handoff.md and cli/README.md first", SchemaVersion)
	}
	check := func(name string, v any, top string, keys ...string) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"schema_version", "ok", "subcommand", "steps", top} {
			if _, ok := got[k]; !ok {
				t.Errorf("%s: missing top-level key %q", name, k)
			}
		}
		inner, _ := got[top].(map[string]any)
		for _, k := range keys {
			if _, ok := inner[k]; !ok {
				t.Errorf("%s: %s missing key %q", name, top, k)
			}
		}
	}
	check("targets", &TargetsResult{Envelope: newEnvelope("targets"), Targets: &TargetsInfo{Terminals: []Target{}}}, "targets", "available", "terminals")
	check("send", &SendResult{Envelope: newEnvelope("send"), Send: &SendInfo{Delivery: "skipped"}}, "send", "delivery", "agent", "created")
	check("run", &RunResult{Envelope: newEnvelope("codex"), Run: &agentrun.Result{Warnings: []string{}}}, "run",
		"provider", "ran", "complete", "exit_code", "duration_ms", "workspace_changed", "warnings")
}
