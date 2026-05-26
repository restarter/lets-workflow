# Parallel work

Two ways to work on more than one task at a time: **`/lets:team`** spawns autonomous agents that each take a task, and **`/lets:worktree`** gives you separate directories so you can drive several sessions yourself.

## `/lets:team` — autonomous agents

```
/lets:team run
```

Pick a set of tasks; the system spawns one teammate per task (the count scales with how many you select). Each teammate:

1. Works in its own isolated git worktree.
2. Creates a plan and waits for your approval — you're the lead.
3. Implements the task once you've approved the plan.
4. Has its commits cherry-picked back.

Other subcommands: `/lets:team status` (how the teammates are doing), `/lets:team stop`.

This is the right tool when you have several independent, well-scoped tasks and want them done in parallel without babysitting each one. For a single task you're actively shaping, plain `/lets:plan` + `/lets:execute` is a better fit — see **[plan-execute.md](plan-execute.md)**.

## `/lets:worktree` — parallel terminals

When *you* want to work on two tasks at once without constantly switching branches:

```bash
/lets:worktree create auth-feature      # in the main repo
cd .worktrees/auth-feature && claude     # new terminal — fresh session
/lets:start                              # pick the task, start working
```

Each worktree gets its own branch (`worktree-<name>` for new branches; or an existing branch if you pass an existing branch name and the auto-detect resolves to attach). The `.lets/` config, sessions, and plans are shared via a symlink; the task database is shared via a targeted `.beads/.env` symlink so `bd` finds the same database via git's common-dir — both terminals see the same backlog and the same config. You get the full LETS workflow in each one. **Credential threat-model:** `.beads/.env` is shared, so don't store cross-context secrets there.

When you're done with a worktree: `/lets:done` (and `/lets:end`) inside it, then `/lets:worktree remove <name>` from the main repo.

A few things not to do inside a worktree:

- Don't create a `feature/` branch — `worktree-<name>` *is* the working branch.
- Don't run `/lets:worktree create` from inside a worktree.
- Don't restructure `.lets/` or `.beads/` — they're shared with the main repo.

Worktrees live in `.worktrees/` at the project root (gitignored).

## See also

- **[plan-execute.md](plan-execute.md)** — `/lets:team` runs this flow per task
- **[workflow.md](workflow.md)** — where parallel work fits the overall loop
- **[commands.md](commands.md)** — `/lets:team` and `/lets:worktree` subcommands
