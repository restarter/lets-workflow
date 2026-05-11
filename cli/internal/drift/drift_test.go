package drift_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/drift"
)

func writeRules(t *testing.T, path, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nversion: " + version + "\n---\n# Rules\n"
	if version == "" {
		body = "# No frontmatter\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheck_AllStates(t *testing.T) {
	tests := []struct {
		name             string
		pluginVer        string
		installedExists  bool
		installedVer     string
		wantState        drift.State
		wantInstalledVer string
		wantPluginVer    string
		wantDetected     bool
		wantMessage      string
	}{
		{"equal", "0.5.0", true, "0.5.0", drift.StateEqual, "0.5.0", "0.5.0", false, ""},
		{"outdated", "0.5.0", true, "0.4.0", drift.StateOutdated, "0.4.0", "0.5.0", true,
			"Workflow rules outdated (installed v0.4.0 < plugin v0.5.0). Run `/lets:update` to update."},
		{"ahead", "0.5.0", true, "0.6.0", drift.StateAhead, "0.6.0", "0.5.0", true,
			"Workflow rules AHEAD of plugin (installed v0.6.0 > plugin v0.5.0). Verify the rules file integrity (rules tampering signal) or upgrade the lets binary. Run `/lets:update` to reset to plugin version."},
		{"missing", "0.5.0", false, "", drift.StateMissing, "", "0.5.0", true,
			"Workflow rules not installed in `.claude/rules/lets-rules.md`. Run `/lets:init` to install."},
		{"unknown_unparseable", "0.5.0", true, "", drift.StateUnknown, "", "0.5.0", true,
			"Workflow rules version unknown - rules may be outdated. Run `/lets:update` to refresh."},
		{"plugin_unreadable", "", true, "0.5.0", drift.StatePluginUnreadable, "", "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			pluginPath := filepath.Join(dir, "plugin", "rules.md")
			installedPath := filepath.Join(dir, "installed", "rules.md")
			writeRules(t, pluginPath, tt.pluginVer)
			if tt.installedExists {
				writeRules(t, installedPath, tt.installedVer)
			}
			got := drift.Check(pluginPath, installedPath)
			if got.State != tt.wantState {
				t.Errorf("State: got %q want %q", got.State, tt.wantState)
			}
			if got.InstalledVersion != tt.wantInstalledVer {
				t.Errorf("InstalledVersion: got %q want %q", got.InstalledVersion, tt.wantInstalledVer)
			}
			if got.PluginVersion != tt.wantPluginVer {
				t.Errorf("PluginVersion: got %q want %q", got.PluginVersion, tt.wantPluginVer)
			}
			if got.Detected() != tt.wantDetected {
				t.Errorf("Detected: got %v want %v", got.Detected(), tt.wantDetected)
			}
			if msg := drift.Message(got); msg != tt.wantMessage {
				t.Errorf("Message: got %q want %q", msg, tt.wantMessage)
			}
		})
	}
}
