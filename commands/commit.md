---
description: Commit changes with proper review and conventional commit message
---

# Git Commit

Review and commit changes with conventional commit format. Links commits to the active beads task.

## Step 1: Check Status

```bash
git status --short
git diff --stat
```

If no changes - inform user and exit.

## Step 2: Active Task Detection

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Example: feature/ji2-beads-deep-integration -> lets-plugin-claude-ji2

# Fallback:
bd list --status=in_progress --format=ids 2>/dev/null | head -1
```

If found, the task ID will be used in commit footer.
If not found - commit normally without task link.

**If multiple in-progress tasks found via fallback**, use AskUserQuestion to pick:

```
AskUserQuestion(
  questions=[{
    question: "Multiple tasks in progress. Which one to link?",
    header: "LETS",
    options: [
      { label: "{task-id-1}", description: "{task title 1}" },
      { label: "{task-id-2}", description: "{task title 2}" },
      { label: "None", description: "Commit without task link" }
    ],
    multiSelect: false
  }]
)
```

**Note:** Max 4 options in AskUserQuestion. If more than 3 tasks - show top 3 by most recent activity + "None".

## Step 3: Review Changes

```bash
git diff
```

Summarize what changed:
- Files modified/added/deleted
- Key changes in each file

## Step 4: Confirm with User

Present the proposed commit summary as plain text (NOT in a code block or blockquote):

`<type>: <description>`
- {file1} - {what changed}
- {file2} - {what changed}

Then use **AskUserQuestion** for structured confirmation:

```
AskUserQuestion(
  questions=[{
    question: "Commit with this message?",
    header: "LETS",
    options: [
      { label: "Commit", description: "Stage all changes and commit" },
      { label: "Cancel", description: "Don't commit, keep changes unstaged" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Commit** -> proceed to Step 5
- **Cancel** -> stop, show LETS box with `/lets:check`
- **Other** (free text) -> treat as edited commit message, proceed to Step 5

## Step 5: Commit

```bash
git add -A
git status  # Verify staging
git commit -m "<type>: <description>

Task: <task-id>"
git status  # Verify clean
```

If no active task detected, omit the `Task:` footer line.

### Commit Message Format

```
<type>: <short description>

<optional body - why, not what>

Task: <task-id>
```

**Types:**
- `feat` - new feature
- `fix` - bug fix
- `refactor` - code restructure
- `docs` - documentation
- `chore` - maintenance, deps
- `test` - tests

### Good Examples

```
feat: Add user authentication with JWT

Task: lets-plugin-claude-abc

fix: Resolve null pointer in PaymentService

Task: proj-def
```

### Bad Examples

```
BAD: update stuff
BAD: fix bug
BAD: WIP
BAD: feat: Add user authentication system with JWT tokens and refresh logic and middleware (too long)
```

## Rules

- **NEVER** commit without user approval
- **ALWAYS** run `git status` before and after commit
- Keep subject line under 50 chars
- Use imperative mood ("Add" not "Added")
- Body explains WHY, diff shows WHAT
- Task ID footer is automatic - don't ask user about it

## Step 6: Task Progress Check

If active task detected, compare what was committed against the task scope:

```bash
bd show <task-id>
```

Brief assessment (2-3 lines max):

```
## Task Progress

Committed: {what this commit covers}
Remaining: {what's left from task description, or "nothing - task scope complete"}
```

- If task scope is fully covered -> suggest `/lets:done`
- If work remains -> show what's left, suggest next step or `/lets:commit` later

## Output

After successful commit:

**If task scope complete:**
```
Committed: <hash> <message>
  Files: X changed, Y insertions, Z deletions
  Task: <task-id> (linked)

Task scope complete.

┌─ LETS ─────────────────────────┐
│  Done?  /lets:done             │
└────────────────────────────────┘
```

**If work remains:**
```
Committed: <hash> <message>
  Files: X changed, Y insertions, Z deletions
  Task: <task-id> (linked)

Remaining: {brief list}

┌─ LETS ─────────────────────────┐
│  Check?  /lets:check           │
│  End?    /lets:end             │
└────────────────────────────────┘
```
