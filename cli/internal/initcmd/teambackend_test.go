package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTeamBackend pins /lets:team on the live harness: one visible session per task on
// any launcher, the Orca addon kept apart, no Agent Teams primitive left, the refusals
// reachable, every launch named and bound to a registered orchestrator, worker branches
// cut from origin, and legacy run records read as stale.
func TestTeamBackend(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	team := string(b)
	lines := strings.Split(team, "\n")

	// the Orca addon: no subagent or peer-send calls, the guide instead of pinned flags
	orca := sectionSpan(team, "\n## Run (backend orca)\n")
	if orca == "" {
		t.Fatal("## Run (backend orca) section missing")
	}
	for _, call := range []string{"TeamCreate(", "Agent(", "SendMessage("} {
		if strings.Contains(orca, call) {
			t.Errorf("the orca run section must not contain %s", call)
		}
	}
	if !strings.Contains(orca, "orca skills get orchestration") {
		t.Error("the orca run section must read the orchestration guide")
	}

	// the Agent Teams primitives are gone from the whole command
	for _, gone := range []string{"TeamCreate(", "TaskCreate(", "TaskUpdate(", "TaskList(", "TeamDelete(", "team_name", `mode="plan"`, "shutdown_request", "~/.claude/teams"} {
		if strings.Contains(team, gone) {
			t.Errorf("team.md must not contain %s", gone)
		}
	}

	// --backend agents, and --backend orca without a running Orca, reach a refusal
	refusal := func(flag, cond string) {
		t.Helper()
		for _, l := range lines {
			if strings.Contains(l, "`"+flag+"`") && strings.Contains(l, cond) && strings.Contains(l, "**Refused:**") && strings.Contains(l, "Stop") {
				return
			}
		}
		t.Errorf("no refusal line for %s (%s)", flag, cond)
	}
	refusal("--backend agents", "TeamCreate / TaskCreate")
	refusal("--backend orca", "running")
	if !strings.Contains(team, "no fallback") || !strings.Contains(team, "Never a fallback") {
		t.Error("neither refusal may fall back to another backend or launcher")
	}

	// the sessions backend exists and its launch steps are named and bound
	sessions := sectionSpan(team, "\n## Run (sessions)\n")
	if sessions == "" {
		t.Fatal("## Run (sessions) section missing")
	}
	const name, orc = "{agent_command} --name '<worker_name>'", `--orc="<lead>"`
	launches := map[string]int{}
	for _, l := range lines {
		if strings.Contains(l, "'/lets:start <") {
			if !strings.Contains(l, name) || !strings.Contains(l, orc) {
				t.Errorf("a launch must carry %s and %s: %s", name, orc, strings.TrimSpace(l))
			}
			for _, sec := range []struct{ key, span string }{{"sessions", sessions}, {"orca", orca}} {
				if strings.Contains(sec.span, l) {
					launches[sec.key]++
				}
			}
		}
		if strings.Contains(l, "--command") && !strings.Contains(l, "--name") {
			t.Errorf("every --command carries --name: %s", strings.TrimSpace(l))
		}
		// a route to /lets:worktree create --team is not a create; a bare create on the same line still is
		if bare := strings.ReplaceAll(l, "/lets:worktree create --team", ""); strings.Contains(bare, "worktree create") && !strings.Contains(bare, `--base "origin/`) {
			t.Errorf("a worker worktree is cut from origin/<merge> only: %s", strings.TrimSpace(l))
		}
	}
	if launches["sessions"] < 2 || launches["orca"] < 1 {
		t.Errorf("launch commands: %v - want the cmux/tmux command and the terminal line in sessions, one in orca", launches)
	}
	for _, want := range []string{`open "<path>" --name "<worker_name>" --command "$CMD"`, `cd "<path>" && ` + name} {
		if !strings.Contains(sessions, want) {
			t.Errorf("the sessions run must launch with %q", want)
		}
	}
	if r45 := sectionSpan(team, "### Step R4.5: Launch Mode"); !strings.Contains(r45, "`{agent_command}` = `team_launch.agent_command` (the `agent_command` frontmatter value of the team file") || !strings.Contains(r45, "**Else** `claude`") ||
		!strings.Contains(sessions, "`{agent_command}`, its class and its source are Step R4.5's") {
		t.Error("the agent command comes from the team file's agent_command, else claude - resolved in R4.5 and used by N2")
	}
	if !strings.Contains(orca, "`{agent_command}` as defined in Step R4.5") {
		t.Error("the orca worker start uses the same agent command")
	}
	if !strings.Contains(sessions, "lets worktree branch-name") || !strings.Contains(sessions, "`dir`") {
		t.Error("the worktree dir comes from lets worktree branch-name's dir field")
	}
	if !strings.Contains(sessions, "worktree_path_exists") {
		t.Error("the run step must name create's worktree_path_exists backstop")
	}

	// the orchestrator is recorded (its own name) before the first launch, and re-checked per launch
	register := strings.Index(team, "lets worktree task-state set --self-orchestrator")
	first := strings.Index(team, `open "<path>"`)
	if t2 := strings.Index(team, `cd "<path>" &&`); t2 >= 0 && (first < 0 || t2 < first) {
		first = t2
	}
	if t3 := strings.Index(team, "--command"); t3 >= 0 && (first < 0 || t3 < first) {
		first = t3
	}
	if register < 0 || first < 0 || register > first {
		t.Errorf("orchestrator record (%d) must precede the first launch (%d)", register, first)
	}
	recheck := strings.Index(sessions, "`self_record.name` = `<lead>`")
	if recheck < 0 || recheck > strings.Index(sessions, `open "<path>"`) {
		t.Error("every launch re-asserts self_record.name = <lead> before it opens a session")
	}

	// the conflict guard reads the records by backend; legacy ones are stale and never block
	guard := strings.Index(team, ".lets/execution/team-*.json` whose `status`")
	if guard < 0 || !strings.Contains(team[guard:guard+300], "backend") {
		t.Fatal("the conflict guard (team records + backend) is missing")
	}
	if i := strings.Index(team, "worker-start"); i >= 0 && i < guard {
		t.Error("the conflict guard must come before any worker-start mention")
	}
	var orcaRow, legacyRow string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "| `orca`"):
			orcaRow = l
		case strings.HasPrefix(l, "| absent, or `agent-teams`"):
			legacyRow = l
		}
	}
	if !strings.Contains(orcaRow, "`backend: sessions` + `launcher: orca`") {
		t.Errorf("a backend: orca record reads as sessions + launcher orca: %q", orcaRow)
	}
	if !strings.Contains(legacyRow, "**stale**") {
		t.Errorf("a record with no backend or agent-teams is stale: %q", legacyRow)
	}
	if !strings.Contains(team, "A stale record never blocks") || !strings.Contains(team, `label: "Mark stopped (Recommended)"`) {
		t.Error("a stale record is listed with an owner-gated mark-stopped offer and never blocks")
	}

	// the record names its backend and launcher
	if !strings.Contains(team, `"backend": "sessions"`) || !strings.Contains(team, `"launcher":`) {
		t.Error("the run record example must carry backend sessions and the launcher")
	}
}

