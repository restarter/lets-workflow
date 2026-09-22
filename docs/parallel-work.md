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

`/lets:team run --tasks A,B,C` names the tasks up front and skips the picker.

Other subcommands: `/lets:team status` (how the teammates are doing), `/lets:team stop`.

**Orca backend (addon).** With `LETS_LAUNCHER=orca` and Orca running, `/lets:team run` offers an Orca supervised run instead (`--backend orca` picks it directly): each task becomes a visible LETS session in its own Orca child worktree, you press that session's gates in its terminal, and this session coordinates - relaying every worker question to you whole and replying only with your words. A run never uses both backends for one task: a team record names its backend, and a run stops when another active record of the other backend already holds a selected task.

This is the right tool when you have several independent, well-scoped tasks and want them done in parallel without babysitting each one. For a single task you're actively shaping, plain `/lets:plan` + `/lets:execute` is a better fit — see **[plan-execute.md](plan-execute.md)**.

**`/lets:team` or delegated `/lets:execute`?** Team takes several *tracker tasks* and runs one implementer per task in its own worktree; with the Orca backend each worker's questions are relayed to you, and the finished work is reviewed at the end. Delegated `/lets:execute` (the Implementers mode) takes one *plan* on this branch, hands it to implementer agents chunk by chunk, and shows you each diff under the agent's name so you can correct that same agent before anything is committed. Both use the same `implementer` agent. Team's Agent Teams backend is being rebuilt for the current Claude Code (lets-7dwc1); its Orca backend is unaffected.

## `/lets:worktree` — parallel terminals

When *you* want to work on two tasks at once without constantly switching branches:

```bash
/lets:worktree create auth-feature      # in the main repo
cd .worktrees/auth-feature && claude     # new terminal — fresh session
/lets:start                              # pick the task, start working
```

Each worktree gets its own branch (`worktree-<name>` for new branches; or an existing branch if you pass an existing branch name and the auto-detect resolves to attach). The `.lets/` config, sessions, and plans are shared via a symlink; the task store is shared through the links your tracker adapter declares (beads: a targeted `.beads/.env` symlink, so `bd` finds the same database via git's common-dir) — both terminals see the same backlog and the same config. You get the full LETS workflow in each one. **Credential threat-model:** the linked store credential (beads: `.beads/.env`) is shared, so don't store cross-context secrets there.

**Attaching an existing branch.** `/lets:worktree create <name> --branch <ref>` puts an existing branch into the worktree - slash refs such as `feature/login-fix` work, while the worktree directory keeps the slash-free `<name>`. Without `--branch`, attach vs new branch is auto-detected from whether a branch called `<name>` exists; `--attach` or `--new-branch` forces one. `/lets:worktree create <task-id>` names the branch by your tracker's convention, and `--flow plan|plan-workflow` / `--auto` choose what the new session starts in (see [autonomous.md](autonomous.md)).

**One live session per worktree.** With a cmux, tmux or Orca launcher, opening a worktree that already has a session does not spawn a second one - it tells you to switch to the existing one (`already_open`).

**`/lets:worktree info`** shows where you are: whether this checkout is a worktree, the main repo path, the branch, and whether the `.lets/` link is intact. `/lets:worktree list` shows them all.

**Removing a worktree.** `/lets:worktree remove <name>` stops - and asks - on two different conditions: **uncommitted changes** (commit or stash first) and **commits not on the remote** (push the branch first - typical right after `/lets:done` opened a PR). Either can be forced, but only after you confirm, because forcing discards that work. A worktree Orca created is archived in Orca instead.

When you're done with a worktree: `/lets:done` (and `/lets:end`) inside it, then `/lets:worktree remove <name>` from the main repo.

A few things not to do inside a worktree:

- Don't create a `feature/` branch — `worktree-<name>` *is* the working branch.
- Don't run `/lets:worktree create` from inside a worktree.
- Don't restructure `.lets/` or `.beads/` — they're shared with the main repo.

Worktrees live in `.worktrees/` at the project root (gitignored).

### Worktrees LETS did not create (Orca, a teammate, `git worktree add`)

A worktree made by another tool has no `.lets` link and no store link, so LETS in it would read an empty config. `lets worktree adopt` links it in place (it is never moved) and records the task when the branch name carries an id. You rarely run it yourself: the SessionStart hook adopts an unlinked worktree of an initialized project before the session's config is built, Orca's `orca.yaml` setup hook runs it when you use `LETS_LAUNCHER=orca`, and `/lets:start` falls back to it. An id guessed from a name like `lets-abc-fix-login` is marked unconfirmed until `/lets:start` claims it. Adopt never deletes: a `.lets` directory that only holds a statusline cache is moved to `.lets.pre-adopt` (safe to delete by hand), anything else stops with an explanation.

With `LETS_LAUNCHER=orca`, `/lets:worktree create <task-id>` asks Orca to create the worktree and open Claude there with `/lets:start <task-id>` — no second terminal. Finish with `/lets:done` inside it, then archive the workspace in Orca: its archive hook runs `lets worktree release`, and if the task was still in progress, the next `/lets:start --main` offers to set it back to open. `/lets:worktree remove` refuses an Orca workspace. `/lets:worktree remove` also offers to sweep other task branches already merged into `origin/<merge-branch>`.

### Across projects: `/lets:hub` (Orca addon)

With `LETS_LAUNCHER=orca`, one session can look across every project Orca knows. `/lets:hub` lists each project's orchestrators and whether they are running. A read-only question to a stopped orchestrator (what is in progress, what is next) is answered by a headless fork of its last session, launched in plan mode with only Read, Grep and Glob, no MCP servers and a filtered environment; anything that would change something wakes the orchestrator in a visible Orca terminal, so you press its gates yourself. The hub never runs a second process on an orchestrator that is alive, and never writes into the other project. How the hub, `/lets:orc` and `/lets:handoff` fit together: **[messaging.md](messaging.md)**; everything Orca changes: **[orca.md](orca.md)**.

## See also

- **[plan-execute.md](plan-execute.md)** — `/lets:team` runs this flow per task
- **[workflow.md](workflow.md)** — where parallel work fits the overall loop
- **[autonomous.md](autonomous.md)** — the autonomous pipeline that automates this with `--flow plan-workflow --auto`
- **[commands.md](commands.md)** — `/lets:team` and `/lets:worktree` subcommands
- **[messaging.md](messaging.md)** — orchestrators, workers and hand-offs: talking to other sessions and agents
- **[orca.md](orca.md)** — the Orca addon
