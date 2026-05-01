---
description: Technical decision analysis from expert agents launched in parallel, with clear recommendation
argument-hint: "[topic or question]"
---

# Technical Decision Analysis

Analyze technical decisions by launching expert agents in parallel. Each agent provides their perspective, then results are aggregated into a clear recommendation.

**DO NOT ask the user which option to choose.** Analyze and recommend.

## Step 1: Frame the Problem

```
**Decision:** {what needs to be decided}
**Options:** A) ... B) ... C) ...
**Constraints:** {time, compatibility, legacy, budget}
```

## Step 2: Select Experts

Based on the decision topic, select relevant experts. Recommended defaults:

| Decision about... | Recommended experts |
|-------------------|-------------------|
| Auth/tokens/encryption | security, architect, backend, pragmatist |
| DB schema/migrations | database, architect, backend, pragmatist |
| Docker/CI/deploy | devops, security, architect, pragmatist |
| API design | architect, backend, security, pragmatist |
| UI/UX/components | frontend, architect, qa, pragmatist |
| Testing strategy | qa, backend, architect, pragmatist |
| Performance | backend, database, devops, pragmatist |
| General architecture | architect, security, backend, pragmatist |
| Code quality | architect, compliance, qa, pragmatist |

Adjust based on decision complexity - add more experts for cross-cutting decisions, fewer for simple binary choices.

**Guidelines:**
- `architect` and `pragmatist` always included
- Table above is a starting point, not a cap - add or remove agents based on context
- `actor` can replace or supplement any domain agent. If actor is selected, use the **actor-fetch-personality** skill (read `${CLAUDE_PLUGIN_ROOT}/skills/actor-fetch-personality/SKILL.md`) to fetch personality. Pass `PERSONALITY:` block in the actor's Task prompt only.
- Agents use their own model from frontmatter (opus for critical agents, session model for others)

**Confirmation gate:** If planning to launch more than 10 experts:

```
AskUserQuestion(
  questions=[{
    question: "Planning {N} experts for this decision. That's a lot - confirm?",
    header: "LETS",
    options: [
      { label: "Launch all", description: "{N} experts, thorough analysis" },
      { label: "Reduce", description: "Suggest fewer, more focused experts" }
    ],
    multiSelect: false
  }]
)
```

## Step 3: Gather Context

```bash
# ROOT = project-root from LETS Config
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

## Step 3.5: Show Expert Panel

Before launching, display the selection:

```
## Expert Panel

Decision: {topic summary}
Selected {N} experts:
1. architect - {reason from topic match}
2. pragmatist - always included
3. {domain agent} - {reason}
...

> Subagents have a separate rate limit - no cost to your conversation.

Analyzing...
```

## Step 4: Launch Agents in Parallel

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

MODE: opinion

PROJECT CONTEXT:
{CLAUDE.md summary}

DECISION: {what needs to be decided}
OPTIONS:
A) {option A description}
B) {option B description}
C) {option C description - if applicable}
CONSTRAINTS: {context, time, legacy, etc.}

**MANDATORY:** Always emit the full structured response as text. If you persist to memory, do it AFTER your text response is complete. Never emit only "Memory persisted" or a tool-call summary as your response."
)
```

## Step 5: Aggregate Results

After all agents respond, synthesize:

### Quick Verdict (TL;DR)

```
**Recommendation:** Option X
**One-liner:** {why in 10 words or less}
```

### Expert Opinions

For each agent, summarize their position:

```
**Architect:** Recommends B - cleaner separation, lower coupling
**Security:** Recommends B - smaller attack surface, but warns about X
**Backend:** Recommends A - faster to implement, but agrees B is better long-term
**Pragmatist:** Recommends B - worth the extra day, saves weeks later
```

### Comparison Table

| Criterion | Option A | Option B | Winner |
|-----------|----------|----------|--------|
| Simplicity | +/~/- | +/~/- | A/B |
| Maintainability | ... | ... | ... |
| Security | ... | ... | ... |
| Time to implement | ... | ... | ... |
| Risk | ... | ... | ... |

### Final Recommendation

```
**Recommendation:** Option X

**Reason:** {1-2 sentences - the deciding factor}

**Risk:** {what could go wrong}

**Action:** {specific next step}
```

## Step 6: Link Decision to Active Task

Record the decision in beads for future context recovery:

Use the **detect-task** skill to find the active task (read `${CLAUDE_PLUGIN_ROOT}/skills/detect-task/SKILL.md` and follow its detection flow).
If multiple tasks found, skip beads comment.
If active task found:

```bash
bd comments add <task-id> "Decision: {topic}. Chose: {recommended option}. Reason: {1-sentence why}"
```

## Rules

- Respond in user's language

## Anti-patterns

- "Both options are valid, you decide"
- "It depends on your preferences"
- Over-explaining without concluding
- Ignoring security implications
- Perfectionism over pragmatism

## Decision Principles

1. **Simple > Clever** - easier to debug at 3 AM
2. **Industry standard > Custom** - less maintenance
3. **Explicit > Implicit** - fewer surprises
4. **Reversible > Perfect** - can change later
5. **Working > Elegant** - ship first, refactor later

## Step 7: Discuss (opt-in)

After presenting the recommendation, offer to explore it deeper:

```
AskUserQuestion(
  questions=[{
    question: "What's next?",
    header: "LETS",
    options: [
      { label: "Discuss", description: "Explore the recommendation - questions, trade-offs, assumptions" },
      { label: "Accept", description: "Recommendation is clear, move on" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Discuss** -> enter exploration loop (see below)
- **Accept** -> proceed to Output
- **Other** (free text) -> treat as question about the recommendation

### Exploration Loop

Interactive discussion about the recommendation. One question/insight at a time.

After each user response:
1. **Acknowledge** - brief, no fluff
2. **Build** - add an insight or connection based on their answer
3. **Probe** - ask the next question that goes deeper

Question types:
- Challenge the recommendation: "The agents recommended X, but what about Y?"
- Probe edge cases: "This works for the common case, but what happens when Z?"
- Surface hidden trade-offs: "Option A is faster, but does it lock you into W?"
- Connect to project context: "Given {existing pattern}, does this recommendation still hold?"

When user is satisfied, proceed to Output.

If active task exists, record the discussion outcome:

```bash
bd comments add <task-id> "Decision discussion: {topic}. Explored: {what was discussed}. Conclusion: {final stance}"
```

## Output

After recommendation (or after discussion), show LETS box based on context:

**If decision is about code changes:**
```
┌─ LETS ─────────────────────────┐
│  Check?     /lets:check        │
└────────────────────────────────┘
```

**If decision is about completed work:**
```
┌─ LETS ─────────────────────────┐
│  Done?    /lets:done           │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

## Note

This command reads project context from CLAUDE.md automatically.
Keep decisions aligned with project constraints and patterns.
