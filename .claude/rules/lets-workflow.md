# Lets Workflow - AI Behavior Rules

Rules for Claude to follow during development sessions. These are automatic - no skill invocation needed.

---

## Session Flow

```
/lets-start → Work → /lets-beads-finish → [/lets-check] → [/lets-review] → /lets-commit → Push → PR
```

**Review options:**
- `/lets-check` - quick sanity check (~30 sec), before any commit
- `/lets-review` - full deep review (~2-3 min), works locally OR on GitHub PR

**When to use which:**
- Small change → `/lets-check` → commit
- Significant change → `/lets-check` → `/lets-review --local` → fix → commit → PR
- PR already exists → `/lets-review <PR>` → comment on PR

---

## AI Behavior Rules

### Session Start Triggers

When conversation starts or user wants to begin working:
- Suggest `/lets-start` to restore context and pick a task

### Task Selection (MANDATORY)

**Never work without a tracked task.** When user wants to start working:
- Show available tasks (`bd ready`)
- User MUST either pick existing task or create new one
- No "free work" - everything goes through beads

### Task Size Assessment

When user describes what they want to do, assess complexity:

| Size | Indicators | Action |
|------|------------|--------|
| Quick fix | Single file, obvious change, < 30 min | Work directly |
| Small | Few files, clear scope, < 2 hrs | Work directly |
| Medium | Multiple files, needs research, 2-8 hrs | Suggest `/feature-dev` or `/brainstorming` |
| Large | Architecture change, many files, > 8 hrs | Require planning + subtasks |

### Choosing Planning Skill

| Situation | Use | Why |
|-----------|-----|-----|
| Clear goal, need implementation plan | `/feature-dev` | Structured: explore codebase → clarify → architect → implement |
| Unclear goal, need to figure out what to build | `/brainstorming` | Asks questions, explores options, helps define the goal |
| "Add X feature to Y" | `/feature-dev` | Goal is clear |
| "I want to improve Z" / "Not sure how to approach this" | `/brainstorming` | Goal needs clarification |

**Quick test:** Can you write a 1-sentence requirement?
- YES → `/feature-dev`
- NO → `/brainstorming` first

### During Work Triggers

- **Technical decision needed** → Suggest `/lets-opinion`
- **Task completed** → Remind about `/lets-beads-finish`
- **Multiple files changed** → Periodic reminder about committing
- **Before commit** → Suggest `/lets-check` for quick sanity check
- **Significant changes** → Suggest `/lets-review` for full deep review (local or PR)
- **Long conversation** → Suggest checking `/context`

### Next Step Suggestions (CRITICAL)

### Work Phases & LETS Boxes

| Phase | When AI detects | LETS Box |
|-------|-----------------|----------|
| **Active work** | Code being written, research ongoing | `opinion` + `check` |
| **Work done** | Feature/fix complete, ready for review | `review` + `beads-finish` + `commit` |
| **After commit** | Commit successful | `end` + `push` |

**Active work phase:**
```
┌─ LETS ─────────────────────────┐
│  Decision?  /lets-opinion      │
│  Check?     /lets-check        │
└────────────────────────────────┘
```

**Work done phase:**
```
┌─ LETS ─────────────────────────┐
│  Review?    /lets-review       │
│  Document?  /lets-beads-finish │
│  Commit?    /lets-commit       │
└────────────────────────────────┘
```

**After commit:**
```
┌─ LETS ─────────────────────────┐
│  End?   /lets-end              │
│  Push?  git push               │
└────────────────────────────────┘
```

### Phase Detection

| Phase | Trigger | LETS box |
|-------|---------|----------|
| **Active work** | AI just edited files | `check` |
| **Work done** | User asks to commit/review | `commit` + `review` |
| **After commit** | Commit succeeded | `end` + `push` |
| **Decision point** | AI presents options | `opinion` |

**Rule:** If AI made changes → always suggest `/lets-check` first.

### Decision Points (CRITICAL)

**When AI presents options - ALWAYS show LETS box:**

```
┌─ LETS ─────────────────────────┐
│  Analyze?  /lets-opinion       │
└────────────────────────────────┘
```

This applies when:
- Presenting 2+ implementation approaches
- Asking user to choose between solutions
- Trade-off decisions (speed vs quality, simple vs flexible)
- Architecture choices

**Pattern:**
```
[Present options table]

Which approach?

┌─ LETS ─────────────────────────┐
│  Analyze?  /lets-opinion       │
└────────────────────────────────┘
```

