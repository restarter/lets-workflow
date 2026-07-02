---
description: Backlog review and cleanup - multi-agent backlog review, quick no-agent pulse (--fast), or interactive triage cleanup
argument-hint: "[review|cleanup] [--fast] [--workflow]"
---

# Backlog

Backlog management in three modes. Review (heavy) launches an explorer + parallel domain agents for multi-perspective backlog insights. `--fast` is a quick no-agent backlog pulse (orchestrator-only, the lightweight sibling of Review). Cleanup is fast interactive triage - no agents.

**Quick no-agent backlog pulse: `/lets:backlog --fast`. For HOW to build something, use `/lets:plan`. For an open question or decision about the project, use `/lets:opinion`.**

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```
/lets:backlog            # Ask which mode, then go
/lets:backlog review     # Multi-agent backlog review (skip menu)
/lets:backlog cleanup    # Interactive triage cleanup (skip menu)
/lets:backlog review --workflow  # Run the Review fan-out + aggregate off-context via a Dynamic Workflow
```

## Step 0: Choose Mode

**Parse the argument first:** strip a `--fast` token (sets fast mode - no agents) and a `--workflow` token (sets workflow mode - Review only) if present; the remaining token is the mode keyword. Precedence/compat:
- `--fast` selects the orchestrator-only **Fast mode** (a quick no-agent pulse) regardless of the `review`/no-arg keyword. `--fast` + `--workflow` -> `--fast` wins (orchestrator-only beats off-context fan-out); note it in one line.
- `--fast` passed with `cleanup` is ignored with a one-line note (Cleanup is already no-agent triage).
- `--workflow` passed with `cleanup` is ignored with a one-line note (Cleanup has no agents).

If an argument is provided, parse it: `--fast` -> Fast mode (below); `review` -> Review Backlog Mode; `cleanup` -> Cleanup Mode.

If no argument, use AskUserQuestion:

```
AskUserQuestion(
  questions=[{
    question: "What do you want to do with the backlog?",
    header: "Backlog",
    options: [
      { label: "Review backlog", description: "Agents analyze project state, generate ideas and surface gaps (~2-3 min)" },
      { label: "Quick pulse", description: "No agents - fast orchestrator-only scan and conversation" },
      { label: "Cleanup", description: "Triage stale tasks - close, merge, reprioritize" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Review backlog** -> Review Backlog Mode (below)
- **Quick pulse** -> Fast mode (below)
- **Cleanup** -> Cleanup Mode (below)

---

## Fast Mode (--fast)

No agents. The orchestrator gathers backlog context directly and enters conversation - a quick pulse, the lightweight sibling of the agent-backed Review below (same house `--fast` convention as `/lets:plan --fast`: no flag = subagents, `--fast` = orchestrator-only).

### Step F1: Gather Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
```

```lets-tracker
stats
list-by-status status=open limit=30
list-by-status status=in_progress
```

```bash
git log --oneline -15
```

Also use the Grep tool (not bash grep) to scan for tech-debt signals:
```
Grep(pattern="TODO|FIXME|HACK|XXX", path="$LETS_PROJECT_ROOT", output_mode="content", head_limit=20)
```

If an area/epic argument was passed alongside `--fast`:
- the tracker's `search` verb on {argument} (if no results, fall back to `list-by-status` and scan titles)
- Grep/Glob for related code

### Step F2: Open with Observations

Based on gathered context, present 3-5 proactive observations:

```
## Backlog Pulse

Based on what I see in the project:

1. **{observation}** - {why it matters, 1-2 sentences}
2. **{observation}** - {why it matters}
3. **{observation}** - {why it matters}

**Question:** {probing question that opens discussion}
```

Categories to draw observations from:
- Gaps: areas with activity but no tasks
- Stale work: tasks open long with no progress
- Patterns: recurring themes across tasks/commits
- Quick wins: small improvements with high impact
- Risks: things that could break or become debt

### Step F3: Interactive Dialog

