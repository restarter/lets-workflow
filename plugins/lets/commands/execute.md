---
description: Execute implementation plan from /lets:plan - load plan and enter native plan mode
argument-hint: "[--status] [--team] [--step|--straight|--auto]"
---

# Execute Plan

Load an implementation plan and execute it using Claude Code's native plan mode. The plan provides the roadmap; native plan mode provides approval gates.

**Plan is a roadmap, not a script.** Read real files before every change. Adapt to current context.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## --auto mode

`/lets:execute --auto` runs the approved plan **autonomously**: skip the per-step "review before moving on" gate and `/lets:commit` at each plan commit point WITHOUT re-asking (matches the established "autonomous commits during execute after plan approval" preference). `--auto` is execution-speed for an *already-approved* plan — it does NOT bypass the hard-stops.

**`--auto` = run under AUTO MODE** (see `.claude/rules/lets-rules.md` `## AUTO MODE`) — do NOT restate the contract here. What still applies under `--auto`: push / PR / `bd close` / external stay gated; a 3×-fail halts; fabrication halts; and **`--auto` on `$LETS_MERGE_BRANCH` REFUSES** (Step 1) rather than auto-entering trunk-mode. When a hard-stop is hit, write the `blocked` marker and fire the execute-blocked notify (Step 6 / the gate-notification block) so an unattended session surfaces instead of stalling.

Parse `--auto` from the argument (it composes with a plan path; it is mutually exclusive with `--status`).

## Pipeline-state marker (under `--auto`)

