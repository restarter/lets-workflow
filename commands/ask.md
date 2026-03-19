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
| 12 | actor | lets:actor | External personality - any expertise via loaded persona |

**Shorthand mapping:** User can type short names like "security" or "sec" - map to the correct agent subagent_type.

**Actor handling:** If expert is `actor`, the remaining argument should contain a personality source (URL or file path) followed by the question. Example: `/lets:ask actor https://example.com/persona.md "question"`. Use the **actor-fetch-personality** skill (read `skills/actor-fetch-personality/SKILL.md`) to fetch and validate the personality. If no source provided, ask via AskUserQuestion: "Personality source? (URL or file path)". Pass fetched content as `PERSONALITY:` block in the Task prompt (see Step 4).

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

## Step 4: Launch Agent

Use the Task tool to spawn the selected agent:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

RESPONSE LANGUAGE: {language from LETS Config, e.g. "English"}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

MODE: ask

{If actor agent: include PERSONALITY block from actor-fetch-personality skill}
PERSONALITY:
{fetched personality content - only for lets:actor, omit for other agents}

PROJECT CONTEXT:
{CLAUDE.md summary - stack, conventions, key rules}

QUESTION: {user_question}"
)
```

## Step 5: Present Results

Show the agent's response:

```
## {Agent Name} says:

{agent response}
```

## Step 6: Link Answer to Active Task

Use the **detect-task** skill to find the active task (read `skills/detect-task/SKILL.md` and follow its detection flow).
If multiple tasks found, skip beads comment.
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
