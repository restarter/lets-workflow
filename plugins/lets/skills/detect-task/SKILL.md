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

The `<task-id>` shape is TRACKER-DEPENDENT (the id sits immediately after `feature/` or `worktree-`, up to the first `-<slug>` boundary):
- beads: `<prefix>-<alphanum>[.<number>]` - e.g. `lets-abc`, `lets-abc.1`, `proj-xyz.42`
- planfix-mcp: a pure-numeric id - e.g. `48647`, so `feature/48647-<slug>`
- other adapters: the tracker's own id shape

**Do NOT apply the beads `<prefix>-<alphanum>` regex on a non-beads project** - it false-positives on a slug word: `feature/48647-lifecycle-test` makes the beads regex capture `lifecycle-test`, NOT the numeric id `48647`. Match using the ACTIVE tracker's id shape (`{LETS_TRACKER}` from LETS Config). When the branch shape is ambiguous, the `.task-<slug>` file (Step 1.5, authoritative) and the `list-by-status` fallback are safer than a branch-name guess.

### Step 1.5: Task-State File (fills the gap when the branch name carries no id)

The `.task-<branch-slug>` file (written by `take-task`) records the CURRENT task - authoritative over the branch name, because a worktree branch is frozen at create time and may host several tasks in sequence (the branch name is only the worktree's home task). Read it after the explicit-arg short-circuit, before the branch-name parse. `{LETS_MERGE_BRANCH}` is from LETS Config:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
FILE_TASK=$(sed -n 's/^task: //p' "$TASK_FILE" 2>/dev/null | head -1)
if [ -n "$FILE_TASK" ]; then
  if [ "{LETS_TRACKER}" = "beads" ] && [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && command -v bd >/dev/null 2>&1; then
    # ALLOWLISTED beads-only liveness probe (lets-rules "Tracker Adapters" carve-out). On the
    # merge-branch ONLY, distinguish a live trunk task from a stale .task-main / main-mode.
    # GATE ON THE ACTIVE TRACKER, not just `command -v bd`: on a non-beads project where bd merely
    # happens to be on PATH, the old `command -v bd` gate would probe the WRONG store. With
    # {LETS_TRACKER}!=beads the AND is false -> fall through to trusting the .task file (non-beads
    # adapters have no cheap shell liveness `show`; the merge-branch-only scope + next-claim-overwrite
    # bound the risk). The {LETS_TRACKER}/{LETS_MERGE_BRANCH} literals are orchestrator-substituted.
    # Parse the FIRST "status" (the task's own top-level field). grep -o isolates each
    # "status": "X" so head -1 takes the first object's status even on compact JSON - a greedy
    # sed '.*"status".*' would grab the LAST (a nested dependency's) and mis-classify.
    STATUS=$(bd show "$FILE_TASK" --json 2>/dev/null | grep -o '"status": *"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/')
    [ "$STATUS" = "in_progress" ] && echo "$FILE_TASK"   # else fall through (stale: closed/abandoned/crash-after-close)
  else
    echo "$FILE_TASK"   # off the merge-branch, or non-beads tracker, or no bd: trust the file directly
  fi
fi
```

If this yields an id, use it; otherwise fall through to the branch-name parse, then the `list-by-status` fallback. (The beads liveness probe parses `bd show --json`'s status field; fall back to `bd show` text parse if the installed bd differs.)

**Precedence (full):** explicit task-id arg -> `.task-<slug>` `task:` (beads-liveness-validated ON THE MERGE-BRANCH) -> branch-name id -> the tracker's `list-by-status` (in_progress), first id. On id-carrying branches `take-task` writes branch + file together so they agree; the file fills the id-less gaps (trunk / custom worktree / attach) and, in a multi-task worktree, reflects the current (switched) task the frozen branch name can't.

**beads-liveness scope (perf + bd-hang safety):** the `bd show` probe runs ONLY on `{LETS_MERGE_BRANCH}` AND only when the active tracker is beads (the wrong-store gate above), where it is the sole signal separating a live trunk task from a stale `.task-main` / main-mode. Off the merge-branch, or on a non-beads tracker, it does NOT run - detect-task is on the hot path of 10+ commands and a per-call status probe would reintroduce the per-command cost and risk hanging (no portable `timeout` on macOS; a non-beads `show` would be an MCP round-trip). Trusting the file otherwise is safe: `feature/<id>` corroborates via the branch, and a just-closed `task:` in a worktree (or a stale `.task-main` on a non-beads merge-branch) is low-severity (the next claim overwrites).

### Step 2: Fallback

If branch parse finds no ID:

```lets-tracker
list-by-status status=in_progress   # returns the in_progress task list; take the first .id
```

### Explicit task-id argument (resolve-and-claim)

When the **calling command was invoked with an explicit `<task-id>` argument** (e.g. a session spawned into a fresh worktree with `/lets:plan-workflow <id>`, where the branch is `worktree-<name>` and carries no id): treat that id as **authoritative** - do NOT parse the branch, skip Steps 1-2. The caller then ensures the task is claimed: if the tracker's `show` reports `<id>` not `in_progress`, invoke `Skill(skill: "lets:take-task", args: "<id>")` (in a worktree `take-task` claims + saves the per-branch session-ref without creating a branch). This is what makes an id-accepting command spawn-able into a fresh worktree.

**AUTO MODE carve-out (entry claim only).** This spawn-time claim runs the `set-status=in_progress` verb - a tracker state change AUTO MODE normally gates. But the claim that *starts* an autonomous spawned session is the authorized **entry action** and is exempt (an unattended session cannot ask, and the claim IS the entry). The exemption covers ONLY this entry claim - every later tracker state change (`close`, status flips, `bd dolt push` on beads) stays gated per AUTO MODE.

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
