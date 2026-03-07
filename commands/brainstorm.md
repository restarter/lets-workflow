---
description: Interactive ideation - review backlog with agents, explore ideas, quick brainstorm, cleanup stale tasks
argument-hint: "[topic or epic name]"
---

# Brainstorm

Interactive ideation with 4 modes. Heavy modes launch explorer + parallel agents for multi-perspective insights. Light modes work instantly.

**This command helps with WHAT to build. For HOW to build it, use `/lets:plan`.**

## Step 0: Choose Mode

If argument provided AND it's clearly an idea/topic (not an epic name), go directly to Explore idea mode.

Otherwise ask:

```
AskUserQuestion(
  questions=[{
    question: "What would you like to do?",
    header: "Brainstorm",
    options: [
      { label: "Review backlog", description: "Agents analyze project state, generate ideas and surface gaps (~2-3 min)" },
      { label: "Explore idea", description: "Agents research a topic from different angles (~2-3 min)" },
      { label: "Quick brainstorm", description: "Fast context scan, direct conversation - no agents" },
      { label: "Cleanup", description: "Triage stale tasks - close, merge, reprioritize" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Review backlog** -> set mode variables (Step R0), then Heavy Mode Flow
- **Explore idea** -> set mode variables (Step E0), then Heavy Mode Flow
- **Quick brainstorm** -> Step Q1
- **Cleanup** -> Step C1
- **Other** (free text) -> treat as topic for Explore idea mode

---

## Heavy Mode Flow

Shared flow for Review backlog and Explore idea modes. Each mode sets variables before entering this flow.

**Mode variables** (set by calling mode before entering flow):
- `{MODE_NAME}`: "Review backlog" or "Explore idea"
- `{EXPLORER_PROMPT}`: mode-specific explorer prompt (see Step R0 / Step E0)
- `{AGENT_PROMPT_TEMPLATE}`: mode-specific prompt for brainstorm agents
- `{FORCED_AGENT}`: agent always included (pragmatist for backlog, architect for explore)
- `{DIALOG_QUESTION}`: mode-specific dialog question text

### Phase 1: Explorer - Gather Context

Launch explorer to build a context profile.

```
Task(
  subagent_type="lets:explorer",
  prompt="{EXPLORER_PROMPT}"
)
```

#### Explorer Failure Guard

If explorer fails, times out, or returns no structured profile:

> "Explorer couldn't gather context. Switching to Quick brainstorm."

Jump to Step Q1 (Quick brainstorm mode).

#### Thin Profile Guard

After explorer returns, check task count from profile's Backlog Summary section.

If < 5 open tasks reported:

> "Project has very few tasks - not enough context for multi-agent analysis. Switching to Quick brainstorm."

Jump to Step Q1 (Quick brainstorm mode).

### Phase 2: Select Brainstorm Agents

Command analyzes the explorer profile and selects agents. Do NOT ask the explorer to recommend agents - explorer maps, command decides.

**Selection logic:**

Scan the profile for signals and match to agents:

| Signal in Profile | Select Agent |
|-------------------|-------------|
| API/endpoint/backend mentions, backend-heavy code | backend |
| UI/frontend/component mentions | frontend |
| Auth/security/credentials/secrets mentions | security |
| Database/schema/migration mentions | database |
| Docker/CI/deploy/infrastructure mentions | devops |
| Test/coverage/quality mentions | qa |
| Docs/README/onboarding mentions | docs |
| Module boundaries, coupling, architecture patterns | architect |
| Many tasks, mixed priorities, scope questions | pragmatist |

**Rules:**
1. Always include `{FORCED_AGENT}` (pragmatist for backlog, architect for explore)
2. Select agents matching signals found in profile
3. Cap at 5 agents total, minimum 3
4. If fewer than 3 signals matched, pad with architect + pragmatist

**Agent eligibility:**

| Agent | Eligible | Brainstorm Strength |
|-------|----------|-------------------|
| architect | Yes | System gaps, module boundaries, structural opportunities |
| pragmatist | Yes | ROI, what to cut, what to prioritize |
| backend | Yes | API gaps, performance bottlenecks |
| frontend | Yes | UX gaps, component patterns |
| security | Yes | Security debt, missing protections |
| database | Yes | Schema evolution, query patterns |
| devops | Yes | CI/CD gaps, infrastructure debt |
| qa | Yes | Testing gaps, coverage holes |
| docs | Yes | Documentation debt, onboarding gaps |
| compliance | No | Rule-checking, not ideation |
| git-historian | No | Historical analysis, not forward-looking |
| implementer | No | Has write access, wrong mode |
| explorer | No | Already used in phase 1 |

Show selection before launching (no user gate - brainstorm = momentum):

```
## Brainstorm Panel

