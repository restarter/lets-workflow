---
description: PREVIEW (experimental) - autonomous /lets:plan via a Dynamic Workflow. Shipped standalone to dogfood across projects and gather edits before folding the chain into native /lets:plan.
argument-hint: "[goal]"
---

# Autonomous Plan (PREVIEW workflow)

**PREVIEW / experimental.** Shipped standalone so the autonomous-planning Dynamic Workflow can be dogfooded across projects and gather edits before it's folded into native `/lets:plan --workflow` (tracked in lets-jsw00). The reusable artifact is `skills/plan-workflow/plan.workflow.js`; this command is a thin dispatcher. The interactive native `/lets:plan` is untouched. Expect rough edges - report them.

Autonomous whole-command planning: you give a GOAL + a RUBRIC up front; the workflow explores, proposes approaches, architects each, judges them against your rubric, evaluates the winner, and writes a plan - all off-context. You approve the plan at the end. **Steer-by-rubric + approve-at-end**, not gate-each-step.

> **IMPORTANT:** Deferred tools (e.g. `AskUserQuestion`) must be loaded and called as specified.

## Step 1: Goal + Rubric

Parse the goal from the argument (or ask "What are we planning?").

Gather the **RUBRIC** - the steering criteria that REPLACE the interactive picks of native `/lets:plan`. Ask the user (free text, or a short list):
- What does "good" look like here? (priority order: simplicity / performance / consistency / minimal-blast-radius / ...)
- Hard constraints (must-not-break, must-not-touch, compatibility).
- Scope boundaries (explicitly OUT).
- Success criteria.

**Do not proceed without a rubric** - it is the only steering signal the autonomous run receives. Echo the rubric back for confirmation before launching.

## Step 2: Context + selection

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/CLAUDE.md" 2>/dev/null | head -150
```

Detect the active task (`detect-task` skill) for context. From goal + rubric + CLAUDE.md derive:
- **focusAreas** - 1-5 non-overlapping areas the explorers should map (each `{name, hint}`).
- **judges** - default `pragmatist, backend, security`. **NEVER `architect`** (it designs the approaches - judging its own work is self-preferential bias).
- **experts** - winner evaluation panel, default `pragmatist` + any domain expert the goal implies.

## Step 3: Preflight + invoke

If the `Workflow` tool is NOT available -> STOP: "needs Claude Code >= 2.1.154 on a paid plan". This command IS the workflow (preview) - it has no Task-based fallback.

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/plan-workflow/plan.workflow.js", args: {
  goal: "...",
  rubric: "...",
  focusAreas: [ { name, hint }, ... ],
  judges: [ { name } ],
  experts: [ { name } ],
  taskContext: "{task title + description, or empty}",
  projectRoot: "{LETS_PROJECT_ROOT}",
  claudeMd: "{CLAUDE.md content from Step 2}"
}})
```

Pass `args` as a real JSON value (the script defensively parses a JSON string too). Runs in the **BACKGROUND** - the tool returns a `runId`; resume on the `<task-notification>`. Tell the user "Autonomous planning running - {N} explorers, then approaches/architect/judge/evaluate/plan".

## Step 4: On completion

Aggregate: `{ plan_markdown, delivered_approach, diverged_from_winner, divergence_reason, decision_log, winner, winner_name, approaches, eval_findings, counts }`.

- **Anti-silent-fail:** if `error` is set, or `plan_markdown` is null, or `counts.explorers === 0` -> surface the failure plainly, do NOT fabricate a plan; offer a re-run (optionally with an adjusted rubric).
- **Judge<->plan divergence:** if `diverged_from_winner` is true, say so PROMINENTLY - the judged `winner` was overridden by the Plan stage after the Evaluate findings (`divergence_reason`); the plan implements `delivered_approach`, not `winner`. Do not present `winner` as the delivered design.
- Save `plan_markdown`:
  ```bash
  LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
  mkdir -p "$LETS_PROJECT_ROOT/.lets/plans"
  # write to .lets/plans/$(date +%Y-%m-%d-%H%M)-<slug>.md
  ```
- Show the **decision log** (winner + judge votes/totals + rationale), the approach list, the eval findings, and the plan summary.
- User approves -> suggest `/lets:execute`. Wants changes -> adjust the rubric and re-run (`Workflow` `resumeFromRunId` caches completed stages while you iterate the script).

## Notes

- Agents the workflow spawns (`lets:explorer`, `lets:architect`, `lets:pragmatist`, domain experts) are EXISTING agents -> they resolve from the loaded plugin (no registration gotcha).
- Preview status: shipped standalone so the autonomous chain can be dogfooded across projects; it will be folded into native `/lets:plan --workflow` once it stabilizes (lets-jsw00). Until then, treat it as experimental and report rough edges.
