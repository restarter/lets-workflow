package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// delegatedContractProblems returns every way the given plugin files break the
// delegated /lets:execute contract (lets-l6ah9); an empty result means none.
// It reads only its argument, so TestImplementerRun can feed it in-memory
// mutants and prove each guard can fail without mutating a repository file.
func delegatedContractProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	skill := f["skill"]
	if !strings.Contains(skill, "user-invocable: false") {
		add("implementer-run must be internal (user-invocable: false)")
	}
	spawn := sectionSpan(skill, "## Step 2: Spawn")
	correct := sectionSpan(skill, "## Step 3: Correct")
	if spawn == "" || correct == "" {
		add("implementer-run must keep its Step 2: Spawn and Step 3: Correct sections")
	} else {
		call := ""
		if i := strings.Index(spawn, "Agent("); i >= 0 {
			call = spawn[i:]
			if j := strings.Index(call, "\n)"); j >= 0 {
				call = call[:j]
			}
		} else {
			add("Step 2 must contain the Agent call")
		}
		if !strings.Contains(call, `subagent_type="lets:implementer"`) {
			add("the Step 2 Agent call must spawn lets:implementer")
		}
		for _, field := range []string{"team_name=", "mode=", "isolation="} {
			if strings.Contains(call, field) {
				add("the Step 2 Agent call must not pass " + field)
			}
		}
		if strings.Contains(correct, "Agent(") {
			add("Step 3: Correct must never spawn")
		}
		if !strings.Contains(correct, "SendMessage(") {
			add("Step 3: Correct must resume the named agent with SendMessage")
		}
	}
	for _, absent := range []string{"TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList"} {
		if strings.Contains(skill, absent) {
			add("implementer-run must not reference the absent " + absent + " tool")
		}
	}

	exec := f["execute"]
	picker := sectionSpan(exec, "## Step 4.5: Choose Execution Mode")
	if n := strings.Count(picker, `{ label: "`); n != 4 {
		add(fmt.Sprintf("the Step 4.5 picker must offer exactly 4 options, found %d", n))
	}
	if !strings.Contains(picker, `label: "Implementers"`) {
		add("the Step 4.5 picker must offer the Implementers locus")
	}
	if !strings.Contains(picker, "is REFUSED") {
		add("Step 4.5 must refuse --auto together with --implementers / --team")
	}
	if strings.Contains(exec, `Skill(skill: "lets:team"`) {
		add("execute.md must not route into /lets:team's Agent Teams backend (lets-7dwc1)")
	}
	split := sectionSpan(exec, "## Step 4.6: Split the plan (Implementers only)")
	if !strings.Contains(split, "depends on something learned during the run") {
		add("Step 4.6 must refuse a plan in which a task's execution depends on something learned during the run")
	}
	delegated := sectionSpan(exec, "## Step 5-D: Delegated run (Implementers)")
	if delegated == "" {
		add("execute.md must carry Step 5-D")
	} else {
		start := strings.Index(delegated, `header: "Start work"`)
		firstSpawn := strings.Index(delegated, "op=spawn")
		if start < 0 || firstSpawn < 0 || start > firstSpawn {
			add("Step 5-D must ask the Start gate before its first spawn")
		}
		if n := strings.Count(delegated, `header: "Review"`); n != 3 {
			add(fmt.Sprintf("Step 5-D must ask exactly 3 Review gates (one per status), found %d", n))
		}
		for _, need := range []string{`Skill(skill: "lets:implementer-run"`, "TaskStop(", "git ls-files --others --exclude-standard", "--untracked-files=all", "git diff HEAD", "git diff --cached --name-only", `args: "approved=review-accept"`, "patch_sha", "**Render review**", "| `pending` |", "| `review` / `paused` |", "| `committing` |", "AMENDMENT to chunk"} {
			if !strings.Contains(delegated, need) {
				add("Step 5-D must contain " + need)
			}
		}
	}

	agent := f["agent"]
	status := sectionSpan(agent, "## Status")
	for _, s := range []string{"`complete`", "`deviation-stopped`", "`blocked`"} {
		if !strings.Contains(status, s) {
			add("implementer.md ## Status must define " + s)
		}
	}
	if !strings.Contains(status, "A failing Verify is never `complete`") {
		add("implementer.md must forbid complete with a failing Verify")
	}
	for _, old := range []string{"parallel team", "spawned exclusively", "TaskUpdate"} {
		if strings.Contains(agent, old) {
			add("implementer.md must no longer carry the team-only " + strconv.Quote(old))
		}
	}

	stale := []string{
		"after its plan-mode approval",
		"inside it the plan-mode approval is the gate",
		"Execute enters native plan mode",
		"its plan-mode approval is the only code-write approval",
		"native plan mode execution with user approval gates. No subagents",
		"**NEVER edit before the plan-mode approval**",
	}
	for _, key := range []string{"rules", "execute", "plan", "planWorkflow", "claude"} {
		for _, s := range stale {
			if strings.Contains(f[key], s) {
				add(key + " still asserts " + strconv.Quote(s) + " - unscoped, it contradicts the delegated Start gate")
			}
		}
	}
	return p
}

// TestImplementerRun pins the delegated /lets:execute contract on the real
// plugin files, then proves each guard can fail: every mutant breaks one
// invariant in memory and must produce a problem naming it.
func TestImplementerRun(t *testing.T) {
	read := func(parts ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(append([]string{pluginDir(t)}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	claude, err := os.ReadFile(filepath.Join(pluginDir(t), "..", "..", "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"skill":        read("skills", "implementer-run", "SKILL.md"),
		"execute":      read("commands", "execute.md"),
		"agent":        read("agents", "implementer.md"),
		"rules":        read("rules", "lets-rules.md"),
		"plan":         read("commands", "plan.md"),
		"planWorkflow": read("commands", "plan-workflow.md"),
		"claude":       string(claude),
	}
	for _, problem := range delegatedContractProblems(files) {
		t.Error(problem)
	}

	mutants := []struct{ name, key, old, repl, want string }{
		{"picker loses Implementers", "execute", `label: "Implementers"`, `label: "Implementer"`, "Implementers locus"},
		{"correct spawns", "skill", "SendMessage(", "Agent(", "Correct must"},
		{"a Review gate is dropped", "execute", `header: "Review"`, `header: "Reviewed"`, "Review gates"},
		{"unscoped gate sentence returns", "rules", "inside it the gate is plan mode for an inline run", "inside it the plan-mode approval is the gate", "still asserts"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(files[m.key], m.old) {
				t.Fatalf("mutant cannot apply: %q is not in %s - update the mutant with the text it guards", m.old, m.key)
			}
			mutated := make(map[string]string, len(files))
			for k, v := range files {
				mutated[k] = v
			}
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, 1)
			for _, problem := range delegatedContractProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
