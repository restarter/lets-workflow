# Protocol: worktrees

Loaded by: `/lets:start` (Step 0.5 adopt), `/lets:done` (worker-PR merge, vanishing worktree), `/lets:team`, skill `take-task` (Step 2T) - Read right before the step that needs it. Core rules keep the always-on part: in a worktree use its branch as-is (never a `feature/` branch), `$LETS_PROJECT_ROOT` is the worktree path, Glob does not follow symlinks, never modify `.lets/` or the store links, never `--delete-branch` a worker's PR from another checkout.

## Location and detection

A worktree may live anywhere: `lets worktree create` puts it in `.worktrees/`, Orca in its own workspace directory. The main checkout always comes from git common-dir.

| check | how |
|---|---|
| boolean | `git rev-parse --git-dir`: `.git` = main repo; contains `worktrees/` = inside a worktree |
| more than the boolean | `lets worktree info [--json]` - `in_worktree`, `main_root`, current branch, symlink status; resolve the main repo from `main_root`, never compute it by hand |

## Inside a worktree

| topic | rule |
|---|---|
| branch | `worktree-<name>` (new) OR an attached existing branch (auto-detected by `/lets:worktree create`) - the working branch; use as-is |
| ids + branch names | from the tracker adapter's `## Worktree` convention (`id:` / `branch:` / `worktree-branch:` / `accept:`), overridden per key by the user-owned `tracker-<name>.board.md`; Go parses and renders them (`lets worktree info --task-candidate`, `lets worktree branch-name`) - never hand-build them |
| shared state | `.lets/` is a whole-dir symlink to the main repo's `.lets/` (config, sessions, plans shared); the tracker store is reached through the links the adapter's `## Worktree` `links:` declares (beads: `.beads/.env` -> the main repo's, so `bd` finds the same database) |
| never | create a `feature/` branch; run `/lets:worktree create` from inside a worktree; modify `.lets/` or the declared store links (LETS-managed, shared with main) |

## Adopt / release

| step | rule |
|---|---|
| adopt | a worktree LETS did not create (Orca, a teammate, `git worktree add`) becomes a LETS worktree through `lets worktree adopt`. Runs from Orca's `orca.yaml` setup hook, and from the SessionStart hook for an unlinked worktree of an initialized project before LETS Config is built (never without `<main>/.lets/.env`, never under `.claude/worktrees/`). `/lets:start` Step 0.5 is only the fallback |
| never deletes | a pre-existing cache-only `.lets` moves to `.lets.pre-adopt[-N]` (safe to delete by hand); anything else stops with exit 22 |
| release | `lets worktree release` (Orca's archive hook) removes the task-state file and leaves a `released-<task-id>` marker for `/lets:start --main` Reopen |

## Task-state file

`.lets/sessions/.task-worktree-<name>`, keyed by branch-slug so parallel sessions never collide. A validated cache, not a source of truth.

| field | meaning |
|---|---|
| `task:` | the active task; outranks the frozen branch name (several tasks can share one worktree) |
| `start:` | read by `/lets:done` |
| `session: <sha> <sid>` | read by `/lets:end`; refreshed by the SessionStart hook on a new session |
| `origin:` | adopt derived the id from the branch or directory name; probed once before use; cleared on claim |
| `orc:` | a worker's orchestrator binding; never on the merge-branch |

- detect-task reads the file first; every reader cross-checks against a live anchor (tracker status on the merge-branch, git ancestry, the session-id) and degrades loudly.
- Written only through `lets worktree task-state` (merge-write under a lock); a writer without the binary replaces only the keys it owns and keeps every other line. `/lets:start` rewrites it.

## Standing-team worktree

`team_<callsign>` (dir and branch), created by `/lets:team create`, claimed by `.lets/teams/<callsign>.md`; its lead session is `<callsign>-lead`, recorded by `lets members` and claimed by `/lets:start`. It outlives its tasks: `/lets:start <id>` moves it to the task's branch through `lets worktree switch` (a new branch is cut from `origin/<merge>`); uncommitted work is committed or parked as `wip(<id>): park` - never stashed - and unparked when the task comes back. Removed only by `/lets:team disband`.

## Remove safety nets

`/lets:worktree remove` blocks on two distinct `error.kind` values - never treat them as the same problem. The skill walks the user through an `AskUserQuestion` for each; follow that prompt, never fall through to generic error handling.

| kind | exit | cause | fix |
|---|---|---|---|
| `dirty_worktree` | 14 | uncommitted changes in the working tree | commit/stash first, or `--force` to discard |
| `unpushed_commits` | 21 | local commits not on upstream | push the branch (typical after `/lets:done` created a PR), or `--force` to discard them with the worktree |

## Lifecycles

| launcher | lifecycle |
|---|---|
| terminal | `/lets:worktree create <name>` (main repo) -> new terminal `cd .worktrees/<name>/ && claude` -> `/lets:start` -> work -> `/lets:done` -> `/lets:worktree remove <name>` (main repo) |
| Orca (`LETS_LAUNCHER=orca`) | `/lets:worktree create` -> Orca creates the worktree and runs `lets worktree adopt` (the SessionStart hook adopts if it did not) -> `/lets:start <id>` -> work -> `/lets:done` -> archive in Orca (`lets worktree release`; `/lets:worktree remove` refuses it with `worktree_external`) -> `/lets:start --main` offers Reopen for archived tasks still `in_progress` |

## A worktree can vanish outside Orca

gh >= 2.99 `gh pr merge --delete-branch`, run from another checkout, removes the head branch's linked worktree - Orca's archive hook (`lets worktree release`) never runs.

- A worker's PR merged from any other checkout (the orchestrator, the main checkout) goes WITHOUT `--delete-branch`. The worker's own `/lets:done` merge from inside its worktree is safe - gh never removes the current worktree.
- Merge a worker's PR from another checkout only after `lets worktree record --task <id> --ref <branch> --json` shows `present`; `missing` or `stale` -> do not merge, ask the worker for `/lets:end` first.
- Archive the worktree in Orca afterwards - its hook records the release; Orca's delete removes the branch too.
