---
description: End a work session - a settlement pass that reconciles uncommitted / unpushed work + session context into git, the tracker, and a session snapshot file. --session and --pre-compact skip settlement and only write the shared snapshot, keeping the session going.
argument-hint: "[--session|--pre-compact]"
---

# Session End

End a work session cleanly. `/lets:end` is a **settlement pass**: it reconciles the volatile session state (uncommitted changes, unpushed commits, in-conversation context) into durable stores (git remote, the tracker, a session-snapshot file) so the window can close - or compact - without losing anything.

**This is NOT task completion.** Use `/lets:done` to finish a TASK; `/lets:end` ends a SESSION.

**Invariant:** `/lets:end` only READS the `.task-<slug>` `session:` boundary - it NEVER writes it. The boundary writers are `take-task` (at `/lets:start`) and the SessionStart hook (refresh on a genuinely new session). End mutating the boundary was the old `--fast` divergence bug; it stays gone.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Modes

One settlement core (the default flow); the flags below are separate paths, NOT modifiers of it. Two of them are snapshot-only and differ ONLY in what the record says it is for:

- **(default)** - full settlement pass (Steps 1-3) + worktree hint + a one-line terminal prose hint (Step 4 / Output). Auto-skip keeps it silent on a tidy session.
- **`--session`** (alias `--snapshot`) - a session record on request, **NOT a session end and NOT a settlement pass**. It runs NO settlement (no commit / push / progress / finish offers) - it ONLY writes the shared session snapshot via `session-snapshot` (`kind=session`) and the session continues. Use it to bank the state of a long session without ending or compacting anything. **Identical to `/lets:note --session`.** See the early-exit at the top of Step 1.
- **`--pre-compact`** (alias `--compact`) - the same snapshot, branded as a pre-compaction record (`kind=precompact`, a `-snapshot-precompact` filename, and a `### Compaction` line naming `/compact`). Use it only when a `/compact` actually follows; otherwise `--session` is the honest flag, since the file is a permanent record and `/lets:start` reads it back. **Identical to `/lets:note --pre-compact`.**
- **`--fast`** - DEPRECATED. It now runs the default flow (which already stays silent when there is nothing to settle). Emit one line - `--fast is deprecated; running the unified /lets:end (it auto-skips when there's nothing to settle).` - then proceed as default. (Accepted for one release; removal is a follow-up.)

## Step 1: Detect (silent)

**Snapshot-only early exit (runs BEFORE any settlement detection):** if invoked with `--session` / `--snapshot` or `--pre-compact` / `--compact`, do NOT run the settlement detect/settle steps. Delegate straight to the snapshot primitive with the matching `kind` - `Skill(skill: "lets:session-snapshot", args: "kind=session pointer=auto")` or `kind=precompact` - then show that flag's Output and STOP. Both paths are byte-identical to the same flag on `/lets:note`. Steps 1-3 below are the DEFAULT (settlement) flow only.

Read all state ONCE, compute which settlements are actionable, prompt nothing here.

First find the active task: use the **detect-task** skill - `Skill(skill: "lets:detect-task")`. (No task -> S2/S3 below auto-skip; main-mode `/lets:end` is a normal, mostly-silent settle.)

Then, **only if detect-task returned a task**, read its STATUS - S2 and S3 both gate on it, and detect-task's own liveness probe runs only on `$LETS_MERGE_BRANCH`, so off the merge-branch nothing here knows the status yet. No task means no read and no id to substitute; skip straight to the state block below:

```lets-tracker
show task=<task-id>   # returns {id,title,status}; read status
```

`/lets:end` runs once per session, so one tracker round-trip is affordable here - unlike the hot paths detect-task deliberately guards. If `show` is unsupported (`LETS_TRACKER=none`) or fails, say so in ONE line and treat the status as UNKNOWN: S2 and S3 both stand down rather than guess a task's state. A settlement offered on a guessed status is the same class of error as a range reported from an unvalidated boundary.