After each user response:
1. **Acknowledge** - brief, no fluff
2. **Build** - add insight or connection
3. **Probe** - ask next question that goes deeper

If the user wants the heavier multi-agent pass: "Want a deeper backlog review? `/lets:backlog review` launches expert agents."

### Step F4: Capture & Exit

If active task:

```lets-tracker
comment-add task=<task-id> body="Backlog pulse: {key takeaways, 2-3 items}"
```

If ideas emerged that deserve tasks, offer to create them (use the `create-task` skill, user approval per task).

---

## Review Backlog Mode

The one heavy mode: explorer scouts the project, parallel domain agents ideate over the backlog, results aggregate.

### Execution path (standard vs workflow)

The scout (Phase 1) and agent selection (Phase 2) ALWAYS run in-context. Only Phases 3-4 (fan-out + aggregate) can move off-context via a Dynamic Workflow - `--workflow` is a transparent perf lever, same findings.

- If `--workflow` was parsed -> use the workflow path (go to `## Workflow Mode` after Phase 2). An explicit `--workflow` always wins.
- Else if the `Workflow` tool is **not** available this session -> silently use the standard Task-based path. Do NOT show the option.
- Else (tool available, no explicit flag) -> ask via `AskUserQuestion`:

```
AskUserQuestion(
  questions=[{
    question: "How should I run the backlog review?",
    header: "Run mode",
    options: [
      { label: "Standard (Recommended)", description: "Agents via Task tool - full per-agent backlog ideas visible inline" },
      { label: "Workflow", description: "Dynamic Workflow - off-context fan-out, only the clustered ideas return" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:** Standard -> standard path (Phase 3 below); Workflow -> treat as if `--workflow` was set (go to `## Workflow Mode` after Phase 2); Other -> honor a named path else default to Standard.

### Phase 1: Explorer - Gather Context

Launch explorer to build a context profile.

```
Task(
  subagent_type="lets:explorer",
  prompt="ultrathink

BRAINSTORM SCOUT MODE. In this mode, your mapping role extends to surfacing signals and gaps - not just structure. Gather project context for a brainstorm session.

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

GOAL: Build a Project State Profile for brainstorming. We need to understand what the project is, what work has been done, what's planned, and where gaps exist.

AVAILABLE CONTEXT SOURCES (read what's relevant, skip what's not):

1. BACKLOG STATE
   Run: bd stats
   Run: bd list --status=open -n 50
   Run: bd list --status=in_progress
   Run: bd list --status=done -n 20
   Purpose: understand task distribution, priorities, what's active

2. TASK DETAILS (selective - pick 5-10 most interesting tasks)
   Run: bd show <task-id>
   Run: bd comments <task-id>
   Purpose: understand context, decisions, blockers on key tasks

3. RECENT SESSION SUMMARIES
   Read: .lets/sessions/*.md (most recent 3-5 files by name)
   Purpose: what was worked on recently, what momentum exists

4. CODEBASE SIGNALS
   Grep for: TODO, FIXME, HACK, XXX across source files
   Read: CLAUDE.md (project structure and architecture)
   Purpose: understand technical debt signals and project shape

5. RECENT GIT ACTIVITY
   Run: git log --oneline -20
   Run: git log --oneline --since='2 weeks ago' --format='%s'
   Purpose: what areas are actively changing

BUDGET: Be efficient. Read backlog first (sources 1-2), then decide which of sources 3-5 add value. If backlog is rich (50+ tasks with comments), you can skip session summaries. If backlog is sparse, lean more on git and codebase signals.

Keep output concise - max ~500 words. This profile will be passed to multiple agents.

OUTPUT FORMAT - Project State Profile:

## Project State Profile

### Project Shape
{what this project is, tech stack, size - from CLAUDE.md}

### Backlog Summary
- Open: {N} tasks
- In progress: {N}
- Done recently: {N}
- By area/label: {breakdown if labels exist}

### Active Momentum
{what's being worked on now, what sessions focused on recently}

### Hot Areas
{parts of codebase with most recent activity - files, modules}

### Gaps & Signals
- Missing coverage: {areas with no tasks but active code}
- Stale tasks: {tasks open for a long time with no activity}
- TODOs in code: {count and themes}
- Recurring themes: {patterns across tasks/comments/sessions}"
)
```

