# Parallel work

Three ways to get more than one thing done at a time: **`/lets:team run`** opens one visible LETS session per task, **`/lets:worktree`** gives you separate directories so you can drive several sessions yourself, and **parallel implementers** (`/lets:execute --implementers --parallel`) split ONE task's plan across agents.

## `/lets:team run` — one session per task

```
/lets:team run
```

Pick a set of tasks (or name them with `/lets:team run --tasks A,B,C`). For each task LETS cuts a branch from `origin/<merge-branch>` into its own worktree and opens a visible LETS session there - a **worker** named `<repo>-<task-id>`, bound to your session as its orchestrator. It works on every launcher: a terminal command, a cmux workspace, a tmux window, or an Orca pane.

1. Each worker runs its own `/lets:start` ... `/lets:done` - its plan, its code, its PR.
2. You press every gate in the worker's own terminal. Your session coordinates through `/lets:orc` (ask / ping / tell) and never decides for a worker; a message from it is never approval.
3. `/lets:team status` shows every worker - live or not, and its tracker status. `/lets:team stop` asks each worker to commit and end its session; worktrees and branches stay.

Start it from the main checkout, with a clean tree; your session is registered as an orchestrator before the first worker opens, and a task another active run holds is refused.

**Orca addon.** With `LETS_LAUNCHER=orca` and Orca running, `/lets:team run` is an Orca supervised run (`--backend orca` picks it directly): each task becomes a visible LETS session in its own Orca child worktree, you press that session's gates in its terminal, and this session coordinates - relaying every worker question to you whole and replying only with your words. A run never uses both backends for one task: a team record names its backend, and a run stops when another active record of the other backend already holds a selected task. `--backend agents` is refused: the Agent Teams primitives it used are gone from Claude Code.

This is the right tool when you have several independent, well-scoped tasks. For a single task you're actively shaping, plain `/lets:plan` + `/lets:execute` is a better fit — see **[plan-execute.md](plan-execute.md)**.

## A standing team — a workspace that outlives its tasks

A **standing team** owns an area of the repo for longer than one task: its own worktree, a lead session, `lets:*` members (architect, skeptic, explorer, implementer) and a team file that remembers what it learned. Continuity is files only: the harness restores no member after the lead restarts, so every decision and finding that matters is written to the team file (`.lets/teams/<callsign>.md`) or a file it lists, and members are respawned from it, never resumed from memory.

**Create.** From the main checkout:

```
/lets:team create [<callsign>] --area "billing API"
```

Teams are named by callsign (snake, frog, otter, ...). Without one, LETS proposes a free callsign - no team file under that name and no live `<callsign>-lead` session - and checks it before anything is created. It then creates the worktree `team_<callsign>` (dir and branch) from `origin/<merge-branch>`, writes the team file (never overwriting one), shows your project's optional `.lets/hooks/team-setup` whole and runs it only on your yes (it can fill the team file's Workspace: a docker prefix, a compose project, a port block), and launches ONE session - the lead, named `<callsign>-lead` - on your launcher. The same command on an existing team reopens it: its lead is launched again.

**Work task by task.** In the lead session, `/lets:start <task-id>` claims the team's lead (a second chat is told who the live lead is) and moves the team worktree to that task's branch - an existing one, or a new one cut from `origin/<merge-branch>`. Uncommitted work on the previous task is committed or **parked**: a `wip(<id>): park` commit on that branch, which is undone (its changes staged again) when you come back to the task and the park was never pushed. Every new file must be named for a park; ignored files are never parked, and nothing is ever stashed. A switch refuses while a merge or rebase is in progress, while an implementer is still writing in the worktree, or onto the merge-branch. `/lets:done` warns about park commits still in the range and offers to promote what this task learned to the team file's Standing knowledge.

**Members.** From the lead's session:

- `/lets:team spawn <role> [name]` spawns one member (`spawn --roster` the whole roster from the team file); a name that is still live is never spawned twice. After a restart, `/lets:start` offers **Respawn roster**.
- `/lets:team roster` shows the roster joined with who is live; `/lets:team dismiss <name>` or `dismiss --all` ends members.

Members only ever talk to the lead.

**Disband.** `/lets:team disband <callsign>` retires the team, and never kills anything: when the recorded lead is still live it asks the lead (through `/lets:orc`) to dismiss its members and wrap up, or lets you say it already has; when LETS cannot tell, it asks you; with no live lead, it lists every session still open in the team worktree for you to close or keep. A task still in progress, or a branch the team parked, stops it until you decide. The worktree is removed through `/lets:worktree remove` with its usual checks (uncommitted changes, unpushed commits), your optional `.lets/hooks/team-teardown` runs only on your yes, and the team file and its members registry stay as history - the callsign stays taken.

## Parallel implementers inside one task

**`/lets:team` or delegated `/lets:execute`?** Team takes several *tracker tasks* and runs each in its own session. Delegated `/lets:execute --implementers` takes one *plan* on this branch: by default ONE persistent implementer works through it chunk by chunk, and you (or the team check, under the gate policy you pick at Start) accept each chunk before it is committed. With `--parallel`, you declare file-disjoint groups of chunks and each group gets its own implementer in an isolated worktree; `lets integrate` lands each finished chunk in your tree as one verified patch - never a merge, never `reset --hard`. File-disjoint is not independence: a chunk that calls code another group adds belongs in the same group. Details: **[plan-execute.md](plan-execute.md)**.

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

- **[plan-execute.md](plan-execute.md)** — each `/lets:team` worker runs this flow for its task; delegated and parallel implementers
- **[workflow.md](workflow.md)** — where parallel work fits the overall loop
- **[autonomous.md](autonomous.md)** — the autonomous pipeline that automates this with `--flow plan-workflow --auto`
- **[commands.md](commands.md)** — `/lets:team` and `/lets:worktree` subcommands
- **[smoke/parallel-implementers.md](smoke/parallel-implementers.md)** — the live smoke procedure for team runs and parallel implementers
- **[messaging.md](messaging.md)** — orchestrators, workers and hand-offs: talking to other sessions and agents
- **[orca.md](orca.md)** — the Orca addon
