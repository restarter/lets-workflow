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

**Team worktree** - `lets worktree info --json` reports a top-level `team`: this worktree belongs to a standing team and moves between tasks through `lets worktree switch`. Skip the generic question below and go to **Step 2T**. No `team` -> continue here.

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

### Step 2T: Team worktree - park, then switch

A team worktree never stashes. `git status --short`; changes present -> list them, and list separately every NEW path a park would need to name - untracked (`git ls-files --others --exclude-standard`) and staged additions (`git diff --cached --diff-filter=ACR --name-only HEAD`). Ignored files are never parked; they stay on disk. Then:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes on {branch} before switching to {task-id}. What to do?",
    header: "Uncommitted",
    options: [
      { label: "Commit first", description: "Commit them with /lets:commit, then switch" },
      { label: "Park", description: "Commit them as wip(<id>): park on this branch; unparked when you come back" },
      { label: "Stay", description: "Skip the switch - keep working on {branch}" }
    ],
    multiSelect: false
  }]
)
```

- **Commit first** -> delegate to the commit skill, then the switch below.
- **Park** -> ask which of the listed new paths go into the park commit. `--include` names only the paths the user confirmed; an unconfirmed new path makes the switch refuse (`untracked_present`, nothing touched) - say so, and let the user confirm it or remove it.
- **Stay** -> no switch; warn about mixed work, then Step 5 as in Step 2's Stay.

No changes -> the switch below directly. The switch, from the team worktree:

```bash
lets worktree switch --task '<task-id>' --title-file .lets/cache/title-<session6>.txt --json
# Park: add --park and one --include '<path>' per confirmed new path
```

(The title file is written as in Step 4, with the Write tool.) Render the result:

- `ok=true` -> "Switched to `{branch}`" (`created` -> "cut from `{base}`", with the staleness warning when `base_stale`); `park` -> name the park commit; `unpark` -> `unparked` (the parked changes are staged again), `parked_pushed` / `parked_unverified` / `parked_kept` (the park stays a commit - say why).
- A refusal -> show `error.kind` and `error.message`, change nothing, stop: `dirty_worktree`, `index_unmerged`, `operation_in_progress`, `detached_head` (exit 14 - the tree must be sorted out first), `untracked_present` (list `new_paths`, back to the Park question), `members_live` (a live implementer writes in this tree - wait for it or dismiss it), `no_remote_base`, `target_is_merge_branch`, `session_held` (switch from the team's lead session).

`lets worktree switch` wrote the target branch's task-state (`task`, `start`, `session` through the session guard, `orc` carried from the old branch), so Step 5's own write - its `--session-sha`, unguarded - is skipped on this path. Its tail still runs on the new branch, passing only what switch did not write:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
# {ORC_FLAG} as in Step 5: --orc '<name>' when args carried orc="<name>", else empty
lets worktree task-state set --clear-origin {ORC_FLAG} --json
lets peers role set worker --task "<task-id>" --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" --json
```

The first clears `origin:` and applies a requested orchestrator binding (an explicit `--orc` wins over the carried one); the second registers this session as the task's worker. Each failure is Step 5's one-line report - never hidden. Then rewrite the team file's `## 7. Current task` section (`.lets/teams/<team>.md`; the lead is its only writer): the task as **Title** (`id`), its branch, the phase, the next step, and `Resume artefacts:`. Then Step 6.

### Step 3: Worktree Check

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

**If in a worktree** (`$GIT_DIR` contains `worktrees/`):
- A team worktree (Step 2T ran) never reaches here - its branch comes from `lets worktree switch`, not from the current branch.
- Skip branch creation - use the current worktree branch as-is
- Go reads the branch against the active tracker's convention: `lets worktree info --json --task-candidate --plugin-root "${CLAUDE_PLUGIN_ROOT}"`. `task_candidate.source=created` (the adapter's `branch:` / `worktree-branch:` shape, default `worktree-<task-id>-<slug>`) is this branch's task: confirm it with the user via the tracker's `show` verb. Any other result is an attached or externally named branch (e.g. `feature/foo`): rely on the task-id passed as the skill argument - never guess an id from the name by eye. No `lets` binary: the shapes are the ones detect-task Step 1 lists.
- **Derived id (`.task` carries `origin: branch` or `origin: dir`).** `lets worktree adopt` guessed the id from the branch or directory name. When take-task did NOT receive the id as its argument, confirm with the user that the derived id is the task, and when Step 1's `show` returned `in_progress` (before this claim) warn in one line that it may already be claimed in another worktree. An explicit id argument supersedes the guess with no question, so unattended `--flow` / `--auto` spawns never stop here. Step 5 clears `origin:`.
- Present: "In worktree, using branch: {branch}"
- Jump to Step 5