<!-- Split seam: the heavy Review path and the no-agent Quick pulse both live in THIS command now (Quick = Fast Mode above). Degrade via a pointer to --fast, do NOT inline-duplicate the F1-F4 body here. -->

#### Explorer Failure Guard

If explorer fails, times out, or returns no structured profile:

> "Explorer couldn't gather context for multi-agent review. For a quick no-agent pulse instead, run `/lets:backlog --fast`."

Stop here (do NOT auto-run agents - the no-agent pulse is `/lets:backlog --fast`, Fast Mode above).

#### Thin Profile Guard

After explorer returns, check task count from profile's Backlog Summary section.

If < 5 open tasks reported:

> "Project has very few tasks - not enough context for multi-agent analysis. For a quick no-agent pulse instead, run `/lets:backlog --fast`."

Stop here (do NOT auto-run agents - the no-agent pulse is `/lets:backlog --fast`, Fast Mode above).

### Phase 2: Select Brainstorm Agents

Command analyzes the explorer profile and selects agents. Do NOT ask the explorer to recommend agents - explorer maps, command decides.

**Selection logic:**

Scan the profile for signals and match to agents:

| Signal in Profile | Select Agent |
|-------------------|-------------|
| API/endpoint/backend mentions, backend-heavy code | backend |
| UI/frontend/component mentions | frontend |
| Auth/security/credentials/secrets mentions | security |
| Database/schema/migration mentions | database |
| Docker/CI/deploy/infrastructure mentions | devops |
| Test/coverage/quality mentions | qa |
| Docs/README/onboarding mentions | docs |
| Module boundaries, coupling, architecture patterns | architect |
| Many tasks, mixed priorities, scope questions | pragmatist |

**Rules:**
1. Always include pragmatist (the forced agent for backlog review)
2. Select agents matching signals found in profile
3. More signals = more agents. No hard cap - include every agent that has a clear signal
4. If only 1-2 signals matched, add architect + pragmatist for broader perspective

**Confirmation gate:** If planning to launch more than 10 agents:

```
AskUserQuestion(
  questions=[{
    question: "Planning {N} review agents. That's a lot - confirm?",
    header: "Confirm",
    options: [
      { label: "Launch all", description: "{N} agents, broad coverage" },
      { label: "Reduce", description: "Suggest fewer, more focused agents" }
    ],
    multiSelect: false
  }]
)
```

**Agent eligibility:**

| Agent | Eligible | Brainstorm Strength |
|-------|----------|-------------------|
| architect | Yes | System gaps, module boundaries, structural opportunities |
| pragmatist | Yes | ROI, what to cut, what to prioritize |
| backend | Yes | API gaps, performance bottlenecks |
| frontend | Yes | UX gaps, component patterns |
| security | Yes | Security debt, missing protections |
| database | Yes | Schema evolution, query patterns |
| devops | Yes | CI/CD gaps, infrastructure debt |
| qa | Yes | Testing gaps, coverage holes |
| docs | Yes | Documentation debt, onboarding gaps |
| compliance | No | Rule-checking, not ideation |
| git-historian | No | Historical analysis, not forward-looking |
| implementer | No | Has write access, wrong mode |
| explorer | No | Already used in phase 1 |
| actor | Yes | External perspective via loaded persona (user must provide source) |

**Actor note:** If actor is selected, invoke `Skill(skill: "lets:actor-fetch-personality", args: "<personality-source-from-user>")` to fetch the personality. Pass the returned `PERSONALITY:` block in the actor's Task prompt only.

Show selection before launching (no user gate - review = momentum):

```
## Backlog Review Panel

Based on project context, selected {N} experts:
1. {agent} - {signal that triggered selection}
2. {agent} - {signal}
...

> Subagents have a separate rate limit - no cost to your conversation.

Launching...
```

