---
description: Execute implementation plan from /lets:brainstorm - adaptive batch execution with checkpoints
argument-hint: "[--status]"
---

# Execute Plan

Execute an implementation plan step by step. Reads plan from `.lets/plans/`, adapts code to current file state (not blind copy-paste), verifies after each task, and tracks progress for multi-session resume.

**Plan is a roadmap, not a script.** Read real files before every change. Adapt to current context. Stop and ask if something doesn't match.

## Step 1: Active Task Detection

```bash
BRANCH=$(git branch --show-current)
# Extract task ID from branch name (feature/<id>-<slug> or worktree-<id>-<slug>)
# Strategy: look for beads ID pattern anywhere in branch name

# Fallback:
bd list --status=in_progress
```

If not on a feature branch and no in-progress task found - ask user which task to execute.

Verify not on main/master:
```bash
# If on main or master - STOP
# "Cannot execute on main branch. Switch to a feature branch first: /lets:start"
```

## Step 2: Load Plan

Plan filename matches the branch slug (branch name without `feature/` prefix).

```bash
# ROOT = project-root from LETS Config

# Primary: derive from branch name
# feature/0nf.10-improve-brainstorm -> 0nf.10-improve-brainstorm.md
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}
cat "$ROOT/.lets/plans/${SLUG}.md" 2>/dev/null

# Fallback: glob match by task-id
ls "$ROOT/.lets/plans/"*{task-id}* 2>/dev/null
```

If no plan found:
> "No plan found for this task. Run `/lets:brainstorm` first to create one."

Exit with LETS box suggesting `/lets:brainstorm`.

Parse plan structure:
- Extract all `### Task N:` sections (the implementation tasks)
- Count total tasks
- Note each task's name, files, verify steps, and commit points

## Step 3: Load Execution State & Handle --status

Check for existing execution state (multi-session resume):

```bash
# ROOT = project-root from LETS Config
cat "$ROOT/.lets/execution/{branch-slug}.json" 2>/dev/null
```

**State file format** (`.lets/execution/{branch-slug}.json`):

```json
{
  "plan": ".lets/plans/{branch-slug}.md",
  "total_tasks": 6,
  "completed": [
    {"task": 1, "name": "Create agent file", "commit": "abc1234"},
    {"task": 2, "name": "Add command", "commit": "def5678"}
  ],
  "last_good_commit": "def5678",
  "started": "2026-02-24",
  "updated": "2026-02-24"
}
```

### Handle --status Flag (exit early)

If argument is `--status`:
- Show execution state or "No execution in progress"
- Show plan task list with completion markers
- Exit without executing or prompting

```
## Execution Status: **{task title}** (`{task-id}`)

Plan: .lets/plans/{branch-slug}.md
Progress: {completed}/{total} tasks

{task list with [x] / [ ] markers}

Last good commit: {hash or "N/A"}
```

```
┌─ LETS ──────────────────────────────────┐
│  Continue?  /lets:execute               │
│  End?       /lets:end                   │
└─────────────────────────────────────────┘
```

Exit after showing status.

### Resume Prompt (interactive)

**If state file exists and NOT --status (resuming):**

Present resume summary:
```
Resuming execution of **{task title}** (`{task-id}`)

Completed: {N}/{total} tasks
- [x] Task 1: {name} (abc1234)
- [x] Task 2: {name} (def5678)
- [ ] Task 3: {name}  <-- next
- [ ] Task 4: {name}
...

Last good commit: {hash}
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Resume execution from Task {N+1}?",
    header: "LETS",
    options: [
      { label: "Continue", description: "Pick up from where we left off" },
      { label: "Start fresh", description: "Re-execute from Task 1 (ignores previous progress)" },
      { label: "Cancel", description: "Don't execute - return to work" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Continue** -> proceed from next incomplete task
- **Start fresh** -> clear state file, start from Task 1
- **Cancel** -> stop

**If no state file (fresh start):** proceed to Step 4.

## Step 4: Critical Review

Before executing ANY code, review the plan against current codebase state.

**For each plan task's referenced files:**
1. Files listed as "Modify" - verify they exist and read current content
2. Files listed as "Create" - verify target path doesn't already exist (could indicate partial execution without state file)
3. Check if file content has changed significantly since plan was written

**Present findings**, then use **AskUserQuestion**:

**If OK or drift detected:**
```
AskUserQuestion(
  questions=[{
    question: "{OK: 'Plan matches codebase.' | Drift: 'Some files changed: {list}.'} Execute {N} tasks?",
    header: "LETS",
    options: [
      { label: "Execute", description: "{OK: 'Start execution' | Drift: 'Adapt implementation to current state'}" },
      { label: "Cancel", description: "Don't execute" }
    ],
    multiSelect: false
  }]
)
```

**If significant mismatch:**
```
AskUserQuestion(
  questions=[{
    question: "Major changes detected: {details}. What to do?",
    header: "LETS",
    options: [
      { label: "Execute anyway", description: "Adapt implementation to current state" },
      { label: "Re-plan", description: "Run /lets:brainstorm to update the plan" },
      { label: "Cancel", description: "Don't execute" }
    ],
    multiSelect: false
  }]
)
```

## Step 5: Execute Batch

Default batch size: 3 tasks (or remaining tasks if fewer than 3 left).

Present batch before starting:
```
## Batch {N}: Tasks {X}-{Z}

- [ ] Task {X}: {name}
- [ ] Task {Y}: {name}
- [ ] Task {Z}: {name}

