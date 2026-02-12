---
name: lets-ask
description: Ask a single expert agent a question - like a Slack ping to a colleague. Use for quick consultations when you need one expert's perspective.
---

# Ask an Expert

Quick consultation with a single expert agent. Like pinging a colleague on Slack.

## Usage

```bash
/lets-ask                          # Interactive - asks who and what
/lets-ask security "is this safe?" # Direct to expert with question
/lets-ask architect                # Opens dialog, asks question after
```

## Difference from /lets-opinion

| | /lets-ask | /lets-opinion |
|---|---|---|
| Purpose | Ask one colleague | Team meeting |
| Experts | Always 1 | 3-5 in parallel |
| Input | Question | Decision + options |
| Output | Direct answer | Comparison table + recommendation |
| Analogy | Slack ping | 30 min meeting |

## Step 1: Determine Expert

If specified in arguments - use it. Otherwise show picker using AskUserQuestion:

Available experts (map to `.claude/agents/` files):

| # | Name | Agent file | Expertise |
|---|------|-----------|-----------|
| 1 | architect | architect | System design, patterns, SOLID |
| 2 | security | security-expert | Vulnerabilities, auth, crypto |
| 3 | backend | backend-expert | API, logic, performance |
| 4 | frontend | frontend-expert | UI, React/Vue, a11y |
| 5 | database | database-expert | Schema, queries, migrations |
| 6 | devops | devops-expert | Docker, CI/CD, infrastructure |
| 7 | qa | qa-expert | Testing, coverage, strategy |
| 8 | docs | docs-expert | Documentation, API docs |
| 9 | compliance | compliance-expert | Project rules, standards |
| 10 | git-historian | git-historian | History, past decisions, blame |
| 11 | pragmatist | pragmatist | ROI, effort vs value, scope |

**Shorthand mapping:** User can type short names like "security" or "sec" - map to the correct agent file name.

## Step 2: Determine Question

If passed as argument - use it. Otherwise ask the user.

## Step 3: Gather Context

Before launching the agent, gather relevant context:

```bash
# Project rules
ROOT=$(git rev-parse --show-toplevel)
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

Also check if the question references specific files - if so, note the file paths for the agent.

## Step 4: Launch Agent

Use the Task tool to spawn the selected agent:

```
Task(
  subagent_type="{agent-file-name}",
  model="sonnet",
  prompt="ASK MODE. A developer is asking you a direct question.

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

## Step 5: Output

Present the agent's response:

```
## {Agent Name} says:

{agent response}
```

Then show the LETS box:

```
┌─ LETS ─────────────────────────┐
│  More detail?  /lets-opinion   │
│  Check code?   /lets-check     │
└────────────────────────────────┘
```

## Notes

- Always use sonnet as the default model
- If the user asks a follow-up question about the same topic, route to the same expert
- If the expert says "this needs a broader discussion", suggest `/lets-opinion`
