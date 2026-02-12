---
description: Document completed work in beads with comments and context
---

# Beads Task Finish

Document work done on beads tasks for history and context recovery.

## Why This Matters

Beads persists across conversation compaction. Good documentation means:
- Future sessions have full context
- Other developers understand what was done
- AI can recover context after compaction
- History is searchable and useful

## Step 1: Identify Worked Tasks

```bash
bd list --status=in_progress
```

Also check conversation context for tasks mentioned.

**Ask user:** "Which tasks did we work on this session?"

## Step 2: For Each Task

### 2a. Review Current State

```bash
bd show <task-id>
```

Check:
- Current description - still accurate?
- Existing comments - need update?

### 2b. Update Description (if needed)

If the task evolved or scope changed:

```bash
bd update <task-id> --description="<updated description>"
```

Include:
- What was implemented
- Key decisions made
- Files changed (main ones)
- Any caveats or notes

### 2c. Add Completion Comment

```bash
bd comments add <task-id> "## Completed {YYYY-MM-DD}

### What was done
- {bullet points of work}

### Key changes
- {files or modules affected}

### Decisions made
- {any important choices}

### Notes for future
- {anything to remember}"
```

### 2d. Close Task (if complete)

```bash
bd close <task-id> --reason="<brief summary>"
```

## Step 3: Document Relationships

If work created new tasks or dependencies:

```bash
# Create follow-up task
bd create --title="..." --type=task

# Link dependency
bd dep add <new-task> <original-task>
```

## Comment Templates

### For Completed Work

```markdown
## Completed {date}

### Summary
{1-2 sentence overview}

### Changes
- {file}: {what changed}
- {file}: {what changed}

### Decisions
- Chose {X} over {Y} because {reason}

### Follow-up
- {any remaining work -> link to new task}
```

### For Partial Progress

```markdown
## Progress {date}

### Done
- {what was completed}

### Remaining
- {what's left to do}

### Blockers
- {if any}

### Context
- {important info for next session}
```

### For Research/Planning

```markdown
## Research {date}

### Findings
- {key discoveries}

### Recommendation
- {suggested approach}

### Open questions
- {unresolved items}
```

## Step 4: Verify Documentation

```bash
bd show <task-id>
bd comments list <task-id>
```

Ensure:
- [ ] Description reflects final state
- [ ] Comment documents what was done
- [ ] Any decisions are recorded
- [ ] Follow-up work is linked

## Rules

- **Always document before closing** - don't close with just "done"
- **Be specific** - "Fixed bug" is useless, "Fixed null check in PaymentService.process()" is useful
- **Record decisions** - future you will thank you
- **Link related work** - creates traceable history
- **Use markdown** - comments support formatting

## Output

After documenting:

```
## Tasks Documented

{task-id}: <title>
  - Description: {updated/unchanged}
  - Comment: {added/none}
  - Status: {closed/in_progress}

┌─ LETS ─────────────────┐
│  Commit? /lets:commit  │
└────────────────────────┘
```