// TestTeamBackend_Members pins the standing-team verbs: spawn and dismiss only through
// member-run, the same-name live refusal, --roster recommending respawn, dismiss from
// the lead's own session, and /lets:start claiming the lead before take-task.
func TestTeamBackend_Members(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	team := string(b)
	for _, route := range []string{"- `spawn <role> [name]` / `spawn --roster` -> go to Spawn", "- `dismiss <name>` / `dismiss --all` -> go to Dismiss", "- `roster` -> go to Roster"} {
		if !strings.Contains(team, route) {
			t.Errorf("Step 1 must route %q", route)
		}
	}
	if strings.Contains(team, "Agent(") {
		t.Error("team.md never calls the Agent tool - members go through member-run")
	}
	for _, l := range strings.Split(team, "\n") {
		if strings.Contains(l, "--name '<worker_name>'") && strings.Contains(l, "--agent") {
			t.Errorf("/lets:team run launches workers with the bare agent_command - never with --agent (lets-1r00e): %s", strings.TrimSpace(l))
		}
	}
	if m16 := between(team, "6. **Brief.**", "7. **Spawn.**"); !strings.Contains(m16, "Your FIRST message to the lead, before any work, announces you: `STATUS: <c>-<name> online - <role> (lets:<role>)`") || !strings.Contains(m16, "`RESULT: the routes you answer and the task you read in section 7 (or no current task)`") {
		t.Error("the M1.6 spawn brief makes every member announce its name, role and routes first (lets-1w0tz)")
	}
	const spawnCall = `Skill(skill: "lets:member-run", args: "op=spawn `
	if n, calls := strings.Count(team, "op=spawn"), strings.Count(team, spawnCall); calls == 0 || n != calls {
		t.Errorf("every spawn goes through %s (op=spawn %d, calls %d)", spawnCall, n, calls)
	}
	dismiss := sectionSpan(team, "\n## Dismiss\n")
	if !strings.Contains(dismiss, `Skill(skill: "lets:member-run", args: "op=dismiss `) {
		t.Error("dismiss goes through member-run op=dismiss")
	}
	if !strings.Contains(dismiss, "`team_lead.match`") || !strings.Contains(dismiss, "/rename") || strings.Contains(dismiss, "**Refused:**") {
		t.Error("dismiss warns with the team_lead /rename line, never refuses")
	}
	if rec, call := strings.Index(dismiss, "BEFORE every `op=dismiss`, append `Dismissed: <name> (lead <session6>)`"), strings.Index(dismiss, `args: "op=dismiss `); rec < 0 || call < rec {
		t.Error("dismiss records `Dismissed: <name>` in the team file before op=dismiss")
	}
	members := sectionSpan(team, "\n## Members\n")
	for _, want := range []string{"lets worktree info --json", "No `team`", "`<c>-<name>`", "`cwd` is the team worktree", "`ListAgents` lists `<c>-<name>`", "`team_lead`", "/rename it if this is the lead", "never refused"} {
		if !strings.Contains(members, want) {
			t.Errorf("Step M0 must state %q", want)
		}
	}
	m0 := sectionSpan(team, "### Step M0: Team")
	if strings.Contains(m0, "lets members") || strings.Contains(m0, "**Refused:**") {
		t.Error("Step M0 reads ListAgents and team_lead - no lets members, and an unknown or other lead is never refused")
	}
	spawn := sectionSpan(team, "\n## Spawn\n")
	refuse := strings.Index(spawn, "**Same name live -> refused.**")
	call := strings.Index(spawn, spawnCall)
	if refuse < 0 || call < 0 || refuse > call {
		t.Error("the same-name live refusal comes before the spawn call")
	}
	for _, want := range []string{`label: "Close it first (Recommended)"`, "the survivor is recorded dismissed", "the survivor keeps running - close it by hand", ".lets/cache/member-<c>-<name>.md", "on the lead's OK", "**Fresh name** -> append `Dismissed: <name> (lead <session6>)`", "This gate is the only guard: member-run does not refuse a duplicate name."} {
		if !strings.Contains(spawn, want) {
			t.Errorf("spawn must carry %q", want)
		}
	}
	respawn := strings.Index(spawn, `label: "Respawn all (Recommended)"`)
	reuse := strings.Index(spawn, `label: "Re-use panes (keeps old context)"`)
	if respawn < 0 || reuse < 0 || respawn > reuse {
		t.Error("spawn --roster recommends respawn first; re-use is labelled keeps old context")
	}
	if !strings.Contains(spawn, "exactly one live session carries that name") {
		t.Error("re-use is offered only for a unique surviving pane")
	}
	if stop := sectionSpan(team, "\n## Stop\n"); !strings.Contains(stop, "`/lets:team dismiss --all`") {
		t.Error("stop with no run points to dismiss --all")
	}
	if status := sectionSpan(team, "\n## Status\n"); !strings.Contains(status, "Step M3") {
		t.Error("status shows the roster")
	}

	s, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "start.md"))
	if err != nil {
		t.Fatal(err)
	}
	step6 := sectionSpan(string(s), "\n## Step 6: Take Task\n")
	ask := strings.Index(step6, "ask Step 5's **Members** question")
	take := strings.Index(step6, `Skill(skill: "lets:take-task"`)
	if ask < 0 || take < 0 || ask > take || strings.Contains(step6, "lets members") || strings.Contains(step6, "lead_held") {
		t.Errorf("start.md Step 6 claims no lead - the Members question stays, then take-task (ask=%d take=%d)", ask, take)
	}
	if step7 := sectionSpan(string(s), "\n## Step 7: Suggest Session Rename\n"); !strings.Contains(step7, "never suggest `/rename {slug}`") || !strings.Contains(step7, "Suggest `/rename {team_lead.name}` only when `team_lead.match` is not `lead`; otherwise suggest nothing.") {
		t.Error("start.md Step 7 must not rename a team's lead away from lead_name")
	}
	step5 := sectionSpan(string(s), "\n## Step 5: ")
	for _, want := range []string{"`team_lead`", "/rename {team_lead.name}", "never blocking", "**Members (team worktree only).**"} {
		if !strings.Contains(step5, want) {
			t.Errorf("start.md Step 5's team paragraph must state %q", want)
		}
	}
}

