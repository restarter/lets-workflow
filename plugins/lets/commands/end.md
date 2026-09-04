---
description: End a work session - a settlement pass that reconciles uncommitted / unpushed work + session context into git, the tracker, and a session snapshot file. --pre-compact skips settlement and only writes the shared snapshot, keeping the session going.
argument-hint: "[--pre-compact]"
---

# Session End

End a work session cleanly. `/lets:end` is a **settlement pass**: it reconciles the volatile session state (uncommitted changes, unpushed commits, in-conversation context) into durable stores (git remote, the tracker, a session-snapshot file) so the window can close - or compact - without losing anything.

**This is NOT task completion.** Use `/lets:done` to finish a TASK; `/lets:end` ends a SESSION.

**Invariant:** `/lets:end` only READS the `.task-<slug>` `session:` boundary - it NEVER writes it. The boundary writers are `take-task` (at `/lets:start`) and the SessionStart hook (refresh on a genuinely new session). End mutating the boundary was the old `--fast` divergence bug; it stays gone.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Modes

One settlement core (the default flow); the two flags below are separate paths, NOT modifiers of it:

- **(default)** - full settlement pass (Steps 1-3) + worktree hint + a one-line terminal prose hint (Step 4 / Output). Auto-skip keeps it silent on a tidy session.
- **`--pre-compact`** (alias `--compact`) - a pre-compaction snapshot, **NOT a session end and NOT a settlement pass**. It runs NO settlement (no commit / push / progress / finish offers) - it ONLY writes the shared session snapshot via `session-snapshot` (`kind=precompact`) and lets the session continue into `/compact`. **Identical to `/lets:note --pre-compact`** - both delegate to the same primitive with the skill owning task detection. See the early-exit at the top of Step 1.
- **`--fast`** - DEPRECATED. It now runs the default flow (which already stays silent when there is nothing to settle). Emit one line - `--fast is deprecated; running the unified /lets:end (it auto-skips when there's nothing to settle).` - then proceed as default. (Accepted for one release; removal is a follow-up.)

## Step 1: Detect (silent)

**`--pre-compact` early exit (runs BEFORE any settlement detection):** if invoked with `--pre-compact` / `--compact`, do NOT run the settlement detect/settle steps. Delegate straight to the snapshot primitive - `Skill(skill: "lets:session-snapshot", args: "kind=precompact pointer=auto")` - then show the `--pre-compact` Output and STOP. This path is byte-identical to `/lets:note --pre-compact`. Steps 1-3 below are the DEFAULT (settlement) flow only.

Read all state ONCE, compute which settlements are actionable, prompt nothing here.

First find the active task: use the **detect-task** skill - `Skill(skill: "lets:detect-task")`. (No task -> S2/S3 below auto-skip; main-mode `/lets:end` is a normal, mostly-silent settle.)

Then one bash block. The `session:` reader is dsdmp's, hardened further (lets-370mx): a recorded SHA is READ, then VALIDATED, and every fallback says so out loud. Do not re-derive it, and do not restore the NON-slugged `.session-start-ref` - one global file answering a per-branch question let any branch adopt another branch's months-old boundary and report it as fact (`done.md` refuses the same file for the same reason).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')

git status --short   # DIRTY if non-empty (S1)

# --- boundary: READ (session: line, then the legacy SLUGGED ref - never a global one) ---
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
SESSION_LINE=$(sed -n 's/^session: //p' "$TASK_FILE" 2>/dev/null | head -1)
START_REF=$(echo "$SESSION_LINE" | awk '{print $1}')
STORED_SID=$(echo "$SESSION_LINE" | awk '{print $2}')
TRUST=exact
# A sid other than this session's means no /lets:start ran here (no take-task / hook refresh), so
# the boundary belongs to a PRIOR session - real and branch-scoped, but not this session's.
[ -n "$START_REF" ] && [ "$STORED_SID" != "$CLAUDE_CODE_SESSION_ID" ] && TRUST=prior-session
if [ -z "$START_REF" ]; then
  START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
  [ -n "$START_REF" ] && TRUST=prior-session
fi
# Defense-in-depth (N1): the boundary is a plugin-written SHA - blank anything non-hex before it
# expands unquoted into the git range, so a hand-edited state file can't inject git option args.
case "$START_REF" in *[!0-9a-f]*) START_REF="" ;; esac

