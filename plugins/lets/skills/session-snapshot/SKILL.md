---
name: session-snapshot
description: Internal skill for commands. Write a recovery-grade session snapshot (file-primary) for /lets:end and for the --session snapshot-only flag of /lets:end and /lets:note. Always writes a .lets/sessions/ file; adds a one-line task pointer only when a task is unambiguously active. Do not trigger on user conversation - only when those commands need the snapshot.
user-invocable: false
---

# Session Snapshot

Shared snapshot primitive for `/lets:end` (settlement + snapshot) and for the `--session` snapshot-only flag on both `/lets:end` and `/lets:note`. **Single source of truth** - every caller delegates here so the template and file/pointer behavior never drift.

Goal: ONE recovery-grade `## RESUME` snapshot, **file-primary** - it ALWAYS lands in a `.lets/sessions/` file (the single trail `/lets:start` reads), regardless of task state (feature / trunk / --main / no-task). The active task gets only a ONE-LINE pointer to that file, and only when a task is unambiguously active (via detect-task, NEVER a `list-by-status | head -1` guess).

> **Contract - this skill ONLY writes the snapshot (file + optional pointer).** It does NOT end the session, push, merge, commit, or close anything. The caller decides what else to do.

## Arguments (from the caller)

Passed via the `Skill` invocation's `args` string as space-separated `key=value` pairs (e.g. `args: "kind=end pointer=off task-id=lets-abc range=session: X..HEAD (3 commits)"`). Put `range=` / `task-id=` LAST when the value contains spaces - each consumes the rest of the string. Any omitted key falls to its default.

- `kind` = `session` (default) | `end` - selects the `### Record` line ONLY; both resolve to the same `artifact-path` kind (`snapshot`), because a mid-session record and a session-end record are the same artifact written at different moments. There is no `precompact` kind: a snapshot taken before a `/compact` is just a mid-session one, and naming the caller's next intention in a permanent file is what made the old flag dishonest. Step 3 owns the filename, never build it here.
- `pointer` = `off` (default) | `auto` - whether the skill writes the standalone one-line task pointer. `off` is the SAFE default (a caller that forgets never double-writes a task comment); a caller that wants the skill to write the pointer passes `auto` explicitly (both snapshot-only callers do). `/lets:end` default passes `off` when it folds the pointer into its own progress comment, `auto` otherwise.
- `range` (optional) - a RANGE_DESC string (e.g. `session: <ref>..HEAD (N commits)`). A caller that passes one WINS - `/lets:end`'s default flow does, because it already read the boundary to gate its own offers. When it is absent, Step 2 resolves it through `session-boundary` rather than omitting the block, for EVERY kind. Every snapshot is read back by the same consumer, `/lets:start`, for the same purpose - where the work began - so none of them has a reason to be written without its range (lets-yprsv).
- `task-id` (optional) - pre-resolved active task from the caller's own detect-task.

## Step 1: Active task

If the caller passed `task-id`, use it. Else run `Skill(skill: "lets:detect-task")`. "Unambiguously active" = detect-task returns exactly one task. No task (or ambiguous) -> file only, no pointer, no prompt.

## Step 2: Gather state

```bash
git branch --show-current
git log --oneline -5
git status --short          # uncommitted / untracked
git rev-parse --short HEAD

# Session id + transcript path - BOTH from the Bash-injected env var (ONE channel; matches take-task Step 5).
SID=$CLAUDE_CODE_SESSION_ID
TRANSCRIPT_PATH=$(find "$HOME/.claude/projects" -maxdepth 2 -name "${CLAUDE_CODE_SESSION_ID}.jsonl" 2>/dev/null | head -1)
TRANSCRIPT_PATH=${TRANSCRIPT_PATH:-"(not found)"}
# ECHO both - a bash var is invisible to the Write tool; the model needs the printed values for the template.
echo "SID=$SID"
echo "TRANSCRIPT_PATH=$TRANSCRIPT_PATH"
```

### Range (when the caller passed none)

If `range` was NOT passed, invoke `Skill(skill: "lets:session-boundary")` and use its echoed `SESSION_RANGE_DESC` as the RANGE_DESC for Step 3's `### Range` block; surface its stderr NOTEs. Do NOT re-derive the boundary here - that ladder lives in `session-boundary` alone (lets-370mx). If the skill is unavailable, omit the `### Range` block rather than writing an unqualified number.