// TestTeamBackend_WorktreeTeam pins `/lets:worktree create --team` (lets-0rgnd): Go
// creates the team worktree for every launcher, then team-init, then the gated setup
// hook, then only the lead is launched - Orca through `lets orca terminal`, never an
// Orca worktree create - and the relaunch is gated.
func TestTeamBackend_WorktreeTeam(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "worktree.md"))
	if err != nil {
		t.Fatal(err)
	}
	wt := string(b)
	lines := strings.Split(wt, "\n")
	if len(lines) < 17 || !strings.Contains(lines[2], "create --team [<callsign>] --area <a>") {
		t.Error("the argument-hint (:3) must offer create --team [<callsign>] --area <a>")
	}
	strip := ""
	for _, l := range lines[:40] {
		if strings.HasPrefix(l, "- `create --team [<callsign>] --area <a>`") {
			strip = l
		}
	}
	if strip == "" || !strings.Contains(strip, "together with `--flow`") || !strings.Contains(strip, "from inside a worktree") || !strings.Contains(strip, "**Refused**") ||
		!strings.Contains(strip, "the launch-mode flags (`--launch-mode <m>` / `--auto` / `--all-in` - the lead's **Launch mode** below)") {
		t.Errorf("Step 1 must route create --team, take the launch-mode flags as the lead's launch mode, and refuse it with --flow and inside a worktree: %q", strip)
	}

	team := sectionSpan(wt, "\n## Create a team\n")
	if team == "" {
		t.Fatal("## Create a team section missing")
	}
	for _, banned := range []string{"orca worktree create", "lets orca create", "lets orca open", "orca worktree rm"} {
		if strings.Contains(team, banned) {
			t.Errorf("the team flow must not use %q", banned)
		}
	}
	check := strings.Index(team, "lets worktree team-init --check --callsign")
	create := strings.Index(team, `lets worktree create "team_<c>" --branch "team_<c>" --new-branch --base "origin/`)
	if check < 0 || create < check || !strings.Contains(team, "a given one and a suggested one alike; it writes nothing") {
		t.Error("the callsign is checked (writing nothing) before the team worktree is created")
	}
	if !strings.Contains(team, "`/lets:worktree remove team_<c>`") {
		t.Error("a late 27 / 34 must name the worktree cleanup")
	}
	init := strings.Index(team, "lets worktree team-init --callsign")
	hook := strings.Index(team, "The hook runs only after that yes")
	failStop := strings.Index(team, "**A hook that fails stops here, before the lead is launched:**")
	launch := strings.Index(team, "### Step T5: Launch the lead")
	orcaTerm := strings.Index(team, "lets orca terminal --worktree")
	if create < 0 || init < create || hook < init || failStop < hook || launch < failStop || orcaTerm < launch {
		t.Errorf("order create (%d) -> team-init (%d) -> gated hook (%d) -> fail stop (%d) -> launch (%d, orca %d)", create, init, hook, failStop, launch, orcaTerm)
	}
	if !strings.Contains(team, "Orca creates the worktree (so Orca registers it)") || strings.Contains(team, "the worktree is the one T2 created") {
		t.Error("on orca the worktree comes from Orca (T2-orca); the orca bullet no longer says the one T2 created")
	}
	// T2-orca: team-create -> adopt -> team-init (T3) -> hook (T4) -> lets orca terminal (T5)
	o := between(team, "### Step T2-orca: Create the worktree through Orca", "### Step T2: Create the worktree (Go)")
	tc := strings.Index(team, `lets orca team-create --name "team_<c>" --base-branch "origin/`)
	adopt := strings.Index(team, `lets worktree adopt --dir "<team.path>"`)
	if o == "" || tc < 0 || adopt < tc || init < adopt || hook < init || orcaTerm < hook {
		t.Errorf("T2-orca order team-create (%d) -> adopt (%d) -> team-init (%d) -> hook (%d) -> orca terminal (%d)", tc, adopt, init, hook, orcaTerm)
	}
	if !strings.Contains(o, "NEVER Step T2 after this - a create was attempted") || !strings.Contains(o, "`not_attempted`") {
		t.Error("an ambiguous or failed Orca create never falls to the Go create; only not_attempted does")
	}
	// the non-orca T2 block is byte-identical to 8b36520
	if got := between(team, "### Step T2: Create the worktree (Go)\n", "### Step T3: Team file"); strings.TrimPrefix(got, "### Step T2: Create the worktree (Go)\n") != goldenT2 {
		t.Errorf("the Go T2 block changed:\n%s", got)
	}
	if !strings.Contains(team, "`launched=false`") || !strings.Contains(team, "`fallback_command`") || !strings.Contains(team, "never a worktree create") {
		t.Error("an Orca refusal falls to the printed command, never an Orca create")
	}
	// the team file's agent command crosses through a quoted heredoc, never a double-quoted argument
	teamLines := strings.Split(team, "\n")
	for i, l := range teamLines {
		if strings.Contains(l, "--command") && !strings.Contains(l, `--command "$CMD"`) {
			t.Errorf("every team --command passes \"$CMD\" from a quoted heredoc: %s", strings.TrimSpace(l))
		}
		if strings.Contains(l, "CMD=$(cat <<") {
			if !strings.Contains(l, "<<'EOF'") || i+1 >= len(teamLines) || !strings.Contains(teamLines[i+1], "--name '<lead_name>' '/lets:start'") {
				t.Errorf("a CMD heredoc must be quoted and carry --name '<lead_name>': %s", strings.TrimSpace(l))
			}
		}
	}
	if n := strings.Count(team, `--command "$CMD"`); n < 2 {
		t.Errorf("cmux/tmux and orca both launch through the heredoc, found %d", n)
	}
	// lets-eolec: the lead starts as a plain session - no --agent, so Claude Code's
	// default system prompt and native memory stay; agent_command stays role-neutral
	// because /lets:team run launches workers with it.
	for _, form := range []string{
		"  {agent_command} --name '<lead_name>' '/lets:start'",
		"`cd \"<path>\" && {agent_command} --name '<lead_name>' '/lets:start'`",
		"  {orca_agent} --name '<lead_name>' '/lets:start'",
	} {
		if !strings.Contains(team, form) {
			t.Errorf("the lead launches as a plain session - missing %q", form)
		}
	}
	const leadTail = " --name '<lead_name>' '/lets:start'"
	if all, plain := strings.Count(team, leadTail), strings.Count(team, "{agent_command}"+leadTail)+strings.Count(team, "{orca_agent}"+leadTail); all != plain {
		t.Errorf("no lead launch form carries --agent - each is the bare {agent_command} / {orca_agent} then --name (%d of %d)", plain, all)
	}
	if !strings.Contains(team, "ends in `/team_<c>`") || !strings.Contains(team, "branch --show-current` prints `team_<c>`") {
		t.Error("the created path must be asserted to end in /team_<c>")
	}
	if !strings.Contains(team, `label: "Show the rest"`) || !strings.Contains(team, "40 lines or fewer") {
		t.Error("the hook gate shows the whole hook, or a way to see the rest before Run")
	}
	reopen := strings.Index(team, "### Reopen a team")
	if reopen < 0 || !strings.Contains(team[reopen:], "through the same quoted heredoc - only after the user's yes") || !strings.Contains(team[reopen:], `header: "Reopen"`) {
		t.Error("the relaunch shows the command verbatim and runs only after the user's yes")
	}

	// T5: "this session is an orchestrator" is a non-null self_record, never source=self
	// (source answers whom this session's messages go to - seams b)
	if p := t5OrcProblems(team); len(p) > 0 {
		t.Errorf("Step T5: %v", p)
	}
	for _, m := range []struct{ old, repl string }{
		{`'/lets:start --orc="<self_record.name>"'`, `'/lets:start --orc="<target.name>"'`},
		{"carries a non-null `self_record`", "answers `source=self`"},
	} {
		if !strings.Contains(team, m.old) {
			t.Fatalf("mutant cannot apply: %q", m.old)
		}
		if len(t5OrcProblems(strings.Replace(team, m.old, m.repl, 1))) == 0 {
			t.Errorf("mutation %q -> %q must fail the T5 check", m.old, m.repl)
		}
	}

	c1 := sectionSpan(wt, "\n### Step C1: Get Name\n")
	if !strings.Contains(c1, "the `dir` field as the dir `<name>`") || strings.Contains(c1, "from `slug` as the dir") {
		t.Error("C1 takes the dir name from branch-name's dir field, never hand-built from slug")
	}
}

