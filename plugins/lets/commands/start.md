---
description: Start session - restore context, show tasks, select work item
argument-hint: "[task-id|--continue|--main]"
---

# Session Start

Restore context and prepare for work. **User MUST select a task before working.**

> **LETS Notice — surface it.** If a `## LETS Notice` block is present in the injected context (a one-time message the SessionStart hook emits — e.g. workflow rules outdated/missing, a migration result), it MUST be the **first line of your output** for `/lets:start` (`⚠️ <notice text>`), before the session summary. Don't omit it — it tells the user a `/lets:update` or `/lets:init` is needed. Mention it once, then continue with the steps below; don't repeat it in later turns.

## Usage

```bash
/lets:start                # Full flow - show history, tasks, select work
/lets:start <task-id>      # Quick start - jump to specific task
/lets:start --continue     # Resume last in_progress task with session context
/lets:start --main         # Project-assistant / PM mode - no task, read + triage on merge-branch
/lets:start --assistant    # Alias of --main
```

## Step 0: Argument Parsing

**If `<task-id>` provided** (e.g., `/lets:start lets-rmcwo`):
- Skip Steps 1, 3, 5 (session history, orient, task selection)
- Run Step 2 (git state) briefly
- the tracker's `show` + `comment-list` for `<task-id>` — read the FULL description and ALL comments, never truncate
- Jump to Step 6 (branch) with this task

**If `--continue`:**
- Run Step 1 (session history) - important for context recovery
- the tracker's `list-by-status` (in_progress) - find task(s)
- If exactly 1 in_progress -> use it, skip Step 5
- If multiple -> show selection with context from recent sessions
- If none -> fall through to full flow

**If `--main` or `--assistant` provided** (project-assistant / PM mode):
- Deliberate **NO-TASK** session stance. Do NOT select, claim, or auto-create a task.
- **Precedence:** mutually exclusive with `<task-id>` and `--continue`. If an explicit task-id or `--continue` is ALSO present, the explicit task **wins** (run the normal task flow) and `--main` is ignored - tell the user it was dropped because a task was specified.
- **Skip** Step 5 (Task Selection), Step 6 (Take Task), Step 8 (Task Size Assessment) - all task-bound.
- **Run** Step 1 (session history), Step 2 (git state). The orient snapshot is rendered once by Main Mode M1 (below), AFTER the session-boundary write - do NOT also run Step 3 (that would render orient twice).
- Then go to `## Main Mode` (below) instead of Steps 4-9.

**If no arguments** -> full flow (Steps 1-9 as below)

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
if git rev-parse --verify --quiet HEAD >/dev/null; then
  git log --oneline -3
else
  echo "(no commits yet — fresh repo)"
fi

# Unfinished PR-review restore. /lets:review keys its record by session id, so the session that
# could act on it is gone - a fresh session is exactly where a stray must be surfaced.
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
for f in "$LETS_PROJECT_ROOT"/.lets/sessions/.review-restore-*; do
  [ -e "$f" ] || continue
  printf 'STRAY REVIEW RESTORE %s -> ref: %s | pr: %s | stash: %s\n' "${f##*/}" \
    "$(sed -n 's/^ref: //p' "$f" | head -1)" \
    "$(sed -n 's/^pr: //p' "$f" | head -1)" \
    "$(sed -n 's/^stash: //p' "$f" | head -1)"
done
```

Report: branch, uncommitted changes, recent commits. **On a `STRAY REVIEW RESTORE` line**, tell the user in one line: a PR review did not finish restoring, `git checkout <ref>` returns them, and a listed `stash:` is still in `git stash list`. Report only - never act: `.lets/` is shared by every worktree of this repo, so the stray may belong to a session that is still running. **If the repo has no commits yet** (the `else` branch above fires), that's fine — say so in plain text; offer `git commit --allow-empty -m "chore: initial setup"` if the user wants an anchor for `git log` to work later. **Don't** raise `/lets:init` here (it's a separate concern) and **don't** treat the missing HEAD as a fatal error.

## Step 3: Orient

Invoke `Skill(skill: "lets:orient")` - it renders Where you are / In flight / Next up (and the Project counts if the tracker provides them). This is the same snapshot `/lets:status` shows; start reuses it, then drives task selection below.

## Step 4: Present

The orient snapshot (Step 3) already shows In flight + Next up - don't repeat them. Present the recovery line, then the snapshot:

```
## Recent Sessions
{1-2 line recovery from the recent session file(s)}

