# Plan → Execute

For anything bigger than a quick fix, LETS splits the work in two: **`/lets:plan`** works out *how* to build it, **`/lets:execute`** builds it — with you approving each step. You get a design you've reviewed before any code is written, and an implementation that doesn't surprise you.

> Rule of thumb: quick fix → just do it. Medium task (a few hours) → `/lets:plan` then `/lets:execute`. Large task → `/lets:plan`, and break it into subtasks.

## `/lets:plan` — design the change

`/lets:plan` runs in stages:

1. **Codebase exploration.** Explorer agents map the parts of the codebase the change touches — existing patterns, integration points, what's already there. The number of explorers scales with the project: a small repo gets one; a large monorepo gets up to ten, each mapping a different area.
2. **Architecture design.** With the lay of the land in hand, the plan is designed — components, data flow, build order, trade-offs. Expert agents, selected by what the plan involves (migrations, API endpoints, Docker, …), evaluate it.
3. **A written plan.** The result is saved to `.lets/plans/` — a step-by-step implementation plan you can read, edit, and hand to `/lets:execute`.

### `--fast` — skip the agents

`/lets:plan --fast` skips the explorer and architect subagent phases and plans collaboratively in the conversation instead. Use it when you already know the codebase and just want a quick talk-through and a written plan.

Not to be confused with `/lets:plan-workflow --fast` — THAT keeps the off-context workflow but runs it lean (~7 agents); this `/lets:plan --fast` runs no subagents at all.

## `/lets:execute` — build it

`/lets:execute` loads the plan from `/lets:plan` and implements it in Claude Code's native plan mode. You approve each step before Claude proceeds — no step happens without your go-ahead. Use `/lets:commit` at natural commit points along the way.

## The full loop

```
/lets:start ─── /lets:plan ─── (review the plan) ─── /lets:execute ─── /lets:commit ─── /lets:done ─── /lets:end
```

If you want to check a plan before executing it, `/lets:check --plan` is a fast inline pass; `/lets:review --plan` is the full multi-agent version, with agents selected from signals in the plan content.

## See also

- **[workflow.md](workflow.md)** — where plan → execute fits in the day-to-day loop
- **[parallel-work.md](parallel-work.md)** — `/lets:team` runs this flow autonomously across several tasks
- **[code-review.md](code-review.md)** — reviewing a plan with `--plan`
- **[commands.md](commands.md)** — `/lets:plan`, `/lets:execute` flags
