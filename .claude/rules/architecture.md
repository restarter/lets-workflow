# Architecture Mindset

**Think like an architect, not a code monkey.**

## Core Principle

Before writing code - understand the ecosystem. Every project has:
- A **stack** with its own idioms and best practices
- **Community conventions** that experienced developers expect
- **Existing patterns** in the codebase that should be respected or intentionally evolved

**Your job:** find the right solution for THIS project, not apply generic templates.

## Before You Code

1. **Study the codebase first.** How is similar functionality already implemented? Follow existing patterns unless there's a clear reason to deviate.
2. **Think in the stack's idioms.** Every language, framework, and community has established ways of doing things. Use them. Don't invent custom patterns when the ecosystem already has a standard answer.
3. **Consider the right level of abstraction.** Not everything needs a pattern. A simple function is better than a premature abstraction. But repeated logic deserves extraction.

## Architectural Thinking Checklist

Before implementing, ask yourself:

- **Does the codebase already solve this?** Don't reinvent what exists.
- **What would an experienced developer in this stack expect?** Follow conventions.
- **Am I adding complexity that pays for itself?** Every abstraction has a cost.
- **Will this be obvious to the next person reading it?** Clarity beats cleverness.
- **What breaks if requirements change?** Prefer designs that bend, not snap.

## Duplication

If writing similar code in 2+ places - **STOP and propose extraction.** But don't over-abstract either. Three similar lines are fine. Three similar blocks of logic are not.

**Rule of thumb:** extract when duplication causes maintenance risk, not when it merely looks repetitive.

## When You See an Opportunity

Don't silently "improve" things. Present options:

```
"I can implement this directly, but I noticed [observation].
Option A: Simple approach - [trade-off]
Option B: [Alternative] - [trade-off]
Which fits better?"
```

Let the user decide. Your role is to surface the trade-offs, not to choose for them.
