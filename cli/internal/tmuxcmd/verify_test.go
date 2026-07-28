//go:build unix

package tmuxcmd

import (
	"encoding/json"
	"testing"
)

// TestResult_SchemaContract pins the envelope shape. SchemaVersion is
// per-package (see CLAUDE.md "JSON envelope"); bumping it is a breaking change
// for commands/worktree.md, which parses these fields by name.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; a bump is a breaking change - update commands/worktree.md and cli/README.md first", SchemaVersion)
	}
	res := &OpenResult{
		Envelope: Envelope{SchemaVersion: SchemaVersion, OK: true, Subcommand: "open", Steps: []Step{}},
		Launch:   &LaunchInfo{Launched: true, WorkspaceName: "demo", Target: "demo:0", Path: "/tmp"},
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"schema_version", "ok", "subcommand", "steps", "launch"} {
		if _, present := got[k]; !present {
			t.Errorf("envelope missing top-level key %q", k)
		}
	}
	launch := got["launch"].(map[string]any)
	// workspace_name is the cross-launcher parity field worktree.md reads for
	// BOTH cmux and tmux - dropping it silently breaks the markdown contract.
	for _, k := range []string{"launched", "workspace_name", "target", "path", "in_existing_session"} {
		if _, present := launch[k]; !present {
			t.Errorf("launch missing key %q", k)
		}
	}
}
