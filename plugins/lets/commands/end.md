---
description: End a work session - a settlement pass that reconciles uncommitted / unpushed work + session context into git, beads, and a summary file. --pre-compact also writes a resume snapshot and keeps the session going.
argument-hint: "[--pre-compact]"
---

# Session End

End a work session cleanly. `/lets:end` is a **settlement pass**: it reconciles the volatile session state (uncommitted changes, unpushed commits, in-conversation context) into durable stores (git remote, beads, a session-summary file) so the window can close - or compact - without losing anything.

**This is NOT task completion.** Use `/lets:done` to finish a TASK; `/lets:end` ends a SESSION.

**Invariant:** `/lets:end` only READS the `.task-<slug>` `session:` boundary - it NEVER writes it. The boundary writers are `take-task` (at `/lets:start`) and the SessionStart hook (refresh on a genuinely new session). End mutating the boundary was the old `--fast` divergence bug; it stays gone.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Modes

One settlement core, two modifiers:

- **(default)** - full settlement pass (Steps 1-3) + worktree hint + a one-line terminal prose hint (Steps 4-5 / Output). Auto-skip keeps it silent on a tidy session.
- **`--pre-compact`** (alias `--resume`) - a pre-compaction snapshot, **NOT a session end**: run the settlement pass **minus** the task-done offer (S3) and **minus** the terminal hint, **plus** an always-written resume snapshot (Step 4). Each step states its own `--pre-compact` delta. The session continues into `/compact`.
- **`--fast`** - DEPRECATED. It now runs the default flow (which already stays silent when there is nothing to settle). Emit one line - `--fast is deprecated; running the unified /lets:end (it auto-skips when there's nothing to settle).` - then proceed as default. (Accepted for one release; removal is a follow-up.)

## Step 1: Detect (silent)

Read all state ONCE, compute which settlements are actionable, prompt nothing here.

First find the active task: use the **detect-task** skill - `Skill(skill: "lets:detect-task")`. (No task -> S2/S3 below auto-skip; main-mode `/lets:end` is a normal, mostly-silent settle.)

Then one bash block (REUSE dsdmp's hardened `session:` reader verbatim - the staleness NOTE, the `.session-start-ref` back-compat back-fill, and the hex guard are load-bearing; do not re-derive):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')

git status --short   # DIRTY if non-empty (S1)

# --- session: boundary (dsdmp canonical reader) ---
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
SESSION_LINE=$(sed -n 's/^session: //p' "$TASK_FILE" 2>/dev/null | head -1)
START_REF=$(echo "$SESSION_LINE" | awk '{print $1}')
STORED_SID=$(echo "$SESSION_LINE" | awk '{print $2}')
# Session-id staleness: a sid different from the current session means no /lets:start ran this
# session (no take-task / hook refresh), so the boundary is from a PRIOR session. Degrade loudly.
if [ -n "$STORED_SID" ] && [ "$STORED_SID" != "$CLAUDE_CODE_SESSION_ID" ]; then
  echo "NOTE: session boundary is from a previous session (no /lets:start this session) - commit counts below are best-effort from ${START_REF}." >&2
fi
# Back-compat: the legacy .session-start-ref IS a session boundary, so it may back-fill session:.
[ -z "$START_REF" ] && START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
[ -z "$START_REF" ] && START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref" 2>/dev/null)  # oldest non-slug, deliberate one-release window
# Defense-in-depth (N1): the boundary is a plugin-written SHA - blank anything non-hex before it
# expands unquoted into the git range, so a hand-edited state file can't inject git option args.
case "$START_REF" in *[!0-9a-f]*) START_REF="" ;; esac

# Commits this session (0 => S2/S3 skip; empty START_REF => unknown, see Step 1 no-boundary rule)
if [ -n "$START_REF" ]; then SESSION_COMMITS=$(git rev-list --count ${START_REF}..HEAD); else SESSION_COMMITS=""; fi
echo "START_REF=${START_REF:-<none>}  SESSION_COMMITS=${SESSION_COMMITS:-<unknown>}"

