---
description: Start session - restore context, show tasks, select work item
---

# Session Start

Restore context and prepare for work. **User MUST select a task before working.**

## Step 1: Previous Session Context

```bash
ROOT=$(git rev-parse --show-toplevel)
cat "$ROOT/.lets/sessions/last-summary.md" 2>/dev/null || echo "No previous session"
```

Read and summarize what was done in the last session.

## Step 2: Git State

```bash
git branch --show-current
git status --short
git log --oneline -3
```

Report: branch, uncommitted changes, recent commits.

## Step 3: Beads Status

Run `/lets:beads-status` command to show full task overview.

## Step 4: Present Summary

```
## Previous Session
{summary from last-summary.md}

## Git
Branch: {branch}
Changes: {clean / X uncommitted files}
Recent: {last 3 commits}

## Tasks
{output from /lets:beads-status}
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
  +- Branch doesn't exist - git checkout -b <branch> $(git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo master)
```

### Handle Uncommitted Changes

If switching branches with uncommitted changes:

**Ask:** "You have uncommitted changes. What to do?"
- **Stash** - `git stash` - switch - remind to `git stash pop` later
- **Commit first** - run `/lets:commit` - then switch
- **Stay on current branch** - skip branch switch, warn about mixed work

## Step 7: Task Size Assessment

Once task is selected, assess complexity:

| Size | Action |
|------|--------|
| Quick fix (< 30 min) | "OK, let's do it" |
| Small (< 2 hrs) | "OK, let's do it" |
| Medium (2-8 hrs) | Suggest planning skill (see below) |
| Large (> 8 hrs) | Require planning + break into subtasks |

### Which Planning Skill?

| Goal clarity | Use |
|--------------|-----|
| Clear goal ("Add X to Y") | `/feature-dev` - structured implementation |
| Unclear goal ("Improve Z", "Not sure how...") | `/brainstorming` - explores options, asks questions |

**Quick test:** Can user write a 1-sentence requirement?
- YES - `/feature-dev`
- NO - `/brainstorming` first, then `/feature-dev`

## Step 8: Ready to Work

After task is selected and branch is ready, show reminders and welcome box.

## Output

```
## Reminders
- Check context window: `/context`
- For technical decisions: `/lets:opinion`
- When done: `/lets:beads-finish` - `/lets:commit` - `/lets:end`

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
- Suggest `/feature-dev` for medium/large tasks
- Respond in user's language (Ukrainian/Russian/English)
