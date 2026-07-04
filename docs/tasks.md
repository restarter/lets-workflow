# Tasks

LETS is task-driven: every session starts by picking a task, every commit links to it. Task tracking goes through a pluggable **tracker adapter** (`LETS_TRACKER`); the default adapter is [beads](https://github.com/steveyegge/beads) (the `bd` CLI / Claude Code plugin), which LETS depends on for the default — alternative adapters (`planfix-mcp`, `none`) are described in [trackers.md](trackers.md) *(ships next release)*. The reason for a tracker rather than an ad-hoc TODO list is persistence — task descriptions, decisions, and discovery notes survive conversation compaction and carry across sessions.

> beads is required for the default adapter. Install it with its own one-liner:
> ```bash
> curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash
> ```
> Both `lets` and `bd` need to be on `$PATH` before `/lets:init` will work end to end.

## The task lifecycle

```
/lets:start  ──►  in_progress  ──►  …work, commits…  ──►  /lets:done  ──►  closed (or PR merged)
```

A task and a session aren't the same thing: a **session** is one conversation (`/lets:start` … `/lets:end`); a **task** is picked at start and finished with `/lets:done`, and may span several sessions. When you come back to an unfinished task, `/lets:start` restores its context from the task's comments.

## Taking a task

`/lets:start` shows the tasks that are ready to work on and asks you to pick one. Shortcuts:

- `/lets:start <task-id>` — jump straight to a specific task.
- `/lets:start --continue` — resume the task you had in progress.

Picking a task sets it to `in_progress` and creates the feature branch (`feature/<task-id>-<slug>`). If you describe work without picking a task, one is created for you, so there's still a traceable record.

In conversation, "take task X" / "work on X" / "switch to task X" triggers the same flow (the `take-task` skill) — it claims the task, handles any uncommitted changes, and prepares the branch.

## Creating a task

Say "create task" (or "new task", or "bd create …") and the `create-task` skill runs. It makes sure the required fields are present and suggests the project's existing labels:

- `--title` — a summary of the issue
- `--type` — `task` / `bug` / `feature`
- `--priority` — `0`–`4` (0 = critical, 2 = medium, 4 = backlog)
- `--description` — why the issue exists and what needs doing
- `--labels` — optional grouping (the skill discovers what the project already uses)

Tasks get short hash-based IDs, collision-free even with several people on a shared database.

A few beads conventions LETS follows:

- **Update tasks with comments, not by overwriting.** `bd update --notes` / `--description` overwrite existing content; `bd comments add` appends. Incremental progress goes in comments.
- **Group with labels, not epics with child IDs** — avoids ID collisions in a multi-user setup.
- **Add dependencies sparingly** — only when task B literally cannot start until task A is done. Most tasks are independent.

## Notes — recording what you learn

`/lets:note` adds a note to the active task: a decision, a gotcha, an infrastructure fact, a reference. These are stored as task comments, so they survive context limits and are there the next time anyone opens the task. Claude will *suggest* `/lets:note` when something worth recording comes up — you run the command, so you stay in control of what gets written.

## `/lets:status` — where things stand

`/lets:status` renders a lean, read-only orient snapshot: where you are (branch + active task), what's in flight, and what's next. It's the same snapshot `/lets:start` and `/lets:backlog --fast` open with. The dependency graph lives in native `bd blocked`; per-epic label progress and AI insights are retired from `/lets:status` (still reachable via `/lets:backlog`).

## `/lets:backlog` — ideation and backlog work

`/lets:backlog` manages the task backlog in three modes:

- **Review backlog** — agents go through your task list, find patterns, suggest priorities.
- **`--fast`** — a quick no-agent pulse: a fast context scan and a direct conversation, no agents.
- **Cleanup** — find stale tasks, broken dependencies, forgotten work.

## Memory — knowledge that outlives a session

beads has a memory feature for insights you want to keep across sessions and conversations: `bd remember "…"` stores one, `bd memories <keyword>` searches them, `bd forget <key>` removes one. Use it for things like "X doesn't work because Y", deployment quirks, or config facts you'll need again — knowledge that isn't tied to one task and shouldn't live in a scratch file.

## Shared backlogs for teams

For more than one developer, beads can point at a shared database via a [Dolt](https://github.com/dolthub/dolt) remote — everyone connects to the same backlog, sees the same task states, and the history is versioned. Setup is on the beads side; see its docs, and `scripts/remote/dolt/README.md` in this repo for the remote deployment.

## See also

- **[workflow.md](workflow.md)** — the session/task loop
- **[configuration.md](configuration.md)** — `lets init` vs `bd init` setup order
- **[commands.md](commands.md)** — `/lets:start`, `/lets:status`, `/lets:note`, `/lets:backlog`