Then resolve the session boundary through the shared reader - `Skill(skill: "lets:session-boundary")`. It echoes `SESSION_BOUNDARY` / `SESSION_TRUST` / `SESSION_COMMITS` / `SESSION_RANGE_DESC`; read them verbatim and surface its stderr NOTEs. Do NOT re-derive any of it here: the validation ladder lives in that skill alone, because a second copy is a second answer, which is how a stale boundary once became a confident "260 commits" for a 6-commit session (lets-370mx). `RANGE_DESC` below IS its `SESSION_RANGE_DESC`.

Then one bash block for the rest of the state:

```bash
# Echoed, not just assigned: a bash var dies with the fence, and three consumers downstream need
# the branch - the Output line, Step 4's worktree hint, and S5's label, whose whole job is to name
# the concrete push target (informed consent).
BRANCH=$(git branch --show-current); echo "BRANCH=$BRANCH"

git status --short   # DIRTY if non-empty (S1)

# Unpushed (S5), upstream-aware (mirrors /lets:done Step 8)
if git rev-parse --abbrev-ref @{u} >/dev/null 2>&1; then
  AHEAD=$(git rev-list --count @{u}..HEAD); echo "AHEAD=$AHEAD"
else
  echo "AHEAD=no-upstream ($(git rev-list --count HEAD 2>/dev/null || echo 0) local commits)"
fi

# Worktree (S6)
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null); echo "GIT_DIR=$GIT_DIR"
```

**Actionable set:** S1 if DIRTY; S2 if the active task is `in_progress` AND `SESSION_COMMITS > 0`; S3 if the active task's status is `open` or `in_progress` AND this session touched it (`SESSION_COMMITS > 0` OR DIRTY OR `AHEAD > 0`); S5 if `AHEAD > 0` (or no-upstream + local commits exist). S4 always; S6 if `GIT_DIR` contains `worktrees/`. (No S7 - the snapshot-only flags are Step 1 early exits, not settlement steps.)

**Boundary-trust rule.** `SESSION_TRUST` never gates an OFFER - it gates what may be CLAIMED. S2 is offered on any boundary; what changes is the wording, because `RANGE_DESC` carries the level with it into the chat, the snapshot's `### Range` block and the progress comment. That is the whole mechanism: a weak boundary degrades the sentence, it does not suppress the record. (`session-boundary` defines the levels - do not restate them here.)

| `SESSION_TRUST` | the range in writing |
|---|---|
| `exact` | stated as this session's work |
| `prior-session` | marked best-effort, NOTE carried |
| `estimate` | marked as the branch's range, not the session's |
| `none` | no number at all |

**S3 is deliberately off this axis.** Whether to refer the user to `/lets:done` is a status fact plus observable activity, never an inference from a commit count about whether the work is finished. That inference is `/lets:done`'s Scope Verification step, which reads the task description; `end` does not attempt it. The gate is `open`/`in_progress` rather than "not closed" because `in_review` means `/lets:done` ALREADY ran (it sets that status after opening the PR) and `blocked` is not a state `done` can advance - referring either one sends the user in a circle. S1/S4/S5/S6 are unaffected by `SESSION_TRUST`.

## Step 2: Settle

If the actionable set (S1/S2/S3/S5) is **empty**, write nothing here and continue - a tidy session produces ZERO prompts.

Otherwise present ONE consolidated prompt with only the actionable options:

```
AskUserQuestion(
  questions=[{
    question: "Found {N} things to settle before ending. Pick what to do (none = end as-is):",
    header: "Settle",
    options: [
      { label: "Commit", description: "Run /lets:commit for the uncommitted changes" },            # S1, only if DIRTY
      { label: "Post progress", description: "Add a Session Progress comment to **{task title}** ({task-id})" },  # S2, only if actionable
      { label: "Finish task", description: "**{task title}** ({task-id}) is still open - hand off to /lets:done" }, # S3, only if actionable (the snapshot-only flags never reach Step 2 - they early-exit in Step 1)
      { label: "Push", description: "Push {N} commits to origin/{branch} (no PR)" }                  # S5, only if AHEAD>0; label names the concrete target = informed consent
    ],
    multiSelect: true
  }]
)
```

**Execution order (run the picks in this fixed order; deselect-all = "end as-is"):**

