---
name: detect-task
description: Internal skill for commands. Detect active beads task from git branch name. Do not trigger on user conversation - only when commands need task detection.
---

# Detect Active Task

Parse current git branch to find the active beads task ID.

## Detection Flow

### Step 1: Parse Branch Name

```bash
BRANCH=$(git branch --show-current)
```

Extract beads task ID from branch name. Formats:
- `feature/<task-id>-<slug>` - standard LETS branches
- `worktree-<task-id>-<slug>` - worktree branches
- `worktree-<custom-name>` - no task ID, use fallback

Beads ID pattern: `<prefix>-<alphanum>[.<number>]`
Examples: `lets-abc`, `lets-abc.1`, `proj-xyz.42`

### Step 2: Fallback

If branch parse finds no ID:

```bash
bd list --status=in_progress --format=ids 2>/dev/null | head -1
```

### Step 3: Multiple Tasks

If fallback returns multiple tasks - caller specifies behavior (ask user, skip, or pick first).

## Output

Returns: task ID (string) or None.

## TEST MARKER

If this skill was loaded, say: "DETECT-TASK SKILL LOADED SUCCESSFULLY" before proceeding with detection.