**If the workflow path was chosen** (`--workflow` or the interactive pick): do NOT run Phase 3 here - go to `## Workflow Mode` below. It builds args from the Phase 1 profile + the Phase 2 agents, invokes the asset, and resumes at Phase 4 with the returned aggregate. Otherwise continue with Phase 3 (standard Task fan-out).

### Phase 3: Launch Brainstorm Agents (Parallel)

CRITICAL: Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent:

<!-- tracker: the prompt's "Use bd commands" line is beads-only BY DESIGN - backlog's orchestrator path is neutralized (lets-tracker verbs), but the explorer/brainstorm SUBAGENTS read bd directly because they don't receive the adapter file. This is the documented Cat-C carve-out, NOT a violation of "subagents never call tracker verbs" (see lets-rules "Tracker Adapters"). On a non-beads project these subagent reads are unavailable. -->

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: brainstorm (review backlog)

Review the project's BACKLOG of tasks from your area of expertise. Surface gaps in WHAT'S BEING TRACKED: missing themes, priority imbalances, areas with no tasks but clear need from your domain.

You are NOT reviewing code, counting duplications, finding stale files, or hunting bugs in source. The PROJECT STATE PROFILE below is your primary source. Use bd commands (bd show, bd comments) for task details. Code reads are allowed only as evidence for a backlog observation ("this critical area has no tasks tracking it"), never as primary investigation.

PROJECT RULES (from CLAUDE.md):
{CLAUDE.md content, first 100 lines - architecture decisions and structure}

PROJECT STATE PROFILE:
{explorer output from Phase 1}

INSTRUCTIONS:
- Anchor on the BACKLOG STATE from the profile above (open/in-progress tasks, recent closures, gaps)
- Generate 3-7 backlog-level insights from your domain lens: what tasks SHOULD exist but don't? what existing tasks need re-prioritization, scope changes, or to be split/closed?
- Each insight must be backlog-actionable: not "improve testing" but "backlog has 0 tasks tracking auth flow regressions despite 4 recent auth changes, propose creating one"
- Prioritize by impact (high/medium)
- Connect to existing tasks: "task X could be extended to cover Y", "task A and B overlap, suggest merging"
- If you reference code, frame as backlog evidence ("Pattern X exists in 5 files but no task tracks consolidation"), never as code review
- Be opinionated: rank and recommend, don't just list

OUTPUT FORMAT:

## {Your Domain} Backlog Review

### Missing from Backlog
1. **{theme or task that should exist}** [Impact: high/medium]
   {why this gap matters from your domain}
   {suggested bd create command or task brief}

### Existing Tasks Needing Adjustment
- **{task-id}** ({title}): {what to change: bump priority, expand scope, split, deprioritize, close as obsolete, with reason}

### Backlog Themes ({domain} lens)
{1-2 observations about distribution, gaps, drift from project goals: what the backlog reveals about project priorities}"
)
```

Agents define their own brainstorm focus in their `## Modes` section - no mandatory context table needed.

### Phase 4: Aggregate & Present

> **Keep in sync (--workflow):** the off-context clustering lives in `skills/backlog-workflow/backlog.workflow.js` - `buildThemes` (semantic merge -> impact-sort with `agents[]`) and `clusterIdeas` (title-only fallback). Any change to the dedupe/merge/impact-sort here MUST be mirrored there, and vice versa. (No unit test pins this - the runtime blocks clean import; keep-in-sync discipline + a live smoke test are the guards.)

After all agents respond:

1. Group ideas by impact (high first)
2. Deduplicate: if two agents suggest the same area, merge and note both perspectives
3. Separate: Top Ideas / Gaps / Quick Wins

```
## Backlog Review Results

{N} ideas from {M} experts.

### High Impact
1. **{idea}** ({agent}) - {one-liner}
2. **{idea}** ({agents if merged}) - {one-liner}
...

### Medium Impact
1. **{idea}** ({agent}) - {one-liner}
...

### Gaps Identified
- {gap} ({agent})
...

### Quick Wins
- {quick win} ({agent})
...
```

