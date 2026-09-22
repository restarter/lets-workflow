---
description: Start session - restore context, show tasks, select work item
argument-hint: '[task-id|--continue|--main|--assistant] [--scope "<part>"] [--orc=<name>]'
---

# Session Start

Restore context and prepare for work. **User MUST select a task before working.**

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

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

### Step 0a: Parse

Parse the arguments only - a task id (it goes through the detect-task id gate before any use), `--continue`, `--main` / `--assistant`, `--scope "<text>"` (main mode: the part of the repo this orchestrator owns). `--orc` is stripped per the detect-task explicit-argument convention before the task-id test and forwarded to take-task; with `--main` it is dropped with a one-line note (an orchestrator is never bound). No tracker verb runs here. Every path below runs **Step 0.5 first**, then its own steps.

**If `<task-id>` provided** (e.g., `/lets:start lets-rmcwo`):
- Step 0.5 (with `--task '<task-id>'`), then:
- Skip Steps 1, 3, 5 (session history, orient, task selection)
- Run Step 2 (git state) briefly
- the tracker's `show` + `comment-list` for `<task-id>` — read the FULL description and ALL comments, never truncate
- Jump to Step 6 (branch) with this task

**If `--continue`:**
- Step 0.5, then:
- Run Step 1 (session history) - important for context recovery
- Run Step 2 (git state) briefly
- the tracker's `list-by-status` (in_progress) - find task(s)
- If exactly 1 in_progress -> use it. **Skip Step 3 (orient) and Step 5** - the task is already known, so the Next up / Project snapshot is noise (and it would re-run `list-by-status` a second time inside orient). Present only the `## Recent Sessions` recovery line (Step 4), then jump to Step 6 (take-task) - its Step 6 context recovery reads the task comments + latest snapshot
- If multiple -> run Step 3 (orient) and show selection with context from recent sessions
- If none -> fall through to full flow

**If `--main` or `--assistant` provided** (project-assistant / PM mode):
- Step 0.5, then:
- Deliberate **NO-TASK** session stance. Do NOT select, claim, or auto-create a task.
- **Precedence:** mutually exclusive with `<task-id>` and `--continue`. If an explicit task-id or `--continue` is ALSO present, the explicit task **wins** (run the normal task flow) and `--main` is ignored - tell the user it was dropped because a task was specified.
- **Skip** Step 5 (Task Selection), Step 6 (Take Task), Step 8 (Task Size Assessment) - all task-bound.
- **Run** Step 1 (session history), Step 2 (git state). The orient snapshot is rendered once by Main Mode M1 (below), AFTER the session-boundary write - do NOT also run Step 3 (that would render orient twice).
- Then go to `## Main Mode` (below) instead of Steps 4-9.

**If no arguments** -> Step 0.5, then the full flow (Steps 1-9 as below)

### Step 0.5: Link an unlinked worktree (fallback self-heal)

Runs on every path BEFORE any `show` / `comment-list` / `list-by-status`. The SessionStart hook normally adopts a worktree someone else created (Orca, a teammate) before this session's LETS Config is built; this is the fallback for a session whose hook could not (a `lets` binary older than the self-heal, a disabled hook). `{TASK_FLAG}` is `--task '<task-id>'` when an explicit id was given (after the id gate), else empty:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
# Absolute on both sides: from a subdirectory of the main checkout --git-dir is absolute and
# --git-common-dir relative, so a plain comparison would "find" a worktree there.
if [ "$(git rev-parse --path-format=absolute --git-dir)" != "$(git rev-parse --path-format=absolute --git-common-dir)" ] && [ ! -L "$LETS_PROJECT_ROOT/.lets" ]; then
  if command -v lets >/dev/null 2>&1; then
    lets worktree adopt --dir "$LETS_PROJECT_ROOT" --plugin-root "${CLAUDE_PLUGIN_ROOT}" {TASK_FLAG} --json
  else echo "LETS_BINARY_MISSING"; fi
