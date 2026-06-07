package statuslinecmd

import (
	"encoding/json"
	"testing"
)

// TestResult_SchemaContract pins the envelope schema version + shape. Bump
// SchemaVersion deliberately and update this test when the contract changes.
func TestResult_SchemaContract(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; bump intentionally + update consumers", SchemaVersion)
	}
	// An ok=false envelope must still marshal to valid JSON with the core keys.
	res := NewErrorResult("/x", ErrUsage("nope"))
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"schema_version", "ok", "subcommand", "project_root", "steps", "error"} {
		if _, ok := m[k]; !ok {
			t.Errorf("envelope missing key %q: %s", k, b)
		}
	}
	if m["ok"] != false {
		t.Errorf("ok should be false on error envelope")
	}
}
