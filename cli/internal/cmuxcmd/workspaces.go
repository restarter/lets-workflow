//go:build unix

package cmuxcmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// workspaceEntry is one row of `cmux workspace list --json`.
type workspaceEntry struct {
	Ref              string `json:"ref"`
	Title            string `json:"title"`
	Selected         bool   `json:"selected"`
	CurrentDirectory string `json:"current_directory"`
}

type workspaceList struct {
	Workspaces []workspaceEntry `json:"workspaces"`
}

// listWorkspacesRaw runs `cmux workspace list --json` and returns stdout.
// Overridable in tests. CMUX_QUIET=1 keeps notices off stderr so stdout is
// clean JSON.
var listWorkspacesRaw = func(ctx context.Context, bin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, "workspace", "list", "--json")
	cmd.Env = append(os.Environ(), "CMUX_QUIET=1")
	return cmd.Output()
}

// listWorkspaces parses the cmux workspace list.
func listWorkspaces(ctx context.Context, bin string) ([]workspaceEntry, error) {
	out, err := listWorkspacesRaw(ctx, bin)
	if err != nil {
		return nil, err
	}
	var wl workspaceList
	if err := json.Unmarshal(out, &wl); err != nil {
		return nil, err
	}
	return wl.Workspaces, nil
}

// findByDir returns the workspace whose current_directory matches dir (compared
// as cleaned absolute paths), or nil.
func findByDir(ws []workspaceEntry, dir string) *workspaceEntry {
	want := cleanPath(dir)
	for i := range ws {
		if cleanPath(ws[i].CurrentDirectory) == want {
			return &ws[i]
		}
	}
	return nil
}

// findSelected returns the active workspace (selected==true), or nil.
func findSelected(ws []workspaceEntry) *workspaceEntry {
	for i := range ws {
		if ws[i].Selected {
			return &ws[i]
		}
	}
	return nil
}

// cleanPath normalizes a path for comparison: Abs, then EvalSymlinks so a
// symlinked path (macOS /var->/private/var, /tmp, a symlinked .worktrees entry)
// matches cmux's resolved current_directory. Falls back to Clean when the path
// doesn't exist locally (cmux may report a path we can't stat). Mirrors
// worktreecmd.sameDir's resolve closure - without symlink resolution the
// duplicate-session guard fails open on macOS and findByDir misses.
func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}