# --- boundary: VALIDATE (a recorded SHA is a claim, not a fact) ---
MERGE_BASE=$(git merge-base HEAD "origin/{LETS_MERGE_BRANCH}" 2>/dev/null || git merge-base HEAD "{LETS_MERGE_BRANCH}" 2>/dev/null)
if [ -n "$START_REF" ]; then
  REJECT=""
  git rev-parse --verify --quiet "${START_REF}^{commit}" >/dev/null 2>&1 || REJECT="not a commit in this repo"
  [ -z "$REJECT" ] && ! git merge-base --is-ancestor "$START_REF" HEAD 2>/dev/null && REJECT="not an ancestor of HEAD (rebase, reset, or another line of work)"
  # Branch-point floor: a session on a feature branch cannot have started before the branch did.
  # Does not apply on {LETS_MERGE_BRANCH}, where there is no branch point to floor against.
  if [ -z "$REJECT" ] && [ "$BRANCH" != "{LETS_MERGE_BRANCH}" ] && [ -n "$MERGE_BASE" ] && [ "$START_REF" != "$MERGE_BASE" ] \
     && git merge-base --is-ancestor "$START_REF" "$MERGE_BASE" 2>/dev/null; then
    REJECT="predates this branch's point off {LETS_MERGE_BRANCH}"
  fi
  # Age floor: a session boundary is hours-to-days old. 14d is the generous outer edge of one
  # session; past it the file is stale, not long-running. Catches the trunk case the floor cannot.
  if [ -z "$REJECT" ]; then
    AGE_D=$(( ( $(date +%s) - $(git show -s --format=%ct "$START_REF") ) / 86400 ))
    [ "$AGE_D" -gt 14 ] && REJECT="${AGE_D} days old - older than any single session"
  fi
  [ -n "$REJECT" ] && { echo "NOTE: recorded boundary ${START_REF} REJECTED (${REJECT}) - not used." >&2; START_REF=""; TRUST=none; }
fi
# Nothing recorded: on a feature branch the branch point is a branch-SCOPED estimate of what this
# branch added - an honest unknown-case answer, unlike a global file written by another branch.
if [ -z "$START_REF" ] && [ "$BRANCH" != "{LETS_MERGE_BRANCH}" ] && [ -n "$MERGE_BASE" ]; then
  START_REF="$MERGE_BASE"; TRUST=estimate
fi

# --- derive the range. RANGE_DESC carries the trust level everywhere it travels (chat, snapshot
# ### Range, progress comment), so a number never appears without the qualifier that earned it. ---
if [ -n "$START_REF" ]; then
  SESSION_COMMITS=$(git rev-list --count ${START_REF}..HEAD)
  case "$TRUST" in
    exact)         RANGE_DESC="session: ${START_REF}..HEAD (${SESSION_COMMITS} commits)" ;;
    prior-session) RANGE_DESC="approx: ${START_REF}..HEAD (${SESSION_COMMITS} commits) - boundary recorded by an earlier session" ;;
    estimate)      RANGE_DESC="approx: ${START_REF}..HEAD (${SESSION_COMMITS} commits) - estimated from the branch point, no boundary was recorded" ;;
  esac
else
  TRUST=none; SESSION_COMMITS=""
  RANGE_DESC="boundary unknown - no valid session boundary for this branch"
fi
# Warn on EVERY fallback, not only on a sid mismatch - a silent fallback is how a stale boundary
# became a confident number (lets-370mx).
case "$TRUST" in
  prior-session) echo "NOTE: boundary is from a previous session (no /lets:start this session) - counts are best-effort." >&2 ;;
  estimate)      echo "NOTE: no session boundary recorded - range estimated from the branch point off {LETS_MERGE_BRANCH}." >&2 ;;
  none)          echo "NOTE: no usable session boundary - commit counts unknown; Finish task will not be offered." >&2 ;;
esac
echo "START_REF=${START_REF:-<none>}  TRUST=${TRUST}  SESSION_COMMITS=${SESSION_COMMITS:-<unknown>}  RANGE_DESC=${RANGE_DESC}"

# Unpushed (S5), upstream-aware (mirrors /lets:done Step 8)
if git rev-parse --abbrev-ref @{u} >/dev/null 2>&1; then
  AHEAD=$(git rev-list --count @{u}..HEAD); echo "AHEAD=$AHEAD"
else
  echo "AHEAD=no-upstream ($(git rev-list --count HEAD 2>/dev/null || echo 0) local commits)"
fi

# Worktree (S6)
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null); echo "GIT_DIR=$GIT_DIR"
```

**Actionable set:** S1 if DIRTY; S2 if active task `in_progress` AND `SESSION_COMMITS > 0`; S3 same gate as S2 AND `TRUST` is `exact` or `prior-session`; S5 if `AHEAD > 0` (or no-upstream + local commits exist). S4 always; S6 if `GIT_DIR` contains `worktrees/`. (No S7 - the pre-compact snapshot is the Step 1 early exit above, not a settlement step.)

**Boundary-trust rule.** `TRUST` decides what may be claimed and what may be offered - a number is only as good as the boundary under it, so the level travels with `RANGE_DESC` into the chat, the snapshot's `### Range` block, and the progress comment:

| `TRUST` | boundary | S2 Post progress | S3 Finish task |
|---|---|---|---|
| `exact` | this session's `session:` line | offer | offer |
| `prior-session` | a `session:` line or legacy slugged ref recorded by an earlier session | offer, NOTE carried | offer, NOTE carried |
| `estimate` | nothing recorded; derived from the branch point off `$LETS_MERGE_BRANCH` | best-effort offer, marked approx | **SKIP** - never finish a task on a guess |
| `none` | nothing usable (rejected, or absent while HEAD is `$LETS_MERGE_BRANCH`) | best-effort offer, no number | **SKIP** |

