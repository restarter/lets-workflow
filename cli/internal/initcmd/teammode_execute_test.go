package initcmd

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// lets-mpjhc / lets-kwd6e: /lets:execute on the id model. The team check and the
// allowlist architect route through member-run op=route (fallback=run), the Step 4.6
// anchor check routes with ask=no (nothing is spawned before Start), every analyst
// call carries its recorded id, liveness is a send to that id (never "not listed"),
// implementers keep the command's own record-based nudge - with a resume text for a
// stopped implementer - and never go through op=receive. teamModeExecuteProblems
// reads only its argument, so TestTeamModeExecute feeds it mutants.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

const (
	executeWhoRoute    = "op=route command=execute roles=explorer,skeptic fallback=run"
	executeAnchorRoute = "op=route command=execute roles=explorer ask=no fallback=inline"
	oldLivenessRule    = "`{agent}` not listed counts as gone"
	resumeText         = "You stopped before your report. Continue chunk {chunk} from where you stopped - do not redo finished work - then write REPORT_FILE {path}"
	oldSingleNudge     = "or, if you have not finished, write a report saying why"
)

// analystCall matches a member-run Skill call (op=next|correct|dismiss) addressed to a
// run analyst or a standing-team member - anything but the implementer {agent}.
var analystCall = regexp.MustCompile(`args: "op=(next|correct|dismiss) scope=(run-\{RUN\}|<callsign>) name=((?:explorer|skeptic|architect)-\{RUN\}|<name>)([^"]*)"`)

// receiveCallName matches every member-run op=receive call in execute.md, capturing the name.
var receiveCallName = regexp.MustCompile(`op=receive scope=[^ "]+ name=([^ "]+)`)

func teamModeExecuteProblems(exec string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	// Who checks: route, never a ListAgents name test.
	who := between(exec, "**Who checks.**", "- **CHECK**")
	if !strings.Contains(who, executeWhoRoute) || !strings.Contains(who, "fallback=run") {
		add("execute.md Who checks must route through member-run " + executeWhoRoute)
	}
	if strings.Contains(exec, "`ListAgents` lists `<callsign>-") {
		add("execute.md must carry no ListAgents name test (`ListAgents` lists `<callsign>-...`) - a member is reached by its recorded id")
	}
	// member_busy: a team member line -> the run's own analyst; a run analyst -> queued.
	if !strings.Contains(who, "`member_busy` on a `member` line (something outside this run woke the team member - execute does not own that round) -> send the check to the run's own analyst instead (`fallback=run`)") {
		add("Who checks must send a busy team member's check to the run's own analyst (fallback=run)")
	}
	if !strings.Contains(who, "`member_busy` on a run analyst (pipelined runs and parallel groups can overlap two chunk checks) -> record `queued`") || !strings.Contains(who, "send it when that analyst's current round returns `RECEIVED` or `GAP`") {
		add("Who checks must queue a busy run analyst's check and send it on that analyst's RECEIVED / GAP")
	}

	// Every analyst call carries an id.
	calls := analystCall.FindAllStringSubmatch(exec, -1)
	if len(calls) == 0 {
		add("execute.md must name its analyst member-run calls")
	}
	for _, m := range calls {
		if !strings.Contains(m[4], " id=") {
			add("every analyst op=" + m[1] + " call carries id=: " + m[0])
		}
	}

	// Step 4.6 anchor check: ask=no.
	if !strings.Contains(between(exec, "**Anchor check against BASE - a WARNING only.**", "**CI checks.**"), executeAnchorRoute) {
		add("the Step 4.6 anchor check must route with ask=no - nothing is spawned before Start")
	}

	// Liveness: the recorded id, agent_gone on a send.
	recovery := sectionSpan(exec, "### 5-D.7 Recovery (a run record exists)")
	if strings.Contains(exec, oldLivenessRule) {
		add("execute.md must drop the old rule \"" + oldLivenessRule + "\" - a completed member drops out of ListAgents and still resumes")
	}
	if !strings.Contains(recovery, "a member is gone only when a send to its recorded `agent_id` returns `agent_gone`") {
		add("5-D.7 Liveness must name agent_gone on a send to the recorded id as the gone signal")
	}

	// Implementers: never op=receive; the not-running row's nudge carries the resume text.
	for _, m := range receiveCallName.FindAllStringSubmatch(exec, -1) {
		if strings.HasPrefix(m[1], "impl-") || m[1] == "{agent}" {
			add("no op=receive call may name an implementer - implementers keep the record-based nudge: " + m[0])
		}
	}
	notRunning := lineStarting(recovery, "| `running` / `correcting` | `{agent}` not running")
	idle := lineStarting(recovery, "| `running` / `correcting` | `ListAgents` shows `{agent}` idle")
	if !strings.Contains(notRunning, "\"You stopped before your report. Continue chunk {chunk} from where you stopped") {
		add("5-D.7's not-running implementer row must nudge with the resume text")
	}

	// 5-D.5's one nudge: two texts, and 5-D.7 points to it.
	report := between(exec, "**What is a report.**", "When `{agent}` reports, record")
	if !strings.Contains(report, "**The one nudge**") || !strings.Contains(report, "a claim without an `OK` file -> the write-report text") || !strings.Contains(report, "the implementer not running and no report claimed -> the resume text (\""+resumeText) || strings.Contains(exec, oldSingleNudge) {
		add("5-D.5's one nudge must carry two texts - resume for a not-running implementer with no claim, write-report for a claim - not the single \"if you have not finished\" text")
	}
	if !strings.Contains(idle, "5-D.5's **The one nudge**") || !strings.Contains(notRunning, "5-D.5's **The one nudge**") {
		add("5-D.7's recovery rows (idle, not running) must reuse 5-D.5's one nudge rule")
	}

	// The run record: analysts with agent_id and dismissed.
	record := sectionSpan(exec, "### 5-D.3 Run record")
	analysts := lineStarting(record, `  "analysts": `)
	for _, role := range []string{"explorer", "skeptic", "architect"} {
		if !strings.Contains(analysts, `"`+role+`": { "agent": "`+role+`-{RUN}", "agent_id": null, "dismissed": false }`) {
			add("the run record must carry analysts." + role + " with agent_id and dismissed")
		}
	}
	return p
}

