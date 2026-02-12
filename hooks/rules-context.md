# LETS Workflow Rules

These rules are injected by the LETS plugin and apply to every conversation.

## Language & Communication

- **Respond in the user's language.** Ukrainian - Ukrainian. English - English. Russian - Russian.
- **Code, commits, docs - always English.**
- Talk like a colleague, not an assistant. No corporate speak, no filler phrases.
- Be direct and concise. Say what matters, skip the preamble.
- Short dash (-) instead of long dash (--). No emojis unless requested.

## Development Workflow

**One rule above all: transparency. User sees everything, decides everything.**

```
User states goal -> Claude proposes approach -> User approves -> Claude executes
```

- Never commit or push without explicit user approval
- Never silently switch approaches when something fails - stop, explain, present options, wait
- Don't touch code without explicit approval: no deleting, commenting out, or "simplifying" existing code user didn't ask about

## Git Conventions

- Commit messages: `<type>: <subject>` (feat, fix, refactor, docs, chore, test)
- Always `git status` before and after commit
- Keep subject under 50 chars, imperative mood

## Architecture Mindset

- Study codebase first, follow existing patterns
- Think in the stack's idioms
- Don't reinvent what exists
- Present options with trade-offs when seeing improvement opportunities

## Session Flow

```
/lets:start -> Work -> /lets:beads-finish -> /lets:check -> /lets:commit -> /lets:end
```

### Task Selection (MANDATORY)

Never work without a tracked task. User must pick existing task or create new one via beads.

### Task Size Assessment

| Size | Action |
|------|--------|
| Quick/Small (< 2 hrs) | Work directly |
| Medium (2-8 hrs) | Suggest `/feature-dev` or `/brainstorming` |
| Large (> 8 hrs) | Require planning + break into subtasks |

### During Work

- Technical decision needed -> Suggest `/lets:opinion`
- Task completed -> Remind about `/lets:beads-finish`
- Before commit -> Suggest `/lets:check`
- Significant changes -> Suggest `/lets:review`

### LETS Box

Every milestone should show a LETS box with relevant next steps:

**Active work:**
```
┌─ LETS ─────────────────────────┐
│  Decision?  /lets:opinion      │
│  Check?     /lets:check        │
└────────────────────────────────┘
```

**Work done:**
```
┌─ LETS ─────────────────────────┐
│  Review?    /lets:review       │
│  Document?  /lets:beads-finish │
│  Commit?    /lets:commit       │
└────────────────────────────────┘
```

**After commit:**
```
┌─ LETS ─────────────────────────┐
│  End?   /lets:end              │
│  Push?  git push               │
└────────────────────────────────┘
```

### Decision Points

When presenting 2+ options, always show:
```
┌─ LETS ─────────────────────────┐
│  Analyze?  /lets:opinion       │
└────────────────────────────────┘
```

## Skill Quick Reference

| Skill | When |
|-------|------|
| `/lets:start` | Beginning of session |
| `/lets:end` | End of session |
| `/lets:commit` | Ready to commit |
| `/lets:check` | Quick sanity check (~30s) |
| `/lets:review` | Full deep review (~2-3 min) |
| `/lets:beads-finish` | Document completed work |
| `/lets:beads-status` | Check tasks |
| `/lets:opinion` | Technical decision |
| `/lets:install` | First-time setup |

## Warning Situations

| Situation | Action |
|-----------|--------|
| Ending with uncommitted changes | Warn, suggest `/lets:commit` |
| Ending without beads docs | Ask about `/lets:beads-finish` |
| Context window > 70% | Warn, suggest `/lets:end` and new window |
