package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetStatusLine_FreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SetStatusLine(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	sl, _ := m["statusLine"].(map[string]any)
	if cmd, _ := sl["command"].(string); cmd != "lets statusline" {
		t.Errorf("statusLine.command = %v, want 'lets statusline'", cmd)
	}
	if _, exists := m["_letsManaged"]; exists {
		t.Errorf("_letsManaged unexpectedly written: %v", m["_letsManaged"])
	}
}

func TestSetStatusLine_PreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{
  "myCustom": {"deep": {"value": 42}},
  "permissions": ["all"],
  "statusLine": {"command": "bash -c 'cat | bash $(git rev-parse --show-toplevel)/.lets/statusline.sh 2>/dev/null'"}
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLine(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["myCustom"]; !ok {
		t.Errorf("myCustom field lost")
	}
	if _, ok := m["permissions"]; !ok {
		t.Errorf("permissions field lost")
	}
}

func TestSetStatusLine_RefusesForeign(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"statusLine": {"command": "/usr/local/bin/my-custom-statusline.sh"}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SetStatusLine(path)
	if err == nil || !strings.Contains(err.Error(), "foreign statusLine") {
		t.Errorf("expected foreign-statusLine error, got %v", err)
	}
}

func TestSetStatusLine_BackupCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"statusLine": {"command": "lets statusline"}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLine(path); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("expected .bak created, got %v", err)
	}
	if string(bak) != initial {
		t.Errorf(".bak content mismatch: got %q, want %q", bak, initial)
	}
	// S17 (review 2026-05-08): settings.json may contain unrelated user
	// fields (auth tokens, custom commands). Backup must not widen permissions.
	fi, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatalf("stat .bak: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != 0o600 {
		t.Errorf(".bak mode = %o, want 0600 (no group/world access)", mode)
	}
}

func TestSetStatusLine_BackupOverwritten(t *testing.T) {
	// N13 (review 2026-05-08): originally only checked .bak existed, which
	// is satisfied even if the second run leaves stale pre-first-run content.
	// Verify the .bak content actually rotates - i.e. after run-2, .bak
	// reflects the post-run-1 state, NOT the pre-run-1 state.
	path := filepath.Join(t.TempDir(), "settings.json")
	preRun1 := `{"statusLine":{"command":"lets statusline"}}`
	if err := os.WriteFile(path, []byte(preRun1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLine(path); err != nil {
		t.Fatalf("run 1: %v", err)
	}

	// Snapshot the post-run-1 state - this is what should land in .bak
	// after the second run.
	postRun1, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate path between runs so we can prove .bak rotated (not just exists).
	if err := os.WriteFile(path, []byte(`{"between":"runs"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetStatusLine(path); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf(".bak missing after second run: %v", err)
	}
	if string(bak) != `{"between":"runs"}` {
		t.Errorf(".bak content didn't rotate.\n got:  %s\n want: %s\n(post-run-1 was: %s)", bak, `{"between":"runs"}`, postRun1)
	}
}

func TestAtomicWriteBytes_PreservesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteBytes(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 (preserved)", fi.Mode().Perm())
	}
}

func TestAtomicWriteBytes_FreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := atomicWriteBytes(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "test" {
		t.Errorf("got %q", data)
	}
}