1. **Commit** -> `Skill(skill: "lets:commit")`. (If both Commit and Finish task are picked, commit runs first, then hand off.)
2. **Finish task** -> **REFER + STOP.** Say "Handing off to /lets:done" and name any other picks being dropped with it (`multiSelect` lets the user select alongside it, and silently discarding a pick is worse than declining it). Then `Skill(skill: "lets:done")` and **STOP the entire /lets:end flow** (do NOT run Step 3, Step 4, or Output). That is the whole rule, and it is deliberately the whole rule: `end` does not enumerate what `done` will redo, because once `end` stops there is nothing left for it to reason about. `done` owns the task lifecycle from here - its own scope check, its commit/push/PR/close, and its terminal menu, which may switch branches or call `/lets:end` again. That re-entry is safe BECAUSE S3 gates on `open`/`in_progress`: `done` leaves the task `closed` or `in_review`, neither of which re-arms this offer. Stopping is what keeps the branch-slugged snapshot off a branch `done` may have moved away from.
3. **Push** (only reached when Finish task was NOT picked) -> upstream-aware push of the current branch, NO PR, never `--force`: `git push` when upstream exists and ahead; `git push -u origin <branch>` on first push. The explicit multiSelect pick IS the approval (same as picking "Commit" authorizes the commit - satisfies the git.md / AUTO MODE "push needs explicit approval" rule, which is why the option label always names the concrete target).
4. **Post progress** (only reached when Finish task was NOT picked) -> remember this pick; the progress comment is written in Step 3b, AFTER Step 3a produces the snapshot file it references. Do NOT write it here.

## Step 3: Write artifacts

(Reached in the DEFAULT flow only; the snapshot-only early exit in Step 1 handled those paths. Skipped on the Finish-task hand-off, which stopped the flow in Step 2.)

### 3a. Session snapshot (ALWAYS, written FIRST)

Write the session-level snapshot that bootstraps the next `/lets:start`, via the shared primitive so every snapshot kind shares one template and none of them drift. The `pointer` arg depends on whether "Post progress" was picked in Step 2:

`Skill(skill: "lets:session-snapshot", args: "kind=end pointer=<off if Post progress was picked, else auto> task-id={task-id} range={RANGE_DESC}")` - `task-id` from Step 1; `range` LAST (contains spaces).

- `pointer=off` (Post progress picked) -> the skill writes NO task comment; end folds the pointer into the progress comment's `### Snapshot` line (3b).
- `pointer=auto` (Post progress NOT picked) -> the skill writes the standalone one-line pointer to its own file; 3b then does nothing task-side. Either way exactly ONE task comment, and the pointer string lives ONLY in the skill (Step 4) - no cross-file duplication.
- `kind=end` -> the `snapshot` artifact kind (no `-precompact` suffix); the path comes from the skill's return, never rebuild it. `range` -> the skill includes the `### Range` block. Capture the returned `SNAP_FILE` path - used in 3b + the Output block.

### 3b. Progress comment (only when "Post progress" was picked)

**If "Post progress" was picked in Step 2** (3a passed `pointer=off`): write the `## Session progress` comment. **MANDATORY:** the `Claude session: $CLAUDE_CODE_SESSION_ID` line MUST appear between `## Session progress` and `### Range` (bash expands the env var at runtime in the heredoc, so the tracker gets the literal UUID). The commit LIST is intentionally dropped (git owns it); keep a range pointer. End the comment with a `### Snapshot` line pointing at 3a's file - this IS the task-side pointer, so do NOT also write a standalone one. The bash block writes the body to a temp file; the `comment-add` verb submits it via `body-file=` (lets-rules "Tracker Adapters" - the orchestrator fills the `{...}` narrative fields first):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
cat > "$LETS_PROJECT_ROOT/.lets/cache/progress-<task-id>.md" <<EOF
## Session progress $(date +%Y-%m-%d)

Claude session: $CLAUDE_CODE_SESSION_ID

### Range
{RANGE_DESC}

### Done
- {narrative of what was completed this session}

