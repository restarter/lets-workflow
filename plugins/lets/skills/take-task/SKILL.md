---
name: take-task
description: This skill should be used when claiming a task to work on - "take task", "візьми таск", "працюю над", "work on task X", "pick task X", "switch to task X", "claim task". Sets task to in_progress, creates/switches branch, saves the per-branch task-state file.
---

# Take Task

Claim a tracker task and prepare the working environment.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

Multiple flows need "claim task + prepare branch": /lets:start, mid-session task switch, direct "візьми таск X". Centralizing ensures consistent branch naming, worktree handling, and session ref saving.

## Flow

### Step 1: Resolve Task

```lets-tracker
show task=<task-id>
```

Verify task exists. Then claim it (set-status is a state change - HARD-FAIL loud if the binding can't run; do NOT proceed to branch setup on a failed claim):

```lets-tracker
set-status task=<task-id> status=in_progress
```

### Step 2: Check Uncommitted Changes

Before switching branches, check for uncommitted changes:

```bash
git status --short
```

If changes exist and branch switch is needed, ask user:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes on current branch. What to do?",
    header: "Uncommitted",
    options: [
      { label: "Stash", description: "git stash, switch branch, remind to pop later" },
      { label: "Commit first", description: "Commit changes, then switch branch" },
      { label: "Stay", description: "Skip branch switch - work on current branch" }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- **Stash** -> `git stash`, proceed with branch switch, remind to `git stash pop` later
- **Commit first** -> delegate to commit skill, then proceed with branch switch
- **Stay** -> skip branch switch, warn about mixed work, then continue to Step 5 (the task was already claimed in Step 1 - save its `.task` state file for the current branch; do NOT stop before it, or detect-task/`/lets:done` are left with a claim but no boundary file).

If staying on current branch (worktree, already correct) or no changes - skip this step.

### Step 3: Worktree Check

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

**If in a worktree** (`$GIT_DIR` contains `worktrees/`):
- Skip branch creation - use the current worktree branch as-is
- Two branch shapes possible (`/lets:worktree create` supports both):
  - `worktree-<task-id>-<slug>` — new-branch mode (the LETS convention); auto-detect task ID
  - any other shape (e.g. `feature/foo`) — attached existing branch via `lets worktree create --attach`; no task ID in the name
- Task-id pattern is TRACKER-DEPENDENT (see detect-task): beads `<prefix>-<alphanum>[.<number>]` (e.g. `lets-abc`, `lets-abc.1`); planfix-mcp a pure-numeric id
- If task ID found in branch name: confirm with user via the tracker's `show` verb
- If branch shape is "attached" (no matching task ID): rely on the task-id passed as the skill argument — don't try to extract from the branch name
- Present: "In worktree, using branch: {branch}"
- Jump to Step 5

**If in main repo** (`$GIT_DIR` is `.git`):
- Continue with Step 4

### Step 4: Branch Logic (main repo only)

**Branch naming:** `feature/<task-id>-<slugified-title>`
Slug rules: lowercase, spaces to hyphens, remove special chars, max 50 chars.
Examples: `feature/proj-ch15-fix-proxy-config`, `feature/proj-ch5-add-mobile-api`

Check current state:
- Already on correct branch -> do nothing, continue to Step 5
- Branch exists elsewhere -> `git checkout <branch>`
- Branch doesn't exist -> offer choice:

```
AskUserQuestion(
  questions=[{
    question: "How do you want to work on this task?",
    header: "Workspace",
    options: [
      { label: "Branch (Recommended)", description: "Regular feature branch in current repo" },
      { label: "Worktree", description: "Separate directory for parallel work in another terminal" },
      { label: "Stay on current branch", description: "Skip branch creation. On {LETS_MERGE_BRANCH}: /lets:done pushes + closes (no PR). On a custom branch: normal PR flow." }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- **Branch** -> `git checkout -b <branch> {LETS_MERGE_BRANCH}` (from LETS Config)
- **Worktree** -> invoke `Skill(skill: "lets:worktree", args: "create <task-id>-<slug>")`, then inform:
  "Worktree created. Open a new terminal and run:"
  `cd {absolute-worktree-path} && claude`
  "Then use `/lets:start` to pick a task."
  Stop here - the worktree session continues in a separate terminal.
- **Stay on current branch** -> skip `git checkout -b`; stay on the current branch (could be `$LETS_MERGE_BRANCH` or any pre-existing branch). Print one line:
  "Staying on `{current branch}`. No new branch created."
  If `HEAD == $LETS_MERGE_BRANCH`, append: " Trunk-mode: `/lets:done` will push + close (no PR — same-source-target)."
  Continue to Step 5 (the `.task` state file is saved as usual).

### Step 5: Save Task State File

Write the per-branch task-state file `.lets/sessions/.task-<branch-slug>` with three fields:
`task:` (identity), `start:` (task boundary - preserved across resume of the same task, reset on a new/changed task or when the recorded SHA is no longer an ancestor of HEAD), and `session: <sha> <session-id>` (this session's boundary, always refreshed). Atomic (`tmp`+`mv`) so a crash never leaves a half-written file. `<task-id>` is the claimed id; `{LETS_MERGE_BRANCH}` is from LETS Config.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
HEAD_SHA=$(git rev-parse HEAD)
MERGE_BRANCH="{LETS_MERGE_BRANCH}"; CLAIMED_ID="<task-id>"; SID="$CLAUDE_CODE_SESSION_ID"

PREV_TASK=""; PREV_START=""
if [ -f "$TASK_FILE" ]; then
  PREV_TASK=$(sed -n 's/^task: //p' "$TASK_FILE" | head -1)
  PREV_START=$(sed -n 's/^start: //p' "$TASK_FILE" | head -1)
fi
# Preserve start: only when resuming the SAME task AND the recorded SHA is still an ancestor of HEAD.
if [ "$PREV_TASK" = "$CLAIMED_ID" ] && [ -n "$PREV_START" ] && git merge-base --is-ancestor "$PREV_START" HEAD 2>/dev/null; then
  START="$PREV_START"
else
  START="$HEAD_SHA"
fi

tmp=$(mktemp "${TASK_FILE}.XXXX")
{
  echo "task: $CLAIMED_ID"
  echo "start: $START"
  echo "session: $HEAD_SHA $SID"
} > "$tmp" && mv -f "$tmp" "$TASK_FILE"
```

### Step 6: Context Recovery (existing branch)

If the branch already existed (continuing a multi-session task):

```lets-tracker
show task=<task-id>
comment-list task=<task-id>
```

Read the full description and ALL comments — they hold the multi-session context. Present: "Resuming **{task title}** (`{task-id}`). Last session: {summary from the latest tracker comment}"

Also surface the most recent session snapshot for this branch (the file trail `/lets:start` full-mode reads - the quick `<task-id>` path skips Step 1, so read it here):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH_SLUG=$(git branch --show-current | tr '/' '-')
LATEST_SNAP=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-${BRANCH_SLUG}.md" 2>/dev/null | head -1)
```

If `$LATEST_SNAP` is non-empty, Read it and fold its `### Remaining + NEXT STEP` into the "Last session: ..." line. If empty, rely on the tracker's comments alone (fresh branch - no snapshot yet).

## Output

When triggered standalone (not via `/lets:start`):

```
Task claimed: **{task title}** (`{task-id}`)
Branch: {branch-name}

┌─ LETS ────────────────────────────────────────────┐
│  Plan?    /lets:plan · --fast · plan-workflow     │
│  Check?   /lets:check                             │
└───────────────────────────────────────────────────┘
```

When invoked by `/lets:start` OR `/lets:done` (via `Skill` tool) - skip this output, the calling command has its own. Per CLAUDE.md "Internal invocation = no box".

## Anti-patterns

- Never create feature/ branch inside a worktree
- Never work without saving session start ref
- Never skip the `set-status task=<id> status=in_progress` claim
- Never auto-create a feature branch when user picked "Stay on current branch"

## Integration

User-facing skill. Auto-triggers on "take task X", "візьми таск". Primary consumer: `/lets:start`.
See: `grep -r "take-task" commands/` for command references.
