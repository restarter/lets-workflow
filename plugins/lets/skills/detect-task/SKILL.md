---
name: detect-task
description: Internal skill for commands. Detect the active task (any tracker adapter) from the task-state file / git branch name. Do not trigger on user conversation - only when commands need task detection.
user-invocable: false
---

# Detect Active Task

Parse the task-state file / current git branch to find the active task ID (tracker-adapter-aware). Used by commands that need to know which task is in progress.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

10+ commands need to detect the active task from branch name. This skill centralizes the logic so branch format changes are updated in one place.

## Detection Flow

### Step 1: Parse Branch Name

```bash
BRANCH=$(git branch --show-current)
```

Extract the task ID from the branch name. Formats:
- `feature/<task-id>-<slug>` - standard LETS branches (main repo)
- `worktree-<task-id>-<slug>` - worktree branches created via `/lets:worktree create` in new-branch mode (the LETS convention)
- `worktree-<custom-name>` - worktree branch without an embedded task ID; use fallback
- any other shape (e.g. `feature/foo`, `bugfix/bar`) - attached existing branch via `/lets:worktree create --attach`; no task ID in the name; use fallback

The `<task-id>` shape is TRACKER-DEPENDENT (the id sits immediately after `feature/` or `worktree-`, up to the first `-<slug>` boundary):
- beads: `<prefix>-<alphanum>[.<number>]` - e.g. `lets-abc`, `lets-abc.1`, `proj-xyz.42`
- a numeric-id tracker: a pure-numeric id - e.g. `48647`, so `feature/48647-<slug>`
- other adapters: the tracker's own id shape

**Do NOT apply the beads `<prefix>-<alphanum>` regex on a non-beads project** - it false-positives on a slug word: `feature/48647-lifecycle-test` makes the beads regex capture `lifecycle-test`, NOT the numeric id `48647`. Match using the ACTIVE tracker's id shape (`{LETS_TRACKER}` from LETS Config). When the branch shape is ambiguous, the `.task-<slug>` file (Step 1.5, authoritative) and the `list-by-status` fallback are safer than a branch-name guess.

### Step 1.5: Task-State File (fills the gap when the branch name carries no id)

