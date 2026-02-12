# lets-plugin-claude

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

```
.claude-plugin/plugin.json   # Plugin manifest
commands/                     # 10 slash commands (/lets:start, /lets:review, /lets:ask, etc.)
agents/                       # 11 universal experts (architect, security-expert, etc.)
hooks/                        # SessionStart hook - injects workflow rules
reference/                    # Reference plugins for studying patterns (gitignored)
```

## Key Concepts

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = universal experts dispatched by `/lets:review`, `/lets:opinion`, `/lets:ask`. `/lets:check` uses general-purpose agent (not lets: experts)
- **Hook** = injects workflow rules into every conversation via SessionStart
- **No skills/** - plugin is workflow-focused, not knowledge-focused

## Architecture Decisions

- Agents define WHO (expertise, scoring, output format). Commands define WHAT to do (provide diff, context)
- `/lets:review`, `/lets:opinion`, `/lets:ask` use `subagent_type: "lets:agent-name"` to dispatch agents via Task tool
- All agents are read-only (Read, Grep, Glob, optionally Bash). No Edit/Write.
- SessionStart hook injects rules from `hooks/rules-context.md`

## Dependencies

- beads plugin (task tracking)
- superpowers plugin (recommended - brainstorming, TDD, debugging)
