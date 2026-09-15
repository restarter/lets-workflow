package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ideaGrep = `grep -v -E -- '-idea(-v[0-9]+)?\.md$'`

// ideaGaps returns the plans lookups in execute.md that could select an idea file.
func ideaGaps(execute string) []string {
	var bad []string
	for _, line := range strings.Split(execute, "\n") {
		if strings.Contains(line, `ls -t "$LETS_PROJECT_ROOT/.lets/plans/"`) && !strings.Contains(line, ideaGrep) {
			bad = append(bad, strings.TrimSpace(line))
		}
	}
	return bad
}

// TestIdeaMode pins /lets:plan --idea: idea files share .lets/plans with plans, so
// every consumer that picks "the latest plan" must skip them, and the idea template
// must never carry the plan's execute banner.
func TestIdeaMode(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(pluginDir(t), rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if !strings.Contains(read("skills/artifact-path/SKILL.md"), "plan|idea) DIR=plans") {
		t.Error("artifact-path must map kind idea to .lets/plans")
	}
	execute := read("commands/execute.md")
	if n := strings.Count(execute, `ls -t "$LETS_PROJECT_ROOT/.lets/plans/"`); n == 0 {
		t.Fatal("execute.md plans lookups not found - update this test")
	}
	for _, g := range ideaGaps(execute) {
		t.Errorf("execute.md plans lookup can pick an idea file: %s", g)
	}
	if !strings.Contains(execute, "This is an idea document - run /lets:plan to turn it into a plan.") {
		t.Error("execute.md must refuse an idea document passed by path")
	}
	plan := read("commands/plan.md")
	if !strings.Contains(plan, "IDEA BANK ENTRY - NOT A PLAN") {
		t.Error("plan.md --idea template banner missing")
	}
	if sec := sectionSpan(plan, "## --idea mode"); sec == "" || strings.Contains(sec, "STOP - THIS PLAN IS NOT A GO") {
		t.Errorf("the --idea section must exist and carry no execute banner")
	}
	check := read("commands/check.md")
	for _, lens := range []string{"[Clarity]", "[Scope]", "[Feasibility]", "[Open questions]", "[Handoff]"} {
		if !strings.Contains(check, lens) {
			t.Errorf("check.md concept lens %s missing", lens)
		}
	}
	// mutation: dropping one grep must be caught
	mutated := strings.Replace(execute, " | "+ideaGrep, "", 1)
	if len(ideaGaps(mutated)) == 0 {
		t.Error("mutation: a plans lookup without the idea grep must fail the test")
	}
}
