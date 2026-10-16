package initcmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// lets-mpjhc / lets-kwd6e: a standing team's members are addressed by the agent id
// each spawn records in the team file's `## 7` (`Member:` lines), the Members
// question lives in team.md Step M6 and is asked at most once per lead session, M5
// reads a member's round through member-run op=receive, and protocol/agents.md
// `## Team mode` is the caller contract every team-mode dispatch points to.
// teamRouteProblems reads only its argument, so TestTeamRoute feeds it in-memory
// mutants and proves each guard can fail.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

// teamLineForms are the `## 7` line forms templates/team.md defines; member-run and
// team.md must spell them the same way (Reports: is the template's and team.md's only).
var teamLineForms = []string{
	"`Member: <name> = <agentId> lets:<role> (lead <session6>)`",
	"`Member: <name> = gone (lead <session6>)`",
	"`Dismissed: <name> (lead <session6>)`",
	"`Members asked: (lead <session6>)`",
}

const (
	m1Record      = "8. **Record.** Append `Member: <name> = <agentId> lets:<role> (lead <session6>)` to `## 7`"
	m16Address    = "A teammate's address is its agent id - the latest `Member: <name> = <agentId> lets:<role> (lead <session6>)` line in the team file's `## 7`. Re-read `## 7` right before you send"
	m6Unattended  = "the current turn is a `/loop` iteration or a scheduled agent's turn"
	m6NoneOption  = `{ label: "None", description: "Spawn nothing now; asked again in a new lead session" }`
	m6MarkerWrite = "append `Members asked: (lead <session6>)` to `## 7`"
	routeCall     = `Skill(skill: "lets:member-run", args: "op=route`
	oldLiveRule   = "a name is live when `listagents` lists `<c>-<name>`"
)

