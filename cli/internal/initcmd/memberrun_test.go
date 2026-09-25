package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// delegatedContractProblems returns every way the given plugin files break the
// delegated /lets:execute contract (lets-l6ah9); an empty result means none.
// It reads only its argument, so TestMemberRun can feed it in-memory
// mutants and prove each guard can fail without mutating a repository file.
func delegatedContractProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	skill := f["skill"]
	if !strings.Contains(skill, "user-invocable: false") {
		add("member-run must be internal (user-invocable: false)")
	}
	// the predecessor's name, spelled in two parts so this file passes the rename grep gate itself
	predecessor := "implementer" + "-run"
	if strings.Contains(skill, predecessor) {
		add("member-run must never name its predecessor, not even as history (the rename grep gate)")
	}
	check0 := sectionSpan(skill, "## Step 0: Binary check")
	spawn := sectionSpan(skill, "## Step 2: Spawn")
	next := sectionSpan(skill, "## Step 3: Next / Correct")
	if check0 == "" || !strings.Contains(check0, "lets members status --scope") || !strings.Contains(check0, "/lets:update") {
		add("member-run must keep Step 0: a lets members status check that stops with /lets:update")
	} else if strings.Index(skill, "## Step 0: Binary check") > strings.Index(skill, "Agent(") {
		add("the Step 0 binary check must precede the first Agent call")
	}
	if spawn == "" || next == "" {
		add("member-run must keep its Step 2: Spawn and Step 3: Next / Correct sections")
	} else {
		call := ""
		agentAt := strings.Index(spawn, "Agent(")
		if agentAt >= 0 {
			call = spawn[agentAt:]
			// the call ends at its own closing line, indented or not
			for i, line := range strings.Split(call, "\n") {
				if i > 0 && strings.TrimSpace(line) == ")" {
					call = strings.Join(strings.Split(call, "\n")[:i], "\n")
					break
				}
			}
		} else {
			add("Step 2 must contain the Agent call")
		}
		if !strings.Contains(call, `subagent_type="{role}"`) {
			add("the Step 2 Agent call must spawn the role it was given")
		}
		for _, field := range []string{"team_name=", "mode=", "isolation="} {
			if strings.Contains(call, field) {
				add("the Step 2 Agent call must not pass " + field)
			}
		}
		if status := strings.Index(spawn, "lets members status"); status < 0 || agentAt < 0 || status > agentAt {
			add("Step 2 must read lets members status before the Agent call")
		}
		if addAt := strings.Index(spawn, "lets members add"); addAt < 0 || agentAt < 0 || addAt < agentAt {
			add("Step 2 must record the member with lets members add right after the Agent call")
		}
		if !strings.Contains(spawn, "pane") || !strings.Contains(spawn, "in_process") {
			add("Step 2 must report which kind was recorded (pane or in_process)")
		}
		if !strings.Contains(spawn, "no_lead") {
			add("a team-scope spawn must stop on no_lead")
		}
		if !strings.Contains(skill, "`<callsign>-<name>` in a team scope") {
			add("member-run must name team-scope agents <callsign>-<name>")
		}
		if strings.Contains(next, "Agent(") {
			add("Step 3: Next / Correct must never spawn")
		}
		send := strings.Index(next, "SendMessage(")
		if send < 0 {
			add("Step 3: Next / Correct must resume the named member with SendMessage")
		}
		if status := strings.Index(next, "lets members status"); status < 0 || send < 0 || status > send {
			add("Step 3 must run lets members status before every SendMessage")
		}
		if strings.Count(skill, "SendMessage(") != strings.Count(next, "SendMessage(") {
			add("every SendMessage must live in Step 3, behind its lets members status")
		}
	}
	for _, absent := range []string{"TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "team_name", "mode="} {
		if strings.Contains(skill, absent) {
			add("member-run must not reference the absent " + absent)
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
	if !strings.Contains(exec, `preview: "{the Step 4.7 launch plan, then the Step 4.6 split table and its warnings}"`) {
		add("the Start gate must carry the launch plan and the Step 4.6 split as its preview - Start approves the split the user sees")
	}
	if !strings.Contains(split, "**Files audit - a GATE.**") || !strings.Contains(split, "when the Files audit fails") {
		add("Step 4.6 must run the Files audit as a gate that refuses delegation")
	}
	for _, need := range []string{"expand `{a,b}` brace sets", "a bare name (no `/`) inherits the directory of the previous path on the same Files line", "it is a repo-root path", "`Files add:` Amendment lines count as its Files", "Only a FULL path", "resolves ambiguously, is a warning", "never a refusal"} {
		if !strings.Contains(split, need) {
			add("the Files audit must resolve paths before it refuses: " + need)
		}
	}
	if !strings.Contains(split, "**Removed-symbol check - a WARNING only.**") || !strings.Contains(split, "it never refuses delegation") {
		add("Step 4.6 must run the removed-symbol check as a warning only, never a refusal")
	}
	anchor := ""
	if at := strings.Index(split, "**Anchor check against BASE - a WARNING only.**"); at >= 0 {
		anchor = split[at:]
		if end := strings.Index(anchor, "\n\n"); end >= 0 {
			anchor = anchor[:end]
		}
	}
	for _, need := range []string{"`file:line` anchor", "pin counter", "Verify command", "role=lets:explorer", "else this session inline", "listed in the Start preview", "adds no gate and no option"} {
		if !strings.Contains(anchor, need) {
			add("the Step 4.6 anchor check must be a warning against BASE naming " + need)
		}
	}
	if !strings.Contains(split, "`CI CHECKS:`") || !strings.Contains(split, "CI workflow") || !strings.Contains(split, "Makefile") {
		add("Step 4.6 must take the brief's CI CHECKS from the CI workflow and the Makefile")
	}
	if !strings.Contains(split, "missing = high") {
		add("Step 4.6 must read a missing Risk as high")
	}
	riskRule := "**Risk:** high|low"
	if !strings.Contains(f["plan"], "**Risk:** {high|low}") || !strings.Contains(f["plan"], "a missing Risk line is read as `high`") {
		add("plan.md's template and quality gates must carry the Risk field (missing = high)")
	}
	for _, fn := range []string{"function planPrompt(", "function planReviewPrompt("} {
		body := f["planWorkflowJS"]
		if at := strings.Index(body, fn); at >= 0 {
			body = body[at:]
			if end := strings.Index(body[len(fn):], "\nfunction "); end >= 0 {
				body = body[:len(fn)+end]
			}
		} else {
			body = ""
		}
		if !strings.Contains(body, riskRule) || !strings.Contains(body, "missing Risk line") {
			add("plan.workflow.js " + fn + " must carry the Risk field (keep-in-sync with plan.md)")
		}
	}
	if !strings.Contains(f["planWorkflowJS"], "KEEP IN SYNC with plan.md") {
		add("plan.workflow.js must mark the Risk rule KEEP IN SYNC with plan.md")
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
		if n := strings.Count(delegated, `preview: "{the review block}"`); n != 3 {
			add(fmt.Sprintf("each of the 3 Review gates must carry the review block as its first option's preview, found %d", n))
		}
		if !strings.Contains(delegated, "\nCI CHECKS: ") {
			add("the chunk brief template must carry CI CHECKS:")
		}
		for _, need := range []string{`Skill(skill: "lets:member-run"`, "brief-file=", "scope=run-{RUN}", "TaskStop(", "git ls-files --others --exclude-standard", "--untracked-files=all", "git diff HEAD", "git diff --cached --name-only", `args: "approved=review-accept"`, "patch_sha", "**Render review**", "**What is a report.**", "malformed-report", "Write a report with the Write tool", "| `pending` |", "| `review` / `paused` |", "| `committing` |", "AMENDMENT to chunk"} {
			if !strings.Contains(delegated, need) {
				add("Step 5-D must contain " + need)
			}
		}
	}

	if strings.Contains(exec, predecessor) || strings.Contains(exec, "chunk-file=") {
		add("execute.md must call member-run with brief-file= - not its predecessor, no chunk-file=")
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
	if !strings.Contains(agent, "NEVER run a command in the background") {
		add("implementer.md must forbid background commands - an idle subagent waiting on its own background task hangs the run")
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

// TestMemberRun pins the delegated /lets:execute contract on the real
// plugin files, then proves each guard can fail: every mutant breaks one
// invariant in memory and must produce a problem naming it.
func TestMemberRun(t *testing.T) {
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
		"skill":          read("skills", "member-run", "SKILL.md"),
		"execute":        read("commands", "execute.md"),
		"agent":          read("agents", "implementer.md"),
		"rules":          read("rules", "lets-rules.md"),
		"plan":           read("commands", "plan.md"),
		"planWorkflow":   read("commands", "plan-workflow.md"),
		"planWorkflowJS": read("skills", "plan-workflow", "plan.workflow.js"),
		"claude":         string(claude),
	}
	for _, problem := range delegatedContractProblems(files) {
		t.Error(problem)
	}

	mutants := []struct{ name, key, old, repl, want string }{
		{"picker loses Implementers", "execute", `label: "Implementers"`, `label: "Implementer"`, "Implementers locus"},
		{"correct spawns", "skill", "SendMessage(", "Agent(", "Correct must"},
		{"status after the send", "skill", "1. `lets members status --scope {scope} --name {name} --json`, before every message.", "1. Check the member before every message.", "before every SendMessage"},
		{"add before the Agent call", "skill", "   lets members add --scope", "   lets memberz add --scope", "right after the Agent call"},
		{"execute back on the old skill", "execute", `Skill(skill: "lets:member-run", args: "op=spawn`, "Skill(skill: \"lets:implementer" + "-run\", args: \"op=spawn", "not its predecessor"},
		{"a Review gate is dropped", "execute", `header: "Review"`, `header: "Reviewed"`, "Review gates"},
		{"files audit becomes a warning", "execute", "**Files audit - a GATE.**", "**Files audit - a WARNING.**", "Files audit as a gate"},
		{"files audit drops bare-name resolution", "execute", "a bare name (no `/`) inherits", "a bare name (no `/`) is refused", "resolve paths before it refuses"},
		{"unresolvable name refuses", "execute", "resolves ambiguously, is a warning", "resolves ambiguously, refuses delegation", "resolve paths before it refuses"},
		{"removed-symbol check refuses", "execute", "it never refuses delegation", "it refuses delegation", "warning only"},
		{"anchor check adds an option", "execute", "adds no gate and no option", "adds an option", "anchor check"},
		{"brief loses CI CHECKS", "execute", "\nCI CHECKS: ", "\nCHECKS: ", "CI CHECKS:"},
		{"plan.workflow.js drops Risk in review", "planWorkflowJS", "every task with a commit point states **Risk:** high|low", "every task with a commit point states a risk", "planReviewPrompt"},
		{"plan.md drops the Risk template line", "plan", "**Risk:** {high|low}", "**Risk:** {level}", "Risk field"},
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

// fixedAllocation matches a hardcoded implementer count - the launch plan is
// reasoned per plan, never a fixed rule.
var fixedAllocation = regexp.MustCompile(`(?i)\b(always|every run uses|by default)\s+(\d+|one|two|three|a single)\s+implementers?\b`)

// launchPlanProblems returns every way execute.md breaks the Step 4.7 launch
// plan contract (lets-7dwc1); an empty result means none.
func launchPlanProblems(exec string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	const heading = "## Step 4.7: Launch plan (proposal)"
	split := strings.Index(exec, "## Step 4.6: Split the plan (Implementers only)")
	launch := strings.Index(exec, heading)
	start := strings.Index(exec, "### 5-D.2 Start")
	if split < 0 || launch < 0 || start < 0 || split > launch || launch > start {
		add("Step 4.7 must exist between Step 4.6 and the 5-D.2 Start gate")
	}
	lp := sectionSpan(exec, heading)
	for _, choice := range []struct{ name, row string }{
		{"Implementers", "| Implementers | how many, each by its name, and the blocks or chunks each one owns"},
		{"Isolation", "| Isolation | which implementers run isolated"},
		{"Integration order", "| Integration order | the order"},
		{"Pipelined", "| Pipelined | whether"},
		{"Gate policy", "| Gate policy | the default"},
	} {
		if !strings.Contains(lp, choice.row) {
			add("the launch plan must name " + choice.name)
		}
	}
	if !strings.Contains(lp, "each with a one-line reason") || !strings.Contains(lp, "| Why ") {
		add("every launch plan choice must carry a one-line reason")
	}
	for _, policy := range []string{"`per-commit`", "`high-only`", "`at-end`"} {
		if !strings.Contains(lp, policy) {
			add("the launch plan's gate policy must name " + policy)
		}
	}
	for _, need := range []string{"Risk", "Files audit", "file-disjoint", "dependencies"} {
		if !strings.Contains(lp, need) {
			add("the launch plan must reason from " + need)
		}
	}
	if !strings.Contains(lp, "Start option's `preview`") || !strings.Contains(exec, `preview: "{the Step 4.7 launch plan`) {
		add("the launch plan must be the Start option's preview")
	}
	if !strings.Contains(lp, "**Owner overrides.**") {
		add("the launch plan must name the owner overrides")
	}
	for _, flag := range []string{"`--parallel`", "`--pipelined`", "`--gate <policy>`"} {
		if !strings.Contains(lp, flag) {
			add("the launch plan must name the override " + flag)
		}
	}
	if !strings.Contains(lp, "not a fixed rule") || fixedAllocation.MatchString(lp) {
		add("the launch plan must be reasoned per plan - no fixed allocation rule")
	}
	if !strings.Contains(lp, "**Change the launch plan**") || !strings.Contains(lp, "Nothing is spawned before Start.") {
		add("Change the launch plan must recompute the preview and nothing may spawn before Start")
	}
	if !strings.Contains(sectionSpan(exec, "## Step 4.5: Choose Execution Mode"), "Step 4.6, Step 4.7, then Step 5-D") {
		add("the Implementers route must pass through Step 4.7")
	}
	return p
}

// TestExecuteLaunchPlan pins the Step 4.7 launch plan on the real execute.md,
// then proves each guard can fail on an in-memory mutant.
func TestExecuteLaunchPlan(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "execute.md"))
	if err != nil {
		t.Fatal(err)
	}
	exec := string(b)
	for _, problem := range launchPlanProblems(exec) {
		t.Error(problem)
	}

	mutants := []struct{ name, old, repl, want string }{
		{"launch plan moves after Start", "## Step 4.7: Launch plan (proposal)", "## Step 4.8: Launch plan (proposal)", "between Step 4.6"},
		{"integration order dropped", "| Integration order | the order", "| Order | the order", "Integration order"},
		{"reasons dropped", "each with a one-line reason", "each with a value", "one-line reason"},
		{"at-end policy dropped", "or `at-end` (", "or `after` (", "`at-end`"},
		{"fixed allocation creeps in", "This is reasoning about THIS plan, not a fixed rule", "Always two implementers, not a fixed rule", "no fixed allocation"},
		{"pipelined override dropped", "`--pipelined`", "`--pipe`", "`--pipelined`"},
		{"preview loses the launch plan", `preview: "{the Step 4.7 launch plan`, `preview: "{the plan`, "Start option's preview"},
		{"spawn before Start", "Nothing is spawned before Start.", "Spawn when ready.", "nothing may spawn"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(exec, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in execute.md - update the mutant with the text it guards", m.old)
			}
			for _, problem := range launchPlanProblems(strings.Replace(exec, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
