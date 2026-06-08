---
description: PREVIEW (experimental) - autonomous /lets:plan via a Dynamic Workflow. Shipped standalone to dogfood across projects and gather edits before folding the chain into native /lets:plan.
argument-hint: "[goal]"
---

# Autonomous Plan (PREVIEW workflow)

**PREVIEW / experimental.** Shipped standalone so the autonomous-planning Dynamic Workflow can be dogfooded across projects and gather edits before it's folded into native `/lets:plan --workflow` (tracked in lets-jsw00). The reusable artifact is `skills/plan-workflow/plan.workflow.js`; this command is a thin dispatcher. The interactive native `/lets:plan` is untouched. Expect rough edges - report them.

Autonomous whole-command planning: you give a GOAL + a RUBRIC up front; the workflow explores, proposes approaches, architects each, judges them against your rubric, evaluates the winner, and writes a plan - all off-context. You approve the plan at the end. **Steer-by-rubric + approve-at-end**, not gate-each-step.

> **IMPORTANT:** Deferred tools (e.g. `AskUserQuestion`) must be loaded and called as specified.

## Step 0: Resolve & claim (spawn entry)

If the argument is an explicit beads **task id** (matches `<prefix>-<alphanum>[.N]`) rather than a free-text goal - as the autonomous pipeline launches it (`/lets:plan-workflow <id>` into a fresh worktree) - resolve-and-claim per the **detect-task** convention: treat the id as authoritative and, if `bd show <id>` is not `in_progress`, `Skill(skill: "lets:take-task", args: "<id>")` to claim it (the spawn-time entry claim is AUTO-MODE-exempt - see `detect-task/SKILL.md`). Then derive the GOAL from the task title + description for Step 1. A free-text goal argument skips this step (no claim).

## Pipeline-state marker (spawn-claimed runs only)

When Step 0 spawn-claimed a task (the autonomous pipeline launched this), record the phase in a **per-task** marker so N parallel worktrees don't collide and the gate-notify can tell autonomous from interactive. **Per-task filename** (NOT a shared `pipeline-state` — `.lets/` is a symlink across worktrees): `.lets/cache/pipeline-state-<task-id>`, line `<task-id>|<phase>|<iso>`, phase ∈ `planning | gate-clarify | gate-approve | executing | blocked | done`. Helper:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
printf '%s|%s|%s\n' "{TASK_ID}" "{PHASE}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$LETS_PROJECT_ROOT/.lets/cache/pipeline-state-{TASK_ID}"
```

plan-workflow writes `planning` at Step 3 (workflow launch), `gate-clarify` at GATE 1 (if clarify is needed), `gate-approve` at Step 4 (plan saved). An interactive (free-text-goal) run writes no marker → no gate notifications. **Keep in sync:** the same contract + helper live in `execute.md` (which writes `executing`/`blocked`/`done`).

## Gate notifications (authoritative)

At a human gate, surface to the operator via cmux — **marker-gated** so only autonomous (spawn-claimed) runs notify; interactive runs have no marker and stay quiet. Resolve the workspace by the worktree path:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
if [ -f "$LETS_PROJECT_ROOT/.lets/cache/pipeline-state-{TASK_ID}" ]; then
  lets cmux notify --cwd "$LETS_PROJECT_ROOT" --title '{gate title}' --body '{gate body}' --json 2>/dev/null || true
fi
```

**Quoting:** the substituted values (`{gate title}`, `{gate body}` — the latter carries the user-authored task title) go in **single quotes**, so a title containing `$(...)`, backticks, or `$VAR` is a literal, not shell-expanded, before it reaches the injection-safe CLI. Only `$LETS_PROJECT_ROOT` stays double-quoted (it's a real shell var). If a substituted value contains a literal `'`, the orchestrator escapes it (`'\''`) when building the command. Degrades silently (`ok=true, notified=false`) off-macOS / cmux-absent / no match; `|| true` covers the non-unix stub. The notification is **best-effort surfacing** — the gate also halts visibly in-band; never rely on it alone. Gates: **GATE 1** "Plan needs your answers" (clarify), **GATE 2** "Plan ready to approve". **Keep in sync:** `execute.md` reuses this block for its execute-blocked notify.

## Step 1: Goal + Rubric

Parse the goal from the argument (or, for a spawn-claimed task, from its title + description; else ask "What are we planning?").

Gather the **RUBRIC** - the steering criteria that REPLACE the interactive picks of native `/lets:plan`. Ask the user (free text, or a short list):
- What does "good" look like here? (priority order: simplicity / performance / consistency / minimal-blast-radius / ...)
- Hard constraints (must-not-break, must-not-touch, compatibility).
- Scope boundaries (explicitly OUT).
- Success criteria.

**Do not proceed without a rubric** - it is the only steering signal the autonomous run receives. Echo the rubric back for confirmation before launching.

**GATE 1 (spawn-claimed runs - the bounded up-front clarify gate).** For a spawn-claimed task (Step 0), the rubric is derived from the task's description + comments (the PM principle: steering lives in the bd task). If the task already carries enough steering, build the rubric from it and proceed with NO questions. Only if it is **insufficient** for a rubric: write the `gate-clarify` marker, fire the GATE 1 notify ("Plan needs your answers"), ask a SHORT bounded clarify set, record the answers to the task (`bd comments add`), then build the rubric and proceed. This is up-front gathering of the rubric (consistent with steer-by-rubric) — NOT a per-step interactive gate.

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

If the `Workflow` tool is NOT available -> STOP: "needs Claude Code >= 2.1.154 on a paid plan". This command IS the workflow (preview) - it has no Task-based fallback. (Spawned via `--flow plan-workflow`? surface the PREVIEW-unavailable message so the operator re-runs `--flow plan`.)

Spawn-claimed run: write the `planning` pipeline-state marker before launching the workflow.

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
  # Same naming contract as /lets:plan (lets-fe788): STAMP=$(date +%Y-%m-%d-%H%M);
  # write to .lets/plans/${STAMP}-<slug>.md  (slug = task-id-based descriptor for this run).
  ```
- Spawn-claimed run: after the plan file is saved, write the `gate-approve` pipeline-state marker and fire the GATE 2 notify (see the gate-notification block) - the plan is ready for human approval.
- Show the **decision log** (winner + judge votes/totals + rationale), the approach list, the eval findings, and the plan summary.
- User approves -> suggest `/lets:execute`. Wants changes -> adjust the rubric and re-run (`Workflow` `resumeFromRunId` caches completed stages while you iterate the script).

## Notes

- Agents the workflow spawns (`lets:explorer`, `lets:architect`, `lets:pragmatist`, domain experts) are EXISTING agents -> they resolve from the loaded plugin (no registration gotcha).
- Preview status: shipped standalone so the autonomous chain can be dogfooded across projects; it will be folded into native `/lets:plan --workflow` once it stabilizes (lets-jsw00). Until then, treat it as experimental and report rough edges.
