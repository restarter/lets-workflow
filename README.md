<div align="center">

# 🌱 LETS Workflow

**A development workflow plugin for Claude Code**

*Stop babysitting your AI. Start shipping with it.*

[![Claude Code](https://img.shields.io/badge/Claude_Code-plugin-blueviolet)](https://claude.com/claude-code)
[![Version](https://img.shields.io/badge/version-0.3.0-blue)](CHANGELOG.md)

</div>

---

Claude Code is powerful, but without structure it drifts - forgets context between sessions, silently changes approach when something fails, reviews its own code with no outside perspective, and loses track of what was decided and why.

**LETS fix this!** Every session has a task. Every commit links to it. Expert agents review your code and help with technical decisions. Context survives across sessions and conversation compaction.

You get a team of 13 expert agents and a structured workflow - but you stay in control. You define the task, approve the plan, review every commit. Agents don't go off on their own - they work within boundaries you set, and report back for your decision.

**What you get:**
- **Session continuity** - context restored automatically, even after compaction
- **Task-driven development** - no random work, everything tracked via [beads](https://github.com/steveyegge/beads)
- **Structured planning** - codebase exploration and architecture design before coding
- **One-command workflow** - Claude suggests the next step, you decide
- **Expert agents on demand** - architect, security, QA, backend, devops and others join automatically when needed
- **Agent Teams** - spawn autonomous agents that implement multiple tasks in parallel
- **Worktrees** - parallel interactive sessions in separate terminals, one task per worktree
- **Uses latest Claude Code features** - ultrathink for deep analysis, interactive UI for decisions, native plan mode for execution
- **Built for teams** - shared task database via [Dolt](https://github.com/dolthub/dolt) remotes, multiple developers see the same backlog in real time
- **GitHub integration** - PR creation, inline code review comments, follow-up and approval - all from the terminal

<div align="center">

<img src="docs/images/review.png" alt="LETS Review" width="600">

*LETS statusline and `/lets:review` command in action*

</div>

## 🚀 Quick Start

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

## 🔧 How It Works

### Session Lifecycle

```
/lets:start ─── choose how to work ─── /lets:commit ─── /lets:done ─── /lets:end
```

**Start** - `/lets:start` restores context from the previous session, shows available tasks, and creates a feature branch. Context survives conversation compaction via beads task comments.

**Choose how to work** - depending on the task, you pick the approach:

```
┌─ You write code yourself ─────────────────────────────────────────────────┐
│  Write code with Claude. Use helpers along the way:                      │
│  /lets:opinion   Technical decision with 3-5 expert agents               │
│  /lets:ask       Quick question to a single expert                       │
│  /lets:check     Quick sanity check (5 perspectives, ~30s)               │
│  /lets:review    Full multi-agent code review (~2-3 min)                 │
├─ You plan, Claude builds ─────────────────────────────────────────────────┤
│  /lets:brainstorm  Think through what to build - backlog, priorities      │
│  /lets:plan        Design how to build it - codebase exploration, arch    │
│  /lets:execute     Claude implements the plan with your approval gates    │
├─ Agents work in parallel ─────────────────────────────────────────────────┤
│  /lets:team        Spawn agents that implement multiple tasks at once     │
│  /lets:worktree    Open parallel sessions in separate terminals           │
└───────────────────────────────────────────────────────────────────────────┘
```

**Commit** - `/lets:commit` reviews changes and creates a conventional commit (`feat:`, `fix:`, `refactor:`) linked to the active task.

**Finish** - `/lets:done` creates a PR on GitHub (or merges locally). `/lets:end` saves a session summary so the next conversation picks up where you left off.

### Under the Hood

**SessionStart Hook** - injects workflow rules into every Claude Code conversation: development practices, git conventions, session flow, discovery logging. This is what makes Claude follow the LETS workflow without you having to remind it.

**LETS Boxes** - after key actions, Claude shows contextual next-step suggestions so you always know what to do next:

```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

## 📖 Commands

### Session & Task

| Command | Description |
|---------|-------------|
| `/lets:start` | Start session - restore context, show tasks, create feature branch |
| `/lets:end` | End session - save progress, sync tasks, create summary |
| `/lets:commit` | Commit with review and conventional commit format |
| `/lets:done` | Finish task - create PR (GitHub mode) or merge locally |
| `/lets:status` | Task overview and project status |
| `/lets:note` | Add note to active task |

### Planning & Execution

| Command | Description |
|---------|-------------|
| `/lets:brainstorm` | Interactive backlog helper - because the backlog must grow! |
| `/lets:plan` | Structured planning - explore codebase, design architecture, write plan |
| `/lets:execute` | Execute plan from `/lets:plan` via native plan mode |
| `/lets:team` | Parallel implementation with Agent Teams |
| `/lets:worktree` | Create/manage worktrees for parallel sessions |

### Review & Analysis

| Command | Description |
|---------|-------------|
| `/lets:check` | Quick inline sanity check (~30s, 5-perspective review) |
| `/lets:review` | Full code review with dynamic agent selection (~2-3 min) |
| `/lets:pr` | PR review lifecycle - analyze, discuss, post inline, follow-up, approve |
| `/lets:opinion` | Technical decision analysis (3-5 expert agents in parallel) |
| `/lets:ask` | Quick expert consultation (single agent) |

### Setup

| Command | Description |
|---------|-------------|
| `/lets:install` | First-time global setup - plugins, environment, workflow |
| `/lets:init` | Initialize LETS in current project |

## 🔍 Review Options

| Need | Command | Time |
|------|---------|------|
| Quick pre-commit check | `/lets:check` | ~30s |
| Full review of local changes | `/lets:review --local` | ~2-3 min |
| Review an implementation plan | `/lets:review --plan` | ~2-3 min |
| Review a GitHub PR | `/lets:review <PR-url>` | ~2-3 min |
| Full PR lifecycle | `/lets:pr <PR>` | Interactive |
| Review a single file | `/lets:review --file <path>` | ~2-3 min |

## 🏗️ Planning

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

## ⚡ Parallel Work

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

## 🤖 Expert Agents

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

## 📋 Task Integration

LETS integrates with [beads](https://github.com/steveyegge/beads) for persistent task tracking:

- Every session starts by selecting a task
- Every commit is linked to the active task
- Session summaries and discovery notes are saved to the task
- Context survives conversation compaction and new sessions
- Task dependencies and blocking relationships are tracked

## ⚙️ Configuration

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

## 📦 Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| [Claude Code](https://claude.com/claude-code) | Yes | AI coding agent (the thing LETS plugs into) |
| [git](https://git-scm.com/) | Yes | Version control, branching, worktrees |
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking and issue management (Claude Code plugin) |
| [gh](https://cli.github.com/) | Optional | GitHub PR workflow (when `github: true`) |
| [jq](https://jqlang.github.io/jq/) | Optional | Statusline JSON parsing |

## License

MIT
