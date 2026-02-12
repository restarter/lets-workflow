# lets - Claude Code Plugin

Development workflow plugin with session management, code review, and task tracking.

## What it does

- **Session management** - start/end sessions with context persistence across conversations
- **Code review** - multi-agent review with dynamic expert selection and confidence scoring
- **Task tracking** - beads integration for issue management across sessions
- **Technical decisions** - structured analysis from multiple expert perspectives
- **Git workflow** - conventional commits with review gates

## Commands

| Command | Description |
|---------|-------------|
| `/lets:start` | Start session - restore context, show tasks, select work item |
| `/lets:end` | End session - check state, save summary, handle cleanup |
| `/lets:commit` | Commit with review and conventional commit format |
| `/lets:check` | Quick sanity check (~30s) - 4 perspectives, single pass |
| `/lets:review` | Full code review (~2-3 min) - up to 11 specialized agents |
| `/lets:opinion` | Technical decision analysis from 5 expert perspectives |
| `/lets:install` | First-time setup guide |
| `/lets:beads-finish` | Document completed work in beads |
| `/lets:beads-status` | Full task report with priorities and blockers |

## Review Agents

The review system uses 11 specialized agents that activate based on change types:

| Agent | Focus |
|-------|-------|
| compliance-expert | CLAUDE.md rules and project conventions |
| backend-expert | API design, business logic, bug detection |
| security-expert | OWASP, auth, secrets, injection |
| architect | SOLID, patterns, coupling, abstractions |
| git-historian | Blame analysis, regression risk |
| docs-expert | Documentation sync and completeness |
| devops-expert | Docker, CI/CD, shell scripts |
| database-expert | Schema, migrations, query performance |
| frontend-expert | Components, state, accessibility |
| qa-expert | Test strategy, coverage, assertions |
| pragmatist | Overengineering, ROI, scope creep |

## Session Flow

```
/lets:start -> Pick task -> Work -> /lets:beads-finish -> /lets:commit -> /lets:end
```

## Dependencies

- [beads](https://github.com/steveyegge/beads) - task tracking plugin
- [superpowers](https://github.com/obra/superpowers) - development workflow plugin (recommended)

## Setup

Run `/lets:install` for guided first-time setup.

## License

MIT