func TestTeamModeExecute(t *testing.T) {
	for _, problem := range teamModeExecuteProblems(readPlugin(t, filepath.Join("commands", "execute.md"))) {
		t.Error(problem)
	}
}

// TestTeamModeExecute_Mutants proves each guard of teamModeExecuteProblems can fail.
func TestTeamModeExecute_Mutants(t *testing.T) {
	exec := readPlugin(t, filepath.Join("commands", "execute.md"))
	mutants := []struct{ name, old, repl, want string }{
		{"Who checks loses its route", executeWhoRoute, "op=next scope=<callsign> name=explorer", "Who checks must route"},
		{"the listing test returns", "**Who checks.** ", "**Who checks.** The standing member when `ListAgents` lists `<callsign>-<name>`. ", "ListAgents name test"},
		{"the skeptic-{RUN} call loses its id", `name=skeptic-{RUN} id={agent_id} brief-file=`, `name=skeptic-{RUN} brief-file=`, "carries id="},
		{"the anchor check drops ask=no", executeAnchorRoute, "op=route command=execute roles=explorer fallback=inline", "ask=no"},
		{"the old liveness rule returns", "**Liveness comes from the recorded id**", "**Liveness comes from `ListAgents`**: " + oldLivenessRule + ". **Liveness comes from the recorded id**", "drop the old rule"},
		{"agent_gone no longer the gone signal", "a member is gone only when a send to its recorded `agent_id` returns `agent_gone`", "a member is gone when it is not listed", "agent_gone on a send"},
		{"an implementer goes through op=receive", "| `stopping` | - | 5-D.6 again |", "| `stopping` | - | `Skill(skill: \"lets:member-run\", args: \"op=receive scope=run-{RUN} name=impl-{RUN} id={agent_id} brief-file=b report-file=r\")` |", "no op=receive call may name an implementer"},
		{"the not-running row loses the resume text", "(\"You stopped before your report. Continue chunk {chunk} from where you stopped - do not redo finished work - then write REPORT_FILE {path} ...\")", "(the write-report text)", "resume text"},
		{"the single nudge text returns", "write your report to REPORT_FILE {path} and send REPORT_WRITTEN. This changes nothing in the chunk;", "write your report to REPORT_FILE {path} and send REPORT_WRITTEN, " + oldSingleNudge + ". This changes nothing in the chunk;", "two texts"},
		{"5-D.7 restates instead of pointing", "otherwise 5-D.5's **The one nudge** - its text", "otherwise the one-nudge rule - its text", "reuse 5-D.5's one nudge"},
		{"the run record drops analysts", `  "analysts": {`, `  "helpers": {`, "analysts.explorer"},
		{"a team member line is queued", "-> send the check to the run's own analyst instead (`fallback=run`)", "-> record `queued` for it", "busy team member's check"},
		{"a busy run analyst is not queued", "-> record `queued` as that check's value", "-> drop that check", "queue a busy run analyst's check"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(exec, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in execute.md - update the mutant with the text it guards", m.old)
			}
			for _, problem := range teamModeExecuteProblems(strings.Replace(exec, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
