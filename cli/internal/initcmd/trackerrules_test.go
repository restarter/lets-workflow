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
// with tracker-TEMPLATE.md's "## Capabilities + bindings" section (marked by the
// PINNED CONTRACT html comment inside it).
const pinnedCapsHeader = "| verb | tier | supported | binding |"

// sectionSpan returns the slice of content from the first occurrence of heading
// to the next heading of the same or higher level ("" if the heading is absent).
// Scoping matchers to the section they claim to pin keeps the assertions
// falsifiable - a whole-file Contains can be satisfied by an unrelated mention
// elsewhere (the branch-review S9 finding: `label` had 5 backticked mentions
// across lets-rules.md, so deleting it from the verb list still passed).
func sectionSpan(content, heading string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}
	rest := content[start+len(heading):]
	level := strings.Count(strings.Split(heading, " ")[0], "#")
	for _, prefix := range []string{"\n## ", "\n### "} {
		if strings.Count(prefix, "#") > level {
			continue // deeper headings stay inside the span
		}
		if end := strings.Index(rest, prefix); end >= 0 {
			rest = rest[:end]
		}
	}
	return rest
}

// capsTable slices an adapter file to its "## Capabilities + bindings" section
// so table matchers can't be satisfied by a 4-column table elsewhere (status
// maps, board tables).
func capsTable(content string) string {
	if s := sectionSpan(content, "## Capabilities + bindings"); s != "" {
		return s
	}
	return content // header assertion elsewhere reports the real problem
}

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
			verbs := tableVerbs(capsTable(content))
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

// TestTrackerBeads_BindsBdCommands pins each beads verb to the bd invocation its
// binding CELL resolves to. Command/skill bodies carry neutral ```lets-tracker
// blocks (lets-rules "Tracker Adapters"); for beads every verb resolves through
// THIS table, so a drifted binding cell silently changes what /lets:* actually
// runs. Each verb pins one or more cell-scoped fragments covering the
// behavior-critical spans (--reason, --status=, --json, the create field flags,
// the body-file/description-file capture) - tight enough to catch a dropped
// flag, unlike a bare command-name substring. Asserting against cell 3 (the
// binding column) of the verb's own row within the Capabilities section, not
// anywhere in the file, stops a fragment in one row from masking a regression
// in another.
func TestTrackerBeads_BindsBdCommands(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(pluginRulesDir(t), "tracker-beads.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	caps := capsTable(content)
	// want[verb] = bd fragments the verb's binding cell MUST contain. Fragments
	// sit before any escaped table pipe (\|) so cell-3 truncation on the
	// set-status row is harmless.
	want := map[string][]string{
		"create":         {"bd create --title=", "--type=", "--priority=", "--labels=", `--description="$(cat <description-file>)"`},
		"show":           {"bd show <id>", "bd show <id> --json"},
		"comment-add":    {`bd comments add <id> "$(cat <body-file>)"`, `bd comments add <id> "<body>"`},
		"set-status":     {"bd update <id> --status="},
		"close":          {"bd close <id> [--reason"},
		"comment-list":   {"bd comments <id>"},
		"list-by-status": {"bd list --status=<status>", "[--json]"},
		"search":         {"bd search <query>"},
		"ready/stats":    {"bd ready [--limit N]", "--limit 0", "bd stats", "bd blocked"},
		"label":          {"bd label list-all", "bd label list <id>", "bd label add <id> <l>"},
		"assignee":       {"bd update <id> --assignee="},
		"set-field":      {"bd update <id> --description="},
	}
	for verb, frags := range want {
		cells := tableRowCells(caps, verb)
		if cells == nil {
			t.Errorf("tracker-beads.md: no binding row for verb %q", verb)
			continue
		}
		for _, frag := range frags {
			if binding := cells[3]; !strings.Contains(binding, frag) {
				t.Errorf("tracker-beads.md: verb %q binding %q must contain %q (a drifted binding changes what /lets:* runs)", verb, strings.TrimSpace(binding), frag)
			}
		}
	}
	// (The label-group progress + priority-histogram Notes bindings were removed with the
	// 5-view /lets:status dashboards in the orient unification (lets-qsgmd); no command
	// renders the NN/MM bars now, so there is nothing left to pin here.)
}

// TestTrackerAdapters_VerbVocabInSync pins the canonical neutral-verb list against
// BOTH the reference adapter (a table row) AND the lets-rules "Tracker Adapters"
// verb list (a backticked mention), so the two can't diverge - a verb renamed in
// the table but not the rule (or vice versa) fails here. The list itself is the
// source of truth. One-directional containment (not strict set-equality) because
// the rule prose groups `ready`/`stats` with slashes; command-body verb spelling
// stays discipline-only (model-read markdown, not mechanically testable).
func TestTrackerAdapters_VerbVocabInSync(t *testing.T) {
	dir := pluginRulesDir(t)
	beads, err := os.ReadFile(filepath.Join(dir, "tracker-beads.md"))
	if err != nil {
		t.Fatal(err)
	}
	rules, err := os.ReadFile(filepath.Join(dir, "lets-rules.md"))
	if err != nil {
		t.Fatal(err)
	}
	beadsVerbs := tableVerbs(capsTable(string(beads)))
	// Scope to the "### Tracker Adapters" section - a whole-file Contains is
	// satisfiable by unrelated backticked mentions (S9: `label` had 5).
	rulesStr := sectionSpan(string(rules), "### Tracker Adapters")
	if rulesStr == "" {
		t.Fatal(`lets-rules.md: "### Tracker Adapters" section not found`)
	}
	for _, v := range []string{
		"create", "show", "comment-add", "set-status", "close",
		"comment-list", "list-by-status", "search", "ready/stats",
		"label", "assignee", "set-field",
	} {
		if !beadsVerbs[v] {
			t.Errorf("neutral verb %q has no row in tracker-beads.md", v)
		}
		for _, part := range strings.Split(v, "/") {
			if !strings.Contains(rulesStr, "`"+part+"`") {
				t.Errorf("neutral verb %q (part %q) not named in lets-rules \"Tracker Adapters\" verb list", v, part)
			}
		}
	}
}

// tableRowCells returns the cells of the first table row whose verb (cell 0)
// equals verb, or nil. Shares the row shape with tableVerbs.
func tableRowCells(content, verb string) []string {
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(l, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(l, "|"), "|")
		if len(cells) < 4 {
			continue
		}
		if strings.TrimSpace(cells[0]) == verb {
			return cells
		}
	}
	return nil
}

