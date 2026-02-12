# lets-plugin-claude

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

```
.claude-plugin/plugin.json   # Plugin manifest
commands/                     # 9 slash commands (/lets:start, /lets:review, etc.)
agents/                       # 11 review experts (architect, security-expert, etc.)
hooks/                        # SessionStart hook - injects workflow rules
reference/                    # Reference plugins for studying patterns (gitignored)
```

## Key Concepts

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = specialized review experts dispatched by `/lets:review`
- **Hook** = injects workflow rules into every conversation via SessionStart
- **No skills/** - plugin is workflow-focused, not knowledge-focused

## Architecture Decisions

- Agents define WHO (expertise, scoring, output format). Commands define WHAT to do (provide diff, context)
- `/lets:review` uses `subagent_type: "lets:agent-name"` to dispatch agents via Task tool
- All agents are read-only (Read, Grep, Glob, optionally Bash). No Edit/Write.
- SessionStart hook injects rules from `hooks/rules-context.md`

## Dependencies

- beads plugin (task tracking)
- superpowers plugin (recommended - brainstorming, TDD, debugging)
