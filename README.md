<div align="center">

# 🌱 LETS Workflow

**A development workflow plugin for Claude Code**

*Stop babysitting your AI. Start shipping with it.*

[![Claude Code](https://img.shields.io/badge/Claude_Code-plugin-blueviolet)](https://claude.com/claude-code)
[![Version](https://img.shields.io/github/v/tag/restarter/lets-workflow?label=version&color=blue)](CHANGELOG.md)

</div>

---

Claude Code is powerful, but without structure it drifts - forgets context between sessions, silently changes approach when something fails, reviews its own code with no outside perspective, and loses track of what was decided and why.

**LETS fix this.** You get a team of 14 specialized AI agents, a structured development workflow, and a PR review system that posts inline comments directly to GitHub - all from the terminal. Every session has a task. Every commit links to it. Context survives across sessions and conversation compaction.

## Why LETS

**You don't just chat with AI. You run a dev team.**

- **14 expert agents** that dynamically select themselves based on your code changes - security agent for auth code, database agent for migrations, architect for structural changes. No manual configuration.
- **Full PR review lifecycle** - agents analyze the PR, you discuss findings, they post inline comments to GitHub, then follow up to verify fixes. Approve or request changes without leaving the terminal.
- **Actor agent** - load any expert personality from a URL or file. Want a senior iOS developer's perspective on your Swift code? A UX designer reviewing your components? Import their personality and get their unique take on your work.
- **Structured planning pipeline** - brainstorm ideas (4 modes), plan architecture with codebase exploration, execute with approval gates. Think, design, build.
- **Agent Teams** - spawn autonomous agents that implement multiple tasks in parallel, each in an isolated worktree with plan approval from the lead.
- **Session continuity** - context restored automatically, even after compaction. Discovery notes, decisions, and progress survive across conversations.
- **Task-driven** - every session starts with a task, every commit links to it, tracked via [beads](https://github.com/steveyegge/beads) with shared database for teams via [Dolt](https://github.com/dolthub/dolt).
- **GitHub-native** - PR creation, inline review comments, follow-up, approval. Or local merge if you prefer.
- **Latest Claude Code features** - ultrathink for deep analysis, interactive UI for decisions, native plan mode for execution.

<div align="center">

<img src="docs/images/review.png" alt="LETS Review" width="600">

*LETS statusline and `/lets:review` command in action*

</div>

## 🚀 Quick Start

### Prerequisites

The plugin requires the `lets` CLI binary on `$PATH` for SessionStart/PreCompact hooks and `lets statusline`. Until release pipelines ship (Homebrew via [lets-odg13](https://github.com/restarter/lets-workflow/issues), curl install via lets-2vb2b, winget/scoop via lets-hdrdr.1), install from a local clone:

```bash
git clone https://github.com/restarter/lets-workflow
cd lets-workflow
make install      # installs to /usr/local/bin or ~/.local/bin (smart fallback)
lets version      # verify
```

`lets` and `bd` (beads) both need to be on `$PATH` before `/lets:install` and `/lets:init` will work.

### Install

**Option A: From marketplace (recommended)**

In Claude Code:
```
/plugin marketplace add restarter/lets-workflow
/plugin install lets
```

**Option B: From local clone**

```bash
git clone https://github.com/restarter/lets-workflow
```

Then in Claude Code:
```
/plugin marketplace add ./lets-workflow
/plugin install lets
```

### Setup

```bash
# First-time global setup (installs beads plugin, verifies environment)
/lets:install

# Initialize current project (creates .lets/ structure, config, statusline)
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
│  Write code with Claude. Use helpers along the way:                       │
│  /lets:opinion   Technical decision with expert agents                     │
│  /lets:ask       Quick question to a single expert                        │
│  /lets:check     Quick sanity check (6 perspectives, ~30s)                │
│  /lets:review    Full multi-agent code review (~2-3 min)                  │
│                                                                           │
├─ You plan, Claude builds ─────────────────────────────────────────────────┤
│  /lets:brainstorm  Ideation: backlog review, explore ideas, brainstorm    │
│  /lets:plan        Design how to build it - codebase exploration, arch    │
│  /lets:execute     Claude implements the plan with your approval gates    │
│                                                                           │
├─ Agents work in parallel ─────────────────────────────────────────────────┤
│  /lets:team        Spawn agents that implement multiple tasks at once     │
│  /lets:worktree    Open parallel sessions in separate terminals           │
└───────────────────────────────────────────────────────────────────────────┘
```

**Commit** - `/lets:commit` reviews changes and creates a conventional commit (`feat:`, `fix:`, `refactor:`) linked to the active task.

**Finish** - `/lets:done` creates a PR on GitHub (or merges locally). `/lets:end` saves a session summary so the next conversation picks up where you left off.

### Under the Hood

**SessionStart + PreCompact Hooks** - run `lets hook session-start` / `lets hook precompact` on every Claude Code conversation. The hooks emit a small `## LETS Config` block (and a drift notice if rules are out of date) - the workflow rules themselves live in `<project>/.claude/rules/lets-rules.md` (copied there by `/lets:init`) and Claude Code loads them as project instructions. SessionStart fires on new/resumed/cleared/compacted sessions; PreCompact ensures rules survive long-session compaction. This is what makes Claude follow the LETS workflow without you having to remind it.

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
| `/lets:brainstorm` | Interactive ideation - backlog review, idea exploration, quick brainstorm, cleanup |
| `/lets:plan` | Structured planning - explore codebase, design architecture, write plan |
| `/lets:execute` | Execute plan from `/lets:plan` via native plan mode |
| `/lets:team` | Parallel implementation with Agent Teams |
| `/lets:worktree` | Create/manage worktrees for parallel sessions |

### Review & Analysis

| Command | Description |
|---------|-------------|
| `/lets:check` | Quick inline sanity check (~30s, 6-perspective review) |
| `/lets:review` | Full code review with dynamic agent selection (~2-3 min) |
| `/lets:pr` | PR review lifecycle - analyze, discuss, post inline, follow-up, approve |
| `/lets:opinion` | Technical decision analysis (dynamic expert agents in parallel) |
| `/lets:ask` | Quick expert consultation (single agent) |

### Setup

| Command | Description |
|---------|-------------|
| `/lets:install` | First-time global setup - plugins, environment, workflow |
| `/lets:init` | Initialize LETS in current project |

## 🔍 Code Review

Three levels of review, from 30-second sanity check to full PR lifecycle:

| Need | Command | Time | What happens |
|------|---------|------|-------------|
| Quick pre-commit check | `/lets:check` | ~30s | Inline 6-lens review: bugs, security, performance, quality, compliance, docs |
| Full code review | `/lets:review` | ~2-3 min | Dynamic agent selection - only relevant experts for your changes |
| Full PR lifecycle | `/lets:pr <PR>` | Interactive | Analyze, discuss, post inline comments, follow up on fixes, approve |

### PR Review Lifecycle (`/lets:pr`)

This is where LETS shines. Instead of reviewing PRs in a browser, you do it from the terminal with expert agents:

```
/lets:pr https://github.com/owner/repo/pull/42
```

1. **Analyze** - agents review the PR based on what actually changed (security agent for auth code, database for migrations, etc.)
2. **Discuss** - you see each finding with code context, decide what to post: inline comment, summary, or drop
3. **Post** - batch-posts inline comments to the exact lines on GitHub, with a review summary
4. **Follow up** - after the author pushes fixes, `/lets:pr --follow-up` checks each finding: fixed, not fixed, or needs discussion
5. **Approve** - `/lets:pr --approve` when ready, or request changes

Authors can respond with `/lets:pr --respond` - triage comments, auto-fix mechanical issues, post replies.

### Dynamic Agent Selection

Agents aren't hardcoded. Each command analyzes your changes and selects only relevant experts:

- Changed auth code? Security + backend + architect join the review
- Pure docs update? Docs + compliance, skip the rest
- Full-stack feature? Up to 12 agents, each focused on their domain

For plan reviews, agents are selected by signals in the plan content (mentions of migrations, API endpoints, Docker configs, etc.).

## 🏗️ Planning

> **Think** → **Design** → **Build**

**Brainstorm** (`/lets:brainstorm`) - 4 modes for different situations:
- *Review backlog* - agents analyze your task list, find patterns, suggest priorities
- *Explore ideas* - deep dive into a specific area with codebase analysis
- *Quick brainstorm* - fast ideation on a topic
- *Cleanup* - find stale tasks, broken dependencies, forgotten work

**Plan** (`/lets:plan`) - codebase exploration with dynamically-scaled explorer agents, then architecture design with expert evaluation. Small project? One explorer. Large monorepo? Up to 10, each mapping a different area.

**Execute** (`/lets:execute`) - implements the plan step by step in native plan mode. You approve each step before Claude proceeds. No surprises.

## ⚡ Parallel Work

Two ways to work on multiple tasks at once:

### Agent Teams (autonomous)

Spawn multiple agents that work in parallel, each in an isolated worktree:

```
/lets:team run    # select tasks, agents start working
```

Each teammate gets one task, creates a plan, waits for your approval, then implements. Commits are cherry-picked back. Dynamic teammate count - the system scales based on how many tasks you select.

### Worktrees (interactive)

Work on multiple tasks yourself in separate terminals:

```bash
/lets:worktree create auth-feature    # Terminal 1 (main repo)
cd .worktrees/auth-feature && claude  # Terminal 2 - start new session
```

Each worktree gets its own branch, shares the task database and config via symlinks. Full LETS workflow in each terminal.

## 🤖 Expert Agents

14 specialized agents, dynamically selected based on your code changes:

| Agent | Expertise | Example trigger |
|-------|-----------|----------------|
| architect | System design, patterns, SOLID | Structural changes > 50 lines |
| backend | APIs, business logic, error handling | Controllers, services, routes |
| frontend | UI components, state, accessibility | JS/TS/CSS changes |
| security | Vulnerabilities, auth, crypto | Auth code, tokens, encryption |
| database | Schema, migrations, query optimization | Migrations, ORM, raw queries |
| devops | Docker, CI/CD, deployment | Dockerfiles, CI configs, scripts |
| qa | Test strategy, coverage, assertions | Test files, testing patterns |
| compliance | Project standards, conventions | Always included in reviews |
| docs | Documentation sync, README accuracy | Always included in reviews |
| pragmatist | ROI analysis, overengineering detection | Large changes (> 200 lines) |
| git-historian | Blame analysis, change patterns | Changes to existing code |
| explorer | Codebase mapping, pattern discovery | Used during `/lets:plan` |
| implementer | Full-stack implementation | Used by `/lets:team` |
| actor | Any personality from URL or file | On explicit user request |

### How agents work

**Dynamic selection** - you don't pick agents. Commands analyze your changes and select only relevant experts. Security agent won't review a docs-only PR. Database agent won't review CSS.

**Tiered scoring** - findings are `[BLOCKER]` (must fix), `[SUGGESTION]` (should fix), or `[NIT]` (nice to have). No noise - agents are trained to skip obvious things and focus on what matters.

**Multiple modes** - each agent operates differently depending on the context: *review* mode for code review, *opinion* mode for technical decisions, *plan* mode for architecture evaluation, *brainstorm* mode for ideation, *ask* mode for direct questions.

**Actor agent** - load any expert personality from a URL or local file. Want a senior iOS developer's perspective? A UX designer's take? Import their personality and get their domain-specific analysis. User confirms each personality before loading.

**Read-only by default** - agents analyze but never modify code. The only exception: `implementer` has write access for parallel work via `/lets:team`.

## 📋 Task Integration

LETS uses [beads](https://github.com/steveyegge/beads) for persistent task tracking that survives conversation compaction:

- Every session starts by selecting a task, every commit links to it
- Discovery notes and decisions are saved as task comments - they survive context window limits
- Task dependencies and blocking tracked - `/lets:status` shows what's ready to work on
- Multi-developer: shared task database via [Dolt](https://github.com/dolthub/dolt) remotes - everyone sees the same backlog

## ⚙️ Configuration

After `/lets:init`, edit `.lets/.env`:

```env
# Response language (English/Ukrainian/Italian/etc)
LETS_LANGUAGE=English

# Target branch for merges and PR base
LETS_MERGE_BRANCH=main

# PR flow: github | bitbucket | local
LETS_PR_FLOW=github

# Task tracker (currently beads supported)
LETS_TRACKER=beads
```

> **Migration from legacy `config.yaml`:** if your project still has `.lets/config.yaml`, run `/lets:init` — `lets init` migrates it to `.lets/.env` (preserving values via allowlist regex). The yaml file is kept for reference but no longer read.

## File Storage

All generated files go to `.lets/` (gitignored):

```
.lets/.env              Project settings (LETS_LANGUAGE, LETS_MERGE_BRANCH, LETS_PR_FLOW, LETS_TRACKER)
.lets/sessions/         Session summaries and start references
.lets/reviews/          Saved review reports
.lets/plans/            Implementation plans
.lets/execution/        PR review state and team records
.lets/cache/            Usage stats and cached data
```

Interactive worktrees are stored in `.worktrees/` (gitignored).

> **Note:** LETS requires the [beads](https://github.com/steveyegge/beads) plugin for task tracking. `/lets:install` will guide you through installing it.

## 📦 Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| [Claude Code](https://claude.com/claude-code) | Yes | AI coding agent (the thing LETS plugs into) |
| [git](https://git-scm.com/) | Yes | Version control, branching, worktrees |
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking and issue management (Claude Code plugin) |
| [gh](https://cli.github.com/) | Optional | GitHub PR workflow (when `LETS_PR_FLOW=github`) |
| [jq](https://jqlang.github.io/jq/) | Optional | Statusline JSON parsing |

## License

MIT
