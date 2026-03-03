---
description: End work session - save progress, sync beads, create summary
argument-hint: "[--fast]"
---

# Session End

End a work session properly. Save context for next session.

**This is NOT task completion.** Use `/lets:done` to finish a task. `/lets:end` ends a SESSION.

## Fast Mode

If `--fast` argument provided, skip to Fast Close below and do NOT run Steps 1-7.

### Fast Close

1. `git status --short` - check uncommitted changes
2. If uncommitted changes exist -> warn: "Uncommitted changes on disk. Run /lets:commit in next session."
   Do NOT run /lets:commit or AskUserQuestion (saves tokens)
3. Save minimal summary:
   ```bash
   # ROOT = project-root from LETS Config
   BRANCH=$(git branch --show-current)
   BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
   GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
   mkdir -p "$ROOT/.lets/sessions"
   # If in worktree, use branch-slug filename; otherwise use default
   if echo "$GIT_DIR" | grep -q "worktrees/"; then
     SUMMARY_FILE="$ROOT/.lets/sessions/last-summary-${BRANCH_SLUG}.md"
   else
     SUMMARY_FILE="$ROOT/.lets/sessions/last-summary.md"
   fi
   ```
   Write to `$SUMMARY_FILE`:
   ```
   ## Fast close {YYYY-MM-DD HH:MM}
   Branch: {branch}
   Task: {task-id or "none"}
   Status: fast close
   ```
4. Save session-start-ref:
   ```bash
   git rev-parse HEAD > "$ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}"
   ```
5. Worktree detection: check `GIT_DIR` as in Step 7. If in worktree and task in progress, add resume path. If task completed, add cleanup reminder.
6. Output fast close block and stop - no AskUserQuestion, no bd sync

### Fast Close Output

```
## Session End (fast)

Branch: {branch}
Task: {task-id or "none"}
Worktree: {name} (if in worktree)
Resume:   cd $ROOT && claude -> /lets:start (if task in progress)
Cleanup:  /lets:worktree remove {name} (if task completed)

┌─ LETS ─────────────────────────┐
│  Resume?  /lets:start          │
└────────────────────────────────┘
```

---

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
# Get this session's commits (per-branch ref supports parallel worktree sessions)
# ROOT = project-root from LETS Config
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
START_REF=$(cat "$ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
if [ -z "$START_REF" ]; then
  # Fallback: try old single-ref format (backwards compatibility)
  START_REF=$(cat "$ROOT/.lets/sessions/.session-start-ref" 2>/dev/null)
fi
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
2. **Latest:** `last-summary-{branch-slug}.md` in worktree, `last-summary.md` in main repo

```bash
# ROOT = project-root from LETS Config
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
mkdir -p "$ROOT/.lets/sessions"
DATED_FILE="$ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M).md"
# If in worktree, use branch-slug filename; otherwise use default
if echo "$GIT_DIR" | grep -q "worktrees/"; then
  LATEST_FILE="$ROOT/.lets/sessions/last-summary-${BRANCH_SLUG}.md"
else
  LATEST_FILE="$ROOT/.lets/sessions/last-summary.md"
fi
```

Write summary to both `$DATED_FILE` and `$LATEST_FILE`.

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

## Step 7: Worktree Cleanup Reminder

Check if in a worktree:

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
# ROOT = project-root from LETS Config
```

If `$GIT_DIR` contains `worktrees/`:
- Extract worktree name from path (last segment of worktree directory)
- If task was completed this session (PR created or merged via `/lets:done`): remind about cleanup
- If task is still in progress: show the worktree path so user can resume next session with a copy-paste command

## Output

```
## Session End

Git: {clean / X uncommitted}
Branch: {current branch}
Tasks in progress: {list with titles}
Summary saved: .lets/sessions/{dated}.md
Beads: synced
```

If in worktree and task is done, add:

```
Worktree: {name} - after PR merges, clean up with `/lets:worktree remove {name}` from main repo
```

If in worktree and task is still in progress, add:

```
Worktree: {name}
Resume:   cd $ROOT && claude -> /lets:start
```

Then use **AskUserQuestion** for next steps:

```
AskUserQuestion(
  questions=[{
    question: "Session saved. What now?",
    header: "LETS",
    options: [
      { label: "Push", description: "git push to remote" },
      { label: "Done", description: "All done - start fresh with /clear + /lets:start" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Push** -> `git push`, then suggest `/clear` + `/lets:start`
- **Done** -> suggest `/clear` + `/lets:start`
- **NEVER push automatically**

## Rules

- **Save progress to beads** for multi-session tasks
- **Check uncommitted changes** before closing
- **Suggest `/lets:done`** if task seems complete
- **NEVER push without explicit user approval**
- Always write session summary (local)
- Respond in user's language
