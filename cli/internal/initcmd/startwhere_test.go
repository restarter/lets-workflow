package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startWhereProblems pins /lets:start's "Where to run?" question (lets-f9e9q): asked before
// take-task, only in the main checkout with standing teams, never unattended; a hand-off goes
// through the orc skill and hands the task off without claiming or releasing it here.
func startWhereProblems(start string) []string {
	var bad []string
	add := func(s string) { bad = append(bad, s) }
	s55 := sectionSpan(start, "## Step 5.5: Where to run (main checkout with standing teams)")
	if s55 == "" {
		return []string{"start.md has no Step 5.5"}
	}
	if a, b := strings.Index(start, "\n## Step 5.5: "), strings.Index(start, "\n## Step 6: "); a < 0 || b < 0 || a > b {
		add("Step 5.5 must come before Step 6 (take-task)")
	}
	for _, want := range []string{
		"BEFORE take-task",
		"this is a linked worktree",
		"an unattended turn",
		"no standing team",
		"`Retired`",
		"lead not running",
		`header: "Where"`,
		`label: "Here (Recommended)"`,
		`label: "Run as worker"`,
		"verb=tell target=",
		"footer=none",
		"this checkout already holds a claim on the task",
		"ownership is NOT \"a task-state file names it\"",
		"carries this session's id on its `session:` line",
		"is held elsewhere (item 1 below), not a skip",
		"`session_held`",
		"this session keeps no claim",
		"held elsewhere",
		"hand-off for team <c> - <task-id> - **<title>**.",
		"a lead holding no task claims it itself",
		`Skill(skill: "lets:worktree", args: "create <task-id> --title-file .lets/cache/title-<session6>.txt")`,
	} {
		if !strings.Contains(s55, want) {
			add("Step 5.5 must state " + want)
		}
	}
	if strings.Contains(s55, "for your owner") {
		add("Step 5.5's hand-off text no longer tells the lead's owner to type /lets:start (Amendment 2026-11-13)")
	}
	if n := strings.Count(s55, `{ label: "`); n > 4 {
		add("Step 5.5 offers at most four options")
	}
	step0 := sectionSpan(start, "### Step 0a: Parse")
	if !strings.Contains(step0, "then Step 5.5, then Step 6 (branch) with this task") || !strings.Contains(step0, "Step 5.5, then Step 6 (take-task)") {
		add("Step 0a runs Step 5.5 on the task-id and --continue paths")
	}
	if s5 := sectionSpan(start, "\n## Step 5: "); strings.Contains(s5, "set-status=in_progress") || !strings.Contains(s5, "nothing is claimed before it") {
		add("Step 5 only selects - no claim before Step 5.5")
	}
	pivot := sectionSpan(start, "### Pivot to code (hand-off)")
	if !strings.Contains(pivot, "then Step 5.5 (Where to run)") || !strings.Contains(pivot, "A hand-off or a worker leaves this session in main mode with no claim.") {
		add("main mode's Pivot to code points at Step 5.5 and stays in main mode on a hand-off")
	}
	return bad
}

func TestStartWhereToRun(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "start.md"))
	if err != nil {
		t.Fatal(err)
	}
	start := string(b)
	for _, p := range startWhereProblems(start) {
		t.Error("start.md: " + p)
	}
	for _, m := range []struct{ name, from, to string }{
		{"no linked-worktree skip", "this is a linked worktree", "this is a checkout"},
		{"no claim skip", "this checkout already holds a claim on the task", "this checkout holds the task"},
		{"no pivot pointer", "then Step 5.5 (Where to run)", "then take-task"},
		{"claim in Step 5", "or name an existing one - take-task claims it", "or claim an existing one (tracker `set-status=in_progress`) - take-task claims it"},
	} {
		if len(startWhereProblems(strings.Replace(start, m.from, m.to, 1))) == 0 {
			t.Errorf("mutation %q must fail the check", m.name)
		}
	}
}