// lineStarting returns the first line of s that starts with prefix ("" when none).
func lineStarting(s, prefix string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func teamRouteProblems(f map[string]string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }
	team := f["team"]

	// M1: the spawn is recorded by id, after the call.
	m1 := sectionSpan(team, "### Step M1: One Member (`spawn <role> [name]`)")
	spawn, record := strings.Index(m1, `Skill(skill: "lets:member-run", args: "op=spawn `), strings.Index(m1, m1Record)
	if spawn < 0 || record < spawn || !strings.Contains(m1, "the agentId from member-run's return") {
		add("team.md M1 must record `Member: <name> = <agentId> lets:<role> (lead <session6>)` in ## 7 after the spawn, with the id member-run returned")
	}
	if !strings.Contains(between(m1, "6. **Brief.**", "7. **Spawn.**"), m16Address) {
		add("team.md M1.6 brief must name the latest Member: line in ## 7 as a teammate's address, re-read before a send")
	}

	// M5: op=receive only.
	m5 := sectionSpan(team, "### Step M5: Member reports")
	if !strings.Contains(m5, `Skill(skill: "lets:member-run", args: "op=receive `) || strings.Contains(m5, "op=peek") || strings.Contains(m5, "op=collect") {
		add("team.md M5 must read a member's round through member-run op=receive, with no op=peek / op=collect of its own")
	}

	// M6: the Members question, at most once per lead session.
	m6 := sectionSpan(team, "### Step M6: Members question (`members`)")
	asked, unattended, ask := lineStarting(m6, "1. `## 7` holds a `Members asked: (lead <session6>)` line"), lineStarting(m6, "2. Unattended turn"), lineStarting(m6, "3. Ask.")
	if asked == "" || !strings.Contains(asked, "ask nothing") || !strings.Contains(asked, "`MEMBERS skipped asked`") || strings.Index(m6, asked) > strings.Index(m6, "AskUserQuestion(") {
		add("team.md M6 must check the Members asked: marker BEFORE asking, and answer MEMBERS skipped asked")
	}
	if !strings.Contains(unattended, "write NO marker") || strings.Contains(unattended, m6MarkerWrite) || !strings.Contains(unattended, "`MEMBERS skipped unattended`") || strings.Count(m6, m6MarkerWrite) != 1 || !strings.Contains(ask, "After the answer - None included - "+m6MarkerWrite) {
		add("team.md M6 must write the Members asked: marker only after an answer - never on the unattended path")
	}
	if !strings.Contains(ask, "`MEMBERS spawned <roles>`") || !strings.Contains(ask, "`MEMBERS none`") {
		add("team.md M6 must return MEMBERS spawned <roles> | none | skipped - the answers member-run op=route reads")
	}
	if strings.Contains(m6, routeCall) || !strings.Contains(m6, "It never calls `op=route`") {
		add("team.md M6 never calls member-run op=route - the direction is route -> M6 only")
	}
	if !strings.Contains(unattended, m6Unattended) || !strings.Contains(m6, m6NoneOption) || strings.Contains(m6, "asked again at the next start") {
		add("team.md M6: the unattended rule names the current turn, and None is asked again in a new lead session, not at the next start")
	}

	// The line forms: defined once in the template, spelled the same by member-run and team.md.
	tmpl7 := sectionSpan(f["tmpl"], "\n## 7. Current task\n")
	for _, form := range append(append([]string{}, teamLineForms...), "`Reports: <dir> (lead <session6>)`") {
		if !strings.Contains(tmpl7, form) {
			add("templates/team.md ## 7 must define the line form " + form)
		}
	}
	for _, form := range teamLineForms {
		for _, key := range []string{"member-run", "team"} {
			if !strings.Contains(f[key], form) {
				add(key + " must use the team-file line form " + form + " exactly as templates/team.md defines it")
			}
		}
	}

	// The old liveness rule is gone everywhere.
	for _, key := range []string{"team", "start", "take"} {
		norm := strings.ToLower(strings.ReplaceAll(f[key], "**", ""))
		if strings.Contains(norm, oldLiveRule) || strings.Contains(f[key], "`ListAgents` lists `<c>-<name>`") {
			add(key + ": the old rule \"a name is live when `ListAgents` lists `<c>-<name>`\" must be gone - a member is reachable by its recorded id")
		}
	}

	// protocol/agents.md ## Team mode: the caller contract.
	mode := sectionSpan(f["agents"], "\n## Team mode\n")
	item := func(n string) string { return lineStarting(mode, n+". ") }
	if i1 := item("1"); !strings.HasPrefix(i1, "1. Before a phase's `op=open` / `op=add`: `"+routeCall) {
		add("agents.md ## Team mode item 1 must route before the phase's op=open / op=add")
	}
	if i2 := item("2"); !strings.Contains(i2, "the first member line of a role `{role}`, the next `{role}-2`") || !strings.Contains(i2, "Two members of one role never share a report file") {
		add("agents.md ## Team mode item 2 must give two members of one role distinct report names ({role}, {role}-2)")
	}
	i4 := item("4")
	if !strings.HasPrefix(i4, "4. Before ANY send to a member id this run already holds - a later phase, opinion's challenge - re-read `## 7`: a `Dismissed:` or `= gone` line for it now -> its items go to the fallback") {
		add("agents.md ## Team mode item 4 must re-read ## 7 before any send to an id the run already holds (opinion's challenge included) and drop a dismissed or gone member to the fallback")
	}
	if send, before := strings.Index(i4, `args: "op=next scope=<c> name=<name> id=<agentId> `), strings.Index(i4, "- BEFORE the phase's fallback Task calls, then make the Task calls as today, in one message."); send < 0 || before < send {
		add("agents.md ## Team mode item 4 must send every member brief BEFORE the phase's fallback Task calls")
	}
	if !strings.Contains(item("5"), "before EVERY `op=receive` re-read the team file's `## 7` and pass `dismissed`") {
		add("agents.md ## Team mode item 5 must re-read ## 7 and pass dismissed before every op=receive")
	}
	if !strings.Contains(item("7"), "`--workflow` -> no route; in a team worktree print once `team worktree - --workflow runs its own agents, not the team's`") {
		add("agents.md ## Team mode must keep the --workflow line")
	}
	return p
}

func teamRouteFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"team":       readPlugin(t, filepath.Join("commands", "team.md")),
		"start":      readPlugin(t, filepath.Join("commands", "start.md")),
		"take":       readPlugin(t, filepath.Join("skills", "take-task", "SKILL.md")),
		"tmpl":       readPlugin(t, filepath.Join("templates", "team.md")),
		"member-run": readPlugin(t, filepath.Join("skills", "member-run", "SKILL.md")),
		"agents":     readPlugin(t, filepath.Join("protocol", "agents.md")),
	}
}

func TestTeamRoute(t *testing.T) {
	for _, problem := range teamRouteProblems(teamRouteFiles(t)) {
		t.Error(problem)
	}
}

// TestTeamRoute_Mutants proves each guard of teamRouteProblems can fail.
func TestTeamRoute_Mutants(t *testing.T) {
	files := teamRouteFiles(t)
	mutants := []struct{ name, key, old, repl, want string }{
		{"M1 drops the Record item", "team", m1Record + ", the agentId from member-run's return and `<role>` the spawned role.\n9. ", "8. ", "record `Member:"},
		{"M6 calls op=route", "team", "It never calls `op=route`.", "It never calls `op=route`. " + routeCall + ` command=team roles=x fallback=inline")`, "never calls member-run op=route"},
		{"M6 writes the marker on the unattended path", "team", "spawn nothing, write NO marker, and print one line", "spawn nothing, " + m6MarkerWrite + ", and print one line", "only after an answer"},
		{"Members asked: renamed in member-run only", "member-run", "`Members asked: (lead <session6>)`", "`Asked members: (lead <session6>)`", "Members asked:"},
		{"M5 restores its own peek", "team", "On EVERY notification from a member - a message, an idle notification, a completion, empty ones included: ", "On EVERY notification from a member - a message, an idle notification, a completion, empty ones included: `Skill(skill: \"lets:agent-report\", args: \"op=peek dir=<Reports dir> names=<its current round file>\")`, then ", "no op=peek / op=collect of its own"},
		{"M1.6 loses the address sentence", "team", m16Address, "Re-read `## 7` right before you send", "teammate's address"},
		{"None back to the next start", "team", "Spawn nothing now; asked again in a new lead session", "Spawn nothing now; asked again at the next start", "new lead session"},
		{"unattended loses the current turn", "team", m6Unattended, "this /lets:start runs as a `/loop` iteration or a scheduled agent's turn", "current turn"},
		{"item 2 allocates {role} twice", "agents", "the next `{role}-2`, `{role}-3`, ...,", "the next `{role}` again,", "distinct report names"},
		{"item 4 drops the re-read", "agents", " - re-read `## 7`: a `Dismissed:` or `= gone` line for it now -> its items go to the fallback, announced.", ".", "item 4 must re-read"},
		{"item 4 sends after the Task calls", "agents", "- BEFORE the phase's fallback Task calls, then make the Task calls as today, in one message.", "- after the phase's fallback Task calls, made as today in one message.", "BEFORE the phase's fallback Task calls"},
		{"item 5 drops the re-read", "agents", "before EVERY `op=receive` re-read the team file's `## 7` and pass `dismissed`", "pass `dismissed`", "before every op=receive"},
		{"item 1 routes after op=open", "agents", "1. Before a phase's `op=open` / `op=add`: ", "1. After a phase's `op=open` / `op=add`: ", "route before"},
		{"the --workflow line dropped", "agents", "7. `--workflow` -> no route;", "7. `--workflow` -> routes too;", "--workflow line"},
		{"the old live rule back in M0", "team", "A name is **reachable** when", "A name is **live** when `ListAgents` lists `<c>-<name>`. A name is **reachable** when", "old rule"},
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
			for _, problem := range teamRouteProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
