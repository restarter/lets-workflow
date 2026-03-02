---
description: Parallel implementation with Agent Teams - spawn teammates in isolated worktrees
argument-hint: "[run|status|stop]"
---

# Team Execution

Spawn teammates in isolated worktrees for parallel implementation. Each teammate gets one task, works independently, and reports back.

**This is for parallel implementation of independent tasks.** For analysis (review, opinion, brainstorm) - use their dedicated commands.

## Step 1: Determine Subcommand

**If argument provided** (e.g., `/lets:team run`), parse it:
- `run` -> go to Run
- `status` -> go to Status
- `stop` -> go to Stop

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What do you want to do with the team?",
    header: "Team",
    options: [
      { label: "Run", description: "Launch teammates to implement tasks in parallel" },
      { label: "Status", description: "Show active team progress" },
      { label: "Stop", description: "Stop active team and preserve branches" }
    ],
    multiSelect: false
  }]
)
```

---

## Run

Launch a parallel team. Select tasks, spawn teammates, monitor progress, merge results.

### Step R1: Guards

```bash
# Guard 1: not in worktree
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
# If GIT_DIR contains "worktrees/" -> stop: "Teams must be created from the main repo, not a worktree."
```

```bash
# Guard 2: no active team
ls ~/.claude/teams/ 2>/dev/null | grep "lets-team"
# If any lets-team-* dirs exist -> stop: "Active team found. Use /lets:team status or /lets:team stop first."
```

```bash
# Guard 3: clean working directory
git status --short
# If dirty -> warn: "Uncommitted changes detected. Commit or stash before running a team."
```

### Step R2: Get Tasks

Two input modes:

**1. Interactive (default):** Show ready tasks and let user pick.

```bash
bd ready -n 10
```

Present as multiSelect AskUserQuestion - user picks 2-5 tasks.

**2. Manual (`--tasks A,B,C`):** Parse comma-separated task IDs from argument.

If more than 5 selected:
> "{N} tasks selected. Agent Teams work best with 3-5 teammates. Continue anyway?"

If fewer than 2 selected:
> "Need at least 2 tasks for parallel work. For a single task, just work on it directly."

### Step R3: Validate Independence

For each selected task:
```bash
bd show <task-id>
```

Check:
- If any task is blocked by another selected task -> error: "**{task A}** (`id`) blocks **{task B}** (`id`). Remove one."
- If task descriptions mention same directories -> warn: "Potential file overlap in `{dir}/`. Watch for conflicts."

### Step R4: Gather Context

```bash
ROOT=$(git rev-parse --show-toplevel)
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -200

# For each task:
bd show <task-id>
bd comments list <task-id>

# Stack detection
ls package.json pyproject.toml Cargo.toml go.mod composer.json Gemfile 2>/dev/null
```

### Step R5: Confirm Launch

Show team composition:

```
## Team Plan

| # | Task | Teammate | Scope |
|---|------|----------|-------|
| 1 | **Fix auth flow** (`proj-a1`) | fix-auth-1 | src/auth/ |
| 2 | **Add search API** (`proj-b2`) | add-search-2 | src/api/ |

Teammates: {N}
Isolation: worktree (auto-cleanup)
Plan approval: required (lead reviews each plan before implementation)
```

```
AskUserQuestion(
  questions=[{
    question: "Launch team with {N} teammates?",
    header: "Team",
    options: [
      { label: "Launch", description: "Spawn all teammates in parallel" },
      { label: "Adjust", description: "Change task selection" },
      { label: "Cancel", description: "Don't launch" }
    ],
    multiSelect: false
  }]
)
```

**Launch** -> continue
**Adjust** -> go back to R2
**Cancel** -> exit

### Step R6: Create Team

```
TeamCreate(team_name="lets-team-{YYYYMMDD-HHMM}")
```

### Step R7: Create Shared Task List

For each task:
```
TaskCreate(
  subject="{task title} ({task-id})",
  description="Assigned to teammate: {name}. Beads task: {task-id}. See teammate prompt for full context.",
  activeForm="Implementing {task title}"
)
```

After creation, assign:
```
TaskUpdate(taskId="{id}", owner="{teammate-name}", status="in_progress")
```

### Step R8: Spawn Teammates

**CRITICAL: All teammates MUST be spawned in a SINGLE message (parallel launch).**

Teammate naming: `{task-slug}-{index}` (e.g., `fix-auth-1`, `add-search-2`). Numeric suffix guarantees uniqueness.

For each task, one Agent call:

```
Agent(
  subagent_type="lets:implementer",
  name="{task-slug}-{index}",
  team_name="lets-team-{timestamp}",
  description="Implement {task-id}",
  prompt="{TEAMMATE PROMPT - see template below}",
  isolation="worktree",
  mode="plan"
)
```

#### Teammate Prompt Template

Each teammate gets this as their entire context. Fill in all `{placeholders}`.

```
You are implementing a specific task as part of a parallel team.
You are in an isolated worktree - your changes won't affect other teammates.

