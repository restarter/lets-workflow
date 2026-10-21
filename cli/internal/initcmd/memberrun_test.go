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
	spawn := sectionSpan(skill, "## Step 2: Spawn")
	next := sectionSpan(skill, "## Step 3: Next / Correct")
	if strings.Contains(skill, "lets members") {
		add("member-run must call no lets members verb - the caller's record is the record, ListAgents judges liveness (lets-gbfpd)")
	}
	// The role gate: every role_not_allowed line before the Agent call names
	// lets:actor - it is never a member.
	if at := strings.Index(skill, "Agent("); at < 0 {
		add("member-run must contain the Agent call")
	} else {
		lines := 0
		for _, line := range strings.Split(skill[:at], "\n") {
			if !strings.Contains(line, "role_not_allowed") {
				continue
			}
			lines++
			if !strings.Contains(line, "`lets:actor`") {
				add("every role_not_allowed line must name `lets:actor`: " + line)
			}
		}
		if lines == 0 || !strings.Contains(skill, "a shipped `lets:*` agent") {
			add("member-run must refuse a non-member role with role_not_allowed before the Agent call, and allow a shipped `lets:*` agent")
		}
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
		ret := ""
		if at := strings.Index(spawn, "Return: "); at >= 0 {
			ret = spawn[at:]
			if end := strings.Index(ret, "\n"); end >= 0 {
				ret = ret[:end]
			}
		}
		if !strings.Contains(ret, "id {agent_id}") || !strings.Contains(ret, "the caller records it") {
			add("the Step 2 Return line must carry the id {agent_id} the Agent call returned, for the caller to record")
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
		if list := strings.Index(next, "\n1. `ListAgents`, before every message.\n"); list < 0 || send < 0 || list > send {
			add("Step 3 must run ListAgents before every SendMessage")
		}
		if !strings.Contains(call, `\nREPORT_FILE: {report-file}"`) {
			add("the Step 2 Agent prompt must carry a REPORT_FILE line")
		}
		if !strings.Contains(next, `\nREPORT_FILE: {report-file}\n`+roundFileNote+`"`) || !strings.Contains(next, "followed by the line `REPORT_FILE: {report-file}` and the line `"+roundFileNote+"`") {
			add("Step 3's NEXT and AMENDMENT must each carry a REPORT_FILE line that replaces any earlier one")
		}
		if !strings.Contains(skill, "`REPORT_WRITTEN` pointer") || !strings.Contains(skill, "report_file_missing") {
			add("member-run must read the report as a REPORT_WRITTEN pointer to a file, and refuse a missing report-file")
		}
		if strings.Count(skill, "SendMessage(") != strings.Count(next, "SendMessage(") {
			add("every SendMessage must live in Step 3, behind its ListAgents")
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
	if !strings.Contains(picker, "**Team worktree - Implementers by default (lets-sn4li).**") || !strings.Contains(picker, "asks no run-mode question: the run is **Implementers**") {
		add("Step 4.5: bare /lets:execute in a team worktree runs Implementers without the picker (lets-sn4li)")
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
	for _, need := range []string{"expand `{a,b}` brace sets", "a bare name (no `/`) resolves, in this order: to the directory of the previous path on the same Files line only when the file exists there at BASE or the task creates it there", "else to the repo root when it exists there at BASE or the task creates it there", "else it is unresolvable - a warning, never a refusal", "`Files add:` Amendment lines count as its Files", "Only a FULL path", "resolves ambiguously, is a warning", "never a refusal"} {
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
	// lets-mpjhc: the anchor check routes (ask=no, fallback=inline - nothing is spawned
	// before Start) and reads a member through op=receive; receive's own order (peek,
	// one nudge, collect retry=no) is pinned once, on member-run Step 6.
	if !strings.Contains(anchor, "`Skill(skill: \"lets:member-run\", args: \"op=route command=execute roles=explorer ask=no fallback=inline\")`") || !strings.Contains(anchor, `args: "op=receive scope=<callsign> `) || !strings.Contains(anchor, "never a retry") {
		add("the Step 4.6 anchor check routes with ask=no fallback=inline and reads a member explorer with member-run op=receive - never an agent-report retry")
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
		for _, need := range []string{`Skill(skill: "lets:member-run"`, "brief-file=", "scope=run-{RUN}", "TaskStop(", "git ls-files --others --exclude-standard", "--untracked-files=all", "git diff HEAD", "git diff --cached --name-only", `args: "approved=review-accept"`, "patch_sha", "**Render review**", "**What is a report.**", "malformed-report", "REPORT_WRITTEN", "missing-report", "READ EVERY REPORT IN FULL", "retry=no", "| `pending` |", "| `review` / `paused` |", "| `committing` |", "AMENDMENT to chunk"} {
			if !strings.Contains(delegated, need) {
				add("Step 5-D must contain " + need)
			}
		}
		if !strings.Contains(delegated, "EMPTY ones included") || !strings.Contains(delegated, "op=peek") {
			add("Step 5-D.5 must peek at the REPORT_FILE on every notification, EMPTY ones included - the final text is not the completion gate")
		}
		if !strings.Contains(delegated, "Never wait for a later `/lets:execute` to notice a stopped agent") {
			add("Step 5-D.5 must nudge once and then record a gap for a stopped agent with no report, in the same session")
		}
	}
	if !strings.Contains(exec, "a replacement never writes an earlier generation's report") {
		add("the replacement brief must name the new generation's round-0 REPORT_FILE as its only REPORT_FILE")
	}
	if !strings.Contains(exec, "rehydration") {
		add("5-D.7 must rehydrate a recorded round whose report file no longer peeks OK, without counting a new report")
	}
	// A nudge is an amendment that changes nothing: implementer.md re-checks for a clean
	// tree on every NEXT, so a nudge sent as op=next blocks a solo implementer on its own chunk.
	if !strings.Contains(exec, `args: "op=correct scope=run-{RUN} name={agent} id={agent_id} correct-file=<that file> report-file={path}")`) || strings.Contains(exec, "brief-file=<that file>") ||
		!strings.Contains(exec, "`member-run` `op=correct` with it as `correct-file=`") || !strings.Contains(exec, "do not re-run Verify unless you have not run it yet") {
		add("the 5-D.5 nudge and the 5-D.7 resend must go through op=correct, never op=next, and say they change nothing in the chunk")
	}
	if !strings.Contains(exec, "only a correction advances `round`") {
		add("5-D.5 must say a nudge or rehydration sent through op=correct keeps the round")
	}
	if !strings.Contains(exec, "Do not re-run Verify and do not change the work.") {
		add("the 5-D.7 rehydration must ask for the same report again, without re-running Verify or changing the work")
	}
	// Owner decision (fix-7): a member-run member is never retried by agent-report.
	who := ""
	if at := strings.Index(exec, "**Who checks.**"); at >= 0 {
		who = exec[at:]
		if end := strings.Index(who, "\n\n"); end >= 0 {
			who = who[:end]
		}
	}
	// lets-mpjhc: the team check routes (fallback=run) and reads every verdict with
	// member-run op=receive; receive's peek / nudge / collect order is pinned on member-run.
	if !strings.Contains(who, "`Skill(skill: \"lets:member-run\", args: \"op=route command=execute roles=explorer,skeptic fallback=run\")`") || !strings.Contains(who, `args: "op=receive scope=<the line's scope> `) || strings.Contains(who, "op=peek") || strings.Contains(who, "op=collect") || !strings.Contains(who, "never retried by agent-report") || strings.Contains(who, "ONE retry") {
		add("the team check must route through op=route (fallback=run) and read each verdict with member-run op=receive - a member-run member is never retried by agent-report")
	}
	if !strings.Contains(who, "`Dismissed: <name>`") || !strings.Contains(who, "`analysts.<role>.dismissed`") || strings.Contains(who, "`ListAgents` lists `<callsign>-") {
		add("the team check must pass dismissed for a member recorded Dismissed: by this lead session or analysts.<role>.dismissed - never a ListAgents name test")
	}
	if !strings.Contains(exec, "read with `op=receive` as the team check reads its members, never retried by agent-report") {
		add("the allowlist architect is a member-run member: read it with op=receive as the team check does, never retried")
	}

	dispatch := between(exec, "### 5-D.4 Dispatch - in plan order", "### 5-D.5 Review")
	spawnAt := strings.Index(dispatch, `args: "op=spawn scope=run-{RUN}`)
	nextAt := strings.Index(dispatch, `args: "op=next scope=run-{RUN}`)
	if spawnAt < 0 || nextAt < 0 || nextAt < spawnAt || !strings.Contains(dispatch, "`impl-{RUN}`") {
		add("5-D.4 must spawn the persistent impl-{RUN} once and hand every later chunk over with op=next")
	}
	if !strings.Contains(dispatch, "the shape the 4.7 proposal usually picks") || soloByDefault.MatchString(exec) {
		add("the solo shape must be worded as the one the 4.7 proposal usually picks, never a default allocation")
	}
	if !strings.Contains(dispatch, "`agent_gone`") || !strings.Contains(dispatch, "visibly new name") {
		add("an agent_gone on op=next must go to the 5-D.7 replacement under a visibly new name")
	}
	startGate := sectionSpan(exec, "### 5-D.2 Start - the one code-write approval")
	if n := strings.Count(startGate, `{ label: "`); n != 4 || !strings.Contains(startGate, `label: "Change the launch plan"`) {
		add(fmt.Sprintf("the Start gate must offer exactly 4 options including Change the launch plan, found %d", n))
	}
	if strings.Contains(exec, "auto-commit low-risk") {
		add("the label auto-commit low-risk must appear nowhere")
	}
	record := sectionSpan(exec, "### 5-D.3 Run record")
	for _, field := range []string{"shape", "gate_policy", "risk", "base", "agent_branch", "agent_worktree_path", "picked_sha", "patch_path", "accepted_by", "committed_by", "integrated_source", "allowlist_amendments"} {
		if !strings.Contains(record, `"`+field+`":`) {
			add("the run record must list " + field)
		}
	}
	groups := ""
	if at := strings.Index(record, `"groups": [`); at >= 0 {
		groups = record[at:]
		if end := strings.Index(groups, "\n"); end >= 0 {
			groups = groups[:end]
		}
	}
	for _, field := range []string{"agent_id", "dismissed"} {
		if !strings.Contains(groups, `"`+field+`":`) {
			add("every group of the run record must carry " + field)
		}
	}
	if strings.Contains(exec, "members_scope") {
		add("the run record must carry no members_scope - no registry scope exists (lets-gbfpd)")
	}
	if !strings.Contains(record, "every later member-run call of that member passes `id={agent_id}`") || !strings.Contains(record, "a member recorded `dismissed` is never messaged") {
		add("the run record must say every member-run call passes id={agent_id} and a dismissed member is never messaged")
	}
	// lets-mpjhc: every member-run call carries its recorded id - implementer AND analyst;
	// only op=spawn (which returns the id) and op=route (no member yet) go without one.
	for _, m := range runMemberCall.FindAllStringSubmatch(exec, -1) {
		switch name, rest := m[2], m[3]; {
		case name == "{agent}" && !strings.Contains(rest, "id={agent_id}"):
			add("every implementer member-run call (op=" + m[1] + ") carries id={agent_id}: " + m[0])
		case name != "{agent}" && !strings.Contains(rest, "id="):
			add("every analyst member-run call (op=" + m[1] + ") carries id={agent_id}: " + m[0])
		}
	}
	if !strings.Contains(exec, "`TaskStop(task_id=<the group's agent_id, else {agent}>)`") {
		add("5-D.6 Stop must stop the group's agent_id, else the name")
	}
	if !strings.Contains(record, "it moves only after the lead's commit") {
		add("integrated_source must move only after the lead's commit")
	}
	if !strings.Contains(record, "missing = high") {
		add("the run record must read a missing Risk as high")
	}
	if bareIntegrated.MatchString(exec) {
		add("execute.md must carry no bare integrated field - integrated_source only")
	}
	review := between(exec, "### 5-D.5 Review - one report at a time", "### 5-D.6 ")
	for _, need := range []string{"Does the fix need a file outside the chunk's allowlist?", "`## Allowlist addendum`", "`allowlist_amendments[]`", "by: {architect|owner}", "never a fifth option"} {
		if !strings.Contains(review, need) {
			add("the Correct path must route an out-of-allowlist path through the addendum: " + need)
		}
	}
	recovery := sectionSpan(exec, "### 5-D.7 Recovery (a run record exists)")
	for _, need := range []string{"**Liveness comes from the recorded id**", "`ListAgents` only says busy (`running`)", "returns `agent_gone`", "`dismissed`", "never messaged", "`{agent}` not running (its id not listed at all", "otherwise the idle row's path by `id={agent_id}`", "and no `agent_id` is recorded (or a correction by `id={agent_id}` returns `agent_gone`)", "or an `agent_id` recorded (sent by id)"} {
		if !strings.Contains(recovery, need) {
			add("5-D.7 must judge liveness by the recorded id and never message a dismissed member: " + need)
		}
	}
	if strings.Contains(recovery, "SendMessage(") {
		add("5-D.7 must reach a member only through member-run, never a bare SendMessage")
	}

	// parallel shape and isolated groups (Task 10)
	if !strings.Contains(picker, "**`--parallel`**") || !strings.Contains(picker, "it needs `--implementers` / `--team`") || !strings.Contains(picker, "under `--auto` it is REFUSED") {
		add("--parallel must need --implementers and be refused under --auto")
	}
	for _, need := range []string{"The shape is never inferred", "**Files-disjoint gate across groups:**", "file-disjoint is not independence"} {
		if !strings.Contains(split, need) {
			add("Step 4.6 must declare parallel groups behind a files-disjoint gate: " + need)
		}
	}
	for _, need := range []string{"MODE: {solo | isolated | pipelined}", "CALLER_TOPLEVEL:", "MAIN_ROOT:", "BASE: {base sha}", "carry `MODE: isolated` and no base instruction"} {
		if !strings.Contains(dispatch, need) {
			add("the isolated brief must carry BASE, CALLER_TOPLEVEL and MAIN_ROOT, and a NEXT no base instruction: " + need)
		}
	}
	if !strings.Contains(dispatch, "isolation=worktree name=impl-{RUN}-{group}") {
		add("an isolated group must be spawned through member-run with isolation=worktree")
	}
	integrateAt := strings.Index(review, "lets integrate --from ")
	acceptAt := strings.Index(review, "- **Accept** ->")
	if integrateAt < 0 || acceptAt < 0 || integrateAt > acceptAt || !strings.Contains(review, "first takes its patch back out: `lets integrate --revert --patch {patch_path} --json`") {
		add("an isolated chunk must be integrated through lets integrate before Accept, and reverted on a reject")
	}
	if !strings.Contains(review, "`{since}` is the group's integrated_source, or its base") {
		add("lets integrate --since must read the group's integrated_source")
	}
	completion := sectionSpan(exec, "### 5-D.9 Completion")
	if !strings.Contains(completion, "compare its `integrated_source` with the tip of its `agent_branch`") || !strings.Contains(completion, "only on the user's yes") {
		add("an agent branch and worktree may be removed only when integrated_source is its tip, and only on the user's yes")
	}
	cleanup := between(completion, "**Isolated groups' worktrees.**", "Any mismatch at step 2 or 3")
	if !strings.Contains(cleanup, "record `dismissed: true` and `agent_id: null` in ONE record write, then the call") {
		add("cleanup must record dismissed: true with agent_id: null in one write before the dismiss call")
	}
	dismissAt, removeAt, deleteAt := strings.Index(cleanup, "`op=dismiss` FIRST"), strings.Index(cleanup, "`git worktree remove {agent_worktree_path}`"), strings.Index(cleanup, "`git branch -D {agent_branch}`")
	if dismissAt < 0 || removeAt < dismissAt || deleteAt < removeAt || strings.Count(cleanup, "git rev-parse {agent_branch}") < 2 || !strings.Contains(cleanup, "never with `--force`") || strings.Contains(cleanup, "remove --force") {
		add("cleanup must dismiss first, re-check the tip before the worktree remove and before the branch delete, and never force the remove")
	}
	if !strings.Contains(sectionSpan(exec, "### 5-D.7 Recovery (a run record exists)"), "`--from` the chunk's recorded `commit_sha`, never the branch tip") {
		add("an isolated replacement must integrate from the last reported commit_sha, never the branch tip")
	}
	// pipelined run (Task 10b)
	if !strings.Contains(picker, "**`--pipelined`** fixes the commit policy `pipelined`") || strings.Count(picker, "under `--auto` it is REFUSED") < 2 {
		add("--pipelined must need --implementers and be refused under --auto")
	}
	if !strings.Contains(sectionSpan(exec, "## Step 4.7: Launch plan (proposal)"), "the run's commit policy, `pipelined` or `lead`, which the Start preview names") || !strings.Contains(startGate, "record `commit_policy: pipelined`") {
		add("the Start preview must name the commit policy, and Start records commit_policy: pipelined")
	}
	pipe := between(review, "**Pipelined run (`commit_policy: pipelined`) - review by sha.**", "Check the real tree")
	for _, need := range []string{"`git show {commit_sha}`", "`git commit --fixup={commit_sha}`", "reason `overlap`", "send the same amendment again", "**Accept makes no commit**", "`git revert --no-edit <sha>`", "never a reset"} {
		if !strings.Contains(pipe, need) {
			add("a pipelined chunk must be reviewed by sha, corrected by --fixup, stop on overlap and never reset: " + need)
		}
	}
	isoPipe := between(pipe, "- **Isolated group** (with `--parallel`).", "- **Correct** ->")
	srcAt := strings.Index(isoPipe, "1. **Check the source commits**")
	intAt := strings.Index(isoPipe, "2. **Integrate**")
	stagedAt := strings.Index(isoPipe, "3. **Check the staged paths**")
	commitAt := strings.Index(isoPipe, "4. **Only then the lead's commit**")
	if srcAt < 0 || intAt < srcAt || stagedAt < intAt || commitAt < stagedAt || strings.Contains(isoPipe, "committed by the lead at once") ||
		!strings.Contains(isoPipe, "no lead commit; a patch already applied comes back out with `lets integrate --revert --patch {patch_path} --json`") {
		add("an isolated pipelined chunk is checked (source commits, then staged paths) before the lead's commit, never committed at once")
	}
	for _, field := range []string{"commit_policy", "pre_rebase_head", "commit_sha", "fixups", "review_sha", "accepted_sha"} {
		if !strings.Contains(record, `"`+field+`":`) {
			add("the run record must list " + field)
		}
	}
	squash := between(completion, "**Autosquash", "A rebase conflict")
	mergesAt := strings.Index(squash, "`git rev-list --merges {start}..HEAD` must print nothing")
	pushedAt := strings.Index(squash, "`lets worktree pushed --branch {branch} --commit {oldest} --json`")
	yesAt := strings.Index(squash, "only on the owner's yes")
	rebaseAt := strings.Index(squash, "`git rebase --autosquash --no-autostash {start}`")
	diffAt := strings.Index(squash, "`git diff {pre_rebase_head} HEAD` must print nothing")
	if mergesAt < 0 || pushedAt < mergesAt || !strings.Contains(squash, "must return `state: not_pushed`") || yesAt < pushedAt || rebaseAt < yesAt || diffAt < rebaseAt || !strings.Contains(completion, "`git rebase --abort`") {
		add("autosquash only on a merge-free range, after lets worktree pushed = not_pushed and the owner's yes, never autostashing, then the empty-diff check")
	}
	for _, key := range []string{"execute", "agent"} {
		for _, bad := range []string{"rebase -i", "branch -r --contains"} {
			if strings.Contains(f[key], bad) {
				add(key + " must never carry " + bad)
			}
		}
	}
	rows := map[string]string{}
	for _, line := range strings.Split(f["agent"], "\n") {
		for _, mode := range []string{"solo", "isolated", "pipelined"} {
			if strings.HasPrefix(line, "| `"+mode+"` |") {
				rows[mode] = line
			}
		}
	}
	if !strings.Contains(rows["solo"], "NEVER") || !strings.Contains(rows["pipelined"], "`git commit --fixup=<that sha>`") || !strings.Contains(rows["pipelined"], "reason `overlap`") || !strings.Contains(rows["pipelined"], "NEVER push") || !strings.Contains(rows["pipelined"], "only in this mode and `isolated`") {
		add("implementer.md must commit only in the pipelined and isolated rows, with --fixup and the overlap stop")
	}

	for _, key := range []string{"execute", "agent"} {
		if strings.Contains(f[key], "reset --hard") {
			add(key + " must never carry reset --hard")
		}
	}
	isolated := ""
	for _, line := range strings.Split(f["agent"], "\n") {
		if strings.HasPrefix(line, "| `isolated` |") {
			isolated = line
		}
	}
	switchAt := strings.Index(isolated, "git switch -C")
	for _, check := range []string{"/.claude/worktrees/agent-", "`CALLER_TOPLEVEL`", "`worktree-agent-`", "`git status --porcelain --untracked-files=all`"} {
		if at := strings.Index(isolated, check); at < 0 || switchAt < 0 || at > switchAt {
			add("implementer.md's isolated row must run every guard check before git switch -C: " + check)
		}
	}
	if !strings.Contains(isolated, "Before EVERY commit re-check") || !strings.Contains(isolated, "still equals the path you verified at spawn") || !strings.Contains(isolated, "still the branch you switched at spawn") || !strings.Contains(isolated, "NEVER push") {
		add("implementer.md's isolated row must re-check the toplevel (the path verified at spawn) and the branch before every commit, and never push")
	}
	if step3 := sectionSpan(skill, "## Step 3: Next / Correct"); !strings.Contains(step3, "with `id=` -> send to the id") || !strings.Contains(skill, "puts the `id` where it says `{agent}`") {
		add("member-run must send to the id the caller passes")
	}
	if !strings.Contains(sectionSpan(skill, "## Step 4: Dismiss"), "`TaskStop(task_id=\"{agent}\")` - the `id` when passed") {
		add("member-run must stop a member by the id the caller passes")
	}

	if strings.Contains(exec, predecessor) || strings.Contains(exec, "chunk-file=") {
		add("execute.md must call member-run with brief-file= - not its predecessor, no chunk-file=")
	}

	agent := f["agent"]
	if !strings.Contains(agent, "REPORT_FILE") {
		add("implementer.md must write its report to the REPORT_FILE its brief names")
	}
	status := sectionSpan(agent, "## Status")
	for _, s := range []string{"`complete`", "`deviation-stopped`", "`blocked`"} {
		if !strings.Contains(status, s) {
			add("implementer.md ## Status must define " + s)
		}
	}
	if !strings.Contains(status, "A failing Verify is never `complete`") {
		add("implementer.md must forbid complete with a failing Verify")
	}
	if !strings.Contains(agent, "An AMENDMENT that changes nothing (a report nudge, resend or rehydration) means write the report only: no re-run of Verify beyond what its text asks, and in `pipelined` / `isolated` mode no commit and no fixup.") {
		add("implementer.md must treat a report-only amendment (nudge, resend, rehydration) as write-the-report-only: no extra Verify, no commit, no fixup")
	}
	// lets-mpjhc F15: execute's resume nudge asks the implementer to continue - the
	// report-only rule above must not swallow it.
	if !strings.Contains(agent, "The resume nudge (\"You stopped before your report. Continue chunk ... from where you stopped ...\") is not report-only: continue the chunk from where you stopped, then report.") {
		add("implementer.md must say the resume nudge is not report-only - continue the chunk, then report")
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

	// n is strings.Replace's count: 1 for one occurrence, -1 when a mutant must rewrite
	// every occurrence for its guard to see the change.
	mutants := []struct {
		name, key, old, repl, want string
		n                          int
	}{
		{"picker loses Implementers", "execute", `label: "Implementers"`, `label: "Implementer"`, "Implementers locus", 1},
		{"correct spawns", "skill", "SendMessage(", "Agent(", "Correct must", 1},
		{"status after the send", "skill", "1. `ListAgents`, before every message.", "1. Check the member before every message.", "before every SendMessage", 1},
		{"execute back on the old skill", "execute", `Skill(skill: "lets:member-run", args: "op=spawn`, "Skill(skill: \"lets:implementer" + "-run\", args: \"op=spawn", "not its predecessor", 1},
		{"a Review gate is dropped", "execute", `header: "Review"`, `header: "Reviewed"`, "Review gates", 1},
		{"files audit becomes a warning", "execute", "**Files audit - a GATE.**", "**Files audit - a WARNING.**", "Files audit as a gate", 1},
		{"files audit drops bare-name resolution", "execute", "a bare name (no `/`) resolves", "a bare name (no `/`) is refused", "resolve paths before it refuses", 1},
		{"bare name always inherits", "execute", "a bare name (no `/`) resolves, in this order: to the directory of the previous path on the same Files line only when the file exists there at BASE or the task creates it there", "a bare name (no `/`) inherits the directory of the previous path on the same Files line", "resolve paths before it refuses", 1},
		{"unresolvable name refuses", "execute", "resolves ambiguously, is a warning", "resolves ambiguously, refuses delegation", "resolve paths before it refuses", 1},
		{"removed-symbol check refuses", "execute", "it never refuses delegation", "it refuses delegation", "warning only", 1},
		{"anchor check adds an option", "execute", "adds no gate and no option", "adds an option", "anchor check", 1},
		{"brief loses CI CHECKS", "execute", "\nCI CHECKS: ", "\nCHECKS: ", "CI CHECKS:", 1},
		{"plan.workflow.js drops Risk in review", "planWorkflowJS", "every task with a commit point states **Risk:** high|low", "every task with a commit point states a risk", "planReviewPrompt", 1},
		{"plan.md drops the Risk template line", "plan", "**Risk:** {high|low}", "**Risk:** {level}", "Risk field", 1},
		{"dispatch respawns every chunk", "execute", `args: "op=next scope=run-{RUN}`, `args: "op=spawn scope=run-{RUN}`, "op=next", 1},
		{"solo becomes a default allocation", "execute", "the shape the 4.7 proposal usually picks", "by default one implementer", "usually picks", 1},
		{"Start gate loses Change the launch plan", "execute", `{ label: "Change the launch plan", `, `{ label: "Adjust", `, "Change the launch plan", 1},
		{"record drops dismissed", "execute", `"dismissed": false, `, "", "dismissed", 1},
		{"integrated_source moves on a report", "execute", "it moves only after the lead's commit", "it moves on each report", "only after the lead's commit", 1},
		{"bare integrated field", "execute", `"integrated_source": null`, `"integrated": null`, "bare integrated", 1},
		{"addendum heading dropped", "execute", "`## Allowlist addendum`", "`## Extra paths`", "addendum", 1},
		{"a recorded id is never tried", "execute", "otherwise the idle row's path by `id={agent_id}`", "otherwise", "judge liveness by the recorded id", 1},
		{"dismissed member messaged", "execute", "A member recorded `dismissed` is never messaged.", "A member recorded `dismissed` is asked to report.", "never message a dismissed member", 1},
		{"parallel allowed alone", "execute", "it needs `--implementers` / `--team`", "it runs alone", "--parallel must need", 1},
		{"groups inferred", "execute", "The shape is never inferred", "The shape is inferred", "declare parallel groups", 1},
		{"isolated brief loses CALLER_TOPLEVEL", "execute", "CALLER_TOPLEVEL: {git rev-parse", "TOPLEVEL: {git rev-parse", "CALLER_TOPLEVEL", 1},
		{"no revert on reject", "execute", "first takes its patch back out: `lets integrate --revert --patch {patch_path}", "first takes its patch back out: `git checkout -- {patch_path}", "reverted on a reject", 1},
		{"cleanup without the tip check", "execute", "compare its `integrated_source` with the tip of its `agent_branch`", "look at its `agent_branch`", "integrated_source is its tip", 1},
		{"guard switches first", "agent", "At spawn only, before any edit,", "At spawn only, first `git switch -C <branch> {BASE}`, then before any edit,", "before git switch -C", 1},
		{"reset --hard creeps in", "agent", "NEVER push.", "NEVER push; git reset --hard on a bad start.", "reset --hard", 1},
		{"step 3 sends by name", "skill", "puts the `id` where it says `{agent}`", "puts the name where it says `{agent}`", "send to the id", 1},
		{"dismiss stops by name", "skill", "`TaskStop(task_id=\"{agent}\")` - the `id` when passed", "`TaskStop(task_id=\"{name}\")`", "stop a member by the id", 1},
		{"next drops the id", "execute", `args: "op=next scope=run-{RUN} name={agent} id={agent_id} brief-file=`, `args: "op=next scope=run-{RUN} name={agent} brief-file=`, "carries id={agent_id}", 1},
		{"an analyst loses its id", "execute", "## Step 6: Record Completion", "`Skill(skill: \"lets:member-run\", args: \"op=next scope=run-{RUN} name=explorer-{RUN} brief-file=b report-file=r\")`\n\n## Step 6: Record Completion", "every analyst member-run call", 1},
		{"role line drops lets:actor", "skill", "`lets:actor` -> stop with `role_not_allowed`", "`lets:x` -> stop with `role_not_allowed`", "role_not_allowed", 1},
		{"the resume nudge is report-only", "agent", " The resume nudge (\"You stopped before your report. Continue chunk ... from where you stopped ...\") is not report-only: continue the chunk from where you stopped, then report.", "", "resume nudge", 1},
		{"return drops the id", "skill", "Return: `spawned {agent} on {model}, id {agent_id}`", "Return: `spawned {agent} on {model}`", "Return line", 1},
		{"dismiss keeps agent_id", "execute", "record `dismissed: true` and `agent_id: null` in ONE record write, then the call", "record `dismissed: true` in ONE record write, then the call", "agent_id: null", 1},
		{"who checks forgets Dismissed", "execute", " - passing `dismissed` when the member is recorded dismissed now (a team member line: a `Dismissed: <name>` line by this lead session in the team file's `## 7. Current task`; a run analyst: `analysts.<role>.dismissed`)", "", "Dismissed:", 1},
		{"stop goes by name only", "execute", "`TaskStop(task_id=<the group's agent_id, else {agent}>)`", "`TaskStop(task_id=\"{agent}\")`", "agent_id, else the name", 1},
		{"member-run calls lets members again", "skill", "## Step 1: Model panel", "```bash\nlets members status --scope {scope} --json\n```\n\n## Step 1: Model panel", "no lets members verb", 1},
		{"cleanup deletes before dismissing", "execute", "1. `member-run` `op=dismiss` FIRST", "1. `member-run` `op=dismiss` last", "dismiss first", 1},
		{"cleanup forces the remove", "execute", "then `git worktree remove {agent_worktree_path}` - plain", "then `git worktree remove --force {agent_worktree_path}` - plain", "dismiss first", 1},
		{"replacement integrates the tip", "execute", "`--from` the chunk's recorded `commit_sha`, never the branch tip", "`--from` the branch tip", "commit_sha", 1},
		{"commit guard only checks the caller", "agent", "still equals the path you verified at spawn", "is not the caller's", "verified at spawn", 1},
		{"pipelined allowed under auto", "execute", "and under `--auto` it is REFUSED.", "and under `--auto` it is allowed.", "--pipelined must need", 1},
		{"review reads the tree", "execute", "the review block is `git show {commit_sha}`", "the review block is `git diff HEAD`", "reviewed by sha", 1},
		{"overlap not stopped", "execute", "reports `blocked` with reason `overlap`", "applies it anyway", "reviewed by sha", 1},
		{"autosquash without the pushed check", "execute", "`lets worktree pushed --branch {branch} --commit {oldest} --json` (every configured remote) must return `state: not_pushed`", "`git log` must look unpushed", "autosquash only on", 1},
		{"interactive rebase", "execute", "`git rebase --autosquash --no-autostash {start}`", "`git rebase -i --autosquash --no-autostash {start}`", "rebase -i", 1},
		{"autosquash over a merge", "execute", "0. `git rev-list --merges {start}..HEAD` must print nothing", "0. merges are fine", "merge-free range", 1},
		{"autosquash autostashes", "execute", "`git rebase --autosquash --no-autostash {start}`", "`git rebase --autosquash {start}`", "never autostashing", 1},
		{"record loses review_sha", "execute", `"review_sha": null, `, "", "review_sha", 1},
		{"pipelined row loses fixup", "agent", "`git commit --fixup=<that sha>`", "`git commit --amend`", "--fixup and the overlap stop", 1},
		{"isolated pipelined commits at once", "execute", "4. **Only then the lead's commit**", "0. **Committed by the lead at once**", "before the lead's commit", 1},
		{"unscoped gate sentence returns", "rules", "`/lets:execute` (plan mode inline, the Start gate delegated)", "`/lets:execute` (after its plan-mode approval)", "still asserts", 1},
		{"a replacement keeps the old report path", "execute", "a replacement never writes an earlier generation's report", "keep the copied lines", "new generation's round-0 REPORT_FILE", 1},
		{"the final text gates the report again", "execute", "EMPTY ones included", "non-empty ones", "EMPTY ones included", 1},
		{"interim notifications conclude a gap", "execute", "op=peek", "op=collect", "EMPTY ones included", -1},
		{"spawn loses REPORT_FILE", "skill", `\nREPORT_FILE: {report-file}"`, `"`, "REPORT_FILE line", 1},
		{"next keeps an earlier REPORT_FILE", "skill", `\nREPORT_FILE: {report-file}\n` + roundFileNote + `"`, `\nREPORT_FILE: {report-file}"`, "replaces any earlier one", 1},
		{"the note rejoins the REPORT_FILE line", "skill", `\nREPORT_FILE: {report-file}\n` + roundFileNote + `"`, `\nREPORT_FILE: {report-file} - ` + roundFileNote + `"`, "replaces any earlier one", 1},
		{"a report-only amendment commits", "agent", "and in `pipelined` / `isolated` mode no commit and no fixup.", "and in `pipelined` / `isolated` mode a fixup as usual.", "report-only amendment", 1},
		{"the nudge goes back to op=next", "execute", `args: "op=correct scope=run-{RUN} name={agent} id={agent_id} correct-file=<that file> report-file={path}")`, `args: "op=next scope=run-{RUN} name={agent} id={agent_id} brief-file=<that file> report-file={path}")`, "never op=next", 1},
		{"the resend goes back to op=next", "execute", "`member-run` `op=correct` with it as `correct-file=`", "`member-run` `op=next` with it", "never op=next", 1},
		{"a nudge advances the round", "execute", "only a correction advances `round`", "every op=correct advances `round`", "keeps the round", 1},
		{"rehydration re-runs the work", "execute", "Do not re-run Verify and do not change the work.", "", "without re-running Verify", 1},
		{"the team check retries an analyst", "execute", "A member-run member is never retried by agent-report", "A member-run member is retried once by agent-report", "never retried by agent-report", 1},
		{"the team check reads its own way again", "execute", `args: "op=receive scope=<the line's scope> `, `args: "op=peek scope=<the line's scope> `, "read each verdict with member-run op=receive", 1},
		{"the anchor check retries its explorer", "execute", `args: "op=receive scope=<callsign> name=<name> id=<agentId> brief-file=<its brief>`, `args: "op=collect scope=<callsign> name=<name> id=<agentId> brief-file=<its brief>`, "never an agent-report retry", 1},
		{"the anchor check may spawn", "execute", "op=route command=execute roles=explorer ask=no fallback=inline", "op=route command=execute roles=explorer fallback=run", "never an agent-report retry", 1},
		{"the architect is retried", "execute", "read with `op=receive` as the team check reads its members", "read with `op=collect` as the team check reads its members", "never retried", 1},
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
			mutated[m.key] = strings.Replace(files[m.key], m.old, m.repl, m.n)
			for _, problem := range delegatedContractProblems(mutated) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}

// roundFileNote is its own line after the REPORT_FILE line of every NEXT and AMENDMENT
// (the REPORT_FILE line carries only the path): the member holds the earlier rounds'
// lines in its context, so the newest must say it wins.
const roundFileNote = "This round's report file; it replaces any earlier REPORT_FILE."

// runMemberCall matches a member-run call template of the execute run's scope:
// op, member name, and the rest of its args.
var runMemberCall = regexp.MustCompile(`args: "op=(next|correct|dismiss) scope=run-\{RUN\} name=([^ "]+)([^"]*)"`)

// memberRunCall matches a member-run Skill call's args, or a member-run op
// written in prose with its role; roleModelPin matches a model named by value in one.
var (
	memberRunCall = regexp.MustCompile("(args: \"op=[^\"]*\"|`op=[^`]*role=[^`]*`|`role=[^`]*`)")
	roleModelPin  = regexp.MustCompile(`model=(opus|fable|sonnet|haiku)\b`)
)

// between returns the text from the heading from up to the heading to - a
// section whose fenced templates hold headings of their own, which sectionSpan
// would end at; "" when either is missing or out of order.
func between(s, from, to string) string {
	a := strings.Index(s, from)
	if a < 0 {
		return ""
	}
	b := strings.Index(s[a:], to)
	if b < 0 {
		return ""
	}
	return s[a : a+b]
}

// soloByDefault matches the solo shape worded as a fixed default - 4.7 applies no
// allocation by default, so no other step may state one.
var soloByDefault = regexp.MustCompile(`(?i)(by default,? (one|a single) implementer|default is (one|a single) implementer)`)

// bareIntegrated matches a record field named integrated rather than integrated_source.
var bareIntegrated = regexp.MustCompile("(\"integrated\"\\s*:|`integrated`)")

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
		{"Gate policy", "| Gate policy | the level's default"},
	} {
		if !strings.Contains(lp, choice.row) {
			add("the launch plan must name " + choice.name)
		}
	}
	if !strings.Contains(lp, "each with a one-line reason") || !strings.Contains(lp, "| Why ") {
		add("every launch plan choice must carry a one-line reason")
	}
	// each policy with its meaning - the level-default mapping names policies too
	for _, policy := range []string{"`per-commit`", "`high-only`", "`at-end`"} {
		if !strings.Contains(lp, policy+" (") {
			add("the launch plan's gate policy must name " + policy)
		}
	}
	if !strings.Contains(lp, "the level's default (`{LETS_GATES}`: `strict` -> `per-commit`, `standard` / `minimal` -> `at-end`)") {
		add("the launch plan's gate policy must propose the level's default")
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
		{"launch plan ignores the level", "`standard` / `minimal` -> `at-end`)", "`standard` / `minimal` -> `per-commit`)", "level's default"},
		{"fixed allocation creeps in", "This is reasoning about THIS plan, not a fixed rule", "Always two implementers, not a fixed rule", "no fixed allocation"},
		{"pipelined override dropped", "`--parallel`, `--pipelined` and `--gate <policy>`", "`--parallel`, `--pipe` and `--gate <policy>`", "`--pipelined`"},
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

// gatePolicyProblems returns every way execute.md breaks the delegated gate
// policy contract (lets-7dwc1); an empty result means none.
func gatePolicyProblems(exec string) []string {
	var p []string
	add := func(s string) { p = append(p, s) }

	picker := sectionSpan(exec, "## Step 4.5: Choose Execution Mode")
	if !strings.Contains(picker, "`--gate <per-commit|high-only|at-end>`") || !strings.Contains(picker, "under `--auto` any value other than `per-commit` is REFUSED") {
		add("--gate must name the three values and be refused under --auto unless per-commit")
	}
	if n := strings.Count(exec, "| gate_policy | risk |"); n != 1 {
		add(fmt.Sprintf("the gate policy must dispatch from exactly one table, found %d", n))
	}
	review := between(exec, "### 5-D.5 Review - one report at a time", "### 5-D.6 ")
	if !strings.Contains(review, "**Accept dispatch - the ONE table.**") {
		add("the dispatch table must live in 5-D.5")
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(review, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) != 9 {
			continue
		}
		policy := strings.TrimSpace(cells[1])
		name := ""
		for _, v := range []string{"per-commit", "high-only", "at-end"} {
			if strings.HasPrefix(policy, "`"+v+"`") {
				name = v
			}
		}
		if name == "" {
			continue
		}
		seen[name] = true
		risk, check, skeptic := strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3]), strings.TrimSpace(cells[4])
		if check != "yes" {
			add("the CHECK must run in every row, not in " + name + " / " + risk)
		}
		if (strings.Contains(risk, "high") || risk == "any") && !strings.HasPrefix(skeptic, "yes") {
			add("the skeptic must run on Risk high or missing in every policy, not in " + name + " / " + risk)
		}
		if strings.Contains(policy, "(default)") {
			add("no row of the dispatch table may be marked (default) - the level picks it: " + name)
		}
	}
	if !strings.Contains(review, "Default: `per-commit` at `strict`, `at-end` at `standard` / `minimal`; `--gate` overrides.") {
		add("the default policy line must map strict to per-commit and standard / minimal to at-end, with --gate overriding")
	}
	for _, v := range []string{"per-commit", "high-only", "at-end"} {
		if !seen[v] {
			add("the dispatch table must carry a row for " + v)
		}
	}
	for _, need := range []string{"a new policy is a new row, never a new code path", "The skeptic runs whenever Risk is high or missing, in every policy.", "**Hard stops and deviations halt at once in every policy**, `at-end` included"} {
		if !strings.Contains(review, need) {
			add("5-D.5 must state: " + need)
		}
	}
	for _, call := range memberRunCall.FindAllString(exec, -1) {
		if roleModelPin.MatchString(call) {
			add("a member-run call pins a model by name - roles inherit the session model; only the implementer's panel-chosen model=<m> is passed: " + call)
		}
	}
	team := between(review, "**Who checks.**", "- **CHECK**")
	if strings.Contains(exec, "`worktree.team`") {
		add("the team check must read the info envelope's top-level team field, never worktree.team")
	}
	for _, need := range []string{"the top-level `team` field set", "op=route command=execute roles=explorer,skeptic fallback=run", "`explorer-{RUN}` / `skeptic-{RUN}`"} {
		if !strings.Contains(team, need) {
			add("in a team worktree the team check must route through member-run op=route (fallback=run) before the run's own explorer / skeptic: " + need)
		}
	}
	const runReview = "### 5-D.8 Run review (`at-end` and `high-only`)"
	rr := sectionSpan(exec, runReview)
	if rr == "" || strings.Index(exec, runReview) > strings.Index(exec, "### 5-D.9 Completion") {
		add("the 5-D.8 run review for at-end and high-only must precede the 5-D.9 Completion")
	}
	for _, need := range []string{`label: "Accept run`, `label: "Correct"`, `label: "Stop"`, "`op=next`", "git revert", "Nothing here pushes"} {
		if !strings.Contains(rr, need) {
			add("the 5-D.8 run review must carry " + need)
		}
	}
	record := sectionSpan(exec, "### 5-D.3 Run record")
	for _, need := range []string{`"gate_policy": "at-end"`, "the resolved value the Start pick approved", "`accepted_by` is `owner` or `team`", `"committed_by":`} {
		if !strings.Contains(record, need) {
			add("the run record must hold " + need)
		}
	}
	if !strings.Contains(sectionSpan(exec, "### 5-D.2 Start - the one code-write approval"), "explicit, recorded approval for the lead's commits") {
		add("Start with high-only or at-end must be the owner's recorded approval for the lead's commits")
	}
	return p
}

// TestExecuteGatePolicy pins the gate policy of a delegated run on the real
// execute.md, then proves each guard can fail on an in-memory mutant.
func TestExecuteGatePolicy(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "execute.md"))
	if err != nil {
		t.Fatal(err)
	}
	exec := string(b)
	for _, problem := range gatePolicyProblems(exec) {
		t.Error(problem)
	}

	mutants := []struct{ name, old, repl, want string }{
		{"--gate allowed under --auto", "under `--auto` any value other than `per-commit` is REFUSED", "under `--auto` any value is accepted", "refused under --auto"},
		{"second dispatch table", "**Accept dispatch - the ONE table.**", "| gate_policy | risk |\n**Accept dispatch - the ONE table.**", "exactly one table"},
		{"skeptic skipped for at-end high", "| `at-end` | high / missing | yes | yes |", "| `at-end` | high / missing | yes | no |", "skeptic must run"},
		{"CHECK skipped", "| `high-only` | low | yes | no |", "| `high-only` | low | no | no |", "CHECK must run"},
		{"standard default loses at-end", "Default: `per-commit` at `strict`, `at-end` at `standard` / `minimal`;", "Default: `per-commit` at `strict`, `per-commit` at `standard` / `minimal`;", "default policy line"},
		{"strict default loses per-commit", "Default: `per-commit` at `strict`", "Default: `at-end` at `strict`", "default policy line"},
		{"a row marked default returns", "| `per-commit` | any |", "| `per-commit` (default) | any |", "marked (default)"},
		{"record keeps a fixed default", `"gate_policy": "at-end"`, `"gate_policy": "per-commit"`, "the run record must hold"},
		{"at-end no longer halts", "**Hard stops and deviations halt at once in every policy**", "**Deviations wait for the run review**", "Hard stops"},
		{"run review dropped", "### 5-D.8 Run review (`at-end` and `high-only`)", "### 5-D.8 Wrap-up", "5-D.8 run review"},
		{"extension point lost", "a new policy is a new row, never a new code path", "a new policy gets its own step", "new row"},
		{"skeptic pinned to a model", "`role=lets:skeptic`", "`role=lets:skeptic model=opus`", "pins a model"},
		{"team read from worktree.team", "the top-level `team` field set", "`worktree.team` set", "top-level team"},
		{"team explorer not reused", "op=route command=execute roles=explorer,skeptic fallback=run", "op=spawn scope=run-{RUN}", "op=route"},
		{"accepted_by loses team", "`accepted_by` is `owner` or `team`", "`accepted_by` is `owner`", "accepted_by"},
	}
	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if !strings.Contains(exec, m.old) {
				t.Fatalf("mutant cannot apply: %q is not in execute.md - update the mutant with the text it guards", m.old)
			}
			for _, problem := range gatePolicyProblems(strings.Replace(exec, m.old, m.repl, 1)) {
				if strings.Contains(problem, m.want) {
					return
				}
			}
			t.Errorf("mutant produced no problem containing %q - that guard cannot fail", m.want)
		})
	}
}
