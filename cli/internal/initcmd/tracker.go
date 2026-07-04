package initcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/restarter/lets-workflow/cli/internal/envfile"
)

// shippedTrackers is the set of plugin-shipped adapter names (the value of
// LETS_TRACKER, == the tracker-<name>.md basename without prefix/suffix).
// cleanupShippedTrackerFiles only ever removes files in this set, so a
// user-authored tracker-<custom>.md, any *.board.md, lets-rules.md, or git.md
// in .claude/rules/ is never touched. Keep in sync with the shipped
// plugins/lets/rules/tracker-*.md files and the CONTRIBUTING recipe.
var shippedTrackers = []string{"beads", "planfix-mcp", "none"}

// trackerNameRe constrains an adapter name to a branch/path-safe shape. The
// value flows into filepath.Join for both the write target and the cleanup
// glob, and reaches model context via the SessionStart hook - so a hand-edited
// .env value like "../../etc/x" must never be used unvalidated.
var trackerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidTrackerName reports whether s is a safe adapter name. Exported so
// updatecmd can apply the same path guard before filepath.Join.
func ValidTrackerName(s string) bool { return trackerNameRe.MatchString(s) }

// resolvedTracker reads the resolved LETS_TRACKER from the project .env.
// Callers run AFTER RegenerateEnv (init Step 5 precedes Step 8b), so the file
// carries the reconciled value (fresh --tracker pick OR a preserved existing
// adapter) - no separate precedence logic. Empty on missing/unparseable file or
// absent key. Mirrors effectiveRulesScope's read-from-.env pattern.
func resolvedTracker(envPath string) string {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}
	vals, err := envfile.Parse(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return vals["LETS_TRACKER"]
}

// CleanupShippedTrackerFiles removes any plugin-shipped tracker-<name>.md in
// rulesDir that is NOT the active adapter (keep), so switching LETS_TRACKER does
// not leave two adapter files loaded at once. Whitelist-gated (see
// shippedTrackers) - never removes user-authored files. Returns the basenames
// removed plus any that exist but could not be removed (permissions etc) - a
// failed removal means a stale adapter is STILL loaded, so callers must surface
// it, not swallow it. Exported so updatecmd's tracker-rules artifact applies the
// same cleanup on the documented edit-.env-then-update switch path.
func CleanupShippedTrackerFiles(rulesDir, keep string) (removed, failed []string) {
	for _, name := range shippedTrackers {
		if name == keep {
			continue
		}
		p := filepath.Join(rulesDir, "tracker-"+name+".md")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Remove(p); err == nil {
			removed = append(removed, "tracker-"+name+".md")
		} else {
			failed = append(failed, "tracker-"+name+".md")
		}
	}
	return removed, failed
}

// ScaffoldBoardOnce copies the adapter's board-profile template into rulesDir if
// (a) the plugin ships one for this adapter and (b) the project copy does not
// already exist. Create-once: a board profile is user-owned and is NEVER
// overwritten (it holds project-specific status-id maps / transitions). Returns
// a step message when it wrote one ("" for the two legitimate no-op cases), and
// a non-nil error on a genuine write failure - the board carries the status-id
// maps the adapter depends on, so a silent no-scaffold must not look like
// "nothing to do". Exported for updatecmd (switch-via-update path).
func ScaffoldBoardOnce(pluginRoot, rulesDir, tracker string) (string, error) {
	boardSrc := filepath.Join(pluginRoot, "rules", "tracker-"+tracker+".board.md")
	data, err := os.ReadFile(boardSrc)
	if err != nil {
		return "", nil // no board template shipped for this adapter
	}
	boardDst := filepath.Join(rulesDir, "tracker-"+tracker+".board.md")
	if _, err := os.Stat(boardDst); err == nil {
		return "", nil // already present - never overwrite user edits
	}
	if err := AtomicWriteBytes(boardDst, data, 0o644); err != nil {
		return "", fmt.Errorf("scaffold tracker-%s.board.md: %w", tracker, err)
	}
	return fmt.Sprintf(".claude/rules/tracker-%s.board.md scaffolded (user-owned - edit freely)", tracker), nil
}
