---
description: Finish a task - document, create PR or merge, close
---

# Task Done

Complete the current task. Document work, create PR or merge locally, close in beads.

**This is NOT session end.** Use `/lets:end` to end a session. `/lets:done` finishes a TASK.

## Step 1: Active Task Detection

```bash
BRANCH=$(git branch --show-current)
# Extract task ID from branch name. Handles all branch formats:
#   feature/<task-id>-<slug>     -> standard LETS branches
#   worktree-<task-id>-<slug>   -> worktree branches (from /lets:worktree create)
#   worktree-<custom-name>      -> no task ID, use fallback
#
# Beads ID pattern: <prefix>-<alphanum>[.<number>]

# Fallback (always works, also primary method for worktree branches without task ID):
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

Use `merge-branch` from LETS Config. Fallback: `git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo main`

```bash
git log ${MERGE_BRANCH}..HEAD --oneline
git diff --stat ${MERGE_BRANCH}..HEAD
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

Show what will happen based on `github` config value from LETS Config:

### If github: true:

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "LETS",
    options: [
      { label: "Finish", description: "Push branch and create PR to {merge-branch}" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

### If github: false (or missing):

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "LETS",
    options: [
      { label: "Finish", description: "Merge to {merge-branch} and delete branch" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

Next steps presented via AskUserQuestion (replaces LETS box).

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

### Worktree Detection

Before finishing, check if we're in a worktree:

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

Set `IN_WORKTREE=true` if `$GIT_DIR` contains `worktrees/` (is NOT `.git`).

If in a worktree, resolve the main repo path:

```bash
MAIN_ROOT=$(cd "$(git rev-parse --git-common-dir)/.." 2>/dev/null && pwd)
```

### If github: true (PR flow):

**Guard: verify gh CLI first**

```bash
gh auth status 2>&1
```

If gh is not installed or not authenticated, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "gh CLI is not available but github mode is enabled. What to do?",
    header: "LETS",
    options: [
      { label: "Local merge", description: "Fall back to local merge for this task" },
      { label: "Cancel", description: "Stop - fix gh auth first (gh auth login)" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Local merge** -> jump to "If github: false" section below
- **Cancel** -> stop, return to work

**If gh is available, proceed with PR:**

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

### If github: false (local merge) AND NOT in worktree:

Use `merge-branch` from LETS Config for target branch.

```bash
git checkout {merge-branch}
git merge <branch>
git branch -d <branch>
```

After merge:
```bash
bd close <task-id> --reason="Merged locally. Commits: {list}"
```

### If github: false (local merge) AND in worktree:

Cannot `git checkout` or `git branch -d` from inside a worktree. Use `git -C` to operate on the main repo:

Use `merge-branch` from LETS Config for target branch.

```bash
MAIN_ROOT=$(cd "$(git rev-parse --git-common-dir)/.." 2>/dev/null && pwd)
BRANCH=$(git branch --show-current)

# Ensure main repo is on the merge branch before merging
MAIN_CURRENT=$(git -C "$MAIN_ROOT" branch --show-current)
if [ "$MAIN_CURRENT" != "{merge-branch}" ]; then
  git -C "$MAIN_ROOT" checkout {merge-branch}
fi

git -C "$MAIN_ROOT" merge "$BRANCH"
```

After merge:
```bash
bd close <task-id> --reason="Merged locally from worktree. Commits: {list}"
```

Do NOT delete the branch or remove the worktree here - `/lets:worktree remove` handles cleanup.

## Step 8: Output

### After PR (github: true), NOT in worktree:

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

**Handle response:**
- **Stay on branch** -> stay on current branch, no checkout. User continues working freely.
- **Next task** -> `git checkout {merge-branch}`, then show `bd ready`, pick new task
- **End session** -> `git checkout {merge-branch}`, then suggest `/lets:end`

### After PR (github: true), IN worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: open (close after PR merge)
Worktree: {worktree path}
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "PR created. What's next?",
    header: "LETS",
    options: [
      { label: "Stay here", description: "Stay in this worktree for PR fixes or follow-up" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

No "Next task" option - can't switch branches in a worktree. To start a new task, user opens a different terminal.

**Handle response:**
- **Stay here** -> stay in worktree. User continues working.
- **End session** -> suggest `/lets:end`. After session ends, remind:
  "After PR merges, clean up: `/lets:worktree remove {name}` from the main repo terminal."

### After local merge (github: false), NOT in worktree:

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

### After local merge (github: false), IN worktree:

```
Task: **{title}** ({task-id}) - CLOSED
Merged to {merge-branch} (from worktree via git -C)
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Merged. Clean up worktree?",
    header: "LETS",
    options: [
      { label: "Remove worktree", description: "Switch to main repo and run /lets:worktree remove {name}" },
      { label: "Keep", description: "Keep worktree for now - clean up later with /lets:worktree remove" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Remove worktree** -> inform: "Switch to main repo terminal and run `/lets:worktree remove {name}`" (cannot remove worktree from inside it)
- **Keep** -> suggest `/lets:end` to save session context

## Rules

- **NEVER push or create PR without user approval**
- **NEVER merge without user approval**
- Document BEFORE finishing (Step 6 before Step 7)
- If PR flow: task stays open, user closes after merge
- If local merge: task closes immediately
- Respond in user's language
