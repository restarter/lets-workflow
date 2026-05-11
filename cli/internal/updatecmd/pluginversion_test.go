package updatecmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writePluginJSON(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadPluginVersion(t *testing.T) {
	cases := []struct {
		name, body, want string
		writeFile        bool
	}{
		{"ok", `{"name":"lets","version":"0.6.0"}`, "0.6.0", true},
		{"no version key", `{"name":"lets"}`, "", true},
		{"malformed json", `{not json`, "", true},
		{"missing file", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.writeFile {
				writePluginJSON(t, root, tc.body)
			}
			if got := ReadPluginVersion(root); got != tc.want {
				t.Fatalf("ReadPluginVersion = %q, want %q", got, tc.want)
			}
		})
	}
}