User can either:
- Pick directly: "A" / "option A" / describe choice
- Request analysis: `/lets-opinion` for 5-expert deep dive

### Commit Triggers

When user wants to commit:
- **ALWAYS** use `/lets-commit` skill
- Never commit directly without the skill

### Session End Triggers

When user wants to end session:
1. Check uncommitted changes → suggest `/lets-commit`
2. Check beads documentation → suggest `/lets-beads-finish`
3. Suggest `/lets-end` to close properly

### Warning Situations

| Situation | AI Action |
|-----------|-----------|
| User ending with uncommitted changes | Warn: "You have uncommitted changes. Commit first?" |
| User ending without documenting in beads | Ask: "Did you document this work in beads? Run `/lets-beads-finish`" |
| Task in progress but no recent commits | Remind: "Consider committing progress with `/lets-commit`" |
| Context window > 70% | Warn: "Context getting full. Consider `/lets-end` and new window" |

---

## Skill Quick Reference

| Skill | When to use |
|-------|-------------|
| `/lets-start` | Beginning of session |
| `/lets-end` | End of session |
| `/lets-commit` | Ready to commit changes |
| `/lets-check` | Quick sanity check before commit (~30 sec) |
| `/lets-review` | Full deep review - local OR GitHub PR (~2-3 min) |
| `/lets-beads-finish` | Task done, need to document |
| `/lets-beads-status` | Check tasks anytime |
| `/lets-ask` | Quick expert consultation (1 agent) |
| `/lets-opinion` | Technical decision needed (3-5 agents) |
| `/lets-install` | First-time setup |
| `/feature-dev` | Clear goal, need implementation plan |
| `/brainstorming` | Unclear goal, need to figure out what to build |

---

## Key Principles

1. **Every session has a task** - no random work without tracking
2. **Big tasks need planning** - use `/feature-dev` for research/design
3. **Document everything** - beads is the source of truth
4. **Skills guide the flow** - each skill prompts next step
5. **Context windows matter** - split sessions when needed
6. **Always suggest next step** - never end response without "Next: /skill-name"

---

## When Adding/Modifying Skills

**IMPORTANT:** When creating or updating lets-* skills, update these files:

| File | What to update |
|------|----------------|
| `.claude/rules/lets-workflow.md` | Skill Quick Reference table |
| `.claude/skills/lets-install/SKILL.md` | Essential Skills / Planning Skills tables |

### Skill Output Requirements

Every lets-* skill MUST end with branded LETS box:

```
┌─ LETS ─────────────────┐
│  [action]? [command]   │
│  [action]? [command]   │
└────────────────────────┘
```

**Box format:**
- Header: `┌─ LETS ─` + padding with `─` + `┐`
- Lines: `│  ` + content + padding + ` │`
- Footer: `└─` + padding with `─` + `┘`
- Min width: 25 chars

**Content guidelines:**
- Short action word + `?` (e.g., "Commit?", "Next?", "Fix?")
- **ONLY `/lets-*` commands** - never raw commands like `bd sync`, `bd update`
- **Exception:** `git push` allowed after `/lets-commit` (logical next step)
- **No skill = no box** - if next step isn't a /lets-* command, just ask in plain text

❌ BAD (no skill available):
```
┌─ LETS ─────────────────┐
│  bd update yii2-ch5    │
└────────────────────────┘
```

✅ GOOD:
```
Work on this now?
```

**Example - active work phase:**
```
┌─ LETS ─────────────────────────┐
│  Decision?  /lets-opinion      │
│  Check?     /lets-check        │
└────────────────────────────────┘
```

**Example - work done phase:**
```
┌─ LETS ─────────────────────────┐
│  Review?    /lets-review       │
│  Document?  /lets-beads-finish │
│  Commit?    /lets-commit       │
└────────────────────────────────┘
```

### Skill Checklist

- [ ] Has LETS box in output section
- [ ] Updates Skill Quick Reference table
- [ ] Updates lets-install Essential Skills table
- [ ] Follows session flow (start → work → finish → commit → end)
- [ ] Description is clear and actionable

---

## Context Window Management

**Check tokens:** `/context`

| Usage | Action |
|-------|--------|
| < 50% | Safe to continue |
| 50-70% | Proceed with caution |
| > 70% | Split session NOW |

**How to split:**
1. Save plan to `docs/plans/YYYY-MM-DD-<task-name>.md`
2. Run `/lets-end`
3. Start new Claude window
4. Run `/lets-start` - continue with plan file
