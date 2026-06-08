---
description: Execute implementation plan from /lets:plan - load plan and enter native plan mode
argument-hint: "[--status]"
---

# Execute Plan

Load an implementation plan and execute it using Claude Code's native plan mode. The plan provides the roadmap; native plan mode provides approval gates.

**Plan is a roadmap, not a script.** Read real files before every change. Adapt to current context.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Active Task Detection

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If not on a feature/worktree branch and no in-progress task found - ask user which task to execute.

If invoked with an explicit `<task-id>` argument and it is not already the active/in-progress task (e.g. a spawned `execute --auto <id>` in a fresh worktree), resolve-and-claim it per the **detect-task** *explicit task-id argument* convention before resolving the plan - don't re-paraphrase the logic; that section is the single source of truth (incl. the AUTO-MODE entry-claim carve-out).

**If on `$LETS_MERGE_BRANCH`** — `/lets:execute` usually expects a feature branch. Soft-gate before proceeding:

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
Present plan summary (title, task count, key files), then proceed to Step 5.

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
- **Execute anyway** -> proceed to Step 5
- **Re-plan** -> invoke `Skill(skill: "lets:plan")`
- **Cancel** -> stop, return to the user

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

**After user approves**, Claude implements step by step. Use `/lets:commit` at natural commit points as indicated in the plan.

**Progress tracking:** After completing each plan task, append `[DONE]` to its `### Task N:` heading in the plan file. This makes resume self-documenting - on re-entry, skip tasks already marked `[DONE]`.

**Fallback:** If `EnterPlanMode` tool is not available or returns an error, skip plan mode tools entirely. Instead:
1. Present each plan task one by one with files to change and expected outcome
2. Implement the task, then ask user to review before moving on
3. Use `/lets:commit` at commit points indicated in the plan
4. Track completed tasks by appending `[DONE]` markers in the plan file

The plan file provides the roadmap; explicit user approval provides the gates.

## Step 6: Record Completion

After implementation is complete (all plan tasks done):

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
