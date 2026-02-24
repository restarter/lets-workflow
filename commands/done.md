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

If uncommitted changes exist:
- **Ask:** "You have uncommitted changes. Commit first?"
- If yes -> run `/lets:commit`, then continue
- If no -> warn and continue

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

**If any requirement is missing:**
- Do NOT proceed to closing
- Ask user: "Fix missing items, or update task description to match actual scope?"
- If user says fix - stop and work on missing items
- If user says update scope - update task description with `bd update`, then proceed

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

**Ask:** "Ready to finish **{task title}** (`{task-id}`)?"

Show what will happen (based on remote detection):

```bash
git remote -v
```

- **Has remote:** "Will push branch and create PR"
- **No remote:** "Will merge to main and delete branch"

Wait for user approval.

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

┌─ LETS ─────────────────────────┐
│  End?  /lets:end               │
└────────────────────────────────┘
```

### After local merge:

```
Task: **{title}** ({task-id}) - CLOSED
Merged to {main branch}
Branch {feature-branch} deleted

┌─ LETS ─────────────────────────┐
│  End?  /lets:end               │
└────────────────────────────────┘
```

## Rules

- **NEVER push or create PR without user approval**
- **NEVER merge without user approval**
- Document BEFORE finishing (Step 6 before Step 7)
- If PR flow: task stays open, user closes after merge
- If local merge: task closes immediately
- Respond in user's language
