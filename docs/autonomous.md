# Autonomous & advanced flows

Most LETS work is hands-on: you start a session, pick a task, and drive it through the [day-to-day loop](workflow.md). This page covers the flows that run *with less of you in the loop* — Dynamic Workflows that move multi-agent work off your conversation, and the autonomous task pipeline that spawns a worktree, plans, and executes a task with only two gates for you to answer.

> New here? Read [workflow.md](workflow.md) (the core loop), [plan-execute.md](plan-execute.md) (plan → execute), and [parallel-work.md](parallel-work.md) (`/lets:team`, `/lets:worktree`) first. This page builds on all three.

## Dynamic Workflows (`--workflow`)

Several fan-out commands take an opt-in `--workflow` flag that runs their multi-agent work in a Claude Code Dynamic Workflow — off your conversation — so only the final aggregate lands back in context. Same findings, less context noise.

| Command | What `--workflow` does |
|---------|------------------------|
| `/lets:review --workflow` | Fan-out → dedupe → adversarial verify → aggregate, off-context |
| `/lets:opinion --workflow` | Expert fan-out + a conditional adversarial-challenge round |
| `/lets:backlog review --workflow` | Backlog review fan-out + aggregate |

It's a pure performance lever — the standard (non-`--workflow`) path produces the same result, just with the intermediate agent output in your conversation. Worth it when the work is a multi-stage off-context chain with no checkpoint in the middle; if you want to steer each step, use the standard path. (`/lets:plan`'s equivalent is the standalone PREVIEW below.)

## The autonomous task pipeline

The pipeline turns one well-described task into a spawn → plan → execute run that you supervise at just three points. It's built for running several parallel sessions without babysitting each one.

### Your three touchpoints

1. **Set it up (on `main`).** Create the bd task with enough steering in the description/comments that planning has something to work from.
2. **Answer the clarify gate (GATE 1).** A bounded set of up-front questions — only if the task lacks enough detail to build a planning rubric. Enough detail → no questions.
3. **Approve the plan (GATE 2).** Review and approve the saved plan file before any code is written.

### How it runs

```
/lets:worktree create <id> --flow plan-workflow --auto
        → opens a worktree, claims the task (resolve-and-claim)
        → [GATE 1] clarify (only if needed)
        → autonomous planning → saves a plan file
        → [GATE 2] you approve the plan
        → /lets:execute --auto runs the plan, /lets:commit at each point
        → stops at push / PR (still gated) → /lets:done
```

- **`--flow`** selects what the spawned session lands in: `plan-workflow` = autonomous planning (PREVIEW; falls back to interactive `--flow plan` when unavailable), `plan` = interactive `/lets:plan`, omitted = plain `/lets:start`. It only changes the launch command — terminal, cmux, and tmux launchers all inherit it.
- **`--auto`** maps to `claude --permission-mode auto`. It speeds up *approved* work; it never bypasses the gates below.
- **Execution** runs the approved plan without per-step prompts and commits at each plan point without re-asking.

### What still stops it (even in `--auto`)

- Push, PR, `bd close`, and any external-facing action stay gated — you approve them.
- Editing the merge-branch with `--auto` is refused outright.
- A tool failing 3× in a row, or detected fabrication, halts the run.

On any hard-stop the session writes a `blocked` marker and (on macOS + cmux) fires a "Execute blocked — needs you" notification.

### Watching N parallel sessions

Each spawned session writes a per-task marker at `.lets/cache/pipeline-state-<id>` (`<id>|<phase>|<iso>`, phases: `planning | gate-clarify | gate-approve | executing | blocked | done`). `cat` it, or watch the cmux sidebar. (The statusline row that renders this marker is a deferred follow-up.) Gate notifications are marker-gated, so only autonomous spawned runs notify — interactive sessions stay quiet.

## When things aren't available (degradation)

The pipeline degrades cleanly rather than failing:

Gate notifications route through `lets notify`, which dispatches on `LETS_LAUNCHER` — so the degradation depends on the active launcher:

| Condition | Behavior |
|-----------|----------|
| `LETS_LAUNCHER=terminal` (default) | `lets notify` is a no-op (`ok=true`, `reason=launcher_terminal`); the run continues without notifications |
| `LETS_LAUNCHER=cmux`, not macOS / no cmux | no-op (`ok=true`); the run continues |
| `LETS_LAUNCHER=tmux`, nobody attached | `reason=no_client` — the run continues and the gate still halts in-band; the operator sees the notification the moment they attach to the tmux session (cmux's sidebar persists; tmux's status line needs an attached client, so this is the launcher's one real limitation vs cmux) |
| `plan-workflow` unavailable | falls back to interactive `--flow plan` |
| Interactive session (no spawn) | no marker is written → no notification noise |
| Windows | `lets notify` returns a parseable `not_supported` envelope (exit 0) |

### Prerequisites

- `lets notify` needs the Go binary built (`make install`).
- `--flow` / `execute --auto` need the released plugin (or `make dev` / `--plugin-dir`).
- A notification channel needs `LETS_LAUNCHER=cmux` (macOS) or `LETS_LAUNCHER=tmux` (Linux/macOS, with a client attached); `terminal` surfaces gates in-band only.
- `plan-workflow` needs Claude Code ≥ 2.1.154 on a paid plan.

> `/lets:plan-workflow` is a PREVIEW — dogfooded across projects before it folds into native `/lets:plan`.

## See also

- [parallel-work.md](parallel-work.md) — the hands-on parallel flows the pipeline automates
- [plan-execute.md](plan-execute.md) — the plan → execute flow the pipeline runs autonomously
- [commands.md](commands.md) — `/lets:worktree`, `/lets:execute`, `/lets:plan-workflow` flags
- Building on the plugin? [`CLAUDE.md`](../CLAUDE.md) has the contributor-facing internals.
