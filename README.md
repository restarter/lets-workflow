<div align="center">

# 🌱 LETS Workflow

**A development workflow plugin for Claude Code**

*Stop babysitting your AI. Start shipping with it.*

[![Claude Code](https://img.shields.io/badge/Claude_Code-plugin-blueviolet)](https://claude.com/claude-code)
[![Version](https://img.shields.io/badge/version-0.2.4-blue)](CHANGELOG.md)

</div>

---

Claude Code is powerful, but without structure it drifts - forgets context between sessions, commits without linking to tasks, reviews its own code with no outside perspective, and loses track of what was decided and why.

LETS fixes that. It gives you a team of 13 expert agents - architect, security engineer, QA, backend, devops and others - that automatically join your workflow when needed. Code review? The right experts are selected based on what you changed. Technical decision? Agents debate the trade-offs and give you a recommendation. Planning? They explore the codebase before you write a line of code.

On top of that - a disciplined workflow where every session has a task, every commit links to it, and context survives across sessions and conversation compaction.

Built for teams. Task tracking via [beads](https://github.com/steveyegge/beads) syncs across developers through [Dolt](https://github.com/dolthub/dolt) remotes - everyone sees the same tasks, dependencies, and progress. Multiple developers work on the same codebase with shared context, not duplicated effort.

**What you get:**
- **Session continuity** - context restored automatically, even after compaction
- **Task-driven development** - no random work, everything tracked via [beads](https://github.com/steveyegge/beads)
- **Team sync** - shared task database via Dolt remotes, multiple developers see the same backlog in real time
- **Expert review panel** - up to 11 specialized agents (security, architecture, backend, QA...) review your code
- **Parallel execution** - multiple agents working on different tasks simultaneously via worktrees
- **Structured planning** - agent-powered codebase exploration and architecture design before coding
- **One-command workflow** - Claude suggests the next step, you decide

<div align="center">

<img src="docs/images/review.png" alt="LETS Review" width="600">

*LETS statusline and `/lets:review` command in action*

</div>

## Quick Start

```bash
# 1. Add plugin to Claude Code
git clone https://github.com/restarter/lets-workflow ~/.claude/plugins/lets

# 2. First-time global setup (installs beads plugin, verifies environment)
/lets:install

# 3. Initialize current project (creates .lets/ structure, config, statusline)
/lets:init
```

Then start working:

```bash
/lets:start          # Restore context, pick a task, create feature branch
# ... write code ...
/lets:check          # Quick sanity check before commit
/lets:commit         # Review + conventional commit linked to task
/lets:done           # Create PR (or merge locally) and close task
/lets:end            # Save session summary for next time
```

## How It Works

### Workflow

```
/lets:start ─── work ─── /lets:commit ─── /lets:done ─── /lets:end
                 │
                 ├── /lets:check       Quick sanity check (5 perspectives, ~30s)
                 ├── /lets:review      Full multi-agent code review (~2-3 min)
                 ├── /lets:opinion     Technical decision with 3-5 expert agents
                 ├── /lets:ask         Quick question to a single expert
                 ├── /lets:plan        Codebase exploration + architecture design
                 └── /lets:brainstorm  Backlog review, priorities, task creation
```

**Start** - `/lets:start` restores context from the previous session, shows available tasks, and creates a feature branch. Context survives conversation compaction via beads task comments.

**Work** - Write code. When you need help along the way:
- Hit a technical decision? `/lets:opinion` launches 3-5 expert agents in parallel and returns a recommendation with trade-offs.
- Need a quick answer? `/lets:ask security "Is this auth flow safe?"` pings one expert.
- Task feels big? `/lets:plan` explores the codebase and designs an architecture before you write a line of code.

**Review** - `/lets:check` for a quick 30-second inline scan, or `/lets:review --local` for a full multi-agent deep review with dynamic expert selection.

**Commit** - `/lets:commit` reviews changes and creates a conventional commit (`feat:`, `fix:`, `refactor:`) linked to the active task.

**Finish** - `/lets:done` creates a PR on GitHub (or merges locally). `/lets:end` saves a session summary so the next conversation picks up where you left off.

### LETS Boxes

After key actions, Claude shows contextual next-step suggestions - you always know what to do next:

```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

### SessionStart Hook

A hook injects workflow rules into every Claude Code conversation - development practices, git conventions, session flow, discovery logging. This is what makes Claude follow the LETS workflow without you having to remind it.

## Commands

### Session & Task

| Command | Description |
|---------|-------------|
| `/lets:start` | Start session - restore context, show tasks, create feature branch |
| `/lets:end` | End session - save progress, sync tasks, create summary |
| `/lets:commit` | Commit with review and conventional commit format |
| `/lets:done` | Finish task - create PR (GitHub mode) or merge locally |
| `/lets:status` | Task overview and project status |
| `/lets:note` | Add note to active task |

### Review & Analysis

| Command | Description |
|---------|-------------|
| `/lets:check` | Quick inline sanity check (~30s, 5-perspective review) |
| `/lets:review` | Full code review with dynamic agent selection (~2-3 min) |
| `/lets:pr` | PR review lifecycle - analyze, discuss, post inline, follow-up, approve |
| `/lets:opinion` | Technical decision analysis (3-5 expert agents in parallel) |
| `/lets:ask` | Quick expert consultation (single agent) |

### Planning & Execution

| Command | Description |
|---------|-------------|
| `/lets:brainstorm` | Interactive backlog helper - review tasks, priorities, patterns |
| `/lets:plan` | Structured planning - explore codebase, design architecture, write plan |
| `/lets:execute` | Execute plan from `/lets:plan` via native plan mode |
| `/lets:worktree` | Create/manage worktrees for parallel sessions |
| `/lets:team` | Parallel implementation with Agent Teams |

### Setup

| Command | Description |
|---------|-------------|
| `/lets:install` | First-time global setup - plugins, environment, workflow |
| `/lets:init` | Initialize LETS in current project |

## Review Options

| Need | Command | Time |
|------|---------|------|
| Quick pre-commit check | `/lets:check` | ~30s |
| Full review of local changes | `/lets:review --local` | ~2-3 min |
| Review an implementation plan | `/lets:review --plan` | ~2-3 min |
| Review a GitHub PR | `/lets:review <PR-url>` | ~2-3 min |
| Full PR lifecycle | `/lets:pr <PR>` | Interactive |
| Review a single file | `/lets:review --file <path>` | ~2-3 min |

## Planning

For medium and large tasks, LETS provides structured planning:

| Situation | Approach |
|-----------|----------|
| Clear goal ("Add X to Y") | `/lets:plan` - explore codebase, design architecture, write plan |
| Unclear goal ("Improve Z") | `/lets:brainstorm` to clarify, then `/lets:plan` |

> **Think** → **Design** → **Build**
>
> `/lets:brainstorm` helps think through *what* to build.
> `/lets:plan` designs *how* to build it.
> `/lets:execute` implements the plan with approval gates.

## Parallel Work

### Worktrees (Interactive)

Work on multiple tasks simultaneously in separate terminals:

```bash
# Terminal 1 (main repo)
/lets:worktree create auth-feature

# Terminal 2
cd .worktrees/auth-feature && claude
/lets:start   # picks task, uses worktree branch as-is
# ... work, commit, done ...

# Terminal 1 - cleanup
/lets:worktree remove auth-feature
```

Each worktree gets its own branch, shares the task database and config via symlinks.

### Teams (Autonomous)

For multiple independent tasks, `/lets:team` spawns parallel agents in isolated worktrees:

```
/lets:plan -> /lets:team run -> monitor & approve plans -> /lets:review --local -> /lets:done
```

Each teammate gets one task, works in isolation with plan approval from the lead, and commits are auto cherry-picked back.

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
| explorer | Codebase structure mapping, pattern identification |
| implementer | Full-stack implementation in isolated worktrees |

Agents are read-only - they analyze code but never modify it. Exception: the implementer agent has write access for parallel implementation via `/lets:team`.

Commands decide which agents to launch based on the type of changes being reviewed. `/lets:check` reviews inline (no subagent) for fast feedback.

## Task Integration

LETS integrates with [beads](https://github.com/steveyegge/beads) for persistent task tracking:

- Every session starts by selecting a task
- Every commit is linked to the active task
- Session summaries and discovery notes are saved to the task
- Context survives conversation compaction and new sessions
- Task dependencies and blocking relationships are tracked

## Configuration

After `/lets:init`, edit `.lets/config.yaml`:

```yaml
language: English       # Response language (English/Ukrainian/Italian/etc)
merge-branch: main      # Target branch for merges and PR base
github: true            # true: PR on done, false: local merge
```

## File Storage

All generated files go to `.lets/` (gitignored):

```
.lets/config.yaml       Project settings
.lets/sessions/         Session summaries and start references
.lets/reviews/          Saved review reports
.lets/plans/            Implementation plans
.lets/execution/        PR review state and team records
.lets/cache/            Usage stats and cached data
```

Interactive worktrees are stored in `.worktrees/` (gitignored).

## Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| [Claude Code](https://claude.com/claude-code) | Yes | AI coding agent (the thing LETS plugs into) |
| [git](https://git-scm.com/) | Yes | Version control, branching, worktrees |
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking and issue management (Claude Code plugin) |
| [gh](https://cli.github.com/) | Optional | GitHub PR workflow (when `github: true`) |
| [jq](https://jqlang.github.io/jq/) | Optional | Statusline JSON parsing |

## License

MIT
