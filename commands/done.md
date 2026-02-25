---
description: Finish a task - document, create PR or merge, close
---

# Task Done

Complete the current task. Document work, create PR or merge locally, close in beads.

**This is NOT session end.** Use `/lets:end` to end a session. `/lets:done` finishes a TASK.

## Step 1: Active Task Detection

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Example: feature/ji2-beads-deep-integration -> lets-plugin-claude-ji2

# Fallback:
bd list --status=in_progress
```

If no active task found - ask user which task to close.

### Epic Guard

Check detected task type with `bd show <task-id>`.
If type is **epic** - do NOT close it automatically:
- Inform user: "This is an epic. Epics stay open for future tasks."
- Offer: close a specific child task instead, or confirm epic closure if user insists.

## Step 2: Check Uncommitted Changes

```bash
git status --short
```

If uncommitted changes exist, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "You have uncommitted changes. What to do?",
    header: "LETS",
    options: [
      { label: "Commit first", description: "Run /lets:commit before finishing task" },
      { label: "Skip", description: "Continue without committing (changes stay unstaged)" },
      { label: "Cancel", description: "Stop - go back to working on the task" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Commit first** -> run `/lets:commit`, then continue
- **Skip** -> warn and continue
- **Cancel** -> stop, return to work

## Step 3: Verify Task Scope

**Before closing - verify ALL requirements from the task description are met.**

```bash
bd show <task-id>
```

Compare the task description against actual changes:

1. Read the full task description and any design/notes fields
2. List each requirement or deliverable mentioned
3. For each one - check if it's actually implemented (read files, grep, verify)
4. Present a checklist to the user:

```
## Scope Verification

Task: **{title}** ({task-id})

- [x] {requirement 1} - done in {file}
- [x] {requirement 2} - done in {file}
- [ ] {requirement 3} - NOT FOUND

{if all done}
All requirements met. Proceeding.
{else}
Missing: {list}. Fix first or update task scope?
```

**If any requirement is missing**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Some requirements are missing. How to proceed?",
    header: "LETS",
    options: [
      { label: "Fix first", description: "Stop closing - go back and implement missing items" },
      { label: "Update scope", description: "Adjust task description to match what was actually done" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Fix first** -> stop, do NOT proceed to closing
- **Update scope** -> update task description with `bd update`, then proceed

**Only continue to Step 4 when all requirements are verified.**

## Step 4: Collect Commits

```bash
MAIN=$(git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo main)
git log ${MAIN}..HEAD --oneline
git diff --stat ${MAIN}..HEAD
```

Show summary:
```
## Task Summary

Commits (N):
- abc1234 feat: Add session restore
- def5678 fix: Handle edge case

Files: X changed, Y insertions, Z deletions
```

## Step 5: Confirm with User

Show what will happen (based on remote detection):

```bash
git remote -v
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "LETS",
    options: [
      { label: "Finish", description: "{action based on remote: 'Push branch and create PR' OR 'Merge to main and delete branch'}" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

Before asking, remind: "Check context window usage: `/context`"

**Handle response:**
- **Finish** -> proceed to Step 6
- **Keep working** -> stop, return to work

## Step 6: Document in Beads

Add completion comment to the task:

```bash
bd comments add <task-id> "## Completed {YYYY-MM-DD}

### Commits
{git log main..HEAD --oneline}

### Summary
{1-2 sentence overview of what was done}

### Key decisions
- {any important choices made during this task}

### Files changed
{git diff --stat main..HEAD}"
```

## Step 7: Finish Task

### If remote exists (PR flow):

```bash
# Push branch
git push -u origin <branch>

# Create PR
gh pr create --title "<type>: <task title>" --body "$(cat <<'EOF'
## Summary
{task description from beads}

## Changes
{git log main..HEAD --oneline}

## Task
{task-id}: {title}

---
Generated with LETS plugin
EOF
)"
```

After PR created:
```bash
bd comments add <task-id> "PR #XX created: <PR URL>"
```

Task stays **open** until PR is merged.

**Do NOT switch branches yet** - user decides in Step 8.

### If no remote (local merge):

```bash
MAIN=$(git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo main)
git checkout $MAIN
git merge <branch>
git branch -d <branch>
```

After merge:
```bash
bd close <task-id> --reason="Merged locally. Commits: {list}"
```

## Step 8: Output

### After PR:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: open (close after PR merge)
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "LETS",
    options: [
      { label: "Stay on branch", description: "Stay on feature branch - for PR fixes or follow-up work" },
      { label: "Next task", description: "Switch to {merge-branch}, pick another task" },
      { label: "End session", description: "Switch to {merge-branch}, run /lets:end" }
    ],
    multiSelect: false
  }]
)
```

Before asking, remind: "Check context window usage: `/context`"

**Handle response:**
- **Stay on branch** -> stay on current branch, no checkout. User continues working freely.
- **Next task** -> `git checkout {merge-branch}`, then show `bd ready`, pick new task
- **End session** -> `git checkout {merge-branch}`, then suggest `/lets:end`

### After local merge:

```
Task: **{title}** ({task-id}) - CLOSED
Merged to {merge-branch}
Branch {feature-branch} deleted
```

Already on merge-branch after merge. Use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "LETS",
    options: [
      { label: "Next task", description: "Pick another task to work on" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Next task** -> show `bd ready`, pick new task
- **End session** -> suggest `/lets:end`

## Rules

- **NEVER push or create PR without user approval**
- **NEVER merge without user approval**
- Document BEFORE finishing (Step 6 before Step 7)
- If PR flow: task stays open, user closes after merge
- If local merge: task closes immediately
- Respond in user's language
