# lets-plugin-claude

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

```
.claude-plugin/plugin.json   # Plugin manifest
commands/                     # 10 slash commands (/lets:start, /lets:review, /lets:ask, etc.)
agents/                       # 12 agents (11 experts + quick-reviewer for /lets:check)
hooks/                        # SessionStart hook - injects workflow rules
reference/                    # Reference plugins for studying patterns (gitignored)
```

## Key Concepts

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = experts dispatched by commands. `/lets:review`, `/lets:opinion`, `/lets:ask` use specialized experts. `/lets:check` uses `quick-reviewer` (single multi-perspective agent)
- **Hook** = injects workflow rules into every conversation via SessionStart
- **No skills/** - plugin is workflow-focused, not knowledge-focused

## Architecture Decisions

- Agents define WHO (expertise, scoring, output format). Commands define WHAT to do (provide diff, context)
- `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:check` use `subagent_type: "lets:agent-name"` to dispatch agents via Task tool
- All agents are read-only (Read, Grep, Glob, optionally Bash). No Edit/Write.
- SessionStart hook injects rules from `hooks/rules-context.md`

## Dependencies

- beads plugin (task tracking)
- superpowers plugin (recommended - brainstorming, TDD, debugging)

## When Adding/Modifying Commands

Update these files:

| File | What to update |
|------|----------------|
| `hooks/rules-context.md` | Skill Quick Reference table |
| `commands/install.md` | Essential Skills / Planning Skills tables |

### Command Output Requirements

Every lets:* command MUST end with branded LETS box:

```
┌─ LETS ─────────────────┐
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
- **ONLY `/lets:*` commands** - never raw commands like `bd sync`, `bd update`
- **Exception:** `git push` allowed after `/lets:commit`
- **No command = no box** - if next step isn't a /lets:* command, just ask in plain text

### Command Checklist

- [ ] Has LETS box in output section
- [ ] Updates Skill Quick Reference in `hooks/rules-context.md`
- [ ] Updates `/lets:install` tables
- [ ] Follows session flow (start -> work -> finish -> commit -> end)
- [ ] Description is clear and actionable
