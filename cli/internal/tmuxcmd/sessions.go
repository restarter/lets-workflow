//go:build unix

package tmuxcmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// paneEntry is one row of `tmux list-panes -a -F …`.
type paneEntry struct {
	Session string
	Window  string // window index, as printed
	Title   string // window name
	Path    string // pane_current_path
}

// Target returns the tmux target spec "session:window_index".
func (p paneEntry) Target() string { return p.Session + ":" + p.Window }

// paneFormat is the -F format string. Tab-separated because a session name may
// contain spaces but never a tab (tmux rejects them in -s/-n).
const paneFormat = "#{session_name}\t#{window_index}\t#{window_name}\t#{pane_current_path}"

// listPanesRaw runs `tmux list-panes -a -F <paneFormat>` and returns stdout.
// Overridable in tests. Fails when no tmux server is running - that is NOT an
// error for the caller (the duplicate guard degrades to "create anyway").
var listPanesRaw = func(ctx context.Context, bin string) ([]byte, error) {
	return exec.CommandContext(ctx, bin, "list-panes", "-a", "-F", paneFormat).Output()
}

// listPanes parses every pane across every tmux session.
func listPanes(ctx context.Context, bin string) ([]paneEntry, error) {
	out, err := listPanesRaw(ctx, bin)
	if err != nil {
		return nil, err
	}
	var panes []paneEntry
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 {
			continue // malformed row: skip, never fail the whole listing
		}
		panes = append(panes, paneEntry{Session: f[0], Window: f[1], Title: f[2], Path: f[3]})
	}
	return panes, nil
}

// findByPath returns the first pane whose current path matches dir (compared as
// cleaned, symlink-resolved absolute paths), or nil.
func findByPath(panes []paneEntry, dir string) *paneEntry {
	want := cleanPath(dir)
	if want == "" {
		return nil
	}
	for i := range panes {
		if cleanPath(panes[i].Path) == want {
			return &panes[i]
		}
	}
	return nil
}

// cleanPath normalizes a path for comparison: Abs, then EvalSymlinks so a
// symlinked path (macOS /var->/private/var, /tmp, a symlinked .worktrees entry)
// matches tmux's resolved pane_current_path. Falls back to Clean when the path
// doesn't exist locally. Mirrors cmuxcmd.cleanPath and worktreecmd.sameDir -
// without symlink resolution the duplicate guard fails open on macOS.
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

// sanitizeName makes s usable as a tmux session/window name. tmux treats ':'
// and '.' as target separators (session:window.pane), so a name carrying them
// silently retargets the command; '\t' and newlines would corrupt paneFormat
// parsing. Everything problematic collapses to '-'. An empty result yields
// "lets" so a target is never the empty string.
func sanitizeName(s string) string {
	repl := func(r rune) rune {
		switch r {
		case ':', '.', '\t', '\n', '\r', ' ':
			return '-'
		}
		return r
	}
	out := strings.Trim(strings.Map(repl, s), "-")
	if out == "" {
		return "lets"
	}
	return out
}

// activeTargetRaw runs `tmux display-message -p <targetFormat>` to get the
// client's current target. Overridable in tests.
var activeTargetRaw = func(ctx context.Context, bin string) ([]byte, error) {
	return exec.CommandContext(ctx, bin, "display-message", "-p", "#{session_name}:#{window_index}").Output()
}

// resolveTarget picks the tmux target: explicit ref wins, else a pane matching
// cwd, else the active window. Returns ("", "") when nothing matches - callers
// degrade with reason "pane_not_found".
func resolveTarget(ctx context.Context, bin, ref, cwd string) (target, title string) {
	if ref != "" {
		return ref, ""
	}
	if cwd != "" {
		panes, err := listPanes(ctx, bin)
		if err != nil {
			return "", ""
		}
		if p := findByPath(panes, cwd); p != nil {
			return p.Target(), p.Title
		}
		return "", ""
	}
	out, err := activeTargetRaw(ctx, bin)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(string(out)), ""
}

// listClientsRaw runs `tmux list-clients -F '#{client_name}'` SERVER-WIDE (no
// -t): every client attached to any session. Overridable in tests. Errors when
// no tmux server is running at all.
var listClientsRaw = func(ctx context.Context, bin string) ([]byte, error) {
	return exec.CommandContext(ctx, bin, "list-clients", "-F", "#{client_name}").Output()
}

// listClients returns the names of every client attached to the tmux server.
// An EMPTY slice with a nil error is the meaningful case: the server is up (our
// detached session may well be in it) but no human is looking. Notify turns that
// into reason=no_client rather than a phantom success - see Notify's doc.
func listClients(ctx context.Context, bin string) ([]string, error) {
	out, err := listClientsRaw(ctx, bin)
	if err != nil {
		return nil, err
	}
	var clients []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			clients = append(clients, line)
		}
	}
	return clients, nil
}