## Your Task

**Task ID:** {task-id}
**Title:** {task title}
**Description:**
{full task description from bd show}

**Design Notes:**
{design field from bd show, or "None"}

**Previous Context:**
{last 3 beads comments, or "No previous context"}

## Your Boundaries

You own these areas of the codebase:
{list of directories/files extracted from task description}

**IMPORTANT: Do NOT modify files outside your boundaries.**
If you need changes in shared code, send a message to the team lead describing what you need and why.

## Project Rules

{CLAUDE.md content, first 200 lines}

## Stack

{detected stack: e.g., "TypeScript, Node.js, npm" or "Python, Poetry" or "Go modules"}
{test command if detectable: e.g., "npm test", "pytest", "go test ./..."}

## Your Workflow

1. **PLAN FIRST** - You are in plan mode. Read relevant code, understand existing patterns,
   then propose your implementation plan. The lead will review and approve before you can
   make changes.

2. **IMPLEMENT** - After plan approval, make your changes. Follow existing patterns.
   Don't add features beyond what's described in the task.

3. **VERIFY** - Run relevant tests. Check that your changes compile/lint.
   If tests exist for your area: run them and confirm they pass.
   If no tests exist: verify manually (e.g., check imports resolve, no syntax errors).

4. **COMMIT** - Stage and commit your changes:
   git add {specific files}
   git status
   git commit -m "<type>: <subject>

   Task: {task-id}"
   Use conventional commit types: feat, fix, refactor, test, docs, chore.

5. **COMPLETE** - Mark your team task as done:
   TaskUpdate(taskId="{team-task-id}", status="completed")
   Then send a completion summary to the lead.

## Communication

Use SendMessage to talk to the team lead:

- **BLOCKED:** "I'm blocked on {issue}. Need {what you need}."
- **CONFLICT:** "I need to modify {file} which is outside my boundaries. Reason: {why}."
- **QUESTION:** "Clarification needed: {question about task requirements}."
- **DONE:** "Task complete. Changed {N} files: {list}. Tests: {pass/fail/none}."

Do NOT message other teammates directly. Coordinate through the lead.

## Quality Checklist

Before marking complete:
- [ ] All changes are committed (no uncommitted files)
- [ ] Commit message follows convention
- [ ] No files modified outside your boundaries
- [ ] Tests pass (if applicable)
- [ ] No TODO/FIXME comments left in new code
```

### Step R9: Monitor Progress

After spawning, monitor. Messages arrive automatically - no polling needed.

**Phase 1: Plan Approval**

As teammates propose plans, each sends a `plan_approval_request` message.

For each plan_approval_request:

1. Read the teammate's plan carefully
2. Check: does the plan stay within the teammate's file boundaries?
3. Check: is the plan reasonable for the task scope?
4. Approve or reject:

```
# Approve
SendMessage(
  type="plan_approval_response",
  request_id="{from the request}",
  recipient="{teammate-name}",
  approve=true
)

