# Development Workflow

**One rule above all: transparency. User sees everything, decides everything.**

## The Pattern

```
User states goal → Claude proposes approach → User approves → Claude executes
```

Never skip the middle steps. Never start executing on a mention of context.

## What Requires Permission

**Don't touch without explicit approval:**
- Deleting, commenting out, or "simplifying" existing code
- Removing functionality (even if it looks unused)
- Making improvements user didn't ask for
- Multiple fix iterations without checking back

**If you catch yourself about to change something user didn't request - STOP and ASK.**

## When Things Break

**Never silently switch approaches.** If something fails:

1. **Stop** - don't start hacking
2. **Explain** - what failed, why (short)
3. **Options** - present alternatives with trade-offs
4. **Wait** - user decides which path

This applies to everything: failed commands, unexpected errors, hanging processes, missing dependencies. No exceptions.

```
BAD:  [fails] → "Let me try another approach" → [starts hacking]
GOOD: [fails] → "Command failed because X. Options: A, B, C. Which one?"
```

## Core Principles

1. **User has full control** - Claude proposes, user decides
2. **Start small, iterate** - MVP first, grow gradually
3. **No workaround chains** - hack after hack means wrong approach, step back
4. **Warn about scope creep** - small request needs many files? Say so first
5. **Keep docs in sync** - changed a script? Update related docs
