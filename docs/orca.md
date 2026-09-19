# Orca addon

[Orca](https://github.com/stablyai/orca) is a desktop app for running coding agents side by side, each in its own worktree and terminal. LETS works fine without it. With `LETS_LAUNCHER=orca` it uses Orca to open task worktrees, keep each worktree's card in step with the task, reach other sessions' panes, look across projects, and hand briefs to other agents' tabs.

## Switch it on

Pick Orca in `/lets:init`, or set `LETS_LAUNCHER=orca` in `.lets/.env` (or in `~/.lets/.env` for every project on this machine - see [configuration.md](configuration.md)). That one key switches on everything below; with any other launcher LETS never looks for an Orca binary, even inside an Orca terminal.

When the project's `.lets/.env` names Orca, `lets init` also writes `orca.yaml`: its setup hook runs `lets worktree adopt` (so a worktree Orca creates is linked to LETS at once) and its archive hook runs `lets worktree release`. Orca asks you once to trust it.

## What it changes

| Area | With `LETS_LAUNCHER=orca` |
|------|---------------------------|
| New task worktree | `/lets:worktree create <task-id>` asks Orca for the worktree and opens Claude there with `/lets:start <task-id>` - no second terminal |
| Linking | Orca's setup hook adopts the new worktree; archiving it in Orca runs `lets worktree release`, and the next `/lets:start --main` offers to set a task that was still in progress back to open. `/lets:worktree remove` refuses an Orca workspace - archive it in Orca |
| The card | `/lets:start` sets it in progress with the task title, a PR moves it to in review, a confirmed close completes it; `/lets:end`, a blocked execute and the plan-workflow gates leave a comment |
| Gate notifications | `lets notify` lands as a comment on the card |
| Peer messages | `/lets:orc` can type straight into a peer's pane when its transcript, screen and Orca all show it idle ([commands/orc.md](commands/orc.md)) |
| Across projects | `/lets:hub` lists every project's orchestrators, asks a stopped one read-only, or wakes it in a visible terminal ([messaging.md](messaging.md)) |
| Parallel runs | `/lets:team run --backend orca` runs each task as a visible LETS session in an Orca child worktree ([parallel-work.md](parallel-work.md)) |
| Hand-offs | `/lets:handoff --send` types a brief into a Codex, Antigravity or Claude tab of this worktree and brings the report back; `--open` opens a new Codex tab for it, and `--execute` hands an open tab an approved plan to implement ([commands/handoff.md](commands/handoff.md)) |

## When Orca is not there

Nothing hard-fails. Orca missing, not running, or refusing a request is a named reason, and each feature degrades on its own: the worktree launcher falls back to cmux, then to the plain `cd … && claude` command; the card and notifications are skipped with a reason; `/lets:hub` and `--send` stop and say why; `/lets:team` uses Agent Teams. On Windows the Orca commands are stubs.

LETS finds the Orca CLI inside the app bundle first (`/Applications/Orca.app/Contents/Resources/bin/orca`) and trusts an `orca` on `PATH` only when it answers like Orca: a broken `/usr/local/bin/orca` symlink and the GNOME screen reader named `orca` both exist in the wild.

## Things worth knowing

Observed with Orca 1.4 (2026-09):

- **Orca reports delivery per agent, honestly.** For Claude and Codex a send confirms the agent started its turn; for Antigravity it only confirms the input was accepted, so LETS reads the tab back instead of assuming.
- **A new agent tab can stop before its prompt.** A Codex tab with an update available shows an update prompt first, and a first-run Antigravity tab asks to trust the workspace. LETS does not answer those for you.
- **Typed text is not always on screen.** In a Claude pane, text typed without Enter lives in Orca's draft, and the next send submits it together with the new text. `/lets:handoff --send` refuses on a draft.
- **Tab titles rarely name the agent** (`lets-w5tm5 | …`, `t0 agy`). `/lets:handoff --send codex` matches the agent, not only the title.
- **A scrolled pane reads as scrolled.** When you scroll a Claude tab up, Orca's screen read returns that part, not the bottom. Reports are read from files, not screens.
- **`--auto` does not pass through Orca.** Orca starts Claude with your own agent command, so `/lets:worktree create --auto` cannot add `--permission-mode auto` there - it opens the worktree through cmux, or prints the terminal command, instead.
- **Orca names the branch `<task-id>-<slug>`.** Not LETS's `worktree-<task-id>-<slug>`: Orca creates the worktree, so the `/`-free name is both the directory and the branch. LETS recognizes that shape when it adopts the worktree, and `/lets:start <id>` confirms the task.

## See also

- **[messaging.md](messaging.md)** - the three lanes: peers, hub, hand-offs
- **[parallel-work.md](parallel-work.md)** - worktrees, adopt / release, `/lets:team --backend orca`
- **[configuration.md](configuration.md)** - `LETS_LAUNCHER` and the other launchers
- `cli/README.md` "## lets orca" - the `lets orca open|notify|status|card|repos|wake` CLI
