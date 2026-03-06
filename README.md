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
| `/lets:done` | Finish task - create PR (github mode) or merge locally |
| `/lets:check` | Quick inline sanity check (5-perspective review) |
| `/lets:review` | Full code review (up to 11 specialized agents, ~2-3 min) |
| `/lets:opinion` | Technical decision analysis (3-5 expert agents in parallel) |
| `/lets:ask` | Quick expert consultation (single agent) |
| `/lets:brainstorm` | Interactive backlog helper - review tasks, priorities, patterns |
| `/lets:plan` | Structured planning - explore codebase, design architecture, write plan |
| `/lets:execute` | Execute plan from `/lets:plan` via native plan mode |
| `/lets:pr` | PR review lifecycle - analyze, discuss, post inline, follow-up, respond, approve |
| `/lets:worktree` | Create/manage worktrees for parallel sessions |
| `/lets:team` | Parallel implementation with Agent Teams (run, status, stop) |
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
| Review an implementation plan | `/lets:review --plan` | ~2-3 min |
| Review a GitHub PR | `/lets:review <PR-url>` | ~2-3 min |
| Full PR lifecycle | `/lets:pr <PR>` | Analyze, discuss, post, follow-up, approve |

### Planning

For medium and large tasks, LETS provides structured planning:

| Situation | Approach |
|-----------|----------|
| Clear goal ("Add X to Y") | `/lets:plan` - explore codebase, design architecture, write plan |
| Unclear goal ("Improve Z") | `/lets:brainstorm` to clarify, then `/lets:plan` |

`/lets:brainstorm` helps think through what to build (backlog review, priorities, task creation). `/lets:plan` designs how to build it (codebase exploration, architecture, implementation plan). `/lets:execute` implements the plan via native plan mode with approval gates.

### Parallel Sessions (Worktrees)

Work on multiple tasks simultaneously in separate terminals:

```bash
# Terminal 1 (main repo)
/lets:worktree create auth-feature

# Terminal 2
cd .worktrees/auth-feature
claude
/lets:start   # picks task, uses worktree branch as-is
# ... work, commit, done ...

# Terminal 1 - cleanup
/lets:worktree remove auth-feature
```

Each worktree gets its own branch (`worktree-<name>`), shares the task database and config via symlinks. Sessions run independently with per-branch tracking.

### Parallel Implementation (Teams)

For multiple independent tasks, `/lets:team` spawns parallel agents in isolated worktrees:

```
/lets:plan -> /lets:team run -> monitor & approve plans -> /lets:review --local -> /lets:done
```

Each teammate gets one task, works in an isolated worktree with plan approval from the lead, and commits are auto cherry-picked back to the current branch.

## Expert Agents

13 specialized agents for code review, exploration, implementation, and technical analysis:

| Agent | Expertise |
|-------|-----------|
| architect | System design, patterns, SOLID principles |
| backend | APIs, business logic, error handling |
| frontend | UI components, state management, accessibility |
| security | Vulnerabilities, auth, crypto, input validation |
| database | Schema design, migrations, query optimization |
| devops | Docker, CI/CD, deployment, infrastructure |
| qa | Test strategy, coverage, assertions, mocking |
| compliance | Project standards and coding conventions |
| docs | API docs, README, inline documentation |
| pragmatist | ROI analysis, overengineering detection |
| git-historian | Blame analysis, change patterns, refactoring impact |
| explorer | Codebase structure mapping, pattern identification, integration points |
| implementer | Full-stack implementation in isolated worktrees for `/lets:team` |

Agents are read-only - they analyze code but never modify it. Exception: the implementer agent has Edit/Write/Bash for parallel implementation in isolated worktrees via `/lets:team`. Commands decide which agents to launch based on the type of changes being reviewed.

`/lets:check` reviews inline (no subagent) for fast feedback.

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
.lets/execution/    PR review state and team records
.lets/cache/        Usage stats and cached data
```

Interactive worktrees are stored in `.worktrees/` (gitignored). Each gets a `.lets/` symlink for shared state.

## Dependencies

| Plugin | Required | Purpose |
|--------|----------|---------|
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking and issue management |

## License

MIT
