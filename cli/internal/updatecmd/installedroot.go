package updatecmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

// ResolveInstalledRoot returns the plugin root Claude Code has INSTALLED now,
// which can be newer than the root this session loaded (${CLAUDE_PLUGIN_ROOT}
// is frozen per session - lets-tg008), and whether that root is VERIFIED: chosen
// from the installPath entries of lets@<marketplace> in
// ~/.claude/plugins/installed_plugins.json, with the version read from
// plugin.json at that path (the index's own `version` field is not trusted - see
// ReadPluginVersion). A missing, malformed or ambiguous index is never guessed
// at: the handed root comes back unverified, with a note.
func ResolveInstalledRoot(handed, home string) (root string, verified bool, note string) {
	if home == "" {
		return handed, false, ""
	}
	cache := filepath.Join(home, ".claude", "plugins", "cache") + string(filepath.Separator)
	if !strings.HasPrefix(handed, cache) {
		return handed, false, "" // a dev checkout: nothing installed to resolve against
	}
	parts := strings.Split(strings.TrimPrefix(handed, cache), string(filepath.Separator))
	if len(parts) < 2 || parts[1] != "lets" {
		return handed, false, ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"))
	if err != nil {
		return handed, false, "installed_plugins.json unreadable - not verified against the installed plugin"
	}
	var f struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &f) != nil {
		return handed, false, "installed_plugins.json unparseable - not verified against the installed plugin"
	}
	best, bestVer, tie := "", "", false
	for _, e := range f.Plugins["lets@"+parts[0]] {
		v := ReadPluginVersion(e.InstallPath)
		if v == "" {
			continue
		}
		switch {
		case bestVer == "" || semver.Compare("v"+v, "v"+bestVer) > 0:
			best, bestVer, tie = filepath.Clean(e.InstallPath), v, false
		case semver.Compare("v"+v, "v"+bestVer) == 0 && filepath.Clean(e.InstallPath) != best:
			tie = true
		}
	}
	switch {
	case best == "":
		return handed, false, "no readable lets entry in installed_plugins.json - not verified"
	case tie:
		return handed, false, fmt.Sprintf("installed_plugins.json lists two installs of v%s - not verified", bestVer)
	}
	return best, true, ""
}