### Remaining
- {what's left}

### Context for next session
- {recovery info}

### Snapshot
- .lets/sessions/{SNAP_FILE basename}
EOF
```

```lets-tracker
comment-add task=<task-id> body-file=.lets/cache/progress-<task-id>.md
```

**If "Post progress" was NOT picked:** do NOTHING here - 3a passed `pointer=auto`, so the skill already wrote the standalone pointer to its own file (the pointer string lives ONLY in the skill's Step 4 - no duplication).

## Step 4: Worktree hint

Output-time, never a prompt. If `GIT_DIR` contains `worktrees/`: extract the worktree name (last path segment), then pick by the status read in Step 1 - `closed` -> the cleanup line (`/lets:worktree remove {name}` from the main repo), anything else -> the resume line.

Both branches are reachable, and not through Step 2: the referral stops end before Step 4, so this is never the hand-off path. The cleanup branch exists for the INDEPENDENT route - `/lets:done` runs on its own, closes the task, and its worktree menu offers "End session", which lands here with a closed task. Printing a resume hint there contradicts what `done` just said. If the status is UNKNOWN (no task, or the read failed), show the resume line: it is the safe default, since it points at work rather than at deletion.

## Output

### Default

```
## Session End

Git: {clean / N uncommitted}
Branch: {branch}
Task: {task-id and title, or "none"}
Settled: {e.g. "committed; pushed 3" / "nothing - tidy session"}
Snapshot: .lets/sessions/{date}-{HHMM}-{task-id}-snapshot.md
```

If in a worktree, append the resume line (`cd {LETS_PROJECT_ROOT from LETS Config} && claude -> /lets:start`).

Then a SINGLE prose line (no AskUserQuestion, no wrap-up card):

> Run `/compact` to keep this window lighter, or `/clear` + `/lets:start` for a fresh session.

(`/compact` and `/clear` are user-typed Claude Code commands - never auto-execute them. `/compact` keeps the window with a compacted summary; the PreCompact hook re-injects the rules and the summary just written survives.)

### --session

```
## Session Snapshot

Snapshot -> .lets/sessions/{date}-{HHMM}-{task-id}-snapshot.md
Task pointer -> {task-id}  (only if a task is unambiguously active; else "none - file only")
Branch: {branch}
Range: {RANGE_DESC returned by the skill, or "none - no valid session boundary"}

Recorded - the session continues. Resume later: /lets:start reads the snapshot file.
```

Then STOP - no AskUserQuestion, no settlement, no push, no `git checkout`. (Identical output contract to `/lets:note --session`.)

### --pre-compact

```
## Pre-Compact Snapshot

Snapshot -> .lets/sessions/{date}-{HHMM}-{task-id}-snapshot-precompact.md
Task pointer -> {task-id}  (only if a task is unambiguously active; else "none - file only")
Branch: {branch}
Range: {RANGE_DESC returned by the skill, or "none - no valid session boundary"}

Safe to /compact now - same window continues. Resume: /lets:start reads the snapshot file (the tracked task holds task-level context).
```

Then STOP - no AskUserQuestion, no settlement, no push, no `git checkout`. The session continues. (Identical output contract to `/lets:note --pre-compact`.)

## Rules

- **end never writes the `.task-<slug>` boundary** - it only reads `session:` (take-task + the hook own writes).
- **Tidy session = silent wrap** - when nothing is actionable, end produces zero prompts.
- **NEVER push without explicit user approval** - the Step 2 multiSelect pick is that approval.
- **Always write the session snapshot** via the shared `session-snapshot` primitive (except when Step 2 referred out and stopped - end wrote nothing because end did not run). The snapshot is file-primary; the task gets at most a one-line pointer.
- **`--session` and `--pre-compact` run NO settlement** (snapshot-only; both early-exit at the top of Step 1). They differ only in what the written record says it is for - never reach for `--pre-compact` when no `/compact` follows.
- **End with a one-line compact/clear prose hint, never a wrap-up card.**
- **Refer to `/lets:done`, never finish a task** - offer the hand-off only when the task is open AND this session touched it, and never judge whether the work is complete. `end` is a SESSION command; the task lifecycle is `done`'s.
- Respond in user's language.