**If in main repo** (`$GIT_DIR` is `.git`):
- Continue with Step 4

### Step 4: Branch Logic (main repo only)

**Branch naming:** the active tracker convention names the branch, and Go renders it. Write the task title with the Write tool to `.lets/cache/title-<session6>.txt` (6 = first chars of `$CLAUDE_CODE_SESSION_ID`; the title is untrusted text and never typed into a shell), then:

```bash
lets worktree branch-name --task '<task-id>' --title-file .lets/cache/title-<session6>.txt --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
```

Use `branch` (e.g. `feature/proj-ch15-fix-proxy-config`; a board file can declare `feature/PWA-45122-fix-login`). No `lets` binary: the documented default `feature/<task-id>-<slug>` (slug: lowercase, spaces to hyphens, special chars removed, max 50 chars).

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
- **Worktree** -> invoke `Skill(skill: "lets:worktree", args: "create <task-id> --title-file .lets/cache/title-<session6>.txt")` (the id, not a name: the worktree command renders the adapter's `worktree-branch:` name from it), then relay how the new session opens - the worktree command's own output names it (a terminal command, or a cmux / tmux / Orca pane already running `/lets:start <task-id>`).
  Stop here - the worktree session continues in a separate terminal or pane.
- **Stay on current branch** -> skip `git checkout -b`; stay on the current branch (could be `$LETS_MERGE_BRANCH` or any pre-existing branch). Print one line:
  "Staying on `{current branch}`. No new branch created."
  If `HEAD == $LETS_MERGE_BRANCH`, append: " Trunk-mode: `/lets:done` will push + close (no PR — same-source-target)."
  Continue to Step 5 (the `.task` state file is saved as usual).

### Step 5: Save Task State File

Write the per-branch task-state file `.lets/sessions/.task-<branch-slug>`: `task:` (identity), `start:` (task boundary - preserved across resume of the same task, reset on a new/changed task or when the recorded SHA is no longer an ancestor of HEAD), and `session: <sha> <session-id>` (this session's boundary, always refreshed); `origin:` is cleared (the id is now confirmed) and every other line - `orc:` included - is kept. `lets worktree task-state` owns the file (merge-write under a lock, validated values); without the binary, an atomic `tmp`+`mv` rewrite keeps the other lines too. `<task-id>` is the claimed id.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
HEAD_SHA=$(git rev-parse HEAD)
CLAIMED_ID="<task-id>"; SID="$CLAUDE_CODE_SESSION_ID"

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

if command -v lets >/dev/null 2>&1; then
  # {ORC_FLAG}: --orc '<name>' when args carried orc="<name>" and this is not the merge-branch (single-quoted, '\'' escaping); else empty
  lets worktree task-state set --task "$CLAIMED_ID" --start "$START" --session-sha "$HEAD_SHA" --session-id "$SID" --clear-origin {ORC_FLAG} --create --json
  lets peers role set worker --task "$CLAIMED_ID" --session "$SID" --cwd "$LETS_PROJECT_ROOT" --json
else
  tmp=$(mktemp "${TASK_FILE}.XXXX")
  {
    [ -f "$TASK_FILE" ] && grep -v -e '^task: ' -e '^start: ' -e '^session: ' -e '^origin: ' "$TASK_FILE"
    echo "task: $CLAIMED_ID"
    echo "start: $START"
    echo "session: $HEAD_SHA $SID"
  } > "$tmp" && mv -f "$tmp" "$TASK_FILE"
fi
```

On `ok=false` surface `error.message` - the claim succeeded but the boundary file did not, so `/lets:done` cannot measure the task until it is fixed.

- **Orchestrator binding (`orc="<name>"` in the args, from detect-task's `--orc` strip).** Pass it as `{ORC_FLAG}`; Go validates the name and a refusal is one line with nothing written. A trunk-mode claim (HEAD is `{LETS_MERGE_BRANCH}`) ignores it with one line - the merge-branch never carries a binding. When the envelope reports `rebound.from`, say in one line that this branch moved from that orchestrator to the new one.
- **Worker role.** The second call registers this session as the task's worker so orchestrators see it in `lets peers who`. A non-ok envelope (e.g. `session_not_in_registry`) is one line - never hidden, a silent failure leaves the worker unregistered. Without the binary the no-binary branch skips it; say so in one line.

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
# Task-id-named snapshot first (artifact-path naming, lets-05c4s); legacy {TS}-{branch}.md as fallback.
LATEST_SNAP=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-<task-id>-snapshot"*.md 2>/dev/null | head -1)
[ -z "$LATEST_SNAP" ] && LATEST_SNAP=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-${BRANCH_SLUG}"*.md 2>/dev/null | head -1)
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
