//go:build unix

package orcacmd

import (
	"encoding/json"
	"testing"
)

// TestResult_SchemaContract pins the envelope shapes commands/worktree.md and
// init.md read by name. SchemaVersion is per-package.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump is a breaking change - update commands/worktree.md and cli/README.md first", SchemaVersion)
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
	env := func(sub string) Envelope {
		return Envelope{SchemaVersion: SchemaVersion, OK: true, Subcommand: sub, Steps: []Step{}}
	}
	check("open", &OpenResult{Envelope: env("open"), Launch: &LaunchInfo{Launched: false, WorkspaceName: "n", Path: "/p", Reason: "orca_not_found", FallbackCommand: "x", Branch: "b", OrcaWorktreeID: "r::/p"}},
		"launch", "launched", "workspace_name", "path", "reason", "fallback_command", "created", "branch", "orca_worktree_id")
	check("card", &CardResult{Envelope: env("card"), Card: &CardInfo{Updated: true, Phase: "start", Status: "in-progress", Target: "r::/p", Reason: "x"}},
		"card", "updated", "phase", "status", "target", "reason")
	check("notify", &NotifyResult{Envelope: env("notify"), Notify: &NotifyInfo{Notified: true, Target: "r::/p", Title: "t", Reason: "x"}},
		"notify", "notified", "target", "title", "reason")
	check("status", &StatusResult{Envelope: env("status"), Status: &StatusInfo{Bin: "/b", Running: true, Version: "1", Reason: "x"}},
		"status", "bin", "running", "version", "reason")
}