# Reject (with feedback)
SendMessage(
  type="plan_approval_response",
  request_id="{from the request}",
  recipient="{teammate-name}",
  approve=false,
  content="{specific feedback: what to change and why}"
)
```

Show progress after each approval:
```
Plan approved: {teammate-name} ({task-id}) - {N} files planned
Waiting for: {list of teammates still planning}
```

**Phase 2: Implementation Monitoring**

After plan approval, teammates implement and go idle when done.

For each idle notification:
- Check `TaskList()` - is the teammate's task marked `completed`?
- If completed: note it, show progress
- If NOT completed but idle - teammate may be stuck:

```
AskUserQuestion(
  questions=[{
    question: "Teammate '{name}' is idle but task not complete. What to do?",
    header: "Team",
    options: [
      { label: "Wait", description: "Give more time" },
      { label: "Message", description: "Send a nudge asking for status" },
      { label: "Stop", description: "Shut down this teammate" }
    ],
    multiSelect: false
  }]
)
```

- **Wait** -> continue monitoring
- **Message** -> `SendMessage(type: "message", recipient: "{name}", content: "Status check - are you blocked on something?")`
- **Stop** -> `SendMessage(type: "shutdown_request", recipient: "{name}", content: "Lead stopping this task.")`

**Progress display** (update after each completion):
```
## Team Progress

| Teammate | Task | Status |
|----------|------|--------|
| fix-auth-1 | **Fix auth flow** (`proj-a1`) | DONE |
| add-search-2 | **Add search API** (`proj-b2`) | WORKING |

Completed: 1/2
```

### Step R10: Completion

When all teammates are done (all team tasks `completed` or stopped):

**10.1: Shutdown all teammates**

For each teammate still active:
```
SendMessage(
  type="shutdown_request",
  recipient="{name}",
  content="All tasks complete. Shutting down team."
)
```

Wait for shutdown confirmations.

**10.2: Discover teammate branches**

Branch discovery algorithm (in order of reliability):

1. **Read team config** (primary):
```bash
cat ~/.claude/teams/{team-name}/config.json 2>/dev/null
```
Parse `members` array - map teammate names to their agent info.

2. **Git worktree list** (reliable fallback):
```bash
git worktree list --porcelain | grep -A2 "/.claude/worktrees/" | grep "branch" | sed 's/branch refs\/heads\///'
```

3. **Recent branches** (last resort):
```bash
git branch --sort=-committerdate --format='%(refname:short) %(committerdate:iso)' | head -20
```

Store `{teammate-name -> branch-name}` mapping for trial merge and reporting.

**10.3: Trial merge for conflict detection**

**IMPORTANT:** All teammates MUST be shut down before trial merge. Worktrees hold a ref on their branch.

```bash
# Verify no active worktrees remain
ACTIVE_WORKTREES=$(git worktree list --porcelain | grep -c "/.claude/worktrees/")
if [ "$ACTIVE_WORKTREES" -gt 0 ]; then
  echo "WARNING: $ACTIVE_WORKTREES worktrees still active. Wait for teammate shutdown."
  # Do NOT proceed with trial merge
fi
```

Run trial merge using configured merge-branch:

```bash
MERGE_BRANCH={merge-branch from LETS Config}
BRANCHES="{space-separated list of teammate branches}"

git stash --include-untracked 2>/dev/null
git checkout -b lets-trial-merge "$MERGE_BRANCH"

CLEAN=true
CONFLICT_BRANCH=""
for BRANCH in $BRANCHES; do
  if ! git merge --no-ff "$BRANCH" -m "trial merge $BRANCH" 2>/dev/null; then
    CLEAN=false
    CONFLICT_BRANCH="$BRANCH"
    git merge --abort
    break
  fi
done

# Report result
if [ "$CLEAN" = true ]; then
  echo "All branches merge cleanly"
else
  echo "Conflict detected merging: $CONFLICT_BRANCH"
fi

# Cleanup - return to original branch
git checkout -
git branch -D lets-trial-merge
git stash pop 2>/dev/null
```

**10.4: Record in beads**

For each completed task:
```bash
bd comments add <task-id> "## Team execution {YYYY-MM-DD}

Teammate: {name}
Branch: {branch}
Commits:
$(git log {merge-branch}..{branch} --oneline)"
```

**10.5: Cleanup team**

```
TeamDelete()
```

**10.6: Save completion record**

Write `.lets/execution/team-{team-name}.json`:

```json
{
  "team_name": "lets-team-{timestamp}",
  "created": "{ISO timestamp}",
  "completed": "{ISO timestamp}",
  "merge_branch": "{configured merge-branch}",
  "base_sha": "{HEAD at team start}",
  "status": "completed",
  "trial_merge": "clean|conflict:{branch}",
  "tasks": [
    {
      "task_id": "{beads-id}",
      "teammate": "{name}",
      "branch": "{worktree-branch}",
      "status": "completed|stopped",
      "commits": 3
    }
  ]
}
```

### Step R11: Output

```
## Team Complete

