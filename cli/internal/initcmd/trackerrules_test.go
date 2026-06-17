package initcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
)

// pinnedCapsHeader is the contract header every adapter's capability table MUST
// carry verbatim. The F4c golden test parses the same header; keep both in sync
// with tracker-TEMPLATE.md's "Capability-table format (PINNED CONTRACT)".
const pinnedCapsHeader = "| verb | tier | supported | binding |"

// coreVerbs every adapter MUST bind (have a row for).
var coreVerbs = []string{"create", "show", "comment-add", "set-status", "close"}

// pluginRulesDir resolves <repo>/plugins/lets/rules from this test file's path
// (cli/internal/initcmd/trackerrules_test.go -> up 3 -> repo root).
func pluginRulesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repoRoot, "plugins", "lets", "rules")
}

// tableVerbs returns the set of first-cell values across all markdown table rows
// (so a missing CORE verb row is detectable). Header/separator rows are included
// but harmless.
func tableVerbs(content string) map[string]bool {
	verbs := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(l, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		verbs[strings.TrimSpace(cells[0])] = true
	}
	return verbs
}

// TestTrackerRules_Contract enforces the adapter contract on every shipped
// tracker-<name>.md (excluding *.board.md and the TEMPLATE skeleton): valid
// frontmatter, the PINNED capability-table header, a row for all 5 CORE verbs,
// and a Degradation section. A malformed adapter fails CI here instead of
// silently skipping the drift copy at runtime.
func TestTrackerRules_Contract(t *testing.T) {
	dir := pluginRulesDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, "tracker-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, path := range matches {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".board.md") || base == "tracker-TEMPLATE.md" {
			continue // user board template / copy-me skeleton are not live adapters
		}
		checked++
		t.Run(base, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)

			if v := frontmatter.ReadVersion(path); v == "" {
				t.Errorf("%s: missing/unparseable `version:` frontmatter (drift.Check would silently skip the copy)", base)
			}
			if !strings.Contains(content, pinnedCapsHeader) {
				t.Errorf("%s: capability table header is not the pinned contract %q", base, pinnedCapsHeader)
			}
			verbs := tableVerbs(content)
			for _, v := range coreVerbs {
				if !verbs[v] {
					t.Errorf("%s: missing a binding row for CORE verb %q", base, v)
				}
			}
			if !strings.Contains(content, "## Degradation") {
				t.Errorf("%s: missing a `## Degradation` section", base)
			}
		})
	}
	if checked == 0 {
		t.Fatalf("no tracker-*.md adapters found in %s - test wiring broken", dir)
	}
}

// TestTrackerBeads_BindsBdCommands pins the beads adapter's CORE-verb bindings to
// the bd commands the LETS commands actually run, so the documented beads mapping
// (the orchestrator reverse-maps a literal bd call -> verb for non-beads adapters)
// cannot silently drift from reality. Byte-for-byte beads is otherwise preserved
// by construction - command bodies keep their literal bd calls (F4 resolves via the
// global "Tracker Adapters" rule, it does not rewrite the bd commands).
func TestTrackerBeads_BindsBdCommands(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(pluginRulesDir(t), "tracker-beads.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	want := map[string]string{
		"create":      "bd create",
		"show":        "bd show",
		"comment-add": "bd comments add",
		"set-status":  "bd update",
		"close":       "bd close",
	}
	for verb, cmd := range want {
		if !strings.Contains(content, cmd) {
			t.Errorf("tracker-beads.md: CORE verb %q should bind %q (documented beads mapping drifted)", verb, cmd)
		}
	}
}