Based on project context, selected {N} experts:
1. {agent} - {signal that triggered selection}
2. {agent} - {signal}
...

Launching...
```

### Phase 3: Launch Brainstorm Agents (Parallel)

CRITICAL: Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="{AGENT_PROMPT_TEMPLATE}"
)
```

The prompt template is set by the calling mode (Step R0 or Step E0). Both templates share this structure:

- ultrathink prefix
- RESPONSE LANGUAGE + PROJECT ROOT
- BRAINSTORM MODE header (review or explore variant)
- Context profile from explorer (keep concise - pass summary, not raw data)
- Instructions specific to the mode
- Mandatory agent context from table below
- Output format

Mandatory agent context (append to prompt based on agent):

| Agent | Append to prompt |
|-------|-----------------|
| pragmatist | "Focus on ROI. Which ideas deliver the most value for least effort? Flag overengineering or premature optimization." |
| architect | "Focus on structural opportunities. Where could the system be simplified, better decomposed, or made more extensible?" |
| security | "Focus on security debt and missing protections. What attack surfaces are unprotected?" |
| docs | "Focus on documentation debt. What's undocumented, stale, or missing for onboarding?" |
| qa | "Focus on quality gaps. What's untested? Where would tests catch real bugs?" |

### Phase 4: Aggregate & Present

After all agents respond:

1. Group ideas by impact (high first)
2. Deduplicate: if two agents suggest the same area, merge and note both perspectives
3. Separate: Top Ideas / Gaps / Quick Wins

```
## Brainstorm Results

{N} ideas from {M} experts.

### High Impact
1. **{idea}** ({agent}) - {one-liner}
2. **{idea}** ({agents if merged}) - {one-liner}
...

### Medium Impact
1. **{idea}** ({agent}) - {one-liner}
...

### Gaps Identified
- {gap} ({agent})
...

### Quick Wins
- {quick win} ({agent})
...
```

### Phase 5: Interactive Dialog

```
AskUserQuestion(
  questions=[{
    question: "{DIALOG_QUESTION}",
    header: "Brainstorm",
    options: [
      { label: "Explore an idea", description: "Dig deeper into a specific idea or insight" },
      { label: "Create tasks", description: "Turn selected ideas into backlog tasks" },
      { label: "Done", description: "Enough for now" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Explore an idea** -> ask which one, enter conversation loop (acknowledge, build, probe)
- **Create tasks** -> ask which ideas, use `bd create` with user approval per task
- **Done** -> Phase 6
- **Free text** -> treat as exploring a specific idea

Dialog continues until user signals done.

### Phase 6: Capture & Exit

If active task:

```bash
bd comments add <task-id> "Brainstorm ({MODE_NAME}): {N} ideas from {M} agents.
Top ideas: {top 2-3 titles}
Tasks created: {list or 'none'}"
```

If exploration produced a clear task idea but none created, ask:
"Want me to create a task for this?" (plain text, not in LETS box).

---

## Review Backlog Mode

### Step R0: Set Mode Variables

```
MODE_NAME = "review backlog"
FORCED_AGENT = pragmatist
DIALOG_QUESTION = "What catches your eye?"
```

**EXPLORER_PROMPT:**

```
"ultrathink

BRAINSTORM SCOUT MODE. Gather project context for a brainstorm session.

RESPONSE LANGUAGE: {language from LETS Config}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

GOAL: Build a Project State Profile for brainstorming. We need to understand
what the project is, what work has been done, what's planned, and where gaps exist.

AVAILABLE CONTEXT SOURCES (read what's relevant, skip what's not):

1. BACKLOG STATE
   Run: bd stats
   Run: bd list --status=open -n 50
   Run: bd list --status=in_progress
   Run: bd list --status=done -n 20
   Purpose: understand task distribution, priorities, what's active

2. TASK DETAILS (selective - pick 5-10 most interesting tasks)
   Run: bd show <task-id>
   Run: bd comments <task-id>
   Purpose: understand context, decisions, blockers on key tasks

