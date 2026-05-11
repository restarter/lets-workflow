package updatecmd

import (
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/mod/semver"
)

// ReadPluginVersion returns the "version" field from
// <pluginRoot>/.claude-plugin/plugin.json, or "" on any failure (missing file,
// malformed JSON, no version key, or a value that isn't a clean semver).
//
// This is the authoritative source for the *installed* plugin version: the
// slash command passes --plugin-root=${CLAUDE_PLUGIN_ROOT}, which is exactly
// the directory Claude Code loaded the plugin from. (We deliberately do NOT
// trust ~/.claude/plugins/installed_plugins.json's "version" field, which is
// empirically "unknown" for most marketplace plugins.)
//
// The semver check both filters the legitimate "unknown" sentinel and stops a
// tampered plugin.json from feeding control characters downstream (the value
// flows into the JSON envelope and the terminal via PrintReport).
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
	if !semver.IsValid("v" + p.Version) {
		return ""
	}
	return p.Version
}
