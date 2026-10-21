package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// lets-mpjhc: /lets:plan, /lets:opinion and /lets:ask send a phase to the standing
// team's members first. Every registered dispatch section routes through member-run
// op=route before it names its report files; every consumer reads a member's round
// with op=receive; opinion's challenge goes back to the same member by id and never
// routes again; plan's HARD-GATE allows exactly the team-mode bookkeeping, and its
// --fast paths stay orchestrator-only. teamModeCommandProblems reads only its
// argument, so TestTeamModeCommands feeds it in-memory mutants.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

const memberRunRoute = `Skill(skill: "lets:member-run", args: "op=route`

// teamModeDispatches are the dispatch sections that route, keyed by file, with the
// command= value their route call names and the section that consumes their reports.
var teamModeDispatches = []struct{ key, command, dispatch, consumer string }{
	{"plan", "plan", "### Launch Explorers", "### Synthesize Codebase Map"},
	{"plan", "plan", "### Architect Brief", "### Checkpoint: Architecture Review"},
	{"plan", "plan", "### Dispatch Experts", "### Checkpoint: Evaluation Results"},
	{"opinion", "opinion", "## Step 4: Launch Agents in Parallel", "## Step 4: Launch Agents in Parallel"},
	{"ask", "ask", "## Step 4: Launch Agent", "## Step 5: Present Results"},
}

// firstReportName returns the first op=open / op=add in s, -1 when none.
func firstReportName(s string) int {
	first := -1
	for _, op := range []string{"op=open", "op=add"} {
		if i := strings.Index(s, op); i >= 0 && (first < 0 || i < first) {
			first = i
		}
	}
	return first
}

func teamModeCommandProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	for _, d := range teamModeDispatches {
		sec := sectionSpan(f[d.key], d.dispatch)
		route, names := strings.Index(sec, memberRunRoute+" command="+d.command+" "), firstReportName(sec)
		if route < 0 || names < 0 || route > names {
			add(d.key + ".md " + d.dispatch + " must route (" + memberRunRoute + " command=" + d.command + ") before its op=open / op=add")
		}
		if !strings.Contains(sectionSpan(f[d.key], d.consumer), "`op=receive`") {
			add(d.key + ".md " + d.consumer + " must read each member with op=receive")
		}
	}

	// opinion Step 4.6: the same member by id, never a new route; busy -> a one-shot.
	challenge := sectionSpan(f["opinion"], "## Step 4.6: Challenge the Leading Option (Adversarial)")
	if !strings.Contains(challenge, `Skill(skill: "lets:member-run", args: "op=next scope=<c> name=<name> id=<agentId> `) || !strings.Contains(challenge, "report name `{role}-challenge`") {
		add("opinion.md Step 4.6 must send a member's challenge to the same member by op=next ... id=, report name {role}-challenge")
	}
	if !strings.Contains(challenge, "`member_busy` -> that challenge goes to a one-shot of the role, announced `{role}: one-shot - the member is busy`") || !strings.Contains(challenge, "`{role}: one-shot - the member was dismissed`") {
		add("opinion.md Step 4.6 must name its member_busy one-shot and its dismissed-or-gone one-shot")
	}
	if strings.Contains(challenge, memberRunRoute) {
		add("opinion.md Step 4.6 must not call op=route - the challenge reuses Step 4's member while it is reachable")
	}
	if !strings.Contains(challenge, "each member critic with `op=receive`") {
		add("opinion.md Step 4.6 must read each member critic with op=receive")
	}

	// plan.md HARD-GATE: exactly the team-mode bookkeeping, never a repository file.
	gate := lineStarting(f["plan"], "**HARD-GATE:")
	for _, need := range []string{"No files are modified except .lets/plans/", ".lets/reports/", "`.lets/cache/route-*`", "`.lets/cache/routes-*`", "`.lets/cache/nudge-*`", "`.lets/cache/member-*`", "`## 3` rows", "`## 7` lines", "`## 8` GAP lines", "never a repository file"} {
		if !strings.Contains(gate, need) {
			add("plan.md HARD-GATE must name " + need)
		}
	}

	// plan.md --fast stays orchestrator-only.
	for _, h := range []string{"## --fast mode", "### --fast: orchestrator-only exploration", "### --fast: orchestrator-only design", "### --fast / Self-evaluation: orchestrator-only evaluation"} {
		sec := sectionSpan(f["plan"], h)
		if sec == "" {
			add("plan.md lost its " + h + " section")
		} else if strings.Contains(sec, "op=route") {
			add("plan.md " + h + " must stay free of op=route - --fast dispatches nothing")
		}
	}
	return p
}

func teamModeCommandFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"plan":    readPlugin(t, filepath.Join("commands", "plan.md")),
		"opinion": readPlugin(t, filepath.Join("commands", "opinion.md")),
		"ask":     readPlugin(t, filepath.Join("commands", "ask.md")),
	}
}

func TestTeamModeCommands(t *testing.T) {
	for _, problem := range teamModeCommandProblems(teamModeCommandFiles(t)) {
		t.Error(problem)
	}
}

// TestTeamModeCommands_Mutants proves each guard of teamModeCommandProblems can fail.
func TestTeamModeCommands_Mutants(t *testing.T) {
	files := teamModeCommandFiles(t)
	mutants := []struct{ name, key, old, repl, want string }{
		{"opinion Step 4 loses its route", "opinion", memberRunRoute + " command=opinion ", "`Skill(skill: \"lets:member-run\", args: \"op=skip command=opinion ", "Step 4: Launch Agents in Parallel must route"},
		{"plan routes after op=add", "plan", "**Team mode** (`protocol/agents.md` `## Team mode`): `" + memberRunRoute + " command=plan roles=architect", "Name them first with op=add. **Team mode** (`protocol/agents.md` `## Team mode`): `" + memberRunRoute + " command=plan roles=architect", "Architect Brief must route"},
		{"4.6 calls op=route", "opinion", "**Team mode challenge:** ", "**Team mode challenge:** `" + memberRunRoute + " command=opinion roles=x fallback=one-shot\")` first. ", "must not call op=route"},
		{"4.6 respawns instead of op=next", "opinion", `Skill(skill: "lets:member-run", args: "op=next scope=<c> name=<name> id=<agentId> `, `Skill(skill: "lets:member-run", args: "op=spawn scope=<c> name=<name> `, "same member by op=next"},
		{"4.6 loses its busy one-shot", "opinion", "`member_busy` -> that challenge goes to a one-shot of the role, announced `{role}: one-shot - the member is busy`", "`member_busy` -> wait", "member_busy one-shot"},
		{"ask Step 5 drops op=receive", "ask", ", or for a member `op=receive`", "", "Present Results must read each member"},
		{"HARD-GATE drops routes-*", "plan", ", `.lets/cache/routes-*` (member-run's routes copy at a spawn)", "", "`.lets/cache/routes-*`"},
		{"HARD-GATE allows repository files", "plan", " - never a repository file.", ".", "never a repository file"},
		{"--fast exploration routes", "plan", "Then jump to **Checkpoint: Exploration Review**.", "`" + memberRunRoute + " command=plan roles=explorer fallback=inline\")` first.\n\nThen jump to **Checkpoint: Exploration Review**.", "must stay free of op=route"},
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
			for _, problem := range teamModeCommandProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
