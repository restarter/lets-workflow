# LETS documentation

Reference docs for the LETS Workflow plugin. If you haven't set it up yet, start with **[installation](installation.md)**; for the day-to-day loop, **[workflow](workflow.md)**.

| Doc | What's in it |
|-----|--------------|
| [installation.md](installation.md) | Install the `lets` binary and the Claude Code plugin, initialize a project. Troubleshooting and uninstall. |
| [workflow.md](workflow.md) | The day-to-day loop — session lifecycle, the three ways to work (you code / you plan & Claude builds / agents in parallel), LETS boxes, and how the hooks keep Claude on track. |
| [plan-execute.md](plan-execute.md) | The plan → execute flow — `/lets:plan` designs the change with codebase exploration and expert review, `/lets:execute` implements it step by step behind approval gates. |
| [code-review.md](code-review.md) | Three levels of review — `/lets:check` (30s sanity check), `/lets:review` (full multi-agent), `/lets:github-pr` (PR lifecycle: analyze, post inline, follow up, approve). Dynamic agent selection. |
| [agents.md](agents.md) | The 14 expert agents, what triggers each, tiered scoring, agent modes, and the actor agent (load any personality from a URL or file). |
| [parallel-work.md](parallel-work.md) | Working on several tasks at once — `/lets:team` (autonomous agents, one task each, isolated worktrees) and `/lets:worktree` (you, in parallel terminals). |
| [tasks.md](tasks.md) | Task tracking with beads — why it's there, the task lifecycle, taking and creating tasks, notes, `/lets:brainstorm`, beads memory, and shared backlogs for teams. |
| [commands.md](commands.md) | Full reference for every `/lets:*` command — flags, when to use which. |
| [configuration.md](configuration.md) | `.lets/.env` settings, the `.lets/` file layout, `lets init` vs `bd init` setup order, and dependencies. |

Building on the plugin itself? See [`CONTRIBUTING.md`](../CONTRIBUTING.md) and [`CLAUDE.md`](../CLAUDE.md) at the repo root.