### Phase 5: Interactive Dialog

```
AskUserQuestion(
  questions=[{
    question: "What catches your eye?",
    header: "Backlog",
    options: [
      { label: "Explore an idea", description: "Dig deeper into a specific idea or insight" },
      { label: "Create tasks", description: "Turn selected ideas into backlog tasks" },
      { label: "Done", description: "Enough for now" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Explore an idea** -> ask which one, enter conversation loop (acknowledge, build, probe)
- **Create tasks** -> ask which ideas, use the `create-task` skill with user approval per task
- **Done** -> Phase 6
- **Free text** -> treat as exploring a specific idea

Dialog continues until user signals done.

### Phase 6: Capture & Exit

If active task:

```lets-tracker
comment-add task=<task-id> body="Backlog review: {N} ideas from {M} agents.
Top ideas: {top 2-3 titles}
Tasks created: {list or 'none'}"
```

If exploration produced a clear task idea but none created, ask:
"Want me to create a task for this?" (plain text, not in LETS box).

---

## Cleanup Mode

No agents. Direct interactive triage of stale/messy backlog items.

### Step C1: Load Backlog

```lets-tracker
stats
list-by-status status=open limit=50
list-by-status status=closed limit=20
label
```

### Step C2: Present Cleanup Targets

Analyze loaded data and group:

```
## Cleanup Targets

### Stale Tasks (open with no recent activity - estimate from the tracker's list output)
- **{title}** (`task-id`) - {status/priority info}
...

### Potential Duplicates
- **{title A}** (`id`) ~ **{title B}** (`id`) - similar scope
...

### Missing Labels/Priority
- **{title}** (`id`) - no labels, priority P4
...

### Done but Not Closed
- **{title}** (`id`) - status done
...

### Orphan Tasks (no labels/epic grouping)
- **{title}** (`id`) - no labels, suggest: epic:{suggestion based on title}
...

### Unassigned Tasks (open, no assignee)
- **{title}** (`id`) - P{N}, suggest assignee based on domain
...
```

If no issues found in a category, skip it.
If backlog is clean, say so and suggest `/lets:backlog review` or `/lets:backlog --fast` instead.

### Step C3: Interactive Triage

For each group, walk through items and ask:

```
AskUserQuestion(
  questions=[{
    question: "{task title} ({task-id}). What to do?",
    header: "Triage",
    options: [
      { label: "Close", description: "No longer needed" },
      { label: "Keep", description: "Still relevant, skip" },
      { label: "Reprioritize", description: "Change priority" }
    ],
    multiSelect: false
  }]
)
```

For closing (tracker `close` - a state change, HARD-FAIL loud if it can't run):
- close the task
- If closing as duplicate: `comment-add` on `<other-id>` ("Absorbed from {closed-id}: {title}") then `close <id>`

For reprioritizing: tracker `set-field task=<id> priority=<0-4>` (beads: `bd update <id> --priority=<N>`); on an adapter without a priority binding, tell the user and skip.

For orphan tasks (no labels), suggest a label and ask:
- tracker `label` - add `epic:{name}`

For unassigned tasks, suggest an assignee based on task domain and ask:
- tracker `assignee` - assign `{name}`

All mutations require user approval.
Continue until all groups processed or user says "enough".

### Step C4: Summary & Exit

```
## Cleanup Summary

