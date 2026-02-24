# lets - Claude Code Plugin

Development workflow plugin with session management, code review, and task tracking.

## What it does

- **Session management** - start/end sessions with context persistence across conversations
- **Code review** - multi-agent review with dynamic expert selection and confidence scoring
- **Task tracking** - beads integration for issue management across sessions
- **Technical decisions** - structured analysis from multiple expert perspectives
- **Git workflow** - conventional commits with review gates

## Commands

| Command | Category | Description |
|---------|----------|-------------|
| `/lets:start` | Session | Start session - restore context, show tasks, select work item |
| `/lets:end` | Session | End session - save progress, sync beads, create summary |
| `/lets:done` | Task | Finish a task - document, create PR or merge, close |
| `/lets:commit` | Code | Commit with review and conventional commit format |
| `/lets:check` | Code | Quick sanity check (~30s) - 5 perspectives, single agent |
| `/lets:review` | Code | Full code review (~2-3 min) - up to 11 specialized agents |
| `/lets:opinion` | Expert | Technical decision analysis from 3-5 expert perspectives |
| `/lets:ask` | Expert | Quick expert consultation - single agent |
| `/lets:status` | Utility | Task overview and project status report |
| `/lets:note` | Utility | Add note to active task |
| `/lets:install` | Setup | First-time setup guide |
| `/lets:migrate` | Setup | One-time storage migration to .lets/ |

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
/lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done -> /lets:end
```

## Dependencies

- [beads](https://github.com/steveyegge/beads) - task tracking plugin
- [superpowers](https://github.com/obra/superpowers) - development workflow plugin (recommended)

## Setup

Run `/lets:install` for guided first-time setup.

## License

MIT