The `.task-<branch-slug>` file (written by `take-task`) records the CURRENT task - authoritative over the branch name, because a worktree branch is frozen at create time and may host several tasks in sequence (the branch name is only the worktree's home task). Read it after the explicit-arg short-circuit, before the branch-name parse. `{LETS_MERGE_BRANCH}` is from LETS Config:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
FILE_TASK=$(sed -n 's/^task: //p' "$TASK_FILE" 2>/dev/null | head -1)
[ -n "$FILE_TASK" ] && printf 'task=%s on_merge_branch=%s\n' "$FILE_TASK" \
  "$([ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && echo yes || echo no)"
```

**`on_merge_branch=no`** - trust `task=` directly and stop here. The branch corroborates the file, and a just-closed id in a worktree is low-severity (the next claim overwrites it).

**`on_merge_branch=yes`** - the file alone cannot tell a live trunk claim from a stale `.task-main` left by a closed task or a main-mode session, so verify it:

```lets-tracker
show task=<FILE_TASK>   # returns {id,title,status,url}; read status
```

`status` is `in_progress` -> use the id. Any other neutral status -> the claim is stale; fall through to the branch-name parse. `show` unsupported (`LETS_TRACKER=none`) or failing -> trust the file and say in one line that liveness could not be checked.

**Precedence (full):** explicit task-id arg -> `.task-<slug>` `task:` (liveness-validated on the merge-branch via the neutral `show`) -> branch-name id -> the tracker's `list-by-status` (in_progress), first id (skipped entirely when the caller passes `fallback=no` - see Optional arguments). On id-carrying branches `take-task` writes branch + file together so they agree; the file fills the id-less gaps (trunk / custom worktree / attach) and, in a multi-task worktree, reflects the current (switched) task the frozen branch name can't.

**Liveness scope (hot path).** The `show` probe runs ONLY on `{LETS_MERGE_BRANCH}`, the sole place a stale `.task-main` is indistinguishable from a live trunk claim. Off the merge-branch it does NOT run - detect-task is on the hot path of 10+ commands. This IS a cost change for non-beads adapters, which previously skipped the probe entirely: an MCP `show` is a network round-trip, and there is no portable `timeout` on macOS to bound it. We accept that on the merge-branch only, because the alternative is a non-beads project trusting a possibly-stale file forever. An adapter whose `show` is known slow may mark it so in its binding cell; the probe then skips and says liveness was not checked. Trusting the file elsewhere is safe: `feature/<id>` corroborates via the branch, and a just-closed `task:` in a worktree is low-severity (the next claim overwrites).

### Step 2: Fallback

If branch parse finds no ID:

```lets-tracker
list-by-status status=in_progress   # returns the in_progress task list; take the first .id
```

### Explicit task-id argument (resolve-and-claim)

When the **calling command was invoked with an explicit `<task-id>` argument** (e.g. a session spawned into a fresh worktree with `/lets:plan-workflow <id>`, where the branch is `worktree-<name>` and carries no id): treat that id as **authoritative** - do NOT parse the branch, skip Steps 1-2. The caller then ensures the task is claimed: if the tracker's `show` reports `<id>` not `in_progress`, invoke `Skill(skill: "lets:take-task", args: "<id>")` (in a worktree `take-task` claims + saves the per-branch session-ref without creating a branch). This is what makes an id-accepting command spawn-able into a fresh worktree.

**AUTO MODE carve-out (entry claim only).** This spawn-time claim runs the `set-status=in_progress` verb - a tracker state change AUTO MODE normally gates. But the claim that *starts* an autonomous spawned session is the authorized **entry action** and is exempt (an unattended session cannot ask, and the claim IS the entry). The exemption covers ONLY this entry claim - every later tracker state change (`close`, status flips, `bd dolt push` on beads) stays gated per AUTO MODE.

### Optional arguments: `branch=<ref>` / `fallback=no`

Passed as space-separated `key=value` via the `Skill` invocation's `args`. Both default OFF - a caller that passes neither gets exactly the flow above, unchanged.

- **`branch=<ref>`** - resolve for THAT ref instead of the checked-out one. It substitutes for `$(git branch --show-current)` in Step 1 and Step 1.5; nothing else changes. **Treat the value as UNTRUSTED**: the caller's ref may be a pull request's head, which its author names, and git permits `` $ ` ; | & ( ) `` in a ref. It reaches a filesystem path here, so derive the slug in-shell and pass only this block's OUTPUT downstream - never substitute the raw value into a path or into a tracker verb:

  ```bash
  LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
  SLUG=$(printf '%s' "<ref>" | tr '/' '-')
  case "$SLUG" in ""|*[!A-Za-z0-9._-]*) echo "UNSAFE REF - no task"; exit 0 ;; esac
  sed -n 's/^task: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-$SLUG" 2>/dev/null | head -1
  ```

  An id parsed out of the ref NAME (Step 1) must clear the same character class before it crosses a tracker verb - on beads that verb resolves to a `bd` command, i.e. a second shell hop.

- **`fallback=no`** - skip Step 2 and return None rather than the first `list-by-status` id.

**Why `fallback=no` exists.** The fallback answers "some task is in progress", not "this branch's task" - on a shared board it returns a colleague's. Tolerable for a tracker comment; NOT tolerable for anything that feeds a prompt (a wrong spec is worse than none). Passing it also makes provenance observable at the call site *by construction*: fallback off + an id returned = the id came from the state file or the ref name, never from the board.

### Step 3: Multiple Tasks

If fallback returns multiple tasks - behavior depends on the caller:
- **commit**: AskUserQuestion to pick task or "None"
- **done**: AskUserQuestion to pick task to close
- **review/check/opinion/ask**: skip the tracker comment if ambiguous. review/check additionally pass `fallback=no` when resolving the SPEC, so an ambiguous id can never reach a reviewer prompt
- **note**: AskUserQuestion to pick task to add note to

The calling command specifies what to do when ambiguous.

## Output

Returns one of:
- Task ID (string) - found active task
- None - no task detected, caller decides how to handle

## Integration

Internal skill used by most commands that need task detection.
See: `grep -r "detect-task" commands/` for current usage.