// TestTrackerRules_CoreSupported asserts every shipped adapter EXCEPT none marks
// all 5 CORE verbs supported. The `supported` cell (column 2) normalizes via a
// prefix match: a process-gated adapter may render set-status/close as a
// footnoted "yes¹", so a bare == "yes" would false-fail. `none` is the
// sanctioned exception - it ships CORE rows supported=no (null adapter, no store).
// Distinct from TestTrackerRules_Contract (which checks the CORE row EXISTS): this
// checks the row's supported VALUE, catching a CORE verb shipped as a silent hole.
//
// Non-beads coverage note (lets-xdjue): the shipped set is beads + none, so only
// beads exercises the CORE-supported=yes path here (none is the null-adapter
// exception). A non-beads adapter that SUPPORTS core verbs (and the footnoted-
// "yes" normalization above) will be re-covered when the planned Jira/Trello
// worked-example adapter lands; no synthetic fixture is added now (YAGNI).
func TestTrackerRules_CoreSupported(t *testing.T) {
	dir := pluginRulesDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, "tracker-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, path := range matches {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".board.md") || base == "tracker-TEMPLATE.md" || base == "tracker-none.md" {
			continue
		}
		checked++
		t.Run(base, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, v := range coreVerbs {
				cells := tableRowCells(capsTable(content), v)
				if cells == nil {
					t.Errorf("%s: no table row for CORE verb %q", base, v)
					continue
				}
				if sup := strings.TrimSpace(cells[2]); !strings.HasPrefix(sup, "yes") {
					t.Errorf("%s: CORE verb %q supported=%q, want yes (a non-none adapter must support every CORE verb)", base, v, sup)
				}
			}
		})
	}
	if checked == 0 {
		t.Fatalf("no non-none tracker-*.md adapters found in %s - test wiring broken", dir)
	}
}

// TestShippedTrackers_MatchOnDisk pins the shippedTrackers whitelist to the actual
// tracker-*.md adapters on disk (excluding *.board.md + the TEMPLATE). Both
// directions: a slice entry with no file = broken whitelist; an adapter file
// missing from the slice = cleanupShippedTrackerFiles would never remove it on a
// switch (the desync this guards). Catches a new adapter added without a whitelist
// entry, or vice versa.
func TestShippedTrackers_MatchOnDisk(t *testing.T) {
	dir := pluginRulesDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, "tracker-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]bool{}
	for _, path := range matches {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".board.md") || base == "tracker-TEMPLATE.md" {
			continue
		}
		onDisk[strings.TrimSuffix(strings.TrimPrefix(base, "tracker-"), ".md")] = true
	}
	inSlice := map[string]bool{}
	for _, n := range shippedTrackers {
		inSlice[n] = true
	}
	for n := range onDisk {
		if !inSlice[n] {
			t.Errorf("tracker-%s.md on disk but missing from shippedTrackers (a switch would never clean it up)", n)
		}
	}
	for n := range inSlice {
		if !onDisk[n] {
			t.Errorf("shippedTrackers lists %q but no tracker-%s.md on disk (broken whitelist)", n, n)
		}
	}
}
