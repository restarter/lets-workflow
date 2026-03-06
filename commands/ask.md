---
description: Ask a single expert agent a question - like a Slack ping to a colleague
argument-hint: "[expert] [question]"
---

# Ask an Expert

Quick consultation with a single expert agent. Like pinging a colleague on Slack.

## Usage

```bash
/lets:ask                          # Interactive - asks who and what
/lets:ask security "is this safe?" # Direct to expert with question
/lets:ask architect                # Opens dialog, asks question after
```

## Difference from /lets:opinion

| | /lets:ask | /lets:opinion |
|---|---|---|
| Purpose | Ask one colleague | Team meeting |
| Experts | Always 1 | 3-5 in parallel |
| Input | Question | Decision + options |
| Output | Direct answer | Comparison table + recommendation |
| Analogy | Slack ping | 30 min meeting |

## Step 1: Determine Expert

If specified in arguments - use it. Otherwise use **AskUserQuestion** to pick.

Available experts (map to `lets:*` agents):

| # | Shorthand | Agent (subagent_type) | Expertise |
|---|-----------|----------------------|-----------|
| 1 | architect | lets:architect | System design, patterns, SOLID |
| 2 | security | lets:security | Vulnerabilities, auth, crypto |
| 3 | backend | lets:backend | API, logic, performance |
| 4 | frontend | lets:frontend | UI, React/Vue, a11y |
| 5 | database | lets:database | Schema, queries, migrations |
| 6 | devops | lets:devops | Docker, CI/CD, infrastructure |
| 7 | qa | lets:qa | Testing, coverage, strategy |
| 8 | docs | lets:docs | Documentation, API docs |
| 9 | compliance | lets:compliance | Project rules, standards |
| 10 | git-historian | lets:git-historian | History, past decisions, blame |
| 11 | pragmatist | lets:pragmatist | ROI, effort vs value, scope |

**Shorthand mapping:** User can type short names like "security" or "sec" - map to the correct agent subagent_type.

**If no expert specified**, select top 4 most relevant based on conversation context and use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Which expert to ask?",
    header: "LETS",
    options: [
      { label: "{expert 1}", description: "{expertise}" },
      { label: "{expert 2}", description: "{expertise}" },
      { label: "{expert 3}", description: "{expertise}" },
      { label: "{expert 4}", description: "{expertise}" }
    ],
    multiSelect: false
  }]
)
```

**Other** (free text) -> match to expert shorthand from table above.

## Step 2: Determine Question

If passed as argument - use it. Otherwise ask the user.

## Step 3: Gather Context

Before launching the agent, gather relevant context:

```bash
# ROOT = project-root from LETS Config
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

Also check if the question references specific files - if so, note the file paths for the agent.

## Step 4: Mandatory Agent Context

If the selected agent appears in this table, append the instruction to the prompt:

| Agent | Instruction |
|-------|-------------|
| `compliance` | "Only flag violations EXPLICITLY mentioned in CLAUDE.md. Quote the rule being violated." |
| `git-historian` | "Use git blame and git log to analyze historical context." |
| `docs` | "Check CLAUDE.md sync, docs/ sync, beads tracking, README/config docs." |
| `pragmatist` | "Assess if the solution is proportional to the problem. Flag overengineering." |

## Step 5: Launch Agent

Use the Task tool to spawn the selected agent:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

RESPONSE LANGUAGE: {language from LETS Config, e.g. "English"}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

ASK MODE. A developer is asking you a direct question.

PROJECT CONTEXT:
{CLAUDE.md summary - stack, conventions, key rules}

QUESTION: {user_question}

INSTRUCTIONS:
- Give a clear, actionable answer
- Use code examples if relevant
- Reference project patterns from CLAUDE.md where applicable
- Be concise - this is a Slack reply, not an essay
- If the question is too vague, say what you'd need to know to answer properly
- If files are referenced, read them before answering"
)
```

## Step 6: Present Results

Show the agent's response:

```
## {Agent Name} says:

{agent response}
```

## Step 7: Link Answer to Active Task

```bash
BRANCH=$(git branch --show-current)
# Extract task ID from branch name (feature/<id>-<slug> or worktree-<id>-<slug>)
# Strategy: look for beads ID pattern anywhere in branch name

# Fallback: bd list --status=in_progress
```

If multiple in-progress tasks found via fallback, skip beads comment.
If active task found:

```bash
bd comments add <task-id> "Asked {agent-name}: {question summary}. Answer: {1-sentence key takeaway}"
```

Skip if the question is generic (not related to the active task).

## Output

```
┌─ LETS ─────────────────────────┐
│  More detail?  /lets:opinion   │
│  Check code?   /lets:check     │
└────────────────────────────────┘
```

## Rules

- Respond in user's language

## Notes

- Agents use their own model from frontmatter (opus for critical agents, session model for others)
- If the user asks a follow-up question about the same topic, route to the same expert
- If the expert says "this needs a broader discussion", suggest `/lets:opinion`
