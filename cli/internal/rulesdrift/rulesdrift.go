// Package rulesdrift compares a checkout's installed LETS rules copies - the
// project .claude/rules/lets-rules.md and the active tracker adapter - with the
// plugin's (lets-5whww). It reads files only, never git, so the SessionStart hook
// can call it. A linked worktree checks out its branch's COMMITTED copies, which
// /lets:update never writes; the messages say how to fix that.
package rulesdrift

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/rulescache"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// Options names what to compare. PluginRoot "" -> nothing is checked.
type Options struct {
	PluginRoot   string // the plugin root (holds rules/)
	CheckoutRoot string // the checkout judged
	MainRoot     string // its main checkout; "" or == CheckoutRoot in the main checkout
	Scope        string // resolved LETS_RULES_SCOPE
	Tracker      string // LETS_TRACKER as set - no default; "" -> no adapter check
	HomeDir      string // for the global rules copy; "" -> treated as absent
}

// File is one copy that lags: State is outdated, unknown or missing.
type File struct {
	Path      string      // ".claude/rules/<name>"
	State     drift.State //
	Installed string      // this checkout's version, "" when missing / unreadable
	Plugin    string      // the plugin's version
	Main      string      // worktree only: the main checkout's version, "" when absent / unreadable
}

const letsRules = "lets-rules.md"

// IsAdapter reports whether f is the tracker adapter (not lets-rules.md).
func (f File) IsAdapter() bool { return filepath.Base(f.Path) != letsRules }

// Check returns every copy that lags the plugin, in a stable order (lets-rules.md
// first). ahead and plugin-unreadable are never returned.
func Check(o Options) []File {
	if o.PluginRoot == "" || o.CheckoutRoot == "" {
		return nil
	}
	names := []string{letsRules}
	if o.Tracker != "" && trackeradapter.NameRe.MatchString(o.Tracker) {
		n := "tracker-" + o.Tracker + ".md"
		if _, err := os.Stat(filepath.Join(o.PluginRoot, "rules", n)); err == nil {
			names = append(names, n)
		}
	}
	worktree := o.MainRoot != "" && filepath.Clean(o.MainRoot) != filepath.Clean(o.CheckoutRoot)
	var out []File
	for _, n := range names {
		rel := filepath.Join(".claude", "rules", n)
		src := filepath.Join(o.PluginRoot, "rules", n)
		r := drift.Check(src, filepath.Join(o.CheckoutRoot, rel))
		if !r.Detected() || r.State == drift.StateAhead {
			continue
		}
		if r.State == drift.StateMissing && n == letsRules && (o.Scope == "user" || globalPresent(o.HomeDir)) {
			continue // the global copy covers this project
		}
		f := File{Path: rel, State: r.State, Installed: r.InstalledVersion, Plugin: r.PluginVersion}
		if worktree {
			f.Main = drift.Check(src, filepath.Join(o.MainRoot, rel)).InstalledVersion
		}
		out = append(out, f)
	}
	return out
}

func globalPresent(home string) bool {
	if home == "" {
		return false
	}
	_, err := os.Stat(rulescache.DstPath(home))
	return err == nil
}

// WorktreeMessage is the ONE line for a linked worktree; "" when files is empty.
// merge appears once (the hook body is capped).
func WorktreeMessage(files []File, merge string) string {
	if len(files) == 0 {
		return ""
	}
	if merge == "" {
		merge = "the merge branch"
	}
	plugin := files[0].Plugin
	mainCurrent := true
	parts := make([]string, 0, len(files))
	for _, f := range files {
		switch f.State {
		case drift.StateOutdated:
			parts = append(parts, fmt.Sprintf("`%s` v%s", f.Path, f.Installed))
		case drift.StateUnknown:
			parts = append(parts, fmt.Sprintf("`%s` (no version)", f.Path))
		default:
			parts = append(parts, fmt.Sprintf("`%s` missing", f.Path))
		}
		if f.Main != f.Plugin {
			mainCurrent = false
		}
	}
	head := fmt.Sprintf("LETS rules in this worktree lag the plugin v%s: %s - a worktree gets its branch's committed copy, and /lets:update does not write it.", plugin, strings.Join(parts, ", "))
	if mainCurrent {
		return head + fmt.Sprintf(" The main checkout already has v%s: land `.claude/rules/` on `%s` there (commit + push, or a PR), then rebase this branch on it; a file you keep out of git goes in `.worktreeinclude` (then `lets worktree adopt`).", plugin, merge)
	}
	return head + fmt.Sprintf(" In the main checkout: /lets:update, land `.claude/rules/` on `%s` (commit + push, or a PR), then rebase this branch on it.", merge)
}

// AdapterMessage is the main checkout's line for a lagging tracker adapter.
func AdapterMessage(f File) string {
	switch f.State {
	case drift.StateMissing:
		return fmt.Sprintf("Tracker adapter `%s` not installed. Run `/lets:update`.", f.Path)
	case drift.StateUnknown:
		return fmt.Sprintf("Tracker adapter `%s` version unknown. Run `/lets:update`, then commit it - worktrees and clones check out the committed copy.", f.Path)
	}
	return fmt.Sprintf("Tracker adapter `%s` outdated (installed v%s < plugin v%s). Run `/lets:update`, then commit it - worktrees and clones check out the committed copy.", f.Path, f.Installed, f.Plugin)
}