A spawned autonomous session records its phase in a **per-task** marker file so N parallel worktrees don't clobber each other and the gate-notify (Step 6) can tell autonomous from interactive runs. **Per-task filename** (NOT a shared `pipeline-state` — `.lets/` is a symlink shared across worktrees): `.lets/cache/pipeline-state-<task-id>`, single line `<task-id>|<phase>|<iso>`, phase ∈ `planning | gate-clarify | gate-approve | executing | blocked | done`. Write helper (run at each transition; only under `--auto` — interactive execute writes no marker, so it triggers no notify):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
printf '%s|%s|%s\n' "{TASK_ID}" "{PHASE}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$LETS_PROJECT_ROOT/.lets/cache/pipeline-state-{TASK_ID}"
```

`execute --auto` writes `executing` when implementation starts (Step 5), `blocked` on a hard-stop (with the notify), `done` on completion (Step 6). **Keep in sync:** the same contract + helper live in `plan-workflow.md` (which writes `planning`/`gate-clarify`/`gate-approve`).

**Execute-blocked notify.** On a hard-stop under `--auto` (3×-fail / fabrication / a gated op reached / `$LETS_MERGE_BRANCH` refused), after writing the `blocked` marker, fire the **marker-gated gate-notification** so an unattended session surfaces instead of stalling. Use the authoritative snippet documented in `plan-workflow.md` "## Gate notifications" (don't re-paraphrase), incl. its single-quote rule for substituted values: `lets cmux notify --cwd "$LETS_PROJECT_ROOT" --title 'Execute blocked — needs you' --body '<reason>' --json 2>/dev/null || true`, guarded by the `pipeline-state-{TASK_ID}` marker existing. Best-effort — the run also halts visibly in-band.

## Step 1: Active Task Detection

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If not on a feature/worktree branch and no in-progress task found - ask user which task to execute.

If invoked with an explicit `<task-id>` argument and it is not already the active/in-progress task (e.g. a spawned `execute --auto <id>` in a fresh worktree), resolve-and-claim it per the **detect-task** *explicit task-id argument* convention before resolving the plan - don't re-paraphrase the logic; that section is the single source of truth (incl. the AUTO-MODE entry-claim carve-out).

**If on `$LETS_MERGE_BRANCH`** — `/lets:execute` usually expects a feature branch.

**Under `--auto`: REFUSE, do NOT show the soft-gate.** Autonomous editing of the merge-branch must never be auto-authorized — trunk-mode is a deliberate human opt-in (via the take-task picker), and an unattended session cannot answer an `AskUserQuestion`. Write the `blocked` marker, fire the execute-blocked notify ("Execute blocked — needs a feature branch"), and stop. A human re-runs on a feature branch, or opts into trunk-mode interactively (without `--auto`). `--auto` speeds an *authorized* path; it is not authorization to start editing `$LETS_MERGE_BRANCH`.

Without `--auto`, soft-gate before proceeding:

```
AskUserQuestion(
  questions=[{
    question: "You're on {LETS_MERGE_BRANCH}. /lets:execute usually wants a feature branch. Proceed?",
    header: "Trunk-mode",
    options: [
      { label: "Take a branch (Recommended)", description: "Stop, take a feature branch via /lets:start" },
      { label: "Continue here", description: "Proceed in trunk-mode (plan lookup uses task-id)" },
      { label: "Cancel", description: "Stop, return to work" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Take a branch (Recommended)** -> stop. Inform user: "Run `/lets:start <id>` and pick **Branch**, then `/lets:execute` again."
- **Continue here** -> proceed to Step 2 (trunk-mode is detected at runtime by Step 2's `[ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]` check; no flag passed).
- **Cancel** -> stop, return to user.

## Step 2: Load Plan

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)

# Derive slug: trunk-mode uses task-id (plan.md saves <date>-<task-id>.md on the merge-branch);
# otherwise the branch slug (covers feature/* and worktree-* branches).
# ${TASK_ID} is substituted by the orchestrator from the Step 1 detect-task result.
if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
  SLUG="${TASK_ID}"
else
  SLUG="${BRANCH#feature/}"
fi

# Guard: an empty slug (detached HEAD, or unresolved task-id in trunk-mode) would collapse the
# glob to *.md -> global latest -> another worktree's plan (the exact bug this task fixes).
if [ -z "$SLUG" ]; then
  echo "Could not derive a plan slug (detached HEAD or unresolved task-id). Pass a path or run /lets:start."
else
  # Latest plan for this slug - matches date-prefixed (YYYY-MM-DD-HHMM-<slug>.md) AND legacy bare
  # <slug>.md. Slug-scoped, NOT global latest: .lets/plans is shared across worktrees via symlink.
  PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | head -1)
  # Fallback: match by task-id (catches trunk-mode plans + naming drift, e.g. plan-workflow output)
  if [ -z "$PLAN" ] && [ -n "${TASK_ID}" ]; then
    PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${TASK_ID}"*.md 2>/dev/null | head -1)
  fi
fi

[ -n "$PLAN" ] && cat "$PLAN"
```

If no plan found:
> "No plan found for this task. Run `/lets:plan` first to create one."

Exit with LETS box suggesting `/lets:plan`.

## Step 3: Handle --status (exit early)

If argument is `--status`:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
# Show plan info (re-resolve: each bash block is a fresh shell). Same guarded lookup as Step 2.
SLUG=${BRANCH#feature/}; [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && SLUG="${TASK_ID}"
PLAN=""
[ -n "$SLUG" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | head -1)
[ -z "$PLAN" ] && [ -n "${TASK_ID}" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${TASK_ID}"*.md 2>/dev/null | head -1)
echo "Plan: ${PLAN:-(none found)}"
bd show <task-id>
# Show commits since session start
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
if [ -n "$START_REF" ]; then
  git log --oneline ${START_REF}..HEAD
fi
```

```
## Execution Status: **{task title}** (`{task-id}`)

Plan: {resolved plan path}
Commits this session: {N}

{commit list}
```

```
┌─ LETS ─────────────────────────┐
│  Continue?  /lets:execute      │
│  End?       /lets:end          │
└────────────────────────────────┘
```

Exit after showing status.

## Step 4: Plan Validation

Quick check before entering plan mode:

1. Read the plan file
2. Extract file paths from `Create:` and `Modify:` sections
3. Check if `Modify:` targets exist
4. Check if `Create:` targets already exist (partial execution?)

**If all OK:**
Present plan summary (title, task count, key files), then proceed to Step 4.5.

**If drift detected** (files missing, already created, etc.), use AskUserQuestion:

```
AskUserQuestion(
  questions=[{
    question: "Plan drift detected: {details}. How to proceed?",
    header: "Drift",
    options: [
      { label: "Execute anyway", description: "Adapt implementation to current state" },
      { label: "Re-plan", description: "Run /lets:plan to update the plan" },
      { label: "Cancel", description: "Don't execute" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Execute anyway** -> proceed to Step 4.5
- **Re-plan** -> invoke `Skill(skill: "lets:plan")`
- **Cancel** -> stop, return to the user

## Step 4.5: Choose Execution Mode

How the approved plan runs is ONE up-front choice. **A mode flag pre-answers it - skip the picker entirely when any is present:** `--auto` (Here · auto), `--team` (Team), `--step` / `--step-by-step` (Here · step-by-step), `--straight` / `--straight-through` (Here · straight-through). `--auto` keeps all its AUTO MODE semantics (Step 1 refuses on `$LETS_MERGE_BRANCH`; hard-stops preserved). Default locus is **Here** (this session, native plan mode); **Team** is the only locus switch.

**Bare `/lets:execute` (no mode flag) - ask exactly once:**

```
AskUserQuestion(
  questions=[{
    question: "How should I execute this plan?",
    header: "Run mode",
    options: [
      { label: "Straight-through (Recommended)", description: "Here, one approval, run all tasks, auto-commit at plan points" },
      { label: "Step-by-step", description: "Here, pause for review after each task; confirm each commit" },
      { label: "Auto", description: "Here, AUTO MODE - unattended; hard-stops + push/PR/close/external still gated" },
      { label: "Team", description: "Parallel implementers in isolated worktrees; review at the end" }
    ],
    multiSelect: false
  }]
)
```

**Commit cadence is DERIVED from the mode - never a separate question:** step-by-step -> confirm each commit; straight-through -> auto-commit at each plan commit point; auto -> auto-commit; team -> one commit per agent at the end.

**Handle response (sets the mode that Step 5 obeys):**
- **Straight-through** -> Step 5 (native plan mode); after the plan-mode approval, implement all tasks with NO per-task pause and `/lets:commit` at each plan commit point without re-asking.
- **Step-by-step** -> Step 5 (native plan mode); after the plan-mode approval, implement one task, pause for user review before the next, and confirm each `/lets:commit`.
- **Auto** -> proceed exactly as `--auto` (Step 5's `--auto` behavior + pipeline-state marker + execute-blocked notify). **Guard:** if on `$LETS_MERGE_BRANCH`, REFUSE Auto here too (same rule as Step 1's `--auto` refuse - AUTO MODE never edits the merge-branch); tell the user to pick step-by-step / straight-through or take a feature branch.
- **Team** -> do NOT enter native plan mode. Hand off to the team flow: `Skill(skill: "lets:team", args: "run")` - it spawns parallel implementers in isolated worktrees from this plan. After it completes, the user reviews via `/lets:review --local`. Skip Steps 5-6.

(A remembered default / `LETS_EXECUTE_MODE` to skip the picker on every run is a deferred follow-up - this ships the picker + flag shortcuts only.)

## Step 5: Enter Native Plan Mode

Present the plan summary to the user, then enter plan mode.

```
## Ready to Execute

**Plan:** {plan title}
**Tasks:** {N} implementation tasks
**Key files:** {top 3-5 files from plan}

Entering plan mode to create an execution strategy.
```

Call `EnterPlanMode`.

**In plan mode**, Claude:
1. Reads the existing plan from `.lets/plans/`
2. Reads the current state of files referenced in the plan (to detect drift)
3. Creates an execution strategy - adapting the plan's tasks to the actual codebase state
4. Writes the execution strategy to the plan file (the file specified by plan mode)
5. Calls `ExitPlanMode` when ready for user approval

**After user approves**, Claude implements per the **Step 4.5 mode**: *step-by-step* implements one task, pauses for review before the next, and confirms each commit; *straight-through* runs all tasks and `/lets:commit`s at each plan commit point without re-asking; *auto* is the `--auto` behavior below. Commit only at the plan's commit points.

**Under `--auto`:** write the `executing` pipeline-state marker, then — the plan-mode approval IS the gate — implement straight through without a per-step "review before moving on" pause, and `/lets:commit` at each plan commit point WITHOUT re-asking (one approval covers the run). Hard-stops still halt (push/PR/`bd close`/external gated; 3×-fail; fabrication; `$LETS_MERGE_BRANCH` refused per Step 1) — on any halt, write the `blocked` marker + fire the execute-blocked notify.

**Progress tracking:** After completing each plan task, append `[DONE]` to its `### Task N:` heading in the plan file. This makes resume self-documenting - on re-entry, skip tasks already marked `[DONE]`.

**Fallback:** If `EnterPlanMode` tool is not available or returns an error, skip plan mode tools entirely. Instead:
1. Present each plan task one by one with files to change and expected outcome
2. Implement the task, then ask user to review before moving on
3. Use `/lets:commit` at commit points indicated in the plan
4. Track completed tasks by appending `[DONE]` markers in the plan file

The plan file provides the roadmap; explicit user approval provides the gates.

## Step 6: Record Completion

After implementation is complete (all plan tasks done). **Under `--auto`:** write the `done` pipeline-state marker (the Pipeline-state marker helper, phase `done`) so the statusline / a watcher sees the run finished.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}; [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && SLUG="${TASK_ID}"
PLAN=""; [ -n "$SLUG" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | head -1)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
bd comments add <task-id> "## Plan execution complete $(date +%Y-%m-%d)

Plan: ${PLAN:-(none found)}
Commits: $(git log --oneline ${START_REF}..HEAD | wc -l | tr -d ' ')

$(git log --oneline ${START_REF}..HEAD)"
```

## Rules

- **NEVER execute blindly** - read actual file state before every change
- **NEVER commit without user approval** - use `/lets:commit` at plan commit points
- **Don't work on `$LETS_MERGE_BRANCH` without explicit opt-in** - Step 1 soft-gates with a prompt; user must pick "Continue here" to enable trunk-mode (plan lookup uses task-id)
- **Adapt, don't paste** - plan intent matters more than plan text
- **Stop on mismatch** - if reality diverges significantly from plan, surface it immediately
- Respond in user's language

## Output

**After completion:**
```
┌─ LETS ─────────────────────────┐
│  Check?   /lets:check          │
│  Review?  /lets:review --local │
│  Done?    /lets:done           │
└────────────────────────────────┘
```

**If no plan found:**
```
┌─ LETS ─────────────────────────┐
│  Plan?  /lets:plan             │
└────────────────────────────────┘
```
