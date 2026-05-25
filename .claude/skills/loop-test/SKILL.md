---
name: loop-test
description: Local sandbox for experimenting with Claude Code's /loop command (interval + dynamic/self-paced scheduling) applied to LETS workflows. Use when the user says "loop test", "loop-test", "test the loop skill", "цикл LETS", "ScheduleWakeup experiment", or wants to design a periodic /loop that polls state or babysits a long-running job. Local-only — not part of the lets plugin payload. For convergent autonomous work toward an end state, use sibling skill `goal-test` (which covers /goal).
---

# loop-test (local experiment)

`/loop` schedules **periodic re-invocations of the same prompt**. Two modes:

| Mode | Syntax | Mechanic |
|---|---|---|
| **Interval** | `/loop 5m /foo` · `/loop /standup 1h` · `/loop check the deploy every 20m` | Cron entry in `.claude/scheduled_tasks.json`. Default `10m` when only `[interval]` shorthand. Survives session restart (durable). |
| **Dynamic / self-paced** | `/loop check the deploy` (no interval anywhere) | Model calls `ScheduleWakeup(delaySeconds, prompt, reason)` at the end of each turn. Skip → loop ends silently. |

**`/loop` is the wrong tool for sequential workflow phases.** It repeats the *same prompt* each tick — to chain `plan → review → check`, the prompt itself has to be phase-aware (read state, advance, write back). For state-convergent autonomy, use `/goal` instead (see `goal-test` skill).

**Right tool for `/loop`:**
- Polling external state (CI runs, deploys, PR comments)
- Periodic checks (every 20m: any new bd-ready tasks?)
- Babysitting a long-running job (check progress, surface failures)
- Continuous monitoring during a long focused session

## How `/loop` actually works

Reverse-engineered from `claude` binary v2.1.150.

### Dynamic-mode contract

This is the mode you care about most — interval is just cron.

1. User runs `/loop <prompt>` (no interval).
2. Model does the work.
3. Before yielding, model calls `ScheduleWakeup(delaySeconds, prompt, reason)`:
   - `delaySeconds` clamped to `[60, 3600]`
   - `prompt` must be **exactly the same string** as the original `/loop` input
   - `reason` is one sentence shown to user + sent to telemetry — be specific (`"polling CI run"` beats `"waiting"`)
4. Harness re-invokes the model at the scheduled time. Loop continues.
5. Skip `ScheduleWakeup` → loop ends silently. No error.

### Cache TTL gotcha

Anthropic prompt cache TTL is 5 minutes. Picking `delaySeconds`:
- `< 270s` → cache stays warm, cheap re-invocation
- `>= 1200s` → commit to the miss, one cache-miss amortizes over a long wait
- **`300s` is worst-of-both** — you pay the miss without amortizing. Avoid exactly 300.

For idle ticks with no specific signal to watch, default `1200-1800s` (20-30 min). User can always interrupt if they need you sooner.

### Cancellation / collisions

- Re-issuing `/loop` with the **same prompt** cancels the previous pending wakeup for that prompt (`ZP5(_)` in binary). One prompt = one live loop.
- User interrupt cancels **all** pending loop wakeups (`b76()` returns count cancelled).
- Recurring max age: `7 days` (`recurringMaxAgeMs = 604800000`). After that the loop auto-ages-out.

### Sentinels (fully autonomous)

For unattended runs where there's no user prompt sitting in the channel:
- `<<autonomous-loop>>` — cron-mode autonomous (CronCreate-backed)
- `<<autonomous-loop-dynamic>>` — ScheduleWakeup-mode autonomous

Useful for `/lets:schedule`-style routines but probably YAGNI for LETS right now.

### Telemetry

- `tengu_kairos_loop_dynamic` — feature flag for dynamic mode
- `tengu_loop_dynamic_wakeup_scheduled` — fired per ScheduleWakeup
- `tengu_loop_dynamic_wakeup_aged_out` — fired when 7-day cap hits

## Designing a useful `/loop` for LETS

Three rules that fit this project:

