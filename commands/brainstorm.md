---
description: Interactive backlog helper - review tasks, suggest priorities, spot patterns, create tasks
argument-hint: "[topic or epic name]"
---

# Brainstorm

Interactive backlog session. Review existing tasks, spot patterns, suggest priorities, create new tasks. No agents - this is a conversation between you and the user about what to build.

**This command helps with WHAT to build. For HOW to build it, use `/lets:plan`.**

## Step 1: Load Backlog Context

```bash
# ROOT = project-root from LETS Config
bd stats
bd list --status=open -n 30
```

If argument provided:
- If it matches an epic label: `bd list --labels epic:<argument>`
- Otherwise: `bd search <argument>` to find relevant tasks

## Step 2: Present Overview

Show a compact backlog summary:

```
## Backlog Overview

**Stats:** {open} open, {in_progress} in progress, {blocked} blocked

### By Epic
- **{epic name}**: {N} tasks ({M} open, {K} done)
- ...

### Ready to Work
{top 5 from bd ready, with priority}

### In Progress
{from bd list --status=in_progress}
```

If argument was provided, focus the overview on that topic/epic.

## Step 3: Interactive Discussion

Enter conversational mode. No rigid steps - respond to what the user wants to discuss.

**Things you can help with:**
- **Prioritization**: Review task priorities, suggest reordering
- **Gap analysis**: What's missing? What epics need more tasks?
- **Pattern spotting**: Similar tasks that could be grouped, duplicate work
- **Task creation**: Help formulate new tasks with proper title, description, labels, priority
- **Epic planning**: Review epic scope, suggest breakdown
- **Project insights**: Trends, bottleneck identification, velocity observations
- **Cleanup**: Find stale tasks, outdated priorities, tasks that can be closed

**When creating tasks:**
- Always use `bd create` with full parameters: `--title`, `--description`, `--type`, `--priority`, `--labels`
- Ask user for approval before creating
- Suggest epic labels for grouping

**When the conversation naturally ends** or user wants to move on, show the LETS box.

## Rules

- **No agents** - this is a direct conversation
- **No plan output** - if user wants structured planning, suggest `/lets:plan`
- **All task mutations require user approval** - don't create/update tasks without asking
- Respond in user's language (Ukrainian/Russian/English)

## Output

```
┌─ LETS ──────────────────────────────────┐
│  Plan a task?  /lets:plan              │
│  Start work?   /lets:start             │
└─────────────────────────────────────────┘
```
