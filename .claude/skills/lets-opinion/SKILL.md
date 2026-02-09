---
name: lets-opinion
description: Technical decision analysis from 5 expert perspectives with clear recommendation
---

# Technical Decision Analysis

Analyze technical decisions from multiple expert perspectives and give a clear recommendation.

## Instructions

**DO NOT ask the user which option to choose.** Analyze and recommend.

### Step 1: Frame the Problem

```
**Decision:** {what needs to be decided}
**Options:** A) ... B) ... C) ...
**Constraints:** {time, compatibility, legacy, budget}
```

### Step 2: Quick Verdict (TL;DR)

Start with the conclusion:

```
**Recommendation:** Option X
**One-liner:** {why in 10 words or less}
```

### Step 3: Expert Panel

Get opinions from 5 experts (2-3 sentences each):

#### [DevOps] DevOps Engineer
- Deployment, monitoring, configuration
- Docker/container practices
- Environment parity

#### [Arch] Software Architect
- Maintainability, coupling, patterns
- Single source of truth
- Future extensibility

#### [Dev] Senior Developer
- Code clarity, conventions
- Debugging experience
- Onboarding ease

#### [Sec] Security Engineer
- Attack surface
- Data protection
- Audit trail

#### [Biz] Pragmatist
- Time-to-market
- ROI vs effort
- "Good enough" vs perfect

### Step 4: Comparison Table

| Criterion | Option A | Option B | Winner |
|-----------|----------|----------|--------|
| Simplicity | +/~/- | +/~/- | A/B |
| Maintainability | ... | ... | ... |
| Security | ... | ... | ... |
| Time to implement | ... | ... | ... |
| Risk | ... | ... | ... |

### Step 5: Final Recommendation

```
**Recommendation:** Option X

**Reason:** [1-2 sentences - the deciding factor]

**Risk:** [what could go wrong]

**Action:** [specific next step]
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
┌─ LETS ───────────────────────┐
│  Continue?  /lets-commit     │
│  Document?  /lets-beads-finish│
└──────────────────────────────┘
```

## Note

This skill reads project context from CLAUDE.md automatically.
Keep decisions aligned with project constraints and patterns.
