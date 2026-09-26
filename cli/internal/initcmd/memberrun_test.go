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
	for _, field := range []string{"shape", "gate_policy", "members_scope", "risk", "base", "agent_branch", "agent_worktree_path", "picked_sha", "patch_path", "accepted_by", "committed_by", "integrated_source", "allowlist_amendments"} {
		if !strings.Contains(record, `"`+field+`":`) {
			add("the run record must list " + field)
		}
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
	for _, need := range []string{"`unknown_pre_upgrade`", "`members_scope`", "members-run-{RUN}.json", "never messaged"} {
		if !strings.Contains(recovery, need) {
			add("5-D.7 must treat a pre-upgrade record's agents as gone and never message them: " + need)
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
	if integrateAt < 0 || acceptAt < 0 || integrateAt > acceptAt || !strings.Contains(review, "lets integrate --revert --patch {patch_path}") {
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
	if step3 := sectionSpan(skill, "## Step 3: Next / Correct"); !strings.Contains(step3, "`agent_id`: set -> the `to:` below is that id") || !strings.Contains(skill, "puts the `agent_id` where it says `{agent}`") {
		add("member-run Step 3 must send to the member's agent_id when it is set")
	}
	if !strings.Contains(sectionSpan(skill, "## Step 4: Dismiss"), "`TaskStop(task_id=\"{agent}\")` - the `agent_id` when set") || !strings.Contains(sectionSpan(skill, "## Step 2: Spawn"), "--agent-id {id}") {
		add("member-run must record an isolated member's agent id and stop it by that id")
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
		{"files audit drops bare-name resolution", "execute", "a bare name (no `/`) resolves", "a bare name (no `/`) is refused", "resolve paths before it refuses"},
		{"bare name always inherits", "execute", "a bare name (no `/`) resolves, in this order: to the directory of the previous path on the same Files line only when the file exists there at BASE or the task creates it there", "a bare name (no `/`) inherits the directory of the previous path on the same Files line", "resolve paths before it refuses"},
		{"unresolvable name refuses", "execute", "resolves ambiguously, is a warning", "resolves ambiguously, refuses delegation", "resolve paths before it refuses"},
		{"removed-symbol check refuses", "execute", "it never refuses delegation", "it refuses delegation", "warning only"},
		{"anchor check adds an option", "execute", "adds no gate and no option", "adds an option", "anchor check"},
		{"brief loses CI CHECKS", "execute", "\nCI CHECKS: ", "\nCHECKS: ", "CI CHECKS:"},
		{"plan.workflow.js drops Risk in review", "planWorkflowJS", "every task with a commit point states **Risk:** high|low", "every task with a commit point states a risk", "planReviewPrompt"},
		{"plan.md drops the Risk template line", "plan", "**Risk:** {high|low}", "**Risk:** {level}", "Risk field"},
		{"dispatch respawns every chunk", "execute", `args: "op=next scope=run-{RUN}`, `args: "op=spawn scope=run-{RUN}`, "op=next"},
		{"solo becomes a default allocation", "execute", "the shape the 4.7 proposal usually picks", "by default one implementer", "usually picks"},
		{"Start gate loses Change the launch plan", "execute", `{ label: "Change the launch plan", `, `{ label: "Adjust", `, "Change the launch plan"},
		{"record drops members_scope", "execute", `"members_scope": "run-{RUN}",`, "", "members_scope"},
		{"integrated_source moves on a report", "execute", "it moves only after the lead's commit", "it moves on each report", "only after the lead's commit"},
		{"bare integrated field", "execute", `"integrated_source": null`, `"integrated": null`, "bare integrated"},
		{"addendum heading dropped", "execute", "`## Allowlist addendum`", "`## Extra paths`", "addendum"},
		{"pre-upgrade member messaged", "execute", "such a member is never messaged", "such a member is asked to report", "pre-upgrade"},
		{"parallel allowed alone", "execute", "it needs `--implementers` / `--team`", "it runs alone", "--parallel must need"},
		{"groups inferred", "execute", "The shape is never inferred", "The shape is inferred", "declare parallel groups"},
		{"isolated brief loses CALLER_TOPLEVEL", "execute", "CALLER_TOPLEVEL: {git rev-parse", "TOPLEVEL: {git rev-parse", "CALLER_TOPLEVEL"},
		{"no revert on reject", "execute", "lets integrate --revert --patch {patch_path}", "git checkout -- {patch_path}", "reverted on a reject"},
		{"cleanup without the tip check", "execute", "compare its `integrated_source` with the tip of its `agent_branch`", "look at its `agent_branch`", "integrated_source is its tip"},
		{"guard switches first", "agent", "At spawn only, before any edit,", "At spawn only, first `git switch -C <branch> {BASE}`, then before any edit,", "before git switch -C"},
		{"reset --hard creeps in", "agent", "NEVER push.", "NEVER push; git reset --hard on a bad start.", "reset --hard"},
		{"step 3 sends by name", "skill", "   - Read the entry's `agent_id`: set -> the `to:` below is that id, not the name.\n", "", "agent_id when it is set"},
		{"cleanup deletes before dismissing", "execute", "1. `member-run` `op=dismiss` FIRST", "1. `member-run` `op=dismiss` last", "dismiss first"},
		{"cleanup forces the remove", "execute", "then `git worktree remove {agent_worktree_path}` - plain", "then `git worktree remove --force {agent_worktree_path}` - plain", "dismiss first"},
		{"replacement integrates the tip", "execute", "`--from` the chunk's recorded `commit_sha`, never the branch tip", "`--from` the branch tip", "commit_sha"},
		{"commit guard only checks the caller", "agent", "still equals the path you verified at spawn", "is not the caller's", "verified at spawn"},
		{"pipelined allowed under auto", "execute", "and under `--auto` it is REFUSED.", "and under `--auto` it is allowed.", "--pipelined must need"},
		{"review reads the tree", "execute", "the review block is `git show {commit_sha}`", "the review block is `git diff HEAD`", "reviewed by sha"},
		{"overlap not stopped", "execute", "reports `blocked` with reason `overlap`", "applies it anyway", "reviewed by sha"},
		{"autosquash without the pushed check", "execute", "`lets worktree pushed --branch {branch} --commit {oldest} --json` (every configured remote) must return `state: not_pushed`", "`git log` must look unpushed", "autosquash only on"},
		{"interactive rebase", "execute", "`git rebase --autosquash --no-autostash {start}`", "`git rebase -i --autosquash --no-autostash {start}`", "rebase -i"},
		{"autosquash over a merge", "execute", "0. `git rev-list --merges {start}..HEAD` must print nothing", "0. merges are fine", "merge-free range"},
		{"autosquash autostashes", "execute", "`git rebase --autosquash --no-autostash {start}`", "`git rebase --autosquash {start}`", "never autostashing"},
		{"record loses review_sha", "execute", `"review_sha": null, `, "", "review_sha"},
		{"pipelined row loses fixup", "agent", "`git commit --fixup=<that sha>`", "`git commit --amend`", "--fixup and the overlap stop"},
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
		if name == "per-commit" && !strings.Contains(policy, "(default)") {
			add("per-commit must be the default policy")
		}
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
	for _, need := range []string{"`worktree.team`", "`op=next scope=<callsign> name=explorer|skeptic`", "Only when the team has no live one", "`explorer-{RUN}` / `skeptic-{RUN}`"} {
		if !strings.Contains(team, need) {
			add("in a team worktree the team check must reuse the standing team's live explorer / skeptic first: " + need)
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
	for _, need := range []string{`"gate_policy": "per-commit"`, "`accepted_by` is `owner` or `team`", `"committed_by":`} {
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
		{"per-commit not default", "| `per-commit` (default) |", "| `per-commit` |", "default policy"},
		{"at-end no longer halts", "**Hard stops and deviations halt at once in every policy**", "**Deviations wait for the run review**", "Hard stops"},
		{"run review dropped", "### 5-D.8 Run review (`at-end` and `high-only`)", "### 5-D.8 Wrap-up", "5-D.8 run review"},
		{"extension point lost", "a new policy is a new row, never a new code path", "a new policy gets its own step", "new row"},
		{"skeptic pinned to a model", "`role=lets:skeptic`", "`role=lets:skeptic model=opus`", "pins a model"},
		{"team explorer not reused", "`op=next scope=<callsign> name=explorer|skeptic`", "`op=spawn scope=run-{RUN}`", "standing team"},
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
