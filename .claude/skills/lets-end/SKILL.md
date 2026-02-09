---
name: lets-end
description: End work session - check for missing steps, sync beads, save summary
---

# Session End

End a work session properly. Auto-detect and prompt for missing steps.

## Step 1: Check State

```bash
git status --short
git branch --show-current
bd list --status=in_progress
```

## Step 2: Check Beads Documentation

**Ask:** "Did you document your work in beads? (task comments, description updates)"

- If NO or unsure - "Let's do that first. Run `/lets-beads-finish`"
- If YES - Continue

## Step 3: Handle Uncommitted Changes

If there are uncommitted changes:

**Ask:** "Uncommitted changes detected. Commit now?"

- If yes - Run `/lets-commit` skill
- If no - Warn: "Changes will be lost if not committed!"

## Step 4: Update Task Status

For each in-progress task, ask:

> "Task `<id>`: `<title>` - complete or still in progress?"

- **Complete:** `bd close <id> --reason="<brief summary>"`
- **In progress:** Leave open, note in summary

## Step 5: Merge Feature Branch

**Only if** on a feature branch (not main/master):

### Task Completed

If the current branch's task was closed in Step 4:

**Ask:** "Task done. Merge `{branch}` into master and delete branch?"

- If yes:
  ```bash
  MAIN=$(git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo master)
  git checkout $MAIN
  git merge {branch}
  git branch -d {branch}
  ```
- If no - stay on feature branch

### Task Still In Progress

Skip merge. Stay on feature branch. Note in summary that work continues.

## Step 6: Create Session Summary (LOCAL ONLY)

Create TWO files:
1. **Dated archive:** `.claude/sessions/YYYY-MM-DD-HHMM.md` (history preserved)
2. **Latest:** `.claude/sessions/last-summary.md` (overwritten each session)

```bash
# Create sessions directory if needed (always relative to repo root)
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$ROOT/.claude/sessions"

# Write summary to both files
DATED_FILE="$ROOT/.claude/sessions/$(date +%Y-%m-%d-%H%M).md"
```

**Summary template:**

```markdown
## Session Summary {YYYY-MM-DD HH:MM}

### Done
- {what was completed}

### In Progress
- {task id}: {what remains}
- Branch: {branch name if still on feature branch}

### Commits
- {hash} {message}

### Key Decisions
- {any important decisions made}

### Next Steps
- {what to do next session}

### Context for Next Session
- Branch: {current branch}
- Task: {active task id and title}
- {any context AI needs to continue}
```

## Step 7: Final Status

```
## Session End Summary

Git: {clean / X uncommitted / X unpushed}
Branch: {current branch}
Beads: {local changes saved}
Tasks closed: {list}
Tasks in progress: {list}
Summary saved: .claude/sessions/{dated}.md + last-summary.md
```

## Step 8: Ask About Sync/Push

**NEVER sync or push automatically.** Ask user in plain text (no LETS box - per workflow rules):

> "Ready to sync to remote? (`bd sync && git push`)"

- If user says "sync" / "push" / "+" - run `bd sync` then `git push`
- If user says "later" / "no" / "local" - skip, remind them to sync later

## Rules

- **Check beads documentation before closing**
- **Check uncommitted changes before closing**
- **Offer branch merge only when task is complete**
- **NEVER push without explicit user approval**
- Always write summary (local)
- Include branch info in session summary for context recovery
- Respond in user's language
