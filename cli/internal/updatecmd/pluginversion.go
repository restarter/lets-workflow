package updatecmd

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ReadPluginVersion returns the "version" field from
// <pluginRoot>/.claude-plugin/plugin.json, or "" on any failure (missing file,
// malformed JSON, no version key).
//
// This is the authoritative source for the *installed* plugin version: the
// slash command passes --plugin-root=${CLAUDE_PLUGIN_ROOT}, which is exactly
// the directory Claude Code loaded the plugin from. (We deliberately do NOT
// trust ~/.claude/plugins/installed_plugins.json's "version" field, which is
// empirically "unknown" for most marketplace plugins.)
func ReadPluginVersion(pluginRoot string) string {
	data, err := os.ReadFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"))
	if err != nil {
		return ""
	}
	var p struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.Version
}
