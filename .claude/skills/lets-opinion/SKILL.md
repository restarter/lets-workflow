---
name: lets-opinion
description: Technical decision analysis from 3-5 expert agents launched in parallel, with clear recommendation
---

# Technical Decision Analysis

Analyze technical decisions by launching expert agents in parallel. Each agent provides their perspective, then results are aggregated into a clear recommendation.

## Instructions

**DO NOT ask the user which option to choose.** Analyze and recommend.

### Step 1: Frame the Problem

```
**Decision:** {what needs to be decided}
**Options:** A) ... B) ... C) ...
**Constraints:** {time, compatibility, legacy, budget}
```

### Step 2: Select Experts

Based on the decision topic, select 3-5 agents from `.claude/agents/`:

| Decision about... | Agents to launch |
|-------------------|-----------------|
| Auth/tokens/encryption | security-expert, architect, backend-expert, pragmatist |
| DB schema/migrations | database-expert, architect, backend-expert, pragmatist |
| Docker/CI/deploy | devops-expert, security-expert, architect, pragmatist |
| API design | architect, backend-expert, security-expert, pragmatist |
| UI/UX/components | frontend-expert, architect, qa-expert, pragmatist |
| Testing strategy | qa-expert, backend-expert, architect, pragmatist |
| Performance | backend-expert, database-expert, devops-expert, pragmatist |
| General architecture | architect, security-expert, backend-expert, pragmatist |
| Documentation | docs-expert, architect, compliance-expert, pragmatist |
| Code quality | architect, compliance-expert, qa-expert, pragmatist |

**Rules:**
- Minimum 3 agents, maximum 5
- `architect` and `pragmatist` always included
- Default model: sonnet for all

### Step 3: Gather Context

```bash
ROOT=$(git rev-parse --show-toplevel)
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

### Step 4: Launch Agents in Parallel

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent:

```
Task(
  subagent_type="{agent-file-name}",
  model="sonnet",
  prompt="OPINION MODE. Evaluate this technical decision.

PROJECT CONTEXT:
{CLAUDE.md summary}

DECISION: {what needs to be decided}
OPTIONS:
A) {option A description}
B) {option B description}
C) {option C description - if applicable}
CONSTRAINTS: {context, time, legacy, etc.}

INSTRUCTIONS:
- Give your expert perspective in 2-3 sentences
- State which option you recommend and why
- Flag risks from your area of expertise
- Be direct - no hedging, no 'it depends without conclusion'
- If you see a risk others might miss, highlight it"
)
```

### Step 5: Aggregate Results

After all agents respond, synthesize:

#### Quick Verdict (TL;DR)

```
**Recommendation:** Option X
**One-liner:** {why in 10 words or less}
```

#### Expert Opinions

For each agent, summarize their position:

```
**Architect:** Recommends B - cleaner separation, lower coupling
**Security:** Recommends B - smaller attack surface, but warns about X
**Backend:** Recommends A - faster to implement, but agrees B is better long-term
**Pragmatist:** Recommends B - worth the extra day, saves weeks later
```

#### Comparison Table

| Criterion | Option A | Option B | Winner |
|-----------|----------|----------|--------|
| Simplicity | +/~/- | +/~/- | A/B |
| Maintainability | ... | ... | ... |
| Security | ... | ... | ... |
| Time to implement | ... | ... | ... |
| Risk | ... | ... | ... |

#### Final Recommendation

```
**Recommendation:** Option X

**Reason:** {1-2 sentences - the deciding factor}

**Risk:** {what could go wrong}

**Action:** {specific next step}
```

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

## Output

After recommendation:

```
┌─ LETS ─────────────────────────┐
│  Document?  /lets-beads-finish │
│  Commit?    /lets-commit       │
└────────────────────────────────┘
```

## Note

This skill reads project context from CLAUDE.md automatically.
Keep decisions aligned with project constraints and patterns.
