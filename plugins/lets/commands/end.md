---
description: End work session - save progress, sync beads, create summary
argument-hint: "[--fast]"
---

# Session End

End a work session properly. Save context for next session.

**This is NOT task completion.** Use `/lets:done` to finish a task. `/lets:end` ends a SESSION.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Fast Mode

If `--fast` argument provided, skip to Fast Close below and do NOT run Steps 1-7.

### Fast Close

1. `git status --short` - check uncommitted changes
2. If uncommitted changes exist -> warn: "Uncommitted changes on disk. Run /lets:commit in next session."
   Do NOT run /lets:commit or AskUserQuestion (saves tokens)
3. Save minimal summary:
   ```bash
   LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
   BRANCH=$(git branch --show-current)
   BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
   mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
   SUMMARY_FILE="$LETS_PROJECT_ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M)-${BRANCH_SLUG}.md"
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
   LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
   BRANCH=$(git branch --show-current)
   BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
   git rev-parse HEAD > "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}"
   ```
5. Worktree detection: check `GIT_DIR` as in Step 7. If in worktree and task in progress, add resume path. If task completed, add cleanup reminder.
6. Output fast close block and stop - no AskUserQuestion, no bd sync

### Fast Close Output

```
## Session End (fast)

Branch: {branch}
Task: {task-id or "none"}
Worktree: {name} (if in worktree)
Resume:   cd {LETS_PROJECT_ROOT from LETS Config} && claude -> /lets:start (if task in progress)
Cleanup:  /lets:worktree remove {name} (if task completed)

┌─ LETS ─────────────────────────┐
│  Resume?  /lets:start          │
└────────────────────────────────┘
```

---

## Step 1: Check State

```bash
git status --short
```

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.

## Step 2: Handle Uncommitted Changes

If there are uncommitted changes, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes detected. What to do?",
    header: "Uncommitted",
    options: [
      { label: "Commit", description: "Run /lets:commit before ending session" },
      { label: "Skip", description: "End without committing - changes stay on disk" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Commit** -> read `${CLAUDE_PLUGIN_ROOT}/skills/commit/SKILL.md` via the Read tool and execute its flow, then continue
- **Skip** -> continue

## Step 3: Save Progress to Beads (task-level context for multi-session work)

For each in-progress task, record this session's work:

```bash
# Get this session's commits (per-branch ref supports parallel worktree sessions)
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)
if [ -z "$START_REF" ]; then
  # Fallback: try old single-ref format (backwards compatibility)
  START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref" 2>/dev/null)
fi
if [ -n "$START_REF" ]; then
  git log ${START_REF}..HEAD --oneline  # this session's commits
else
  git log --oneline -5  # fallback: recent commits
fi
```

Add progress comment. **MANDATORY:** the `Claude session: $CLAUDE_CODE_SESSION_ID` line MUST appear in the comment between `## Session progress` and `### Commits this session` — don't drop it. `$CLAUDE_CODE_SESSION_ID` is the Bash subprocess env var Claude Code injects; bash expands it inside the double-quoted argument at runtime, so `bd` receives the literal session UUID.

```bash
bd comments add <task-id> "## Session progress {YYYY-MM-DD}

Claude session: $CLAUDE_CODE_SESSION_ID

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
    header: "Task status",
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
- **Ready to finish** -> read `${CLAUDE_PLUGIN_ROOT}/commands/done.md` via the Read tool and execute its flow first, then return to `/lets:end`

## Step 5: Create Session Summary (session-level context for next session bootstrap)

Single dated file with branch slug in the name. No `last-summary` files - `/lets:start` reads recent sessions by date.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
SUMMARY_FILE="$LETS_PROJECT_ROOT/.lets/sessions/$(date +%Y-%m-%d-%H%M)-${BRANCH_SLUG}.md"

# Resolve actual transcript path (Claude Code stores it as <session-id>.jsonl
# under a project-slug subdirectory at depth 2; find avoids guessing the slug
# algorithm). $CLAUDE_CODE_SESSION_ID is the Bash subprocess env var Claude
# Code injects — bash expands it here at runtime.
TRANSCRIPT_PATH=$(find "$HOME/.claude/projects" -maxdepth 2 -name "${CLAUDE_CODE_SESSION_ID}.jsonl" 2>/dev/null | head -1)
TRANSCRIPT_PATH=${TRANSCRIPT_PATH:-"(not found)"}
```

Write summary to `$SUMMARY_FILE` using `$TRANSCRIPT_PATH` in the template below. In the template, `${CLAUDE_SESSION_ID}` is Claude Code's command-load-time template substitution — it's already the literal session UUID by the time you read this spec, so write it through verbatim.

**Summary template:**

```markdown
## Session Summary {YYYY-MM-DD HH:MM}

### Claude Session
- ID: `${CLAUDE_SESSION_ID}`
- Transcript: `$TRANSCRIPT_PATH`

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

## Step 6: Worktree Cleanup Reminder

Check if in a worktree:

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
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
Resume:   cd {LETS_PROJECT_ROOT from LETS Config} && claude -> /lets:start
```

Then use **AskUserQuestion** for next steps:

```
AskUserQuestion(
  questions=[{
    question: "Session saved. What now?",
    header: "Wrap up",
    options: [
      { label: "Push", description: "git push to remote, then reset context for a fresh start" },
      { label: "Done", description: "Wrap up — reset context manually to start fresh next time" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Push** -> `git push`, then tell the user: "Run `/clear` then `/lets:start` to begin a fresh session." (Do NOT auto-execute — `/clear` must wipe context BEFORE `/lets:start` so the new session is truly fresh; auto-Reading `start.md` would defeat that.)
- **Done** -> tell the user: "Run `/clear` then `/lets:start` to begin a fresh session." Same reasoning as above.
- **NEVER push automatically**

## Rules

- **Save progress to beads** for multi-session tasks
- **Check uncommitted changes** before closing
- **Suggest `/lets:done`** if task seems complete
- **NEVER push without explicit user approval**
- Always write session summary (local)
- Respond in user's language