| Task | Branch | Commits | Status |
|------|--------|---------|--------|
| **Fix auth flow** (`proj-a1`) | worktree-abc | 3 | done |
| **Add search API** (`proj-b2`) | worktree-def | 5 | done |

Trial merge: {clean / conflict in {branch}}

{if clean}
All branches merge cleanly with {merge-branch}.
{end if}

{if conflict}
Conflict detected merging {branch}. Merge branches manually in order, resolving conflicts.
{end if}

Branches ready for review. Each branch has independent changes on top of {merge-branch}.
```

```
┌─ LETS ──────────────────────────────────┐
│  Review?  /lets:review {branch}         │
│  Done?    /lets:done                    │
└─────────────────────────────────────────┘
```

---

## Status

Show active team progress.

### Step S1: Find Active Team

```bash
ls ~/.claude/teams/ 2>/dev/null | grep "lets-team"
```

Also check `.lets/execution/team-*.json` for recent records.

If no active team found:
> "No active team. Use `/lets:team run` to start one."

If team dir exists but session is different (orphaned):
> "Found orphaned team {name}. Teammates may be stopped. Branches preserved."
> Show branches from state file or `git branch --list "worktree-*"`

### Step S2: Read Team State

```
TaskList()
```

Read `~/.claude/teams/{team-name}/config.json` for member list.

### Step S3: Recovery Detection

If state file exists with `status: "running"` but team dir is gone:
- Mark as orphaned
- Show which tasks were completed vs failed
- List preserved branches
- Suggest: "Run `/lets:team stop` to clean up, or manually review branches."

### Step S4: Output

```
## Team Status: {team-name}

| Teammate | Task | Status | Branch |
|----------|------|--------|--------|
| fix-auth-1 | **Fix auth** (`proj-a1`) | completed | worktree-abc |
| add-search-2 | **Search** (`proj-b2`) | in_progress | worktree-def |

Progress: 1/2 completed
Started: {time}
```

```
┌─ LETS ──────────────────────────────────┐
│  Stop?  /lets:team stop                 │
└─────────────────────────────────────────┘
```

---

## Stop

Stop active team and preserve branches.

### Step T1: Find Active Team

Same as Status S1.

### Step T2: Confirm

```
AskUserQuestion(
  questions=[{
    question: "Stop all teammates? In-progress work stays on worktree branches.",
    header: "Team",
    options: [
      { label: "Stop", description: "Shutdown all teammates, clean up team" },
      { label: "Cancel", description: "Keep team running" }
    ],
    multiSelect: false
  }]
)
```

### Step T3: Shutdown Teammates

For each active teammate:
```
SendMessage(
  type="shutdown_request",
  recipient="{name}",
  content="Team lead requesting shutdown. Commit your current work before stopping."
)
```

Wait for shutdown responses.

### Step T4: Cleanup

```
TeamDelete()
```

### Step T5: Save State

Write/update `.lets/execution/team-{name}.json` with `status: "aborted"`.
Include list of completed and in-progress tasks for recovery reference.

### Step T6: Output

```
## Team Stopped

Teammates stopped: {N}
Branches preserved: {list}

In-progress work is on worktree branches. Review or discard:
  git diff {merge-branch}...{branch}  # review changes
  git branch -D {branch}              # discard

┌─ LETS ──────────────────────────────────┐
│  Review?  /lets:review                  │
└─────────────────────────────────────────┘
```

---

## Rules

- **Main repo only** - teams cannot be created from a worktree
- **One team at a time** - check for existing teams before creating
- **Lead handles beads** - teammates don't touch beads, lead records everything
- **Plan approval required** - all teammates spawn with `mode: "plan"`, lead reviews before implementation
- **Parallel spawn** - all teammates launched in a single message for concurrent work
- **Graceful shutdown** - always request shutdown before cleanup, preserve branches
- **Trial merge before reporting** - detect conflicts early, report to user
- Respond in user's language
