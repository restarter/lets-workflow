---
description: Quick interactive ideation on a topic - fast context scan, no agents (for backlog review/cleanup use /lets:backlog)
argument-hint: "[topic]"
---

# Brainstorm

Fast interactive ideation - a quick context scan (no agents), then a direct conversation. For a specific topic, hands off to `/lets:explore`.

**For backlog review or cleanup, use `/lets:backlog`. To explore a specific topic in depth, use `/lets:explore`. For HOW to build something, use `/lets:plan`.**

## Quick Brainstorm Mode

No agents. Command gathers context directly and enters conversation.

### Step Q0: Topic-or-Backlog Gate

If the argument is a specific idea/topic to explore in depth, suggest `/lets:explore <topic>` (multi-perspective topic exploration lives there) and stop - do NOT continue into Q1.

Otherwise (no argument, or the argument is an epic/area to use as backlog context): fall through to Step Q1, passing any argument along as optional backlog context.

### Step Q1: Gather Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
bd stats
bd list --status=open -n 30
bd list --status=in_progress
git log --oneline -15
```

Also use Grep tool (not bash grep) to scan for tech debt signals:
```
Grep(pattern="TODO|FIXME|HACK|XXX", path="$LETS_PROJECT_ROOT", output_mode="content", head_limit=20)
```

If argument provided (topic/epic):
- `bd search {argument}` (if no results, fall back to `bd list` and scan titles)
- Grep/Glob for related code

### Step Q2: Open with Insights

Based on gathered context, present 3-5 proactive observations:

```
## Quick Brainstorm

Based on what I see in the project:

1. **{observation}** - {why it matters, 1-2 sentences}
2. **{observation}** - {why it matters}
3. **{observation}** - {why it matters}

**Question:** {probing question that opens discussion}
```

Categories to draw observations from:
- Gaps: areas with activity but no tasks
- Stale work: tasks open long with no progress
- Patterns: recurring themes across tasks/commits
- Quick wins: small improvements with high impact
- Risks: things that could break or become debt

### Step Q3: Interactive Dialog

After each user response:
1. **Acknowledge** - brief, no fluff
2. **Build** - add insight or connection
3. **Probe** - ask next question that goes deeper

If user wants deeper expert analysis, suggest:
"Want a deeper backlog review? `/lets:backlog review` launches expert agents."

### Step Q4: Capture & Exit

If active task:

```bash
bd comments add <task-id> "Quick brainstorm: {key takeaways, 2-3 items}"
```

If ideas emerged that deserve tasks, offer to create them.

## Output

```
┌─ LETS ─────────────────────────┐
│  Go deeper?  /lets:backlog     │
│  Plan?       /lets:plan        │
│  Start?      /lets:start       │
└────────────────────────────────┘
```

## Rules

- All task mutations (create) require user approval
- No agents - Quick brainstorm is a direct conversation
- If the argument is a specific topic to explore, hand off to /lets:explore (do not run Q1-Q4)
- Respond in user's language