S1/S4/S5/S6 are unaffected by `TRUST`. A rejected boundary is never silently swapped for a plausible-looking one: rejection degrades to `estimate` only through the branch point, which is branch-scoped by construction, and on `$LETS_MERGE_BRANCH` it degrades to `none` rather than to a number.

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
      { label: "Post progress", description: "Add a Session Progress comment to {task-id}" },        # S2, only if actionable
      { label: "Finish task", description: "Run /lets:done - work looks complete" },                 # S3, only if actionable (pre-compact never reaches Step 2 - it early-exits in Step 1)
      { label: "Push", description: "Push {N} commits to origin/{branch} (no PR)" }                  # S5, only if AHEAD>0; label names the concrete target = informed consent
    ],
    multiSelect: true
  }]
)
```

**Execution order (run the picks in this fixed order; deselect-all = "end as-is"):**

1. **Commit** -> `Skill(skill: "lets:commit")`. (If both Commit and Finish task are picked, commit runs first, then hand off.)
2. **Finish task** -> **HAND-OFF + STOP.** Say "Task looks done - handing off to /lets:done." then `Skill(skill: "lets:done")` and **STOP the entire /lets:end flow** (do NOT run Step 3, Step 4, or Output). `/lets:done` owns everything after: it commits/pushes/PRs/closes AND ends with its own terminal menu (which may switch branches or even call `/lets:end`). If end "continued" past the hand-off, the branch-slugged snapshot could land on the wrong branch and end<->done could recurse. So done SUPERSEDES S5 (push), S2 (done writes its own completion comment), S4 (no end-snapshot when finishing) and S6 (done shows its own worktree hint). The one residual path - done's menu -> "End session" -> a fresh `/lets:end` - is safe: that second end finds everything settled and silently wraps.
3. **Push** (only reached when Finish task was NOT picked) -> upstream-aware push of the current branch, NO PR, never `--force`: `git push` when upstream exists and ahead; `git push -u origin <branch>` on first push. The explicit multiSelect pick IS the approval (same as picking "Commit" authorizes the commit - satisfies the git.md / AUTO MODE "push needs explicit approval" rule, which is why the option label always names the concrete target).
4. **Post progress** (only reached when Finish task was NOT picked) -> remember this pick; the progress comment is written in Step 3b, AFTER Step 3a produces the snapshot file it references. Do NOT write it here.

## Step 3: Write artifacts

(Reached in the DEFAULT flow only; the `--pre-compact` early exit in Step 1 handled that path. Skipped on the Finish-task hand-off, which stopped the flow in Step 2.)

### 3a. Session snapshot (ALWAYS, written FIRST)

Write the session-level snapshot that bootstraps the next `/lets:start`, via the shared primitive so end's snapshot and the pre-compact snapshot never drift. The `pointer` arg depends on whether "Post progress" was picked in Step 2:

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

Output-time, never a prompt. If `GIT_DIR` contains `worktrees/`: extract the worktree name (last path segment); if the task finished this session show the cleanup line, else show the resume line. (When the task was finished via the Step 2 hand-off, `/lets:done` already showed this - end never reaches here in that case.)

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

If in a worktree, append one line - cleanup (task done) or resume (`cd {LETS_PROJECT_ROOT from LETS Config} && claude -> /lets:start`).

Then a SINGLE prose line (no AskUserQuestion, no wrap-up card):

> Run `/compact` to keep this window lighter, or `/clear` + `/lets:start` for a fresh session.

(`/compact` and `/clear` are user-typed Claude Code commands - never auto-execute them. `/compact` keeps the window with a compacted summary; the PreCompact hook re-injects the rules and the summary just written survives.)

### --pre-compact

```
## Pre-Compact Snapshot

Snapshot -> .lets/sessions/{date}-{HHMM}-{task-id}-snapshot-precompact.md
Task pointer -> {task-id}  (only if a task is unambiguously active; else "none - file only")
Branch: {branch}

Safe to /compact now - same window continues. Resume: /lets:start reads the snapshot file (the tracked task holds task-level context).
```

Then STOP - no AskUserQuestion, no settlement, no push, no `git checkout`. The session continues. (Identical output contract to `/lets:note --pre-compact`.)

## Rules

- **end never writes the `.task-<slug>` boundary** - it only reads `session:` (take-task + the hook own writes).
- **Tidy session = silent wrap** - when nothing is actionable, end produces zero prompts.
- **NEVER push without explicit user approval** - the Step 2 multiSelect pick is that approval.
- **Always write the session snapshot** via the shared `session-snapshot` primitive (except on the Finish-task hand-off, where `/lets:done` owns the record). The snapshot is file-primary; the task gets at most a one-line pointer.
- **`--pre-compact` runs NO settlement** (snapshot-only; it early-exits at the top of Step 1).
- **End with a one-line compact/clear prose hint, never a wrap-up card.**
- **Suggest `/lets:done`** only when there is real work this session (S3 gate) - don't nag every end.
- Respond in user's language.
