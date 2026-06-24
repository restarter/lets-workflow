---
name: pre-compact-note
description: Internal skill for commands. Write a recovery-grade RESUME snapshot of the current session to the active task before /compact. Invoked by /lets:note --pre-compact and /lets:end --pre-compact. Do not trigger on user conversation - only when those commands need the snapshot.
user-invocable: false
---

# Pre-Compact Resume Snapshot

Shared resume-snapshot step for the `--pre-compact` modes of `/lets:note` and `/lets:end`. **Single source of truth** for the snapshot — both commands delegate here so the template never drifts.

Goal: ONE recovery-grade `## RESUME` comment on the active task so a future you / another agent can fully reconstruct the working context after `/compact` summarizes the conversation — what is decided, where everything lives, and the exact next step.

> **Contract — this skill ONLY writes the snapshot.** It does NOT end the session, push, merge, commit, or close anything. The caller decides what else to do.

## Step 1: Active task

The caller passes the active task id (from its own `detect-task`). If none was passed, run `Skill(skill: "lets:detect-task")`. If still no active task -> use the no-task fallback (Step 4).

## Step 2: Gather state

```bash
git branch --show-current
git log --oneline -5
git status --short          # uncommitted / untracked
```

## Step 3: Write the snapshot to the active task

English; one continuous line per paragraph - no hard wrap:

```bash
# tracker: comment-add binding (beads); non-beads resolves via the adapter (lets-rules "Tracker Adapters")
bd comments add <task-id> "## RESUME {YYYY-MM-DD} - {short label}

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
- NEXT: {the single concrete next action + how to resume it}

### Compaction
- snapshot taken before /compact; resume with: bd show <task-id> + bd comments <task-id>"
```

## Step 4: No active task (fallback) - never drop the snapshot

Write the same RESUME block via the Write tool to a session file and report where it landed:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
BRANCH_SLUG=$(git branch --show-current | tr '/' '-')
# Write the RESUME block (Step 3) via the Write tool to:
#   $LETS_PROJECT_ROOT/.lets/sessions/<YYYY-MM-DD-HHMM>-precompact-${BRANCH_SLUG}.md
```

## Return

Report to the caller where the snapshot landed (task id, or the session-file path). The caller handles any further output.