3. RECENT SESSION SUMMARIES
   Read: .lets/sessions/*.md (most recent 3-5 files by name)
   Purpose: what was worked on recently, what momentum exists

4. CODEBASE SIGNALS
   Grep for: TODO, FIXME, HACK, XXX across source files
   Read: CLAUDE.md (project structure and architecture)
   Purpose: understand technical debt signals and project shape

5. RECENT GIT ACTIVITY
   Run: git log --oneline -20
   Run: git log --oneline --since='2 weeks ago' --format='%s'
   Purpose: what areas are actively changing

BUDGET: Be efficient. Read backlog first (sources 1-2), then decide which
of sources 3-5 add value. If backlog is rich (50+ tasks with comments),
you can skip session summaries. If backlog is sparse, lean more on git
and codebase signals.

Keep output concise - max ~500 words. This profile will be passed to multiple agents.

OUTPUT FORMAT - Project State Profile:

## Project State Profile

### Project Shape
{what this project is, tech stack, size - from CLAUDE.md}

### Backlog Summary
- Open: {N} tasks
- In progress: {N}
- Done recently: {N}
- By area/label: {breakdown if labels exist}

### Active Momentum
{what's being worked on now, what sessions focused on recently}

### Hot Areas
{parts of codebase with most recent activity - files, modules}

### Gaps & Signals
- Missing coverage: {areas with no tasks but active code}
- Stale tasks: {tasks open for a long time with no activity}
- TODOs in code: {count and themes}
- Recurring themes: {patterns across tasks/comments/sessions}"
```

**AGENT_PROMPT_TEMPLATE:**

```
"ultrathink

RESPONSE LANGUAGE: {language from LETS Config}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

BRAINSTORM MODE. Generate ideas and surface gaps from your area of expertise.

You are NOT reviewing code or evaluating a decision. You are looking at a project's
current state and generating actionable insights about what to build, fix, or improve.

PROJECT STATE PROFILE:
{explorer output from Phase 1}

INSTRUCTIONS:
- Scan the project from YOUR expertise lens
- Read files in areas relevant to your domain if needed (you have Read/Grep/Glob)
- Generate 3-7 ideas: things to build, improve, fix, or investigate
- Each idea must be specific and actionable (not vague like 'improve testing')
- Prioritize ideas by impact (high/medium)
- Flag gaps: areas with no tasks but clear need from your perspective
- Connect to existing tasks when relevant ('task X could be extended to also cover Y')
- Be opinionated - rank and recommend, don't just list

{mandatory agent context}

OUTPUT FORMAT:

## {Your Domain} Perspective

### Top Ideas
1. **{idea title}** [Impact: high/medium]
   {2-3 sentences: what, why it matters, rough scope}
   {connection to existing task if any}

### Gaps I See
- {gap}: {why this matters from your expertise}

### Quick Wins
- {small improvement that could be done in one session}"
```

Then enter Heavy Mode Flow (Phase 1).

---

## Explore Idea Mode

### Step E0: Capture Topic & Set Mode Variables

If argument provided: use it as topic.
If not: ask "What idea or topic do you want to explore?"

Wait for answer before proceeding.

```
MODE_NAME = "explore: {topic}"
FORCED_AGENT = architect
DIALOG_QUESTION = "What resonates?"
```

**EXPLORER_PROMPT:**

```
"ultrathink

BRAINSTORM SCOUT MODE. Gather project context relevant to a specific topic.

RESPONSE LANGUAGE: {language from LETS Config}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

TOPIC: {user's topic}

AVAILABLE CONTEXT SOURCES (read what's relevant to this topic):

1. RELATED TASKS
   Run: bd search {topic keywords}
   Run: bd list --status=open -n 30
   Purpose: find tasks related to this topic
   Note: if bd search returns no results, fall back to bd list and scan titles manually

2. TASK DETAILS (for related tasks found above)
   Run: bd show <task-id>
   Run: bd comments <task-id>
   Purpose: understand existing thinking on this topic

3. RELATED CODE
   Grep for: {topic keywords} across source files
   Glob for: files in areas related to the topic
   Read: key files that relate to the idea
   Purpose: understand what exists already

4. PROJECT CONTEXT
   Read: CLAUDE.md
   Purpose: understand where this topic fits in the project

5. SESSION HISTORY
   Read: .lets/sessions/*.md (scan for topic mentions)
   Purpose: has this been discussed before?

BUDGET: Focus on sources 1-3. Source 4 is always worth reading.
Source 5 only if sources 1-2 are sparse.

Keep output concise - max ~500 words. This profile will be passed to multiple agents.

OUTPUT FORMAT - Topic Context Profile:

## Topic Context Profile: {topic}

### What Exists Already
{code, tasks, or prior work related to this topic}

### Related Tasks
{list of tasks touching this area, with status}

### Prior Discussions
{anything from task comments or sessions about this topic}

### Codebase Touchpoints
{files and modules this topic would affect}"
```

**AGENT_PROMPT_TEMPLATE:**

```
"ultrathink

RESPONSE LANGUAGE: {language from LETS Config}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

BRAINSTORM MODE. Explore a specific topic from your area of expertise.

You are NOT reviewing code or evaluating a decision. You are thinking through
an idea and generating insights, questions, and angles from your domain.

TOPIC: {user's topic}

TOPIC CONTEXT PROFILE:
{explorer output from Phase 1}

INSTRUCTIONS:
- Think about this topic through YOUR expertise lens
- Read relevant code if needed to ground your thinking
- Generate 2-4 insights: non-obvious angles, risks, opportunities
- Surface questions the user should answer before proceeding
- Suggest approaches or patterns from your domain that apply
- If this topic has been partially explored before (see context), build on it
- Be specific to THIS project, not generic advice

{mandatory agent context}

OUTPUT FORMAT:

## {Your Domain} Perspective on: {topic}

### Insights
1. **{insight}**
   {2-3 sentences: why this matters, how it connects}

### Questions to Consider
- {question from your domain perspective}

### Suggested Approach
{concrete recommendation for how to approach this topic from your domain}"
```

Then enter Heavy Mode Flow (Phase 1).

---

## Quick Brainstorm Mode

No agents. Command gathers context directly and enters conversation.

### Step Q1: Gather Context

```bash
# ROOT = project-root from LETS Config
bd stats
bd list --status=open -n 30
bd list --status=in_progress
git log --oneline -15
grep -rn "TODO\|FIXME\|HACK\|XXX" --include="*.md" --include="*.ts" --include="*.py" --include="*.php" --include="*.go" --include="*.js" --include="*.sh" "$ROOT" 2>/dev/null | head -20
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
"Want to go deeper? `/lets:brainstorm` (Review backlog) launches expert agents."

### Step Q4: Capture & Exit

If active task:

```bash
bd comments add <task-id> "Quick brainstorm: {key takeaways, 2-3 items}"
```

If ideas emerged that deserve tasks, offer to create them.

---

## Cleanup Mode

No agents. Direct interactive triage of stale/messy backlog items.

### Step C1: Load Backlog

```bash
bd stats
bd list --status=open -n 50
bd list --status=done -n 20
```

### Step C2: Present Cleanup Targets

Analyze loaded data and group:

```
## Cleanup Targets

### Stale Tasks (open with no recent activity - estimate from bd list output)
- **{title}** (`task-id`) - {status/priority info}
...

### Potential Duplicates
- **{title A}** (`id`) ~ **{title B}** (`id`) - similar scope
...

### Missing Labels/Priority
- **{title}** (`id`) - no labels, priority P4
...

### Done but Not Closed
- **{title}** (`id`) - status done
...
```

If no issues found in a category, skip it.
If backlog is clean, say so and suggest Review backlog or Explore idea instead.

### Step C3: Interactive Triage

For each group, walk through items and ask:

```
AskUserQuestion(
  questions=[{
    question: "{task title} ({task-id}). What to do?",
    header: "Triage",
    options: [
      { label: "Close", description: "No longer needed" },
      { label: "Keep", description: "Still relevant, skip" },
      { label: "Reprioritize", description: "Change priority" }
    ],
    multiSelect: false
  }]
)
```

For closing:
- `bd close <id>`
- If closing as duplicate: `bd comments add <other-id> "Absorbed from {closed-id}: {title}"` then `bd close <id>`

All mutations require user approval.
Continue until all groups processed or user says "enough".

### Step C4: Summary & Exit

```
## Cleanup Summary

- Closed: {N} tasks
- Reprioritized: {M} tasks
- Skipped: {K} tasks
```

If active task:

```bash
bd comments add <task-id> "Cleanup: closed {N}, reprioritized {M}"
```

---

## Output

### After Review Backlog / Explore Idea modes:

```
┌─ LETS ──────────────────────────────────┐
│  Plan a task?  /lets:plan               │
│  Start work?   /lets:start              │
└─────────────────────────────────────────┘
```

### After Quick Brainstorm:

```
┌─ LETS ──────────────────────────────────┐
│  Go deeper?  /lets:brainstorm           │
│  Plan?       /lets:plan                 │
│  Start?      /lets:start               │
└─────────────────────────────────────────┘
```

### After Cleanup:

```
┌─ LETS ─────────────────────────┐
│  Brainstorm?  /lets:brainstorm │
│  Start?       /lets:start      │
└────────────────────────────────┘
```

## Rules

- All task mutations (create, close, update) require user approval
- No agents in Quick brainstorm and Cleanup modes
- Explorer + agents only in Review backlog and Explore idea modes
- All agents launched in a SINGLE message (parallel)
- If explorer fails or returns thin profile (<5 tasks), degrade to Quick mode
- Explorer profile max ~500 words (passed to multiple agents)
- Respond in user's language
