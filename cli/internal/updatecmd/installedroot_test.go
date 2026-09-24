package updatecmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installRelease lays out <home>/.claude/plugins/cache/lets-workflow/lets/<dir>
// with plugin.json + rules at ver, and returns the root.
func installRelease(t *testing.T, home, dir, ver string) string {
	t.Helper()
	root := filepath.Join(home, ".claude", "plugins", "cache", "lets-workflow", "lets", dir)
	writePluginJSON(t, root, fmt.Sprintf(`{"name":"lets","version":%q}`, ver))
	rulesFile(t, filepath.Join(root, "rules", "lets-rules.md"), ver)
	return root
}

// writeIndex writes installed_plugins.json listing paths under lets@lets-workflow.
func writeIndex(t *testing.T, home string, paths ...string) {
	t.Helper()
	type entry struct {
		InstallPath string `json:"installPath"`
	}
	var es []entry
	for _, p := range paths {
		es = append(es, entry{InstallPath: p})
	}
	data, err := json.Marshal(map[string]any{"version": 2, "plugins": map[string][]entry{"lets@lets-workflow": es}})
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), string(data))
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveInstalledRoot_NewerInstallWins(t *testing.T) {
	home := t.TempDir()
	old := installRelease(t, home, "0.9.1", "0.9.1")
	neu := installRelease(t, home, "0.9.2", "0.9.2")
	writeIndex(t, home, old, neu)
	root, verified, note := ResolveInstalledRoot(old, home)
	if root != neu || !verified || note != "" {
		t.Fatalf("got %q verified=%v note=%q, want %q verified", root, verified, note, neu)
	}
}

func TestResolveInstalledRoot_OnlyHandedListed(t *testing.T) {
	home := t.TempDir()
	old := installRelease(t, home, "0.9.1", "0.9.1")
	writeIndex(t, home, old)
	root, verified, note := ResolveInstalledRoot(old, home)
	if root != old || !verified || note != "" {
		t.Fatalf("got %q verified=%v note=%q", root, verified, note)
	}
}

func TestResolveInstalledRoot_EntryWithoutManifestIgnored(t *testing.T) {
	home := t.TempDir()
	old := installRelease(t, home, "0.9.1", "0.9.1")
	ghost := filepath.Join(home, ".claude", "plugins", "cache", "lets-workflow", "lets", "0.9.9") // no plugin.json
	writeIndex(t, home, old, ghost)
	root, verified, _ := ResolveInstalledRoot(old, home)
	if root != old || !verified {
		t.Fatalf("got %q verified=%v, want the handed root verified", root, verified)
	}
}

func TestResolveInstalledRoot_BadIndexIsNotGuessed(t *testing.T) {
	for name, index := range map[string]string{"unreadable": "", "malformed": "{not json"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			old := installRelease(t, home, "0.9.1", "0.9.1")
			installRelease(t, home, "0.9.2", "0.9.2") // newer on disk, but the index can't vouch for it
			if index != "" {
				writeRaw(t, filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), index)
			}
			root, verified, note := ResolveInstalledRoot(old, home)
			if root != old || verified || !strings.Contains(note, "installed_plugins.json") {
				t.Fatalf("got %q verified=%v note=%q, want handed + unverified + note", root, verified, note)
			}
		})
	}
}

func TestResolveInstalledRoot_TwoInstallsOfOneVersion(t *testing.T) {
	home := t.TempDir()
	a := installRelease(t, home, "0.9.2", "0.9.2")
	b := installRelease(t, home, "0.9.2-rebuild", "0.9.2")
	writeIndex(t, home, a, b)
	root, verified, note := ResolveInstalledRoot(a, home)
	if root != a || verified || !strings.Contains(note, "two installs") {
		t.Fatalf("got %q verified=%v note=%q", root, verified, note)
	}
}

func TestResolveInstalledRoot_OutsideCache(t *testing.T) {
	home := t.TempDir()
	dev := t.TempDir()
	root, verified, note := ResolveInstalledRoot(dev, home)
	if root != dev || verified || note != "" {
		t.Fatalf("got %q verified=%v note=%q", root, verified, note)
	}
}

// An index entry outside the marketplace's cache dir is never chosen, however
// good its manifest looks.
func TestResolveInstalledRoot_EntryOutsideCacheIgnored(t *testing.T) {
	home := t.TempDir()
	old := installRelease(t, home, "0.9.1", "0.9.1")
	outside := filepath.Join(t.TempDir(), "lets")
	writePluginJSON(t, outside, `{"name":"lets","version":"0.9.9"}`)
	rulesFile(t, filepath.Join(outside, "rules", "lets-rules.md"), "0.9.9")
	writeIndex(t, home, old, outside)
	if root, verified, _ := ResolveInstalledRoot(old, home); root != old || !verified {
		t.Fatalf("got %q verified=%v, want the installed %q", root, verified, old)
	}
}

// A manifest that is not LETS (wrong name) or whose version differs from its
// rules is not an installed LETS root.
func TestResolveInstalledRoot_WrongIdentityIgnored(t *testing.T) {
	home := t.TempDir()
	old := installRelease(t, home, "0.9.1", "0.9.1")
	wrongName := installRelease(t, home, "0.9.8", "0.9.8")
	writePluginJSON(t, wrongName, `{"name":"not-lets","version":"0.9.8"}`)
	mismatched := installRelease(t, home, "0.9.9", "0.9.9")
	rulesFile(t, filepath.Join(mismatched, "rules", "lets-rules.md"), "0.9.7")
	writeIndex(t, home, old, wrongName, mismatched)
	if root, verified, _ := ResolveInstalledRoot(old, home); root != old || !verified {
		t.Fatalf("got %q verified=%v, want the installed %q", root, verified, old)
	}
}
