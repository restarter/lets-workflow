---
name: orient
description: Internal skill for commands. Render the shared orientation snapshot - active task, what's in flight, what's next - tracker-universal. Do not trigger on user conversation; only when a command needs the snapshot.
user-invocable: false
---

# Orient Snapshot

Render the shared "where am I / what's in flight / what's next" snapshot. Consumed by `/lets:status` (snapshot, then stop), `/lets:start` (snapshot + pick/claim), and `/lets:backlog --fast` (snapshot + dialog). Single source of truth so the three never drift.

> This skill ONLY gathers + renders the snapshot block. It does NOT select, claim, mutate, or advise. The caller appends its own follow-through.

Built on git + `detect-task` + neutral tracker verbs, so it degrades cleanly on any tracker (`beads` | `planfix-mcp` | `none`).

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract.

## Step 1: Active task

Invoke `Skill(skill: "lets:detect-task")` -> an id or None. (A skill may invoke another skill; this is a normal nested `Skill`-tool call in the same orchestrator context.)

**Inline fallback** (only if the nested `Skill` call is unavailable): read the task-state file, else parse the branch name - the same precedence `detect-task` uses, minimal form:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
sed -n 's/^task: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}" 2>/dev/null | head -1
# else: id is the segment after feature/ or worktree- , up to the first -<slug> boundary
```

If an id resolves, get its title with the tracker's `show`:

```lets-tracker
show task=<id>   # returns {id, title, status, url}
```

## Step 2: Git state

```bash
git branch --show-current
git status --short   # empty = clean; else count the lines
```

Detect context (one compact line, only if applicable): worktree (git-dir contains `worktrees/`), trunk-mode (HEAD == `{LETS_MERGE_BRANCH}` AND an active task), or main-mode (HEAD == `{LETS_MERGE_BRANCH}`, no task).

## Step 3: In flight

```lets-tracker
list-by-status status=in_progress
```

Mark the active task (from Step 1) with ` <- active`. If `{LETS_PR_FLOW}` == github, add open PRs:

```bash
gh pr list --state open --author @me --json number,title,url,reviewDecision --limit 5 2>/dev/null
```

## Step 4: Next up (flat top-N ready)

```lets-tracker
ready limit=5   # fallback if the tracker marks `ready` absent: list-by-status status=open limit=5
```

Priority-sorted, NOT grouped by label. If more than 5 ready, note `(+N more ready)`.

## Step 5: Project counts (OPTIONAL - adapter capability)

Use the tracker's `stats` capability for the totals - do NOT hand-roll per-status counts or `jq` over task JSON (that fragile path is exactly what this redesign removes).

```lets-tracker
stats
```

- beads -> read the totals `bd stats` already computes (open / in_progress / closed) - native, correct, no pagination risk.
- planfix-mcp -> `stats` is `partial`; OMIT the block in v1.
- none -> `stats` is absent; OMIT the block.

If the active adapter marks `stats` absent, OMIT the `## Project` section entirely - never render an empty/broken block.

## Step 6: Render

```
## Where you are
Branch: `{branch}`  -  {Task: **{title}** (`{id}`) | no active task}  -  {clean | {N} uncommitted}
{context line: worktree / trunk-mode / main-mode - only if applicable}
{Tracker: none - task tracking off   - only when the active tracker is `none`}

## In flight
- **{title}** (`{id}`){ <- active}
- PR #{n} **{title}** - {reviewDecision or pending}     {github only}
{if none: "Nothing in flight."}

## Next up
  P{n}  **{title}** (`{id}`)
  (+{N} more ready)
{if none: "No ready tasks."}

## Project                                               {only if stats present}
{open} open - {wip} in progress - {closed} closed
```

On the `none` tracker, In flight / Next up / Project have no data source - omit those three sections and show only `## Where you are` + the `Tracker: none` line.

## Rules

- Read-only. Never mutate, never claim, never advise - the caller owns follow-through.
- Every task reference is `**Title** (`id`)`, never a bare id.
- Respond in the user's language.
- Degrade section-by-section; a missing capability drops its section, never the whole snapshot.