fi
```

No output -> a main checkout or an already linked worktree; continue silently. Otherwise:
- **`ok=true`:** when `moved_aside` is set, one line naming the moved directory (safe to delete by hand); when `task.origin` is `branch` or `dir`, one line `task <id> derived from the <branch|directory> name - take-task confirms it`; a step warning `adapter_not_in_checkout` / `board_not_in_checkout` -> one line each (this session did not load that file: commit `.claude/rules` or restart). Then one line: this session's LETS Config was computed before the worktree was linked - `/clear` re-injects it.
- **Any error envelope** (exit 22, 23, 24, 25, 17, 19 or anything else): print `error.kind` + `error.remediation` and STOP. Never retry with a force flag - adopt deletes nothing and the conflict is the user's to resolve.
- **`LETS_BINARY_MISSING`:** STOP and print the manual steps: symlink `.lets` to the main checkout's `.lets`, then create each link the active tracker adapter's `## Worktree` `links:` declares (beads: `.beads/.env` -> the main checkout's, mode 0600).

## Step 1: Previous Session Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"

# Read last 3 snapshots: task-id-named first (artifact-path naming, lets-05c4s), then the branch
# slug (legacy {TS}-{branch}.md AND taskless {branch}-{hex}-snapshot), then any branch.
TASK_ID=$(sed -n 's/^task: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}" 2>/dev/null | head -1)
SESSIONS=""
[ -n "$TASK_ID" ] && SESSIONS=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-${TASK_ID}-snapshot"*.md 2>/dev/null | head -3)
[ -z "$SESSIONS" ] && SESSIONS=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*"-${BRANCH_SLUG}"*.md 2>/dev/null | head -3)
[ -z "$SESSIONS" ] && SESSIONS=$(ls -t "$LETS_PROJECT_ROOT/.lets/sessions/"*.md 2>/dev/null | head -3)
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

**Orchestrator pointer (worktree only).** Only when `git rev-parse --path-format=absolute --git-dir` differs from `git rev-parse --path-format=absolute --git-common-dir`, and never in a spawned `--flow` / `--auto` run (a `.lets/cache/pipeline-state-<id>` marker exists, or AUTO MODE is active):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
command -v lets >/dev/null 2>&1 && lets peers orchestrator --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" --json 2>/dev/null
```

- `bound` / `single` with a live target: `lets peers tail --to-session <target.session> --addressed-to-session "$CLAUDE_CODE_SESSION_ID" --count-only --json 2>/dev/null` returns `addressed_to_me{count, last_at}` and no text. Render `Orchestrator: {name} ({bound | the only one alive}, {target.state})`, plus `{N} message(s) from {name} - /lets:orc read` when the count is non-zero. No peer text enters this session unless the user asks.
- `ambiguous`: `Orchestrators: {name (scope)}, ... - this branch is not bound; /lets:start <id> --orc="<name>" binds it`.
- bound but not alive: `Orchestrator: {name} (bound, not alive)`. `none`: `no orchestrator alive`. No binary or a stub reason: say nothing.
- a `refused[]` entry with no `target`: `Orchestrator: {name} ({reason})` plus the `hint` when present. Nothing is sent and no offer is shown.
- `source=none` with `reason=budget_exhausted` or `reason=branch_unreadable`: resolution did not complete (not the same as no orchestrator being alive). Render `Orchestrator: resolution did not complete ({reason})`. Nothing is sent and no offer is shown.

## Step 3: Orient

Invoke `Skill(skill: "lets:orient", args: "caller=start")` - it renders Where you are / In flight / Next up (and the Project counts if the tracker provides them). This is the same snapshot `/lets:status` shows; start reuses it, then drives task selection below.

## Step 4: Present

The orient snapshot (Step 3) already shows In flight + Next up - don't repeat them. Present the recovery line, then the snapshot:

```
## Recent Sessions
{1-2 line recovery from the recent session file(s)}

{orient snapshot from Step 3 - Where you are / In flight / Next up / Project}
```

`--continue` with exactly one in_progress task skipped Step 3 - present only `## Recent Sessions` + one line naming the task being resumed, no snapshot.

## Step 5: What do we do? (task selection - MANDATORY)

**Every session needs a task.** From the orient snapshot, offer the moves:

