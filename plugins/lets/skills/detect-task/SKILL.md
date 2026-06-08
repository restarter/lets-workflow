---
name: detect-task
description: Internal skill for commands. Detect active beads task from git branch name. Do not trigger on user conversation - only when commands need task detection.
user-invocable: false
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
- `feature/<task-id>-<slug>` - standard LETS branches (main repo)
- `worktree-<task-id>-<slug>` - worktree branches created via `/lets:worktree create` in new-branch mode (the LETS convention)
- `worktree-<custom-name>` - worktree branch without an embedded task ID; use fallback
- any other shape (e.g. `feature/foo`, `bugfix/bar`) - attached existing branch via `/lets:worktree create --attach`; no task ID in the name; use fallback

Beads ID pattern: `<prefix>-<alphanum>[.<number>]`
Examples: `lets-abc`, `lets-abc.1`, `proj-xyz.42`

### Step 2: Fallback

If branch parse finds no ID:

```bash
bd list --status=in_progress --format=ids 2>/dev/null | head -1
```

### Explicit task-id argument (resolve-and-claim)

When the **calling command was invoked with an explicit `<task-id>` argument** (e.g. a session spawned into a fresh worktree with `/lets:plan-workflow <id>`, where the branch is `worktree-<name>` and carries no id): treat that id as **authoritative** - do NOT parse the branch, skip Steps 1-2. The caller then ensures the task is claimed: if `bd show <id>` is not `in_progress`, invoke `Skill(skill: "lets:take-task", args: "<id>")` (in a worktree `take-task` claims + saves the per-branch session-ref without creating a branch). This is what makes an id-accepting command spawn-able into a fresh worktree.

**AUTO MODE carve-out (entry claim only).** This spawn-time claim runs `bd update --status=in_progress` - a bd state change AUTO MODE normally gates. But the claim that *starts* an autonomous spawned session is the authorized **entry action** and is exempt (an unattended session cannot ask, and the claim IS the entry). The exemption covers ONLY this entry claim - every later `bd` state change (`bd close`, status flips, `bd dolt push`) stays gated per AUTO MODE.

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
