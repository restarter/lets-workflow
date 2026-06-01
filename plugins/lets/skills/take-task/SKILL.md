---
name: take-task
description: This skill should be used when claiming a task to work on - "take task", "візьми таск", "працюю над", "work on task X", "pick task X", "switch to task X", "claim task". Sets task to in_progress, creates/switches branch, saves session start ref.
---

# Take Task

Claim a beads task and prepare the working environment.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

Multiple flows need "claim task + prepare branch": /lets:start, mid-session task switch, direct "візьми таск X". Centralizing ensures consistent branch naming, worktree handling, and session ref saving.

## Flow

### Step 1: Resolve Task

```bash
bd show <task-id>
```

Verify task exists. Then claim it:

```bash
bd update <task-id> --status=in_progress
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
- **Stay** -> skip branch switch, warn about mixed work. Stop here.

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
- Beads ID pattern (when applicable): `<prefix>-<alphanum>[.<number>]` (e.g. `lets-abc`, `lets-abc.1`)
- If task ID found in branch name: confirm with user via `bd show`
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
  Continue to Step 5 (session-start-ref is saved as usual).

### Step 5: Save Session Start Reference

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
git rev-parse HEAD > "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}"
```

### Step 6: Context Recovery (existing branch)

If the branch already existed (continuing a multi-session task):

```bash
bd show <task-id>
bd comments <task-id>
```

Read the full description and ALL comments — they hold the multi-session context. Present: "Resuming **{task title}** (`{task-id}`). Last session: {summary from latest beads comment}"

## Output

When triggered standalone (not via `/lets:start`):

```
Task claimed: **{task title}** (`{task-id}`)
Branch: {branch-name}

┌─ LETS ────────────────────────┐
│  Plan?    /lets:plan          │
│  Check?   /lets:check         │
└───────────────────────────────┘
```

When invoked by `/lets:start` OR `/lets:done` (via `Skill` tool) - skip this output, the calling command has its own. Per CLAUDE.md "Internal invocation = no box".

## Anti-patterns

- Never create feature/ branch inside a worktree
- Never work without saving session start ref
- Never skip `bd update --status=in_progress`
- Never auto-create a feature branch when user picked "Stay on current branch"

## Integration

User-facing skill. Auto-triggers on "take task X", "візьми таск". Primary consumer: `/lets:start`.
See: `grep -r "take-task" commands/` for command references.