> - **Resume** the active task if In flight shows one you want to continue.
> - **Pick** one from Next up (the top 5 ready - for the full list, `/lets:backlog`).
> - **Create** a new task (the `create-task` skill - tracker `create` verb), or claim an existing one (tracker `set-status=in_progress`).

**If user doesn't want to pick a task** but describes work (e.g., "just want to fix proxy config"):
- Auto-create via the `create-task` skill (e.g. "Fix proxy config", a P3 task)
- Inform user: "Created task XX, working in feature branch"
- This keeps traceability without friction

**Wait for user to select, create, or describe work.**

## Step 6: Take Task

After task is selected, delegate to the **take-task** skill to claim it and prepare the branch: `Skill(skill: "lets:take-task", args: "<task-id>")`.

The take-task skill handles: setting task to `in_progress`, uncommitted changes check, worktree detection, branch creation/switching, offering worktree option, context recovery, saving session start ref.

**Orca card (only when `{LETS_LAUNCHER}` is `orca`; otherwise skip this whole paragraph - no `lets` call, no `show`).** Reuse the title take-task already resolved. Only when that title is not in context:

```lets-tracker
show task=<id>   # returns {id,title,status}; <id> already passed the detect-task gate
```

```bash
[ "{LETS_LAUNCHER}" = "orca" ] && lets orca card --phase start --comment 'task {id}: {title}' --json 2>/dev/null || true
```

The `none` adapter, or a `show` that is absent or failed: comment `'task {id}'` (never an improvised title). Single-quote the values (`'\''` for a quote inside). No id: skip.

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

Steps 1-2 already ran (sessions, git). Main mode skips `take-task`, so - **only when `HEAD == $LETS_MERGE_BRANCH`** - save the **session boundary** here (so `/lets:end` can still diff the session). Main mode claims no task, so it does NOT create a `task:`/`start:` (that would make `.task-main` mis-fire trunk-mode). But it must NOT destroy a live trunk claim either - so it **preserves** every existing line (`task:`, `start:`, lines a newer LETS added; merge-write, like the SessionStart hook), refreshes `session:`, and clears `orc:` (the merge-branch never carries an orchestrator binding). A genuine main-mode file (no prior `task:`) stays `session:`-only; a preserved live claim then surfaces through orient below instead of being silently clobbered. Then add a one-line backlog pulse:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
# Write the session boundary ONLY on the merge-branch (main mode is a merge-branch stance); on any
# other branch this is a read-only overview - skip the write entirely (see the Guard above).
if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
  mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
  TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
  # Preserve an existing live trunk claim (task:/start:) - a full-file session:-only write would
  # destroy it, and its /lets:done would then hard-abort with no start:. `lets worktree task-state`
  # owns the file (merge-write under a lock); without the binary, replace only session: and orc:.
  if command -v lets >/dev/null 2>&1; then
    lets worktree task-state set --clear-orc --session-sha "$(git rev-parse HEAD)" --session-id "$CLAUDE_CODE_SESSION_ID" --create --json
  else
    tmp=$(mktemp "${TASK_FILE}.XXXX")
    { [ -f "$TASK_FILE" ] && grep -v -e '^session: ' -e '^orc: ' "$TASK_FILE"; printf 'session: %s %s\n' "$(git rev-parse HEAD)" "$CLAUDE_CODE_SESSION_ID"; } > "$tmp" && mv -f "$tmp" "$TASK_FILE"
  fi
fi
```

**Register as an orchestrator (merge-branch only, after the boundary write).** `{SCOPE_FLAG}` is `--scope '<text>'` when `--scope` was given (single-quoted, `'\''` escaping), else empty:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
command -v lets >/dev/null 2>&1 && lets peers role set orchestrator --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" {SCOPE_FLAG} --json || echo "LETS_BINARY_MISSING"
```

