package updatecmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/rulescache"
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
	// Paths are compared RESOLVED - the definition rulescache.CheckInstalledRoot
	// uses - so a symlinked cache, or a handed / indexed path on either side of
	// the link, still resolves to the same install.
	sep := string(filepath.Separator)
	cache, err := filepath.EvalSymlinks(filepath.Join(home, ".claude", "plugins", "cache"))
	if err != nil {
		return handed, false, "" // no installed-plugin cache: nothing to resolve against
	}
	realHanded, err := filepath.EvalSymlinks(handed)
	if err != nil {
		return handed, false, ""
	}
	rel, err := filepath.Rel(cache, realHanded)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if err != nil || len(parts) < 2 || parts[0] == ".." || parts[1] != "lets" {
		return handed, false, "" // a dev checkout: nothing installed to resolve against
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
	best, bestReal, bestVer, tie := "", "", "", false
	mpDir := filepath.Join(cache, parts[0]) + sep
	for _, e := range f.Plugins["lets@"+parts[0]] {
		// Only an entry that is provably an installed LETS plugin of THIS
		// marketplace may be chosen - the same check rulescache applies before it
		// writes (cache layout, manifest name/version, rules frontmatter).
		real, err := filepath.EvalSymlinks(e.InstallPath)
		if err != nil || !strings.HasPrefix(real+sep, mpDir) || rulescache.CheckInstalledRoot(e.InstallPath, home) != "" {
			continue
		}
		v := ReadPluginVersion(e.InstallPath)
		if v == "" {
			continue
		}
		switch {
		case bestVer == "" || semver.Compare("v"+v, "v"+bestVer) > 0:
			best, bestReal, bestVer, tie = filepath.Clean(e.InstallPath), real, v, false
		case semver.Compare("v"+v, "v"+bestVer) == 0 && real != bestReal:
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
