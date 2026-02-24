# lets - Development Workflow Plugin for Claude Code

Structured development workflow with session management, expert code review, and task tracking for Claude Code.

LETS turns Claude Code into a disciplined development partner. Every session has a task. Every commit links to a task. Every review is done by specialized experts. Claude suggests the next step - you decide.

## Quick Start

```bash
# Clone into Claude Code plugins directory
git clone https://github.com/nickolay-umbo/lets-plugin-claude ~/.claude/plugins/lets

# Run setup (inside Claude Code)
/lets:install
```

`/lets:install` checks dependencies and walks you through initial configuration.

## Commands

| Command | Description |
|---------|-------------|
| `/lets:start` | Start session - restore context, show tasks, select work item |
| `/lets:end` | End session - save progress, sync tasks, create summary |
| `/lets:commit` | Commit with review and conventional commit format |
| `/lets:done` | Finish task - create PR or merge, close task |
| `/lets:check` | Quick sanity check (~30s, single multi-perspective agent) |
| `/lets:review` | Full code review (up to 11 specialized agents, ~2-3 min) |
| `/lets:opinion` | Technical decision analysis (3-5 expert agents in parallel) |
| `/lets:ask` | Quick expert consultation (single agent) |
| `/lets:brainstorm` | Explore ideas and requirements before implementation |
| `/lets:status` | Task overview and project status |
| `/lets:note` | Add note to active task |
| `/lets:install` | First-time setup and dependency check |

## Workflow

```
/lets:start -> work -> /lets:check -> /lets:commit -> /lets:done -> /lets:end
```

**Start a session** - `/lets:start` restores context from the previous session, shows available tasks, and creates a feature branch.

**Work** - Write code. Use `/lets:opinion` when you face a technical decision - it launches 3-5 expert agents in parallel and returns an aggregated recommendation.

**Review before commit** - `/lets:check` for a quick 30-second sanity check, or `/lets:review --local` for a full multi-agent review.

**Commit** - `/lets:commit` reviews your changes and creates a conventional commit linked to the active task.

**Finish** - `/lets:done` creates a PR (or merges locally) and closes the task. `/lets:end` saves a session summary for the next conversation.

### Review Options

| Need | Command | Time |
|------|---------|------|
| Quick pre-commit check | `/lets:check` | ~30s |
| Full review of local changes | `/lets:review --local` | ~2-3 min |
| Review a GitHub PR | `/lets:review <PR-url>` | ~2-3 min |

### Planning

For medium and large tasks, LETS provides structured planning:

| Situation | Approach |
|-----------|----------|
| Clear goal ("Add X to Y") | `/lets:brainstorm` - define requirements, then execute plan |
| Unclear goal ("Improve Z") | `/lets:brainstorm` - explore options and constraints first |

`/lets:brainstorm` helps clarify what needs to be built before writing code. Once the plan is ready, LETS guides execution step by step with review checkpoints.

## Expert Agents

12 specialized agents for code review and technical analysis:

| Agent | Expertise |
|-------|-----------|
| architect | System design, patterns, SOLID principles |
| backend-expert | APIs, business logic, error handling |
| frontend-expert | UI components, state management, accessibility |
| security-expert | Vulnerabilities, auth, crypto, input validation |
| database-expert | Schema design, migrations, query optimization |
| devops-expert | Docker, CI/CD, deployment, infrastructure |
| qa-expert | Test strategy, coverage, assertions, mocking |
| compliance-expert | Project standards and coding conventions |
| docs-expert | API docs, README, inline documentation |
| pragmatist | ROI analysis, overengineering detection |
| git-historian | Blame analysis, change patterns, refactoring impact |
| quick-reviewer | Fast 5-perspective review (used by `/lets:check`) |

Agents are read-only - they analyze code but never modify it. Commands decide which agents to launch based on the type of changes being reviewed.

## How It Works

### SessionStart Hook

A hook injects workflow rules into every Claude Code conversation - development practices, git conventions, session flow guidance. This is what makes Claude follow the LETS workflow automatically.

### LETS Boxes

After key actions, Claude shows contextual next-step suggestions:

```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

These boxes appear at natural transition points - after writing code, after committing, after completing a task.

### Task Integration

LETS integrates with [beads](https://github.com/steveyegge/beads) for persistent task tracking:

- Every session starts by selecting a task
- Every commit is linked to the active task
- Session summaries and notes are saved to the task
- Context survives conversation compaction and new sessions

### File Storage

All generated files go to `.lets/` (gitignored):

```
.lets/sessions/     Session summaries and start references
.lets/reviews/      Saved review reports
.lets/plans/        Implementation plans
```

## Dependencies

| Plugin | Required | Purpose |
|--------|----------|---------|
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking and issue management |

## License

MIT
