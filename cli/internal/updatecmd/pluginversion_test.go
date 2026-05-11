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
		{"ok with prerelease", `{"version":"0.6.0-rc.1"}`, "0.6.0-rc.1", true},
		{"no version key", `{"name":"lets"}`, "", true},
		{"malformed json", `{not json`, "", true},
		{"non-semver sentinel", `{"version":"unknown"}`, "", true},
		{"version with control chars", "{\"version\":\"1.0.0]0;x\"}", "", true},
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
