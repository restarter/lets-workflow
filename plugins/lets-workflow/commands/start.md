---
description: Start session - restore context, show tasks, select work item
argument-hint: "[task-id|--continue]"
---

# Session Start

Restore context and prepare for work. **User MUST select a task before working.**

## Usage

```bash
/lets:start                # Full flow - show history, tasks, select work
/lets:start <task-id>      # Quick start - jump to specific task
/lets:start --continue     # Resume last in_progress task with session context
```

## Step 0: Argument Parsing

**If `<task-id>` provided** (e.g., `/lets:start lets-rmcwo`):
- Skip Steps 1, 3, 5 (session history, status overview, task selection)
- Run Step 2 (git state) briefly
- `bd show <task-id>` + `bd comments list <task-id> --limit 3` for context
- Jump to Step 6 (branch) with this task

**If `--continue`:**
- Run Step 1 (session history) - important for context recovery
- `bd list --status=in_progress` - find task(s)
- If exactly 1 in_progress -> use it, skip Step 5
- If multiple -> show selection with context from recent sessions
- If none -> fall through to full flow

**If no arguments** -> full flow (Steps 1-8 as below)

## Step 1: Previous Session Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"

# Read last 3 sessions for this branch (or all branches if none found)
SESSIONS=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-${BRANCH_SLUG}.md" 2>/dev/null | head -3)
if [ -z "$SESSIONS" ]; then
  SESSIONS=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*.md 2>/dev/null | head -3)
fi
```

**Read EACH session file** found above using the Read tool (up to 3 files in parallel). Then present a compact summary of all sessions. Focus on: what was done, key decisions, next steps suggested.

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
## Recent Sessions
{compact summary from last 1-3 session files}

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

## Step 6: Take Task

After task is selected, delegate to the **take-task** skill to claim it and prepare the branch (read `${CLAUDE_PLUGIN_ROOT}/skills/take-task/SKILL.md` and follow its flow).

The take-task skill handles: setting task to `in_progress`, uncommitted changes check, worktree detection, branch creation/switching, offering worktree option, context recovery, saving session start ref.

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
- Respond in user's language