{orient snapshot from Step 3 - Where you are / In flight / Next up / Project}
```

## Step 5: What do we do? (task selection - MANDATORY)

**Every session needs a task.** From the orient snapshot, offer the moves:

> - **Resume** the active task if In flight shows one you want to continue.
> - **Pick** one from Next up (the top 5 ready - for the full list, `/lets:backlog`, or `bd ready` on beads).
> - **Create** a new task (the `create-task` skill - tracker `create` verb), or claim an existing one (tracker `set-status=in_progress`).

**If user doesn't want to pick a task** but describes work (e.g., "just want to fix proxy config"):
- Auto-create via the `create-task` skill (e.g. "Fix proxy config", a P3 task)
- Inform user: "Created task XX, working in feature branch"
- This keeps traceability without friction

**Wait for user to select, create, or describe work.**

## Step 6: Take Task

After task is selected, delegate to the **take-task** skill to claim it and prepare the branch: `Skill(skill: "lets:take-task", args: "<task-id>")`.

The take-task skill handles: setting task to `in_progress`, uncommitted changes check, worktree detection, branch creation/switching, offering worktree option, context recovery, saving session start ref.

## Step 7: Suggest Session Rename

After the task is claimed, suggest renaming the Claude Code session so the statusline reflects the active task. `/rename` is a built-in slash command the **user** invokes — the assistant cannot run it — so present it as a ready-to-paste suggestion, not an action:

> Tip: name this session for quick context — `/rename {slug}`

- Slug: short, lowercase, dash-separated, derived from the task title (e.g. **Add CHANGELOG step to /lets:done** -> `changelog-step-done`). Keep under ~30 chars; compress long titles to the 2-4 most distinctive words.
- This is a one-line suggestion, not a gate — continue regardless of whether the user runs it.

## Step 8: Task Size Assessment

Once task is selected, assess complexity:

| Size | Action |
|------|--------|
| Quick fix (< 30 min) | "OK, let's do it" |
| Small (< 2 hrs) | "OK, let's do it" |
| Medium (2-8 hrs) | Suggest `/lets:plan` then `/lets:execute` |
| Large (> 8 hrs) | Require `/lets:plan` + break into subtasks |

**For Medium/Large tasks, offer the plan-family picker** (skip for Quick/Small - just start working):

```
AskUserQuestion(
  questions=[{
    question: "How do you want to approach this task?",
    header: "Plan",
    options: [
      { label: "Full plan (Recommended)", description: "/lets:plan - explorer + architect + expert agents, thorough" },
      { label: "Fast plan", description: "/lets:plan --fast - orchestrator-only, talk it through together" },
      { label: "Autonomous plan", description: "/lets:plan-workflow - PREVIEW, needs Claude Code >= 2.1.154 + paid plan + released plugin; off-context autonomous planning" },
      { label: "Work directly", description: "Small enough - skip planning, start now" }
    ],
    multiSelect: false
  }]
)
```

If the user picks a `/lets:*` option, auto-invoke it via the `Skill` tool per the AskUserQuestion follow-through rule (Rule 7). "Work directly" -> proceed to Step 9.

## Step 9: Ready to Work

After task is selected and branch is ready, show reminders and welcome box.

## Main Mode (--main / --assistant)

A persistent project-assistant / personal-PM session stance. NOT tied to a task. Read + triage only on `$LETS_MERGE_BRANCH`.

### Persona (adopt for the whole session)

You are the **project orchestrator** - a pragmatic technical PM for THIS repository. For this session you:
- Discuss general and strategic questions about the project.
- Triage and groom the backlog: surface stale / duplicate / mis-prioritized tasks, propose structure and labels.
- Create and refine tracker tasks (via the `create-task` skill - user approves each).
- Capture decisions, facts, and gotchas (`/lets:note`, or point the user to it).
- Route the user to the right `/lets:*` command when concrete work starts.

You do **NOT** write or edit code in this mode. The moment the user wants to implement / edit something, hand off to a task (see **Pivot to code**) - do not start editing.

> This persona is hardcoded for v1. A future persona registry (separate epic) will let it be swapped or customized; keep it as one self-contained block.

### Step M1: Orient

**Guard first.** Main mode expects `HEAD == $LETS_MERGE_BRANCH`. If on another branch (a worktree or feature branch - which are task-bound), say so in one line, suggest the normal task flow instead, and **skip the session-boundary write below** - stay strictly read-only if the user still wants the overview.

Steps 1-2 already ran (sessions, git). Main mode skips `take-task`, so - **only when `HEAD == $LETS_MERGE_BRANCH`** - save the **session boundary** here (so `/lets:end` can still diff the session). Main mode claims no task, so it does NOT create a `task:`/`start:` (that would make `.task-main` mis-fire trunk-mode). But it must NOT destroy a live trunk claim either - so it **preserves** any existing `task:`/`start:` (merge-write, like the SessionStart hook) and refreshes only `session:`. A genuine main-mode file (no prior `task:`) stays `session:`-only; a preserved live claim then surfaces through orient below instead of being silently clobbered. Then add a one-line backlog pulse:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
# Write the session boundary ONLY on the merge-branch (main mode is a merge-branch stance); on any
# other branch this is a read-only overview - skip the write entirely (see the Guard above).
if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
  mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
  TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
  # Preserve an existing live trunk claim (task:/start:) - a full-file session:-only write would
  # destroy it, and its /lets:done would then hard-abort with no start:. Merge-write, like the hook.
  PREV_TASK=""; PREV_START=""
  if [ -f "$TASK_FILE" ]; then
    PREV_TASK=$(sed -n 's/^task: //p' "$TASK_FILE" | head -1)
    PREV_START=$(sed -n 's/^start: //p' "$TASK_FILE" | head -1)
  fi
  tmp=$(mktemp "${TASK_FILE}.XXXX")
  {
    [ -n "$PREV_TASK" ] && echo "task: $PREV_TASK"
    [ -n "$PREV_START" ] && echo "start: $PREV_START"
    printf 'session: %s %s\n' "$(git rev-parse HEAD)" "$CLAUDE_CODE_SESSION_ID"
  } > "$tmp" && mv -f "$tmp" "$TASK_FILE"
fi
```

