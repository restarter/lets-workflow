---
name: create-task
description: This skill should be used when creating a beads task - "create task", "create issue", "bd create", "new task", "add task", "add issue". Ensures all required fields are provided and suggests epic labels. Triggers on any task creation in any context - commands, brainstorm, planning, or direct conversation.
---

# Create Task

Standardized task creation that enforces required fields and suggests labels. Fires in any context where a beads task is being created - inside commands, during brainstorm, or in direct conversation.

## Why This Exists

Tasks created without labels, priority, or description become orphaned and hard to find. This skill ensures every task has full metadata from the start.

## Required Fields

Every `bd create` call MUST include ALL of these:

| Flag | Required | Rules |
|------|----------|-------|
| `--title` | Always | Imperative mood, clear action. Under 80 chars |
| `--type` | Always | One of: `task`, `bug`, `feature`, `epic` |
| `--priority` | Always | 0-4 (0=critical, 1=high, 2=medium, 3=low, 4=backlog) |
| `--description` | Always | Why this task exists + acceptance criteria |
| `--labels` | Always | At least one `epic:<name>` label for grouping |

## Label Selection

Labels are project-specific. ALWAYS discover them dynamically - never hardcode.

### Step L1: Discover Existing Labels

```bash
bd list --all 2>/dev/null | grep -oE '\[epic:[a-z0-9_-]+\]' | tr -d '[]' | sort -u
```

If no labels found, also check closed epics for label conventions:

```bash
bd list --all --type=epic 2>/dev/null
```

### Step L2: Present Labels to User

Show discovered labels and ask which fits:

> Found these epic labels in the project:
> - `epic:features` - (N open tasks)
> - `epic:quality` - (N open tasks)
> - ...
>
> Which label fits? Or suggest a new one.

### Step L3: New Label Flow

If no existing label fits:
1. Propose a new `epic:<name>` label with a short explanation of the grouping
2. Ask if an epic task should be created for this theme: `bd create --title="<Theme Name>" --type=epic --labels="epic:<name>" ...`
3. Wait for user approval before creating either the label or the epic

## Creation Flow

### Step 1: Gather Information

If the user provides a clear description (title + context), proceed directly. Otherwise ask concisely:

> What's the task? (title, what problem it solves)

Do NOT ask for each field separately - infer type, priority, and labels from context.

### Step 2: Compose Fields

From the user's input, derive:

1. **Title** - imperative mood, specific action (e.g., "Add retry logic to API client")
2. **Type** - infer from context: bug (something broken), feature (new capability), task (chore/refactor), epic (theme)
3. **Priority** - infer from urgency. Default to P2 unless context suggests otherwise
4. **Description** - why + acceptance criteria. Write as markdown with clear structure
5. **Labels** - run Label Selection flow (above). Present discovered labels, let user choose or create new

### Step 3: Present for Approval

Show the full `bd create` command before executing:

```
bd create \
  --title="Add retry logic to API client" \
  --type=feature \
  --priority=2 \
  --labels="epic:quality" \
  --description="## Problem
API calls fail silently on network errors.

## Acceptance criteria
- Retry up to 3 times with exponential backoff
- Log each retry attempt
- Surface final error to user"
```

Wait for user approval before running.

### Step 4: Execute

Run `bd create` with all fields. Report the created task ID.

## Bulk Creation

When creating multiple tasks (e.g., during brainstorm or planning):

- Present all tasks as a table first for review
- After approval, create in parallel using subagents or sequential `bd create` calls
- Every task still needs all required fields - no shortcuts

## Anti-patterns

- **Never** create a task without `--labels`
- **Never** use bare `bd create --title="..."` without other fields
- **Never** use `--parent` flag (causes merge collisions in multi-user setup)
- **Never** skip user approval for task creation
- **Never** use priority words ("high", "medium") - use numbers 0-4

## Integration

User-facing skill. Auto-triggers on "create task", "new task", "bd create".
Commands that create tasks (`/lets:start`, `/lets:brainstorm`, `/lets:plan`) trigger this skill implicitly via description match.
See: `grep -r "bd create" commands/` for commands that create tasks.
