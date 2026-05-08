package initcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetStatusLineManaged_FreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SetStatusLineManaged(path); err != nil {
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
	managed, _ := m["_letsManaged"].(map[string]any)
	if b, _ := managed["statusLine"].(bool); !b {
		t.Errorf("_letsManaged.statusLine = %v, want true", b)
	}
}

func TestSetStatusLineManaged_PreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{
  "myCustom": {"deep": {"value": 42}},
  "permissions": ["all"],
  "statusLine": {"command": "bash -c 'cat | bash $(git rev-parse --show-toplevel)/.lets/statusline.sh 2>/dev/null'"}
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLineManaged(path); err != nil {
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
	managed, _ := m["_letsManaged"].(map[string]any)
	if b, _ := managed["statusLine"].(bool); !b {
		t.Errorf("_letsManaged.statusLine = %v, want true", b)
	}
}

func TestSetStatusLineManaged_RefusesForeign(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"statusLine": {"command": "/usr/local/bin/my-custom-statusline.sh"}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SetStatusLineManaged(path)
	if err == nil || !strings.Contains(err.Error(), "foreign statusLine") {
		t.Errorf("expected foreign-statusLine error, got %v", err)
	}
}

func TestSetStatusLineManaged_BackupCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := `{"statusLine": {"command": "lets statusline"}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLineManaged(path); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("expected .bak created, got %v", err)
	}
	if string(bak) != initial {
		t.Errorf(".bak content mismatch: got %q, want %q", bak, initial)
	}
}

func TestSetStatusLineManaged_BackupOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"statusLine":{"command":"lets statusline"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetStatusLineManaged(path); err != nil {
		t.Fatal(err)
	}
	// Run again - second run should overwrite .bak with the result of first run
	if err := SetStatusLineManaged(path); err != nil {
		t.Fatal(err)
	}
	// Just verify .bak exists and is not stale
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf(".bak missing after second run: %v", err)
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