// TestTeamBackend_TeamTaskFlow pins take-task / start / done in a team worktree
// (lets-0rgnd): take-task parks and switches through `lets worktree switch` and
// never stashes, start offers the roster respawn only in a team worktree, and done
// warns about park commits and asks - never refuses.
func TestTeamBackend_TeamTaskFlow(t *testing.T) {
	read := func(parts ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(append([]string{pluginDir(t)}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	take := read("skills", "take-task", "SKILL.md")
	route := strings.Index(take, "Skip the generic question below and go to **Step 2T**")
	stash := strings.Index(take, `label: "Stash"`)
	if route < 0 || stash < 0 || route > stash {
		t.Error("a team worktree must be routed to Step 2T before the generic Stash question")
	}
	step2t := sectionSpan(take, "### Step 2T: Team worktree - park, then switch")
	if step2t == "" {
		t.Fatal("take-task Step 2T missing")
	}
	if strings.Contains(step2t, "Stash") || strings.Contains(step2t, "git stash") {
		t.Error("the team path must never offer or run a stash")
	}
	for _, want := range []string{`label: "Commit first"`, `label: "Park"`, `label: "Stay"`, "lets worktree switch --task", "--include '<path>' per confirmed new path", "names only the paths the user confirmed", "`## 7. Current task`", "so Step 5's own session write is skipped on this path", "operation_in_progress", "Ignored files are never parked"} {
		if !strings.Contains(step2t, want) {
			t.Errorf("take-task Step 2T must carry %q", want)
		}
	}
	sw := strings.Index(step2t, "lets worktree switch --task")
	tail := strings.Index(step2t, "lets worktree task-state set --clear-origin {ORC_FLAG} --json")
	if sw < 0 || tail < sw {
		t.Error("the `task-state set --clear-origin` call stays after the switch")
	}
	if strings.Contains(step2t, "peers role set") {
		t.Error("Step 2T records no role and no /clear proof - the proof is task-state session_proof")
	}
	if strings.Contains(step2t, "--session-sha") || !strings.Contains(step2t, "so Step 5's own session write is skipped on this path") {
		t.Error("Step 2T never writes session: - switch wrote it through the session guard")
	}
	if strings.Count(step2t, "lets worktree task-state set") != 1 || strings.Contains(step2t, "task-state set --task") {
		t.Error("the team path's task-state write passes only what switch did not write")
	}
	// the Worker-name /rename nudge never fires in a team worktree: the lead keeps lead_name (seams c)
	if p := workerNameProblems(take); len(p) > 0 {
		t.Errorf("take-task: %v", p)
	}
	if len(workerNameProblems(strings.Replace(take, "not trunk-mode, not a team worktree)", "not trunk-mode)", 1))) == 0 {
		t.Error("mutation: a Worker-name nudge without the team-worktree exemption must fail the check")
	}
	if !strings.Contains(sectionSpan(take, "### Step 3: Worktree Check"), "never reaches here - its branch comes from `lets worktree switch`") {
		t.Error("take-task Step 3 must not use a team worktree's branch as-is")
	}

	start := read("commands", "start.md")
	// a lead launched with '/lets:start --orc="<name>"' (no task id) is bound on team_<c> at once (seams d)
	if p := orcNoTaskProblems(start); len(p) > 0 {
		t.Errorf("start.md: %v", p)
	}
	if len(orcNoTaskProblems(strings.Replace(start, "command -v lets >/dev/null 2>&1 && lets worktree task-state set --create --orc '<name>' --json", "", 1))) == 0 {
		t.Error("mutation: start.md without the no-task-id --orc write must fail the check")
	}
	step5 := sectionSpan(start, "## Step 5: What do we do? (task selection - MANDATORY)")
	team := strings.Index(step5, "**Team worktree**")
	block := between(step5, "**Members (team worktree only).**", "**Wait for user to select")
	if team < 0 || block == "" || strings.Index(step5, "**Members (team worktree only).**") < team {
		t.Error("start.md Step 5 asks the Members question only in a team worktree, after the lead line (lets-7k6ae)")
	}
	for _, want := range []string{
		"`ListAgents`", `header: "Members"`, "is not shown a second time", `Skill(skill: "lets:team", args: "spawn --roster")`,
		`Skill(skill: "lets:team", args: "spawn <role>")`, "A row is never deleted", "nobody answers: ask nothing, spawn nothing", "Every other `/lets:start` asks", "a permission mode (`auto`, `bypassPermissions`) or a `.lets/cache/pipeline-state-*` marker is not unattended",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("start.md's Members question must state %q (lets-7k6ae)", want)
		}
	}
	if strings.Contains(start, "**Respawn roster**") || strings.Contains(start, "Respawn the roster?") {
		t.Error("the Members question replaces start.md's Respawn roster move and hint (lets-7k6ae)")
	}
	for _, spawn := range []string{"spawn --roster", "spawn <role>"} {
		if in, all := strings.Count(block, spawn), strings.Count(start, spawn); in != all {
			t.Errorf("start.md names %q outside the Members question (%d of %d inside)", spawn, in, all)
		}
	}

	done := read("commands", "done.md")
	park := strings.Index(done, "**Park commits.**")
	gate := strings.Index(done, `header: "Park commit"`)
	confirm := strings.Index(done, "## Step 6: Confirm with User")
	if park < 0 || gate < park || confirm < gate || !strings.Contains(done, "never a refusal") || !strings.Contains(done, `label: "Push them anyway"`) {
		t.Error("done.md warns about park commits and asks before Step 6 - never refuses")
	}
	know := sectionSpan(done, "### Team worktree: promote to Standing knowledge")
	if !strings.Contains(know, "Only when `lets worktree info --json` reports a `team`") || !strings.Contains(know, "`## 9. Standing knowledge`") || !strings.Contains(know, "multiSelect: true") {
		t.Error("done.md promotes to Standing knowledge only in a team worktree, as a separate multi-select question")
	}
	if n := strings.Count(know, `{ label: "`); n > 4 || !strings.Contains(know, `{ label: "None", description: "Promote nothing" }`) || !strings.Contains(know, "at most 3 candidates") {
		t.Errorf("the knowledge question offers at most 3 facts plus None (%d options)", n)
	}
}

// TestTeamBackend_CreateDisband pins /lets:team create (a route only) and disband:
// the lead from the lets members record, never a kill, the in-progress refusal, the
// team's parked branches from lets worktree parked (owned vs UNOWNED), removal through
// /lets:worktree remove for every launcher, and the file kept as history.
func TestTeamBackend_CreateDisband(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	team := string(b)
	if !strings.Contains(strings.Split(team, "\n")[2], "|create|disband]") {
		t.Error("the argument-hint must offer create|disband")
	}
	for _, route := range []string{"- `create [<callsign>] --area <a>` -> go to Create", "- `disband <callsign>` -> go to Disband"} {
		if !strings.Contains(team, route) {
			t.Errorf("Step 1 must route %q", route)
		}
	}
	create := sectionSpan(team, "\n## Create\n")
	if !strings.Contains(create, `Skill(skill: "lets:worktree", args: "create --team [<callsign>] --area <a>")`) || !strings.Contains(create, "`/lets:worktree create --team`") {
		t.Error("create routes to /lets:worktree create --team")
	}
	for _, dup := range []string{"team-init", "lets worktree create", "lets orca", "lets cmux", "lets tmux"} {
		if strings.Contains(create, dup) {
			t.Errorf("create duplicates the worktree flow: %q", dup)
		}
	}

	disband := sectionSpan(team, "\n## Disband\n")
	if disband == "" {
		t.Fatal("## Disband section missing")
	}
	if !strings.Contains(disband, "lets peers who --json") || !strings.Contains(disband, "the session NAMED by the team file's `lead_name`") || !strings.Contains(disband, "never counted or guessed") || strings.Contains(disband, "exactly one session") || strings.Contains(disband, "lets members lead") {
		t.Error("disband finds the lead by the team file's lead_name in lets peers who, never a session count")
	}
	if strings.Contains(disband, "TaskStop(") || !strings.Contains(disband, "no `TaskStop`") {
		t.Error("disband never kills a session")
	}
	tellGate := strings.Index(disband, `label: "Tell the lead (Recommended)"`)
	tell := strings.Index(disband, `Skill(skill: "lets:orc", args: "verb=tell`)
	if tellGate < 0 || tell < tellGate {
		t.Error("the tell to a live lead is gated")
	}
	if !strings.Contains(disband, "`who` failed or degraded -> the same gate with `{state}` = `unknown`") || !strings.Contains(disband, "never treated as gone") || !strings.Contains(disband, "- No row, or only `dead` ones -> list every live session whose cwd is `<path>`") || !strings.Contains(disband, "- One or more rows whose `alive` is not `dead` -> ask the owner") {
		t.Error("an unknown lead is asked about like a live one, never treated as gone")
	}
	wrapped := strings.Index(disband, `label: "Lead has wrapped up"`)
	if wrapped < 0 || !strings.Contains(disband, "- **Lead has wrapped up** -> go on to D2") || !strings.Contains(disband, "then end with /lets:end") {
		t.Error("a live lead's gate must offer going on to D2, and the tell asks the lead to /lets:end")
	}
	if !strings.Contains(disband, "`status` is `in_progress` -> **Refused:**") {
		t.Error("disband refuses a team branch with a task in progress")
	}
	for _, want := range []string{"lets worktree parked --team '<c>' --json", "**UNOWNED**", "never resolved or counted as this team's", "Parks of another callsign are not listed", "`/lets:start --main`", "**Disband never proceeds silently past a parked branch:**", "a resolved branch drops out of `owned[]`"} {
		if !strings.Contains(disband, want) {
			t.Errorf("disband's parked step must carry %q", want)
		}
	}
	parked := strings.Index(disband, "### Step D3: Parked tasks")
	d4 := between(disband, "### Step D4: Remove the worktree", "### Step D5")
	goRoute := between(d4, "**Under `<main checkout>/.worktrees/`**", "- **Anywhere else**")
	orcaRoute := between(d4, "- **Anywhere else**", "### Step D5")
	if orcaRoute == "" {
		orcaRoute = d4[strings.Index(d4, "- **Anywhere else**"):]
	}
	if parked < 0 || strings.Index(disband, "### Step D4") < parked || !strings.Contains(d4, "decided by WHERE the worktree lives") || !strings.Contains(d4, "never by an Orca answer") {
		t.Error("D4 routes by the worktree's location, after the parked check")
	}
	if !strings.Contains(goRoute, `Skill(skill: "lets:worktree", args: "remove team_<c>")`) || !strings.Contains(goRoute, "no Orca call") || strings.Contains(goRoute, "lets orca") {
		t.Error("a worktree under .worktrees/ goes to /lets:worktree remove with no Orca call")
	}
	if !strings.Contains(orcaRoute, `lets orca team-remove --name "team_<c>" --json`) || strings.Contains(orcaRoute, `Skill(skill: "lets:worktree"`) {
		t.Error("a worktree outside .worktrees/ goes to lets orca team-remove, never to the Go remove")
	}
	if !strings.Contains(orcaRoute, "- `not_attempted` (Orca absent or not running) -> stop") || !strings.Contains(orcaRoute, "- `not_listed` (19) -> stop") {
		t.Error("team-remove's not_attempted and not_listed are named stops")
	}
	if n := strings.Count(d4, "`git worktree list` no longer shows the path"); n < 2 {
		t.Errorf("both removal paths end with the git worktree list check, found %d", n)
	}
	for _, banned := range []string{"--force", "--allow-failed-archive-hook", "git worktree remove", "rm -rf"} {
		if strings.Contains(disband, banned) {
			t.Errorf("disband must not carry %q", banned)
		}
	}
	if !strings.Contains(disband, "run it only on the user's yes") || !strings.Contains(disband, "kept as history - never deleted") || !strings.Contains(disband, "`## 8. Decisions`") {
		t.Error("the teardown hook is gated and the team file is retired, kept as history")
	}
}

// goldenT2 is the Go T2 block of worktree.md at 8b36520 - the non-orca path is unchanged by fix-5.
const goldenT2 = "\nFetch as `lets worktree switch` does: `git fetch --no-tags origin {LETS_MERGE_BRANCH}` bounded to 20 s; it fails and `origin/{LETS_MERGE_BRANCH}` exists -> go on with a one-line staleness warning; no `origin/{LETS_MERGE_BRANCH}` -> stop (`no_remote_base`) - the local merge-branch is never a base. Then:\n\n```bash\nLETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)\ncd \"$LETS_PROJECT_ROOT\"\nlets worktree create \"team_<c>\" --branch \"team_<c>\" --new-branch --base \"origin/{LETS_MERGE_BRANCH}\" --plugin-root \"${CLAUDE_PLUGIN_ROOT}\" --json\n```\n\n`ok=false` -> surface `error.message` and stop. Assert, then go on: `worktree.path` ends in `/team_<c>`, and `git -C \"<path>\" branch --show-current` prints `team_<c>`; anything else -> stop and show both.\n\n"

// membersLinkProblems pins D1-D3 (owner, 2026-09-26) on team.md's member sections.
func membersLinkProblems(team string) []string {
	var p []string
	intro := between(team, "\n## Members\n", "### Step M0: Team")
	if !strings.Contains(intro, "`link: peer`") || !strings.Contains(intro, "`spawn --roster`") || !strings.Contains(intro, "members do not message each other") {
		p = append(p, "the ## Members intro must state D2: with link: peer members do not message each other, the lead respawns the roster")
	}
	m1 := sectionSpan(team, "### Step M1: One Member (`spawn <role> [name]`)")
	for _, need := range []string{"Message another teammate directly when you need its input", "A decision you reach with another teammate goes into your REPORT_FILE.", "With `link: peer` (the lead was restarted) send no member-to-member messages", "APPENDS a second `Reports:` line"} {
		if !strings.Contains(m1, need) {
			p = append(p, "Step M1 must carry "+need)
		}
	}
	// fix-7: a reader takes only this lead session's Reports line, and the brief file
	// carries the REPORT_FILE an implementer reads from it (implementer.md ## Output).
	if !strings.Contains(m1, "Take the line whose `<session6>` is this session's (`$CLAUDE_CODE_SESSION_ID`)") {
		p = append(p, "Step M1.5 must read only the Reports line this lead session opened")
	}
	if !strings.Contains(m1, "then its own last line `REPORT_FILE: <path>`") {
		p = append(p, "the Step M1 brief file must carry its REPORT_FILE line")
	}
	if m2 := sectionSpan(team, "### Step M2: The Whole Roster (`spawn --roster`)"); !strings.Contains(m2, "by Step M1.5's rule") || !strings.Contains(m2, "Each member's brief and prompt then carry its own `REPORT_FILE: <path>` line") {
		p = append(p, "Step M2 must open or add the report files by Step M1.5's rule and hand each brief its REPORT_FILE")
	}
	if m2 := sectionSpan(team, "### Step M2: The Whole Roster (`spawn --roster`)"); !strings.Contains(m2, "that `ListAgents` does not list as `<c>-<name>` and that has no `Dismissed: <name>` line by this lead session is a candidate") || strings.Contains(m2, "not live (Step M0)") {
		p = append(p, "Step M2 must skip a roster row recorded Dismissed: by this lead session")
	}
	if m5 := sectionSpan(team, "### Step M5: Member reports"); !strings.Contains(m5, "A member with a `Dismissed: <name> (lead <session6>)` line by this lead session in the team file's `## 7. Current task` is never messaged: no nudge - straight to item 3's `op=collect ... retry=no`") {
		p = append(p, "Step M5 must never nudge a member recorded Dismissed: by this lead session")
	}
	m4 := sectionSpan(team, "### Step M4: Dismiss (`dismiss <name>` / `dismiss --all`)")
	if !strings.Contains(m1, "else the name of the roster row for this role that is not live and has no `Dismissed: <name>` line by this lead session; else `<role>` under the same condition; else the next free `-N` suffix") || !strings.Contains(m1, "takes Step 4's **Fresh name** path at once") {
		p = append(p, "Step M1's name choice must skip a name recorded Dismissed: by this lead session")
	}
	if !strings.Contains(m4, "every `<c>-*` agent `ListAgents` (the harness's agent registry) lists, roster row or not, that has no `Dismissed:` line by this lead session") {
		p = append(p, "Step M4 dismiss --all must cover only the listed roster names with no Dismissed: line by this lead session")
	}
	if strings.Contains(m4, "the same call") || strings.Count(m4, `Skill(skill: "lets:member-run", args: "op=dismiss scope=<c> name=<name>")`) != 2 {
		p = append(p, "Step M4 must name member-run op=dismiss for dismiss <name> and dismiss --all alike")
	}
	return p
}

func TestTeamBackend_MembersLink(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "team.md"))
	if err != nil {
		t.Fatal(err)
	}
	team := string(b)
	for _, problem := range membersLinkProblems(team) {
		t.Error(problem)
	}
	const d2 = "With `link: peer` (lead restarted) members do not message each other; the lead restores the team with `spawn --roster` (D2, owner 2026-09-26)."
	if !strings.Contains(team, d2) {
		t.Fatal("mutant cannot apply: the D2 line is missing")
	}
	if len(membersLinkProblems(strings.Replace(team, d2, "", 1))) == 0 {
		t.Error("removing the D2 line must fail the pin")
	}
	mutants := []struct{ name, old, repl, want string }{
		{"a reader takes any Reports line", "Take the line whose `<session6>` is this session's (`$CLAUDE_CODE_SESSION_ID`)", "Take the latest line", "only the Reports line this lead session opened"},
		{"the brief loses its REPORT_FILE", ", then its own last line `REPORT_FILE: <path>` (Step 5's path)", "", "brief file must carry its REPORT_FILE"},
		{"M2 skips the M1.5 rule", "first the report files, by Step M1.5's rule, for every name at once", "first the report files for every name at once", "Step M2 must open or add"},
		{"dismiss --all says the same call again", "then `Skill(skill: \"lets:member-run\", args: \"op=dismiss scope=<c> name=<name>\")` for every registry member - every `<c>-*` agent", "then the same call for every registry member - every `<c>-*` agent", "Step M4 must name member-run op=dismiss"},
		{"M2 respawns a dismissed row", " and that has no `Dismissed: <name>` line by this lead session is a candidate", " is a candidate", "Step M2 must skip a roster row recorded Dismissed:"},
		{"M2 drops surviving panes again", "that `ListAgents` does not list as `<c>-<name>` and that has", "that is not live (Step M0) and has", "Step M2 must skip a roster row recorded Dismissed:"},
		{"dismiss --all forgets Dismissed", " that has no `Dismissed:` line by this lead session.", ".", "Step M4 dismiss --all must cover only"},
		{"M5 nudges a dismissed member", "2. A member with a `Dismissed: <name> (lead <session6>)` line by this lead session in the team file's `## 7. Current task` is never messaged: no nudge - straight to item 3's `op=collect ... retry=no`, and its GAP is recorded. Otherwise, not", "2. Not", "Step M5 must never nudge a member recorded Dismissed:"},
		{"M1 reuses a dismissed name", "for this role that is not live and has no `Dismissed: <name>` line by this lead session;", "for this role that is not live;", "Step M1's name choice must skip"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(team, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in team.md", m.old)
			}
			for _, problem := range membersLinkProblems(strings.Replace(team, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}

// t5OrcProblems checks the team create's Step T5: the lead's --orc comes from this
// session's own self_record, as in worktree.md's Orchestrator binding.
func t5OrcProblems(team string) []string {
	t5 := team
	if i := strings.Index(t5, "### Step T5: Launch the lead"); i >= 0 {
		t5 = t5[i:]
	}
	var bad []string
	if strings.Contains(t5, "source=self") {
		bad = append(bad, "T5 tests source=self for \"this session is an orchestrator\"")
	}
	if !strings.Contains(t5, "carries a non-null `self_record`") || !strings.Contains(t5, `'/lets:start --orc="<self_record.name>"'`) {
		bad = append(bad, "T5 binds the lead with '/lets:start --orc=\"<self_record.name>\"' when self_record is non-null")
	}
	return bad
}

// workerNameProblems checks take-task's Worker-name bullet: a team worktree (the lead,
// named by lead_name) is exempt from the /rename <repo_slug>-<task-id> nudge.
func workerNameProblems(take string) []string {
	i := strings.Index(take, "- **Worker name.**")
	if i < 0 {
		return []string{"the Worker name bullet is missing"}
	}
	line := take[i:]
	if j := strings.Index(line, "\n"); j >= 0 {
		line = line[:j]
	}
	var bad []string
	if !strings.Contains(line, "not trunk-mode, not a team worktree)") || !strings.Contains(line, "**A team worktree**") || !strings.Contains(line, "`team_lead` hint owns that name") {
		bad = append(bad, "the Worker name nudge must exempt a team worktree and defer to the team_lead hint")
	}
	return bad
}

// orcNoTaskProblems checks start.md Step 0a: `--orc` with no task id records orc: on
// the current branch (after Step 0.5), which lets worktree switch then carries.
func orcNoTaskProblems(start string) []string {
	i := strings.Index(start, "**`--orc` without a task id**")
	j := strings.Index(start, "**If `<task-id>` provided**")
	if i < 0 || j < i {
		return []string{"Step 0a must carry the `--orc` without a task id paragraph before the per-path steps"}
	}
	block := start[i:j]
	var bad []string
	if !strings.Contains(block, "command -v lets >/dev/null 2>&1 && lets worktree task-state set --create --orc '<name>' --json") || strings.Contains(block, "--task") {
		bad = append(bad, "the binding is one task-state write of --orc on the current branch, no --task")
	}
	if !strings.Contains(block, "after Step 0.5") || !strings.Contains(block, "`lets worktree switch` carries that `orc:`") {
		bad = append(bad, "the write runs after Step 0.5 and names the switch carry")
	}
	return bad
}

// TestPeersProtocol_PaneMemberNames pins the documented limitation (seams f): pane
// members are `<callsign>-<name>` with no repository part, and callsigns are unique
// per repository only.
func TestPeersProtocol_PaneMemberNames(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "protocol", "peers.md"))
	if err != nil {
		t.Fatal(err)
	}
	peers := string(b)
	if p := paneNameProblems(peers); len(p) > 0 {
		t.Errorf("protocol/peers.md: %v", p)
	}
	if len(paneNameProblems(strings.Replace(peers, "## Pane-member names across repos", "## Pane members", 1))) == 0 {
		t.Error("mutation: peers.md without the pane-member names section must fail the check")
	}
	if len(paneNameProblems(strings.Replace(peers, "`lets peers who` marks both `duplicate_name`", "both are listed", 1))) == 0 {
		t.Error("mutation: the section without the duplicate_name consequence must fail the check")
	}
}

func paneNameProblems(peers string) []string {
	sec := sectionSpan(peers, "\n## Pane-member names across repos\n")
	if sec == "" {
		return []string{"the ## Pane-member names across repos section is missing"}
	}
	var bad []string
	for _, want := range []string{"unique per repository only", "`<repo>-<callsign>-lead`", "`<callsign>-<name>` with no repository part", "`lets peers who` marks both `duplicate_name`", "`[ref]`", "In-process members (no pane) are not affected"} {
		if !strings.Contains(sec, want) {
			bad = append(bad, "missing "+want)
		}
	}
	return bad
}

// TestTeamFile_SwitchKeepsSessionLines pins the same-session dismissal contract
// across a task switch: the `## 7. Current task` rewrite replaces only the
// task-specific content and keeps every `Dismissed:` / `Reports:` line - take-task
// Step 2T (the writer) and the template's section intro both say so.
func TestTeamFile_SwitchKeepsSessionLines(t *testing.T) {
	files := pluginTexts(t)
	take, tmpl := files["skills/take-task/SKILL.md"], files["templates/team.md"]
	if p := switchKeepProblems(take, tmpl); len(p) > 0 {
		t.Errorf("%v", p)
	}
	const takeKeep = "and KEEP every existing `Dismissed: <name> (lead <session6>)` and `Reports: <dir> (lead <session6>)` line as it is"
	const tmplKeep = "Every `Dismissed: <name> (lead <session6>)` and `Reports: <dir> (lead <session6>)` line is KEPT as it is across a switch"
	if !strings.Contains(take, takeKeep) || !strings.Contains(tmpl, tmplKeep) {
		t.Fatal("mutants cannot apply: a keep-rule sentence is missing")
	}
	if len(switchKeepProblems(strings.Replace(take, takeKeep, "", 1), tmpl)) == 0 {
		t.Error("mutation: take-task Step 2T without the keep-rule must fail the check")
	}
	if len(switchKeepProblems(take, strings.Replace(tmpl, tmplKeep, "The lead rewrites the whole section", 1))) == 0 {
		t.Error("mutation: the template without the keep-rule must fail the check")
	}
}

func switchKeepProblems(take, tmpl string) []string {
	var bad []string
	step2t := sectionSpan(take, "### Step 2T: Team worktree - park, then switch")
	i := strings.Index(step2t, "Then rewrite the team file's `## 7. Current task` section")
	if i < 0 {
		return []string{"take-task Step 2T must rewrite the team file's ## 7. Current task section"}
	}
	rewrite := step2t[i:]
	if j := strings.Index(rewrite, "Then Step 6."); j >= 0 {
		rewrite = rewrite[:j]
	}
	for _, want := range []string{"replace only the task-specific content", "KEEP every existing `Dismissed: <name> (lead <session6>)` and `Reports: <dir> (lead <session6>)` line", "other lead sessions' lines too"} {
		if !strings.Contains(rewrite, want) {
			bad = append(bad, "take-task Step 2T rewrite: missing "+want)
		}
	}
	sec := sectionSpan(tmpl, "\n## 7. Current task\n")
	for _, want := range []string{"rewrites the task-specific content of this section", "Every `Dismissed: <name> (lead <session6>)` and `Reports: <dir> (lead <session6>)` line is KEPT as it is across a switch"} {
		if !strings.Contains(sec, want) {
			bad = append(bad, "templates/team.md ## 7: missing "+want)
		}
	}
	if strings.Contains(sec, "The lead rewrites this section on every task switch") {
		bad = append(bad, "templates/team.md ## 7 still says the whole section is rewritten")
	}
	return bad
}