# Unpushed (S5), upstream-aware (mirrors /lets:done Step 8)
if git rev-parse --abbrev-ref @{u} >/dev/null 2>&1; then
  AHEAD=$(git rev-list --count @{u}..HEAD); echo "AHEAD=$AHEAD"
else
  echo "AHEAD=no-upstream ($(git rev-list --count HEAD 2>/dev/null || echo 0) local commits)"
fi

# Worktree (S6)
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null); echo "GIT_DIR=$GIT_DIR"
```

**Actionable set:** S1 if DIRTY; S2 if active task `in_progress` AND `SESSION_COMMITS > 0`; S3 same gate as S2; S5 if `AHEAD > 0` (or no-upstream + local commits exist). S4 always; S6 if `GIT_DIR` contains `worktrees/`; S7 if `--pre-compact`.

**No-boundary rule:** when `START_REF` is empty (no `session:`, no back-compat ref) `SESSION_COMMITS` is unknown - **S3 SKIPS** (never offer to finish a task on a guess) and **S2 is a best-effort OFFER** carrying the staleness NOTE (range approximate). S1/S4/S5/S6 are unaffected.

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
      { label: "Finish task", description: "Run /lets:done - work looks complete" },                 # S3, only if actionable; OMIT under --pre-compact
      { label: "Push", description: "Push {N} commits to origin/{branch} (no PR)" }                  # S5, only if AHEAD>0; label names the concrete target = informed consent
    ],
    multiSelect: true
  }]
)
```

**Execution order (run the picks in this fixed order; deselect-all = "end as-is"):**

1. **Commit** -> `Skill(skill: "lets:commit")`. (If both Commit and Finish task are picked, commit runs first, then hand off.)
2. **Finish task** -> **HAND-OFF + STOP.** Say "Task looks done - handing off to /lets:done." then `Skill(skill: "lets:done")` and **STOP the entire /lets:end flow** (do NOT run Step 3, Step 4, or Output). `/lets:done` owns everything after: it commits/pushes/PRs/closes AND ends with its own terminal menu (which may switch branches or even call `/lets:end`). If end "continued" past the hand-off, the branch-slugged summary could land on the wrong branch and end<->done could recurse. So done SUPERSEDES S5 (push), S2 (done writes its own completion comment), S4 (no end-summary when finishing) and S6 (done shows its own worktree hint). The one residual path - done's menu -> "End session" -> a fresh `/lets:end` - is safe: that second end finds everything settled and silently wraps.
3. **Push** (only reached when Finish task was NOT picked) -> upstream-aware push of the current branch, NO PR, never `--force`: `git push` when upstream exists and ahead; `git push -u origin <branch>` on first push. The explicit multiSelect pick IS the approval (same as picking "Commit" authorizes the commit - satisfies the git.md / AUTO MODE "push needs explicit approval" rule, which is why the option label always names the concrete target).
4. **Post progress** (only reached when Finish task was NOT picked) -> write the bd progress comment (Step 3 template).

## Step 3: Write artifacts

(Reached in the default + `--pre-compact` flows; skipped only on the Finish-task hand-off, which stopped the flow in Step 2.)

### 3a. bd progress comment (only if "Post progress" was picked)

Records task-level context for multi-session work. **MANDATORY:** the `Claude session: $CLAUDE_CODE_SESSION_ID` line MUST appear between `## Session progress` and `### Range` - bash expands the env var at runtime, so `bd` gets the literal UUID. The commit LIST is intentionally dropped (git owns it); keep a range pointer. (Decision A: the range is `session:`-anchored, so a second `/lets:end` in one session may re-cover already-reported commits - acceptable by design.)

```bash
bd comments add <task-id> "## Session progress $(date +%Y-%m-%d)

Claude session: $CLAUDE_CODE_SESSION_ID

### Range
session: {START_REF}..HEAD ({SESSION_COMMITS} commits)

### Done
- {narrative of what was completed this session}

### Remaining
- {what's left}

### Context for next session
- {recovery info}"
```

### 3b. Session summary file (ALWAYS)