- `granted=true`: nothing to say.
- `granted=false reason=name_held`: another live session already holds this name (name the holder's `alive` when it is `unknown`):

```
AskUserQuestion(
  questions=[{
    question: "Session {holder.name} ({holder.session6}, {holder.alive}) is already the orchestrator under this name. What now?",
    header: "Role",
    options: [
      { label: "Rename this session (Recommended)", description: "/rename to another name, then /lets:start --main again; unregistered until then" },
      { label: "Take over {holder.name}", description: "The other session becomes a plain peer; nothing is sent to it" }
    ],
    multiSelect: false
  }]
)
```

  **Take over** -> rerun `lets peers role set orchestrator --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" {SCOPE_FLAG} --takeover --json`, then the prose hint `/lets:orc tell {holder.name} ...` (tell it, if it should know). **Rename** -> stop registering; main mode continues unregistered.
- `orchestrator_needs_name`: one line `this session has no name - /rename <name>, then /lets:start --main again`; main mode continues unregistered.
- `session_not_in_registry`: one line naming the registry reason; main mode continues unregistered.
- `LETS_BINARY_MISSING` or a `not_supported` stub reason: one line, then continue.

Invoke `Skill(skill: "lets:orient", args: "caller=start")` - with no active task it degrades to branch + no-task + In flight + Next up + Project, which IS the PM triage surface. Keep it short - if the tracker has a deeper native dashboard, point the user at it in one line.

**Reopen archived and orphaned claims (merge-branch only).** `lets worktree release` (Orca's archive hook) leaves `.lets/cache/released-<task-id>` with one line `<id>|<branch>|<iso>|dirty=<bool>|unpushed=<bool>[|snapshot=<present|stale|missing>]` (older markers have no `snapshot=`) when a worktree goes away while it still named a task. List those markers oldest first; an id outside the detect-task gate class (`[A-Za-z0-9._-]`, no leading `-`) -> delete that marker with a one-line note. Resolve each remaining id, one block per id:

```lets-tracker
show task=<id>   # returns {id,title,status}
```

- `show` absent or a no-op (the none adapter): delete every marker, print one line `tracker keeps no task status - nothing to reopen`, ask nothing.
- `show` FAILED at runtime: keep that id's marker and print one line naming the failure (retried at the next main start).
- `closed`, or any status other than `in_progress`: delete the marker silently - nothing is claimed.
- `in_progress`: a candidate.

**Orphans - a worktree that vanished without a marker.** A worktree removed outside Orca's archive hook (gh >= 2.99 `gh pr merge --delete-branch` removes the head's linked worktree) leaves no marker at all. Find the claims that lost theirs:

```lets-tracker
list-by-status status=in_progress   # returns [{id,title,status}]
```

Keep the ids that pass the detect-task gate class and have no marker offered above, then ask Go about all of them in ONE call (single-quote each id):

```bash
command -v lets >/dev/null 2>&1 && lets worktree record --task '<id1>' --task '<id2>' --json
```

A row with `orphan=true` is a candidate too: in progress, a local trace (a task-state file or a local branch), no worktree holds it, no marker. It joins the same question and the same 3-id cap. An orphan has no marker to delete, so an unpicked one is offered again at the next main start - it stays visible until the task leaves `in_progress`. `list-by-status` absent or failed, `record` failed, or no `lets` binary: one line, no orphans.

No candidates -> ask nothing. Otherwise ask, offering at most 3 ids (the rest keep their markers; orphans past the cap are offered next time):

```
AskUserQuestion(
  questions=[{
    question: "These tasks are still in progress but their worktree was archived or vanished{, with uncommitted or unpushed work when the marker says so}. A task with an open PR is in progress too - pick only abandoned work. Set which back to open?",
    header: "Reopen",
    options: [
      { label: "<id>", description: "Branch {branch} archived{; dirty/unpushed}; picking sets the task back to open" },
      { label: "<orphan id>", description: "No worktree and no marker; record {record.state}; picking sets the task back to open" },
      { label: "Keep all", description: "Nothing changes; archived markers are not shown again" }
    ],
    multiSelect: true
  }]
)
```

For each picked id (`Keep all` picks none):

```lets-tracker
set-status task=<id> status=open
```

`set-status` absent -> print the degradation line (nothing changed); failed -> say so loudly, never report it reopened. Then delete the markers of every offered marker id, picked or not.


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
| Deep dashboard / dependency view | the tracker's own native view, if it has one |
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
