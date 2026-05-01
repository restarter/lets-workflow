---
name: detect-task
description: Internal skill for commands. Detect active beads task from git branch name. Do not trigger on user conversation - only when commands need task detection.
---

# Detect Active Task

Parse current git branch to find the active beads task ID. Used by commands that need to know which task is in progress.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

10+ commands need to detect the active task from branch name. This skill centralizes the logic so branch format changes are updated in one place.

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

If fallback returns multiple tasks - behavior depends on the caller:
- **commit**: AskUserQuestion to pick task or "None"
- **done**: AskUserQuestion to pick task to close
- **review/check/opinion/ask**: skip beads comment if ambiguous
- **note**: AskUserQuestion to pick task to add note to

The calling command specifies what to do when ambiguous.

## Output

Returns one of:
- Task ID (string) - found active task
- None - no task detected, caller decides how to handle

## Integration

Internal skill used by most commands that need task detection.
See: `grep -r "detect-task" commands/` for current usage.