Session-level context that bootstraps the next `/lets:start`. Always written (the future no-beads user has only this). `--pre-compact` delta: same template, written to a `-precompact-`-slugged filename to mark a mid-session snapshot.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
SUMMARY_FILE="$LETS_PROJECT_ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M)-${BRANCH_SLUG}.md"            # default
# --pre-compact: SUMMARY_FILE="$LETS_PROJECT_ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M)-precompact-${BRANCH_SLUG}.md"

# Resolve the actual transcript path ($CLAUDE_CODE_SESSION_ID is the Bash-injected env var).
TRANSCRIPT_PATH=$(find "$HOME/.claude/projects" -maxdepth 2 -name "${CLAUDE_CODE_SESSION_ID}.jsonl" 2>/dev/null | head -1)
TRANSCRIPT_PATH=${TRANSCRIPT_PATH:-"(not found)"}
```

Write `$SUMMARY_FILE` with this template - **no `### Commits` list**, a `### Range` pointer instead. `${CLAUDE_SESSION_ID}` is command-load-time template substitution (already the literal UUID) - write it through verbatim.

```markdown
## Session Summary {YYYY-MM-DD HH:MM}

### Claude Session
- ID: `${CLAUDE_SESSION_ID}`
- Transcript: `$TRANSCRIPT_PATH`

### Range
- session: {START_REF}..HEAD ({SESSION_COMMITS} commits) - regenerate with `git log {START_REF}..HEAD`

### Done
- {what was completed - narrative, not a commit dump}

### In Progress / Next
- {task id}: {what remains}; next: {concrete next action}

### Key Decisions
- {decisions / trade-offs}

### Context for Next Session
- Branch: {branch}; Task: {task-id and title}; {anything needed to continue}
```

## Step 4: Resume snapshot (--pre-compact only)

Default flow: skip this step.

`--pre-compact`: write the recovery-grade `## RESUME` snapshot via the shared skill - `Skill(skill: "lets:pre-compact-note")`. This runs UNCONDITIONALLY (even if the user skipped every settlement in Step 2). The skill detects the active task, writes the snapshot, and falls back to a `.lets/sessions/` file when there is no active task. Single source of truth shared with `/lets:note --pre-compact` - the template never drifts.

## Step 5: Worktree hint

Output-time, never a prompt. If `GIT_DIR` contains `worktrees/`: extract the worktree name (last path segment); if the task finished this session show the cleanup line, else show the resume line. (When the task was finished via the Step 2 hand-off, `/lets:done` already showed this - end never reaches here in that case.)

## Output

### Default

```
## Session End

Git: {clean / N uncommitted}
Branch: {branch}
Task: {task-id and title, or "none"}
Settled: {e.g. "committed; pushed 3" / "nothing - tidy session"}
Summary: .lets/sessions/{dated}-{slug}.md
```

If in a worktree, append one line - cleanup (task done) or resume (`cd {LETS_PROJECT_ROOT from LETS Config} && claude -> /lets:start`).

Then a SINGLE prose line (no AskUserQuestion, no wrap-up card):

> Run `/compact` to keep this window lighter, or `/clear` + `/lets:start` for a fresh session.

(`/compact` and `/clear` are user-typed Claude Code commands - never auto-execute them. `/compact` keeps the window with a compacted summary; the PreCompact hook re-injects the rules and the summary just written survives.)

### --pre-compact

```
## Pre-Compact Snapshot

Resume comment -> {task-id}  (or {session-file path} if no active task)
Session summary -> .lets/sessions/{dated}-precompact-{branch}.md
Branch: {branch}

Safe to /compact now - same window continues. Resume context: bd show {task-id} + bd comments {task-id}
```

Then STOP - no AskUserQuestion, no push, no `git checkout`. The session continues.

## Rules

- **end never writes the `.task-<slug>` boundary** - it only reads `session:` (take-task + the hook own writes).
- **Tidy session = silent wrap** - when nothing is actionable, end produces zero prompts.
- **NEVER push without explicit user approval** - the Step 2 multiSelect pick is that approval.
- **Always write the session summary** (except on the Finish-task hand-off, where `/lets:done` owns the record).
- **End with a one-line compact/clear prose hint, never a wrap-up card.**
- **Suggest `/lets:done`** only when there is real work this session (S3 gate) - don't nag every end.
- Respond in user's language.
