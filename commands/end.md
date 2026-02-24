---
description: End work session - save progress, sync beads, create summary
---

# Session End

End a work session properly. Save context for next session.

**This is NOT task completion.** Use `/lets:done` to finish a task. `/lets:end` ends a SESSION.

## Step 1: Check State

```bash
git status --short
git branch --show-current
bd list --status=in_progress
```

## Step 2: Handle Uncommitted Changes

If there are uncommitted changes, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes detected. What to do?",
    header: "LETS",
    options: [
      { label: "Commit", description: "Run /lets:commit before ending session" },
      { label: "Skip", description: "End without committing - changes stay on disk" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Commit** -> run `/lets:commit`, then continue
- **Skip** -> continue

## Step 3: Save Progress to Beads (task-level context for multi-session work)

For each in-progress task, record this session's work:

```bash
# Get this session's commits
ROOT=$(git rev-parse --show-toplevel)
START_REF=$(cat "$ROOT/.lets/sessions/.session-start-ref" 2>/dev/null)
if [ -n "$START_REF" ]; then
  git log ${START_REF}..HEAD --oneline  # this session's commits
else
  git log --oneline -5  # fallback: recent commits
fi
```

Add progress comment:

```bash
bd comments add <task-id> "## Session progress {YYYY-MM-DD}

### Commits this session
{git log from start-ref}

### Done
- {what was completed this session}

### Remaining
- {what's left to do}

### Context for next session
- {important info for AI to recover context}"
```

**Skip if** no meaningful work was done this session (no commits, no decisions).

## Step 4: Ask About Task Status

For each in-progress task, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "{task title} - still in progress?",
    header: "LETS",
    options: [
      { label: "In progress", description: "Leave open - continue next session" },
      { label: "Ready to finish", description: "Run /lets:done first, then come back to /lets:end" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **In progress** -> leave open, progress already saved in Step 3
- **Ready to finish** -> suggest `/lets:done` first, then come back to `/lets:end`

## Step 5: Create Session Summary (session-level context for next session bootstrap)

Create TWO files:
1. **Dated archive:** `.lets/sessions/YYYY-MM-DD-HHMM.md`
2. **Latest:** `.lets/sessions/last-summary.md` (overwritten each session)

```bash
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$ROOT/.lets/sessions"
DATED_FILE="$ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M).md"
```

**Summary template:**

```markdown
## Session Summary {YYYY-MM-DD HH:MM}

### Done
- {what was completed}

### In Progress
- {task id}: {what remains}
- Branch: {branch name if on feature branch}

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

## Step 6: Sync Beads

```bash
bd sync --flush-only
```

## Output

```
## Session End

Git: {clean / X uncommitted}
Branch: {current branch}
Tasks in progress: {list with titles}
Summary saved: .lets/sessions/{dated}.md
Beads: synced
```

Then ask about push in plain text (no LETS box):

> "Push to remote? (`git push`)"

- If yes - `git push`
- If no - skip
- **NEVER push automatically**

## Rules

- **Save progress to beads** for multi-session tasks
- **Check uncommitted changes** before closing
- **Suggest `/lets:done`** if task seems complete
- **NEVER push without explicit user approval**
- Always write session summary (local)
- Respond in user's language
