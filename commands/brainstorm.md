---
description: Interactive backlog helper - review tasks, suggest priorities, spot patterns, create tasks
argument-hint: "[topic or epic name]"
---

# Brainstorm

Interactive backlog session. Review existing tasks, spot patterns, suggest priorities, create new tasks. No agents - this is a conversation between you and the user about what to build.

**This command helps with WHAT to build. For HOW to build it, use `/lets:plan`.**

## Step 0: Choose Mode

If argument provided AND it's clearly an idea/topic (not an epic name), go directly to Explore mode.

Otherwise ask:

```
AskUserQuestion(
  questions=[{
    question: "What would you like to do?",
    header: "LETS",
    options: [
      { label: "Review backlog", description: "Review tasks, priorities, gaps, patterns" },
      { label: "Explore idea", description: "Think through an idea together - questions, insights, trade-offs" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Review backlog** -> proceed to Step 1 (Backlog mode)
- **Explore idea** -> jump to Step E1 (Explore mode)
- **Other** (free text) -> treat as topic for Explore mode

---

## Backlog Mode

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

## Rules (Backlog Mode)

- **No agents** - this is a direct conversation
- **No plan output** - if user wants structured planning, suggest `/lets:plan`
- **All task mutations require user approval** - don't create/update tasks without asking
- Respond in user's language (Ukrainian/Russian/English)

## Output (Backlog Mode)

```
┌─ LETS ──────────────────────────────────┐
│  Plan a task?  /lets:plan              │
│  Start work?   /lets:start             │
└─────────────────────────────────────────┘
```

---

## Explore Mode

Interactive exploration of an idea or concept. Ask probing questions, share insights, challenge assumptions. Build shared understanding before jumping to implementation.

**This is a conversation, not a checklist.** One question or insight at a time.

### Step E1: Capture Topic

If argument provided: use it as topic.
If not: ask "What idea or concept do you want to explore?"

### Step E2: Open with Insights

Start with 1-2 observations or connections, then ask the first probing question:

```
## Let's explore: {topic}

**Insight:** {initial observation - what's interesting, non-obvious, or connected to something in the project}

**Question:** {probing question that helps define scope or challenges an assumption}
```

### Step E3: Conversation Loop

After each user response:

1. **Acknowledge** - brief, no fluff ("Good point." / "That changes things.")
2. **Build** - add an insight or connection based on their answer
3. **Probe** - ask the next question that goes deeper or shifts angle

### Question Types

- **Assumption challenges:** "You're assuming X - what if Y instead?"
- **Edge case probes:** "What happens when Z?"
- **Trade-off surfacing:** "This gives you A but costs B - is that worth it?"
- **Scope questions:** "Do you actually need X, or is Y enough for now?"
- **User perspective:** "From the end user's view, what changes?"
- **Feasibility:** "What's the simplest version of this that still delivers value?"
- **Connections:** "This reminds me of {existing thing} - could we reuse/extend it?"

### What Makes Good Insights

- Connect dots between the idea and existing project patterns
- Surface things that "almost exist" already
- Flag decisions that are easy to change now but expensive later
- Show non-obvious implications of a choice
- Note when two goals conflict

### Step E4: Exit & Capture

Signals to wrap up:
- User gives short answers (topic exhausted)
- Key questions covered
- User explicitly says "enough" / "let's move on"

When wrapping up, summarize key findings:

```
## Exploration Summary

**Topic:** {what we explored}

**Key insights:**
- {insight 1}
- {insight 2}

**Decisions made:**
- {decision 1}

**Open questions:**
- {anything unresolved}
```

If active task exists, record to beads:

```bash
bd comments add <task-id> "Exploration: {topic}
- {key insight 1}
- {key insight 2}"
```

### Rules (Explore Mode)

- **No agents** - direct conversation
- **No code changes** - exploration produces understanding, not code
- Respond in user's language

### Output (Explore Mode)

```
┌─ LETS ─────────────────────────┐
│  Plan?   /lets:plan            │
│  Start?  /lets:start           │
└────────────────────────────────┘
```

If exploration produced a clear task idea, ask: "Want me to create a task for this?" (plain text, not in LETS box).
