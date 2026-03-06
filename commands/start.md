---
description: Start session - restore context, show tasks, select work item
---

# Session Start

Restore context and prepare for work. **User MUST select a task before working.**

## Step 1: Previous Session Context

```bash
# ROOT = project-root from LETS Config
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
cat "$ROOT/.lets/sessions/last-summary-${BRANCH_SLUG}.md" 2>/dev/null \
  || cat "$ROOT/.lets/sessions/last-summary.md" 2>/dev/null \
  || echo "No previous session"
```

Read and summarize what was done in the last session.

## Step 2: Git State

```bash
git branch --show-current
git status --short
git log --oneline -3
```

Report: branch, uncommitted changes, recent commits.

## Step 3: Task Status

Run `/lets:status overview` to show compact task overview.

## Step 4: Present Summary

```
## Previous Session
{summary from last-summary.md}

## Git
Branch: {branch}
Changes: {clean / X uncommitted files}
Recent: {last 3 commits}

## Tasks
{output from /lets:status}
```

## Step 5: Task Selection (MANDATORY)

**Every session needs a task.** Ask:

> "Which task are you working on today?"
>
> Pick from ready tasks above, or:
> - `bd create --title="..." --type=task --priority=2` for new task
> - `bd update <id> --status=in_progress` to claim existing

**If user doesn't want to pick a task** but describes work (e.g., "just want to fix proxy config"):
- Auto-create: `bd create --title="Fix proxy config" --type=task --priority=3`
- Inform user: "Created task XX, working in feature branch"
- This keeps traceability without friction

**Wait for user to select, create, or describe work.**

## Step 6: Create/Switch Branch

After task is selected (e.g., `proj-ch15` "Fix proxy config"):

### Worktree Detection

First, check if we're in a worktree:

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

**If in a worktree** (`$GIT_DIR` contains `worktrees/`, i.e. is NOT `.git`):
- Skip branch creation entirely - use the current worktree branch as-is
- The worktree already has its own branch (e.g., `worktree-auth-feature`)
- **Auto-detect task ID from worktree branch name:**
  - Parse beads ID pattern from branch: `worktree-<task-id>-<slug>` (e.g., `worktree-lets-hpi.3-worktree-start` -> `lets-hpi.3`)
  - Beads ID pattern: `<prefix>-<alphanum>[.<number>]` (same as `commit.md` Step 2)
  - If task ID found: use it for Step 5 (Task Selection) - show title via `bd show`, ask user to confirm, skip manual selection
  - If not found (custom name like `worktree-auth-feature`): fall through to normal Step 5 task selection
- Present: "In worktree, using branch: {branch}"
- Jump directly to **Save Session Start Reference** below (skip Branch Naming, Branch Logic, Context Recovery)

**If in main repo** (`$GIT_DIR` is `.git`):
- Continue with normal branch logic below

### Branch Naming

Format: `feature/<task-id>-<slugified-title>`

Examples:
- `feature/proj-ch15-fix-proxy-config`
- `feature/proj-ch5-add-mobile-api`

Slug rules: lowercase, spaces to hyphens, remove special chars, max 50 chars.

### Branch Logic

```
Check current state:
  |
  +- Already on correct branch - do nothing, continue
  |
  +- Branch exists elsewhere - git checkout <branch>
  |
  +- Branch doesn't exist - ask Branch or Worktree (see below)
```

### New Branch: Branch or Worktree?

When creating a new branch (branch doesn't exist yet), offer the choice:

```
AskUserQuestion(
  questions=[{
    question: "How do you want to work on this task?",
    header: "LETS",
    options: [
      { label: "Branch (Recommended)", description: "Regular feature branch in current repo" },
      { label: "Worktree", description: "Separate directory for parallel work in another terminal" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Branch** -> `git checkout -b <branch> {merge-branch}` (standard flow, use `merge-branch` from LETS Config)
- **Worktree** -> run `/lets:worktree create <task-id>-<slug>`, then inform:
  "Worktree created. Open a new terminal and run:"
  ```bash
  cd {absolute-worktree-path} && claude
  ```
  "Then use `/lets:start` to pick a task."
  Stop here - the worktree session continues in a separate terminal.

### Context Recovery (resuming existing branch)

If the branch already exists (continuing a multi-session task):

```bash
bd show <task-id>
bd comments list <task-id>
```

Present: "Resuming **{task title}** (`{task-id}`). Last session: {summary from latest beads comment}"

This recovers context even after conversation compaction.

### Save Session Start Reference

After branch is ready, save the starting point for this session. Uses per-branch naming to support parallel sessions in worktrees:

```bash
# ROOT = project-root from LETS Config
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$ROOT/.lets/sessions"
git rev-parse HEAD > "$ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}"
```

This lets `/lets:end` know which commits belong to this session, even when multiple sessions run in parallel.

### Handle Uncommitted Changes

If switching branches with uncommitted changes:

Use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes on current branch. What to do?",
    header: "LETS",
    options: [
      { label: "Stash", description: "git stash, switch branch, remind to pop later" },
      { label: "Commit first", description: "Run /lets:commit, then switch branch" },
      { label: "Stay", description: "Skip branch switch - work on current branch" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Stash** -> `git stash`, switch branch, remind to `git stash pop` later
- **Commit first** -> run `/lets:commit`, then switch
- **Stay** -> skip branch switch, warn about mixed work

## Step 7: Task Size Assessment

Once task is selected, assess complexity:

| Size | Action |
|------|--------|
| Quick fix (< 30 min) | "OK, let's do it" |
| Small (< 2 hrs) | "OK, let's do it" |
| Medium (2-8 hrs) | Suggest `/lets:plan` then `/lets:execute` |
| Large (> 8 hrs) | Require `/lets:plan` + break into subtasks |

## Step 8: Ready to Work

After task is selected and branch is ready, show reminders and welcome box.

## Output

```
## Reminders
- Check context window: `/context`
- For technical decisions: `/lets:opinion`
- When task done: `/lets:commit` - `/lets:done` - `/lets:end`

┌─ LETS ─────────────────────────┐
│  Working on: {task-id}         │
│  {task title, truncated}       │
│  Branch: {branch-name}         │
└────────────────────────────────┘
```

## Rules

- **NEVER start working without a selected task** (auto-create counts)
- **Every branch maps to exactly one task**
- If uncommitted changes exist, ask user what to do with them first
- If previous session had in-progress work, highlight it
- If user is already on the correct feature branch, skip branch creation
- Suggest `/lets:plan` for medium/large tasks
- Respond in user's language (Ukrainian/Russian/English)
