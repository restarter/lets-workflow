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
		if strings.Contains(l, "worktree create") && !strings.Contains(l, `--base "origin/`) {
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
	if !strings.Contains(sessions, "`{agent_command}` = the `agent_command` frontmatter value of the team file") || !strings.Contains(sessions, "else `claude`") {
		t.Error("the agent command comes from the team file's agent_command, else claude")
	}
	if !strings.Contains(orca, "`{agent_command}` as defined in Step N2") {
		t.Error("the orca worker start uses the same agent command")
	}
	if !strings.Contains(sessions, "lets worktree branch-name") || !strings.Contains(sessions, "`dir`") {
		t.Error("the worktree dir comes from lets worktree branch-name's dir field")
	}
	if !strings.Contains(sessions, "worktree_path_exists") {
		t.Error("the run step must name create's worktree_path_exists backstop")
	}

	// the orchestrator is registered before the first launch, and re-checked per launch
	register := strings.Index(team, "lets peers role set orchestrator")
	first := strings.Index(team, `open "<path>"`)
	if t2 := strings.Index(team, `cd "<path>" &&`); t2 >= 0 && (first < 0 || t2 < first) {
		first = t2
	}
	if t3 := strings.Index(team, "--command"); t3 >= 0 && (first < 0 || t3 < first) {
		first = t3
	}
	if register < 0 || first < 0 || register > first {
		t.Errorf("orchestrator registration (%d) must precede the first launch (%d)", register, first)
	}
	recheck := strings.Index(sessions, "source=self")
	if recheck < 0 || recheck > strings.Index(sessions, `open "<path>"`) {
		t.Error("every launch re-asserts source=self before it opens a session")
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
	const spawnCall = `Skill(skill: "lets:member-run", args: "op=spawn `
	if n, calls := strings.Count(team, "op=spawn"), strings.Count(team, spawnCall); calls == 0 || n != calls {
		t.Errorf("every spawn goes through %s (op=spawn %d, calls %d)", spawnCall, n, calls)
	}
	dismiss := sectionSpan(team, "\n## Dismiss\n")
	if !strings.Contains(dismiss, `Skill(skill: "lets:member-run", args: "op=dismiss `) {
		t.Error("dismiss goes through member-run op=dismiss")
	}
	if !strings.Contains(dismiss, "`lead.session` is not `$CLAUDE_CODE_SESSION_ID`") || !strings.Contains(dismiss, "**Refused:**") {
		t.Error("only the lead's own session dismisses")
	}
	members := sectionSpan(team, "\n## Members\n")
	for _, want := range []string{"lets worktree info --json", "No `team`", "`<c>-<name>`", "`cwd` is the team worktree"} {
		if !strings.Contains(members, want) {
			t.Errorf("Step M0 must state %q", want)
		}
	}
	spawn := sectionSpan(team, "\n## Spawn\n")
	refuse := strings.Index(spawn, "**Same name live -> refused.**")
	call := strings.Index(spawn, spawnCall)
	if refuse < 0 || call < 0 || refuse > call {
		t.Error("the same-name live refusal comes before the spawn call")
	}
	for _, want := range []string{`label: "Close it first (Recommended)"`, "the survivor is recorded dismissed", "the survivor keeps running - close it by hand", ".lets/cache/member-<c>-<name>.md", "on the lead's OK"} {
		if !strings.Contains(spawn, want) {
			t.Errorf("spawn must carry %q", want)
		}
	}
	respawn := strings.Index(spawn, `label: "Respawn all (Recommended)"`)
	reuse := strings.Index(spawn, `label: "Re-use panes (keeps old context)"`)
	if respawn < 0 || reuse < 0 || respawn > reuse {
		t.Error("spawn --roster recommends respawn first; re-use is labelled keeps old context")
	}
	if !strings.Contains(spawn, "`send` is `orca` or `claude`") || !strings.Contains(spawn, "exactly one live session carries that name") {
		t.Error("re-use is offered only for a unique surviving pane with an orca or claude send route")
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
	claim := strings.Index(step6, "lets members lead --claim --scope")
	take := strings.Index(step6, `Skill(skill: "lets:take-task"`)
	if claim < 0 || take < 0 || claim > take {
		t.Errorf("start.md claims the team lead before take-task (claim=%d take=%d)", claim, take)
	}
	for _, want := range []string{"`lead_held` -> stop", "`registry_unavailable` included", "then continue"} {
		if !strings.Contains(step6, want) {
			t.Errorf("start.md's lead claim must state %q", want)
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
	if strip == "" || !strings.Contains(strip, "`--flow` or `--auto`") || !strings.Contains(strip, "from inside a worktree") || !strings.Contains(strip, "**Refused**") {
		t.Errorf("Step 1 must route create --team and refuse it with --flow / --auto and inside a worktree: %q", strip)
	}

	team := sectionSpan(wt, "\n## Create a team\n")
	if team == "" {
		t.Fatal("## Create a team section missing")
	}
	for _, banned := range []string{"orca worktree create", "lets orca create", "lets orca open"} {
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
	if !strings.Contains(team, "Go creates the worktree for EVERY launcher, Orca included") || !strings.Contains(team, "the worktree is the one T2 created") {
		t.Error("every launcher, orca included, uses the Go-created worktree")
	}
	if !strings.Contains(team, "`launched=false`") || !strings.Contains(team, "`fallback_command`") || !strings.Contains(team, "never an Orca worktree create") {
		t.Error("an Orca refusal falls to the printed command, never an Orca create")
	}
	// the team file's agent command crosses through a quoted heredoc, never a double-quoted argument
	teamLines := strings.Split(team, "\n")
	for i, l := range teamLines {
		if strings.Contains(l, "--command") && !strings.Contains(l, `--command "$CMD"`) {
			t.Errorf("every team --command passes \"$CMD\" from a quoted heredoc: %s", strings.TrimSpace(l))
		}
		if strings.Contains(l, "CMD=$(cat <<") {
			if !strings.Contains(l, "<<'EOF'") || i+1 >= len(teamLines) || !strings.Contains(teamLines[i+1], "--name '<c>-lead' '/lets:start'") {
				t.Errorf("a CMD heredoc must be quoted and carry --name '<c>-lead': %s", strings.TrimSpace(l))
			}
		}
	}
	if n := strings.Count(team, `--command "$CMD"`); n < 2 {
		t.Errorf("cmux/tmux and orca both launch through the heredoc, found %d", n)
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

	c1 := sectionSpan(wt, "\n### Step C1: Get Name\n")
	if !strings.Contains(c1, "the `dir` field as the dir `<name>`") || strings.Contains(c1, "from `slug` as the dir") {
		t.Error("C1 takes the dir name from branch-name's dir field, never hand-built from slug")
	}
}
