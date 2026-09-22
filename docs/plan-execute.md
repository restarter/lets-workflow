# Plan → Execute

For anything bigger than a quick fix, LETS splits the work in two: **`/lets:plan`** works out *how* to build it, **`/lets:execute`** builds it — after one approval, at the pace you pick. You get a design you've reviewed before any code is written, and an implementation that doesn't surprise you.

> `/lets:start` sizes the task for you: for a medium or large one it offers `/lets:plan`, `/lets:plan --fast`, `/lets:plan-workflow` (PREVIEW) or working directly.
>
> Rule of thumb: quick fix → just do it. Medium task (a few hours) → `/lets:plan` then `/lets:execute`. Large task → `/lets:plan`, and break it into subtasks.

## `/lets:plan` — design the change

`/lets:plan` runs in stages:

1. **Codebase exploration.** Explorer agents map the parts of the codebase the change touches — existing patterns, integration points, what's already there. The number of explorers scales with the project: a small repo gets one; a large monorepo usually gets up to ten, each mapping a different area; more than ten asks you to confirm first.
2. **Architecture design.** With the lay of the land in hand, the plan is designed — components, data flow, build order, trade-offs. Expert agents, selected by what the plan involves (migrations, API endpoints, Docker, …), evaluate it. You choose how hard it is evaluated at a checkpoint: **Full panel** (pragmatist + the domain experts the feature calls for), **Pragmatist only** (a quick overengineering check), **Self-evaluation** (no agents - the orchestrator critiques its own design) or **Skip evaluation**.
3. **A written plan.** The result is saved to `.lets/plans/` — a step-by-step implementation plan you can read, edit, and hand to `/lets:execute`. Every plan opens with a STOP banner: approving the plan (or a plan review) approves the document, never the code — `/lets:execute` is the only way into implementation, and Claude will not start it on its own.

### `--fast` — skip the agents

`/lets:plan --fast` skips the explorer and architect subagent phases and plans collaboratively in the conversation instead. Use it when you already know the codebase and just want a quick talk-through and a written plan.

Not to be confused with `/lets:plan-workflow --fast` — THAT keeps the off-context workflow but runs it lean (~7 agents); this `/lets:plan --fast` runs no subagents at all.

### `--idea` — capture the wish first

When it is too early to design anything, `/lets:plan --idea` writes a concept document instead of a plan: the problem, what exists today, the behaviour you want, what triggers it, constraints, open questions, and who takes it over. It asks about the wish, not the code, and never explores the codebase. The file lands in `.lets/plans/` as `…-<task-id>-idea.md` with an "IDEA BANK ENTRY - NOT A PLAN" header: `/lets:execute` refuses it, `/lets:check --plan` reviews it with concept lenses, and running `/lets:plan` on the same task later picks it up as input.

## `/lets:execute` — build it

`/lets:execute` loads the plan from `/lets:plan` and implements it - in this session under Claude Code's native plan mode, or by handing each commit-point chunk to a named implementer agent you review and correct. You approve before any code is written, then pick how it runs:

| Run mode | Flag | What happens | Commits |
|----------|------|--------------|---------|
| Straight-through (default) | `--straight` | One approval, then every task with no pause | At each commit point the plan names, without re-asking |
| Step-by-step | `--step` | A pause for your review after each task | Each one confirmed |
| Auto | `--auto` | Unattended under AUTO MODE; push / PR / close / external actions stay gated, hard-stops still halt it, refused on the merge branch | At the plan's commit points |
| Implementers | `--implementers` (alias `--team`) | Each commit-point chunk goes to a named implementer agent; every diff comes back for your review, and a correction goes to the same agent, which keeps its context. Interactive only - refused with `--auto` | One per chunk, after you accept it |

A flag pre-answers the picker; bare `/lets:execute` asks once. `/lets:execute <task-id>` or `<plan-path>` picks the plan explicitly, and `--status` shows where the current plan stands.

If reality diverges from the plan's approach mid-run — a tool behaving differently than assumed, a step that can't be done as written — Claude stops and asks instead of quietly re-planning in place (under `--auto` that is a hard stop). Use `/lets:commit` at natural commit points along the way.

### Delegated runs - review and correct

With **Implementers**, this session stops writing code and starts reviewing it. First you see how the plan splits: every task is either done here (steps that write no files, such as checks) or part of a *chunk* - the tasks up to the next commit point. One Start approval hands the first chunk to a named agent; you pick its model (Opus by default, Fable for the hardest chunks, Sonnet or Haiku when the work is mechanical). When its report comes back you see the real diff under that agent's name and choose: accept it (it is committed), send a correction (the same agent continues with everything it already knows), or stop. Stopping an agent while it works ends it: the chunk then goes to a fresh agent that gets the brief and every correction so far, but not the first agent's memory. The next chunk goes to a fresh agent. A delegated run starts only on a clean working tree, nothing is committed until you accept it, and an interrupted run resumes from its record the next time you run `/lets:execute`.

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
