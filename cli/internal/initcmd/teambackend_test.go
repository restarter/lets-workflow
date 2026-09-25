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