## Step 3: Write the snapshot FILE (ALWAYS)

Resolve the path via `Skill(skill: "lets:artifact-path", args: "kind=snapshot ext=md")` for both kinds; pass `task=<id>` when the caller already resolved one. The echoed `ARTIFACT_FILE` is `$SNAP_FILE` and its basename is `$SNAP_BASENAME` - reuse both VERBATIM in Step 4 + the Return, never recompute (a second `date` drifts the pointer off the file actually written). Shape: `.lets/sessions/{date}-{HHMM}-{task-id|branch-slug-6hex}-snapshot[-vN].md` - task-scoped, `-vN` on collision, so parallel worktrees sharing `.lets/` never overwrite each other (lets-05c4s).

Write `$SNAP_FILE` (the echoed path) via the Write tool with the template below, substituting the bash-captured `$SID` / `$TRANSCRIPT_PATH` from Step 2 - and reuse `$SNAP_BASENAME` verbatim in Step 4 + the Return, never recomputing the minute-precise timestamp. Use ONLY that single bash session-id channel (`$CLAUDE_CODE_SESSION_ID`, captured as `$SID`) - do NOT use the command-load-time template channel (the `CLAUDE_SESSION_ID` template variable in `${...}` form), which is fragile inside a multiline Write arg (lets-bdkvd QA #13) and would itself be substituted here if written literally. English; one continuous line per paragraph - no hard wrap. For any section with nothing to record, write a single `- (none)` stub, never a blank block - EXCEPT `### Range`, which is OMITTED ENTIRELY (not stubbed) when no RANGE_DESC was passed or resolved: when one exists, insert a `### Range` block (`- {RANGE_DESC}`) between `### Remaining + NEXT STEP` and `### Record`. So the literal template below has no Range section. If a plan file exists for the task and `/lets:execute` has not approved implementation in this session, the `NEXT:` line MUST be `/lets:execute <plan or task>` - never "implement Task N" / "continue with the code"; a resumed session re-reads the plan, whose banner says the same.

    ## RESUME {YYYY-MM-DD HH:MM} - {short label}

    ### Claude Session
    - ID: `{SID}`
    - Transcript: `{TRANSCRIPT_PATH}`

    ### Where things live
    - repo / branch: {branch} @ {short-sha}; key paths touched: {file:line, ...}
    - external sources: {PR #, links, other-project paths, index / recovery commands}

    ### State
    - committed/merged: {...}; uncommitted/untracked: {git status}; frozen artifacts + SHAs: {...}

    ### Decided (do NOT re-litigate)
    - {decision -> reasoning}
    - verified vs code: {claim -> file:line}

    ### Remaining + NEXT STEP
    - {open items}
    - NEXT: {the single concrete next action + how to resume it; with an unexecuted plan this is `/lets:execute`, never "implement Task N"}

    ### Record
    - {session: snapshot written mid-session; the session continued past this point - resume via /lets:start or --continue, which read this file}
      {end: session-end snapshot}

## Step 4: One-line task pointer (conditional)

If `pointer=auto` AND a task is unambiguously active, compose the one-line pointer to a temp file (the heading date is `$(date +%Y-%m-%d)`; the snapshot basename is the `SNAP_BASENAME` echoed in Step 3 - reuse it VERBATIM, do NOT recompute the minute-precise timestamp, or the pointer drifts off the file actually written), then submit it via the tracker `comment-add` verb with `body-file=` (lets-rules "Tracker Adapters"):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
cat > "$LETS_PROJECT_ROOT/.lets/cache/pointer-<task-id>.md" <<EOF
## RESUME $(date +%Y-%m-%d) - snapshot: .lets/sessions/<SNAP_BASENAME echoed in Step 3>
EOF
```

```lets-tracker
comment-add task=<task-id> body-file=.lets/cache/pointer-<task-id>.md
```

Otherwise (`pointer=off`, or no unambiguous task): write nothing to the task - the file is the record.

## Return

Report to the caller, and these ARE the contract - a caller renders only what this list names, never a value it improvised:

- the snapshot file path (the `SNAP_FILE` echoed in Step 3)
- the branch the snapshot was written on
- the RANGE_DESC used, or `none` when no range was passed or resolved and the `### Range` block was therefore omitted
- the task id, if a pointer was written

The caller handles any further output. A caller that prints a range MUST print the `none` case as such - reconstructing a range of its own is the unqualified-number failure `session-boundary` exists to prevent.