- Closed: {N} tasks
- Reprioritized: {M} tasks
- Labeled: {L} tasks
- Assigned: {A} tasks
- Skipped: {K} tasks
```

If active task:

```lets-tracker
comment-add task=<task-id> body="Cleanup: closed {N}, reprioritized {M}, labeled {L}, assigned {A}"
```

---

## Workflow Mode (--workflow)

Runs when the workflow path was chosen (Review mode only). Replaces Phase 3 (Task launch) + Phase 4's in-context aggregation: the ideate fan-out + cluster happen inside the Dynamic Workflow so per-agent backlog ideas never enter this conversation - only the clustered result returns. Phases 1, 2, 5, 6 are unchanged and stay in-context.

### W1: Preflight

If the `Workflow` tool is NOT available this session, STOP and tell the user:

> `--workflow` needs Claude Code >= 2.1.154 on a paid plan (Dynamic Workflows is a research preview). Re-run without `--workflow` to use the standard agent flow.

Do NOT silently fall back.

### W2: Build args

Use the agents SELECTED in Phase 2 (do not re-select). **Exclude `lets:actor`** - the skeleton does not inject the `PERSONALITY:` block; if actor was selected, drop it and run with the rest (tell the user), or run the standard path if actor was the only non-default expert.

```
{
  profile: "{explorer Project State Profile from Phase 1}",
  agents: [ { name: "pragmatist" }, { name: "architect" }, ... ],   // short names, no "lets:" prefix
  projectRoot: "{LETS_PROJECT_ROOT from LETS Config}",
  claudeMd: "{CLAUDE.md content, first 100 lines}"
}
```

Pass `args` as a real JSON value (the skeleton defensively parses a JSON string too).

### W3: Invoke the workflow

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/backlog-workflow/backlog.workflow.js", args: <W2 args> })
```

The skeleton is a committed asset (`skills/backlog-workflow/backlog.workflow.js`) - NOT reproduced inline. See `skills/backlog-workflow/SKILL.md` for the `args` contract and stage flow (ideate -> cluster). **Runs in the BACKGROUND** - the tool returns a `runId`, not the result; a `<task-notification>` arrives on completion and the orchestrator resumes. Optionally tell the user "Backlog review running in the background - {N} agents".

### W4: Rejoin (on completion)

The returned aggregate `{ ideas, counts }` is the only thing that enters context (per-agent idea dumps stayed off-context). `ideas` are already **semantically clustered** (themes merge ideas that make the same point across agents) + impact-sorted; each carries `agents[]` attribution - **`agents[]` length > 1 means multiple agents converged on that theme: lead with those as the high-signal findings and note the agent count**. `counts.clustering` reports which path ran (`semantic` = cluster agent; `fallback_title` = title-only fallback after a cluster-agent error; `trivial` = <=1 idea). Render Phase 4 from `ideas[]` exactly as the standard path does - the schema carries `category` (`missing`/`adjust`/`theme`) + `impact` only, so Phase 4's "Gaps Identified" and "Quick Wins" buckets are derived by judgment at render time (`category:missing` -> Gaps candidates; low-effort high-value items -> Quick Wins), same as the standard path derives them from the agents' markdown. Then continue Phase 5 (dialog) + Phase 6 (capture). If `counts.agents_responded === 0` (all ideators errored), surface that and offer a re-run; do not render a fabricated result. If the workflow failed or returned nothing, offer the standard Task-based path.

---

## Output

### After Review Backlog mode:

```
┌─ LETS ─────────────────────────┐
│  Plan a task?  /lets:plan      │
│  Start work?   /lets:start     │
└────────────────────────────────┘
```

### After Cleanup:

```
┌─ LETS ─────────────────────────┐
│  Pulse?  /lets:backlog --fast  │
│  Start?  /lets:start           │
└────────────────────────────────┘
```

## Rules

- All task mutations (create, close, update) require user approval
- No agents in Cleanup mode
- `--fast` (Fast Mode) is orchestrator-only - a quick no-agent backlog pulse (house `--fast` convention); `--fast` wins over `--workflow`, and is ignored with `cleanup`
- Explorer + agents only in Review Backlog mode
- All agents launched in a SINGLE message (parallel)
- If explorer fails or returns thin profile (<5 tasks), surface and point to `/lets:backlog --fast` for a quick no-agent pulse
- Explorer profile max ~500 words (passed to multiple agents)
- `--workflow` (Review mode only) runs Phases 3-4 off-context in a Dynamic Workflow - a transparent perf lever, same findings as the standard path; Cleanup ignores `--workflow`
- Respond in user's language
