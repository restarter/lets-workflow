package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// lets-nobb5: what LETS puts into every turn has a byte budget. The core rules
// are always-on; a tracker adapter is auto-loaded today and stays in the session
// from first use once it is loaded on demand; a board profile rides with it.
// Command-time procedure belongs in the lazy protocol layer, not here - see
// CONTRIBUTING.md "## Rules layers".
//
// Raising a number below is a design decision, not a fix for a red test: it is
// the owner's call and is recorded in CONTRIBUTING "Rules layers" -> Budgets
// (the core went 12,288 -> 16,384 B that way, 2026-09-27).
const (
	coreRulesBudget = 16384 // rules/lets-rules.md
	adapterBudget   = 8192  // rules/tracker-*.md, the TEMPLATE included
	boardBudget     = 4096  // rules/tracker-*.board.md
)

// budgetFor returns the byte budget of a file under plugins/lets/rules, and
// false for a file the budgets do not cover.
func budgetFor(name string) (int, bool) {
	switch {
	case name == "lets-rules.md":
		return coreRulesBudget, true
	case strings.HasPrefix(name, "tracker-") && strings.HasSuffix(name, ".board.md"):
		return boardBudget, true
	case strings.HasPrefix(name, "tracker-") && strings.HasSuffix(name, ".md"):
		return adapterBudget, true
	}
	return 0, false
}

// rulesBudgetProblems reads only its argument (file name -> size in bytes), so
// the table below can prove every budget fails when exceeded.
func rulesBudgetProblems(sizes map[string]int) []string {
	names := make([]string, 0, len(sizes))
	for n := range sizes {
		names = append(names, n)
	}
	sort.Strings(names)
	var bad []string
	for _, n := range names {
		budget, ok := budgetFor(n)
		if !ok {
			continue
		}
		if sizes[n] > budget {
			bad = append(bad, fmt.Sprintf("plugins/lets/rules/%s is %d B, over its %d B budget by %d B - move command-time text to the protocol layer or rationale to CONTRIBUTING; raising the budget is an owner decision (CONTRIBUTING.md \"## Rules layers\" -> Budgets)", n, sizes[n], budget, sizes[n]-budget))
		}
	}
	return bad
}

func TestRulesBudget(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(pluginRulesDir(t), "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	sizes := map[string]int{}
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		sizes[filepath.Base(p)] = int(fi.Size())
	}
	for _, must := range []string{"lets-rules.md", "tracker-beads.md", "tracker-none.md", "tracker-TEMPLATE.md"} {
		if _, ok := sizes[must]; !ok {
			t.Fatalf("plugins/lets/rules/%s not found - the budget test would silently cover nothing", must)
		}
	}
	for _, p := range rulesBudgetProblems(sizes) {
		t.Error(p)
	}

	// each budget can fail: one byte over fails, exactly at the budget passes
	for _, tc := range []struct {
		name   string
		budget int
	}{
		{"lets-rules.md", coreRulesBudget},
		{"tracker-beads.md", adapterBudget},
		{"tracker-TEMPLATE.md", adapterBudget},
		{"tracker-custom.board.md", boardBudget},
	} {
		if got, ok := budgetFor(tc.name); !ok || got != tc.budget {
			t.Errorf("budgetFor(%q) = %d, %v; want %d", tc.name, got, ok, tc.budget)
		}
		if p := rulesBudgetProblems(map[string]int{tc.name: tc.budget + 1}); len(p) != 1 {
			t.Errorf("%s at %d B (budget %d) must fail, got %v", tc.name, tc.budget+1, tc.budget, p)
		}
		if p := rulesBudgetProblems(map[string]int{tc.name: tc.budget}); len(p) != 0 {
			t.Errorf("%s at exactly its budget must pass, got %v", tc.name, p)
		}
	}
	// a board is measured against the board budget, not the larger adapter one
	if p := rulesBudgetProblems(map[string]int{"tracker-beads.board.md": boardBudget + 1}); len(p) != 1 {
		t.Errorf("a board file over %d B must fail even though it is under the adapter budget, got %v", boardBudget, p)
	}
	// files outside the budgets are not measured
	if _, ok := budgetFor("git.md"); ok {
		t.Error("budgetFor(git.md) must not claim a budget")
	}
}