Starting Task {X}...
```

**For each task in the batch:**

### 5a: Pre-task Context Read

Read ALL files listed in the task's "Files" section - both "Modify" and "Create" targets (and their neighbors for context). Compare actual state vs what the plan assumed.

### 5b: Adaptive Implementation

- If actual file state matches plan assumptions -> follow plan closely
- If file has changed (imports moved, functions renamed, lines shifted) -> adapt the plan's INTENT to the current code
- If the gap is too large to adapt confidently -> STOP and use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Task {N}: Plan expected {X} but found {Y}. How to proceed?",
    header: "LETS",
    options: [
      { label: "Adapt", description: "Adapt implementation to current state" },
      { label: "Skip", description: "Skip this task, continue with next" },
      { label: "Stop", description: "Stop execution entirely" }
    ],
    multiSelect: false
  }]
)
```

### 5c: Verification

Run the task's `**Verify:**` step from the plan.

- If verification passes -> continue
- If verification fails -> STOP and use **AskUserQuestion**:

Present what failed (expected vs actual), then:

```
AskUserQuestion(
  questions=[{
    question: "Verification failed for Task {N}. How to proceed?",
    header: "LETS",
    options: [
      { label: "Fix & retry", description: "Fix the issue and re-run verification" },
      { label: "Skip", description: "Skip verification, continue with next task" },
      { label: "Stop", description: "Stop execution entirely" }
    ],
    multiSelect: false
  }]
)
```

### 5d: Commit Point

If the plan has a `**Commit:**` section for this task:
- Suggest running `/lets:commit`
- If multiple trivial tasks in sequence: "Tasks {X} and {Y} are both small. Commit together?"
- Each commit uses `/lets:commit` skill (which handles user approval)

### 5e: Update State

After each task completes (committed or not):

Update `.lets/execution/{branch-slug}.json`:
```bash
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/execution"
# Write updated state with newly completed task
```

### 5f: Context Check

After every 3rd completed task, remind user:

```
{completed}/{total} tasks done. Check context: `/context`
If usage > 60%, recommend pausing: `/lets:end` -> new session -> `/lets:execute`
Progress is saved - will continue from Task {N+1}.
```

## Step 6: Batch Checkpoint

After all tasks in the batch complete:

### Save Progress to Beads

```bash
bd comments add <task-id> "## Execution progress {YYYY-MM-DD}

### Completed (batch {N})
- Task {X}: {name} - {commit hash}
- Task {Y}: {name} - {commit hash}
- Task {Z}: {name} - {commit hash}

### Status
Completed: {M} of {total} tasks
Next: Task {M+1}: {name}

### Adaptations
- {any deviations from plan, or 'None'}

### Context for next session
- {important state for resume}"
```

### Continue or Pause

If more tasks remain, show batch summary then use **AskUserQuestion**:

```
## Batch {N} Complete

Done: {list with commit hashes}
Progress: {completed}/{total} tasks
```

```
AskUserQuestion(
  questions=[{
    question: "Continue with next batch (Tasks {X}-{Z})?",
    header: "LETS",
    options: [
      { label: "Continue", description: "Execute next batch of tasks" },
      { label: "Pause", description: "Save progress and end session (/lets:end)" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Continue** -> proceed with next batch
- **Pause** -> suggest `/lets:end`

## Step 7: Completion

When ALL plan tasks are done:

### Final Verification

Run the plan's `## Verification` section (the final checklist).
Run the plan's `## Success Criteria` checks.

### Clean Up State File

Delete `.lets/execution/{branch-slug}.json` - execution is complete, state no longer needed. Beads comment preserves the history.

```bash
# ROOT = project-root from LETS Config
rm "$ROOT/.lets/execution/{branch-slug}.json" 2>/dev/null
```

### Record Completion

```bash
bd comments add <task-id> "## Execution complete {YYYY-MM-DD}

All {N} tasks completed.

### Commits
{list of all commit hashes + messages}

### Adaptations
{any deviations from original plan, or 'None'}"
```

### Show Summary

```
## Execution Complete: **{task title}** (`{task-id}`)

Plan: .lets/plans/{branch-slug}.md
Tasks: {N}/{N} completed

### Commits
- {hash} {message}
- {hash} {message}
...

{if adaptations}
### Adaptations from Plan
- {deviation}: {why}
{end if}
```

## Rules

- **NEVER execute blindly** - read actual file state before every task
- **NEVER skip verification** - if verify fails, stop and ask
- **NEVER commit without user approval** - delegate to `/lets:commit`
- **NEVER work on main/master** - verify feature branch in Step 1
- **ALWAYS save state after each task** - `.lets/execution/{branch-slug}.json` is the resume mechanism
- **ALWAYS save beads comment after each batch** - human-readable progress mirror
- **Adapt, don't paste** - plan intent matters more than plan text
- **Stop on mismatch** - if reality diverges significantly from plan, surface it immediately
- Respond in user's language

## Output

**After completion:**
```
┌─ LETS ──────────────────────────────────┐
│  Review?  /lets:review --local          │
│  Done?    /lets:done                    │
└─────────────────────────────────────────┘
```

**After batch pause:**
```
┌─ LETS ──────────────────────────────────┐
│  Continue?  /lets:execute               │
│  End?       /lets:end                   │
└─────────────────────────────────────────┘
```

**If no plan found:**
```
┌─ LETS ──────────────────────────────────┐
│  Plan?  /lets:brainstorm                │
└─────────────────────────────────────────┘
```