1. **Loop only on read-only state polls.** Anything that mutates (commit, push, bd close) needs explicit user approval per AUTO MODE rule — auto-loop violates it. `/lets:status`, `/lets:check`, `bd ready` are safe; `/lets:commit`, `/lets:done` are not.
2. **One sentence of new information per tick, or nothing.** If state hasn't changed, the tick should yield "no change" and reschedule. Don't re-report the same status over and over — the user sees ticks pile up.
3. **Pick the delay for the actual signal, not a round number.** Polling a CI run that takes ~8min? `delaySeconds=480` is better than `delaySeconds=600`. Avoid 300 (cache trap).

## Concrete `/loop` templates for LETS

### Template A — poll for PR feedback

```
/loop check `gh pr view <N> --json comments,reviewDecision` for new activity since last check. If nothing new: report "no change" and reschedule (1200s). If new comments or review state changed: report what's new and stop the loop so user can act.
```

Use case: opened a PR, want to know within 20min when reviewer pings, without keeping the window in focus.

### Template B — backlog watcher

```
/loop run `bd ready --json | jq length`. If count increased since last tick: report which new tasks appeared (bd show each new id, one-line summary). If unchanged: report "no change". Reschedule 1800s.
```

Use case: long deep-work session on a feature, want to surface if a higher-priority bug landed in the backlog without breaking focus.

### Template C — long-running job babysit

```
/loop check the deploy status at <URL or command>. If still running: report "running, T+Xmin" + estimated remaining. If failed: surface the error and stop. If succeeded: confirm and stop. Reschedule 180s while running.
```

Use case: triggered a deploy or migration, want hands-off progress reporting.

## Combining `/loop` with `/goal` (open research)

Interesting question: nest `/loop` inside `/goal`?

```
/goal Repo passes /lets:check three times in a row over the next hour.
/loop 5m /lets:check
```

The loop schedules the *work*; the goal defines the *termination condition*. Untested — see `goal-test` for the /goal half and try.

## Concrete experiment

```
/loop check git status; if no uncommitted files and `git rev-list main..HEAD --count` is 0, report "clean" and stop. Otherwise report the dirty state and reschedule 90s.
```

Watch in findings:
- Does `ScheduleWakeup` fire after each tick?
- Is `delaySeconds=90` honored, or clamped to 60?
- How does the UI display loop ticks between user messages?
- Does the model self-terminate when the stop-condition is met (no ScheduleWakeup call), or does it loop indefinitely?

## Open questions

- How does the cron-mode (`/loop 5m ...`) persist? Just file on disk, or also stored in session state? Does it survive `claude` restart?
- Can a dynamic loop call another dynamic loop? (Probably idempotent on prompt-string — second `/loop` cancels first.)
- Does `/loop` interact with worktrees? Per-worktree state files, or global to the user?
- What's the practical max number of concurrent loops? (Both cron and dynamic.)
- How does `/loop` interact with `/goal`? Does a Stop hook block a `/loop` tick from yielding?

## Findings (fill in during experiments)

Track in sibling `findings.md`. One entry per experiment:

```
### <date> · <prompt>
- Mode:              (interval / dynamic)
- delaySeconds:      requested → actual (clamped?)
- Ticks observed:    N
- Self-terminate?:   yes/no (did it skip ScheduleWakeup when condition met?)
- UI experience:     (intrusive? unobtrusive? useful?)
- Promote to plugin? yes/no + rationale
```

## Promote criteria

Skill graduates to a real `/lets:watch` (or similar) command in `plugins/lets/commands/` only if:

- At least one template (A/B/C) produces useful repeat-value across 3+ different real situations
- The "loop only on read-only polls" boundary is enforceable as a command-level guardrail
- We have a clear answer for: "how does the user stop the loop without losing the context?"

Otherwise: keep notes in `findings.md`, close `lets-53buy` with a "investigated, parking the pattern" comment.

## See also

- **Sibling skill `goal-test`** — for convergent autonomous work (Stop hook condition). `/goal` is the right tool for `plan → review → check → execute → review → check` chaining; `/loop` is not.
- LETS Rules `AUTO MODE` section in `.claude/rules/lets-rules.md` — gating rules every loop must respect.
- Binary symbols (claude v2.1.150): `tengu_kairos_loop_dynamic`, `tengu_loop_dynamic_wakeup_scheduled`, `recurringMaxAgeMs`, `ScheduleWakeup` tool.