Invoke `Skill(skill: "lets:orient")` - with no active task it degrades to branch + no-task + In flight + Next up + Project, which IS the PM triage surface. Keep it short - on beads, point to `bd stats` / `bd blocked` for the deep dashboard.


### Step M2: Set the stance

Tell the user, in one short paragraph: you're in main / assistant mode - no active task, staying on `$LETS_MERGE_BRANCH`, here to triage / groom / decide / route. Name the tools available: `/lets:backlog` (review / cleanup / `--fast` pulse), `/lets:opinion` (think through a decision or topic), `/lets:research` (sourced external answer), `/lets:status`, `create-task`, `/lets:note`.

Do **NOT** suggest `/rename` to a task slug (there is no task). Offer a generic slug like `/rename main` only if the user wants it.

### Step M3: Work the session (interactive)

Respond as the persona. Common moves and where they route:

| User intent | Route to |
|-------------|----------|
| Review / clean up the backlog | `/lets:backlog` |
| Quick no-agent backlog pulse | `/lets:backlog --fast` |
| Think through a decision / topic | `/lets:opinion <topic>` |
| Orient snapshot (where are we) | `/lets:status` |
| Deep beads dashboard / dependency view | `bd stats` / `bd blocked` (beads-native) |
| New task | `create-task` skill (user approves) |
| Capture a decision / fact / gotcha | `/lets:note` |

Stay non-pushy; one suggestion at a time. The session continues in this stance until the user pivots to code or ends.

### Pivot to code (hand-off)

When the user signals concrete implementation ("let's build / fix / edit X"):

- **STOP** - main mode does not edit code. Per the merge-branch boundary, a task + branch are required first.
- Offer via `AskUserQuestion` (header `Start work`):
  - **Claim a task** -> ask which, then `Skill(skill: "lets:take-task", args: "<id>")`.
  - **Create a task** -> `create-task` skill, then `take-task`.
  - **Stay in main mode** -> not ready to code yet; continue triage.
- On claim, `take-task` creates / switches the branch and the session leaves main mode (normal task flow resumes).

### Main Mode Output

(No "Working on: {task}" box - there is no task.)

```
## Main Mode
No active task - project-assistant / PM stance on `{LETS_MERGE_BRANCH}`.

┌─ LETS - main ──────────────────┐
│  Triage?      /lets:backlog    │
│  Decide?      /lets:opinion    │
│  Status?      /lets:status     │
│  Start work?  /lets:start <id> │
└────────────────────────────────┘
```

## Output

(Main mode has its own output above; the box below is for the normal task flow.)

```
## Reminders
- Name this session: `/rename {slug}`
- Check context window: `/context`
- For technical decisions: `/lets:opinion`
- When task done: `/lets:commit` - `/lets:done` - `/lets:end`

**Working on:** **{task title}** (`{task-id}`) on `{branch-name}`

┌─ LETS ─────────────────────────┐
│  Plan?    /lets:plan           │
│  Check?   /lets:check          │
│  Note?    /lets:note           │
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
