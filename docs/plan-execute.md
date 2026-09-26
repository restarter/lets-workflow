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

`/lets:execute` loads the plan from `/lets:plan` and implements it - in this session under Claude Code's native plan mode, or by handing its commit-point chunks to implementer agents you review and correct. You approve before any code is written, then pick how it runs:

| Run mode | Flag | What happens | Commits |
|----------|------|--------------|---------|
| Straight-through (default) | `--straight` | One approval, then every task with no pause | At each commit point the plan names, without re-asking |
| Step-by-step | `--step` | A pause for your review after each task | Each one confirmed |
| Auto | `--auto` | Unattended under AUTO MODE; push / PR / close / external actions stay gated, hard-stops still halt it, refused on the merge branch | At the plan's commit points |
| Implementers | `--implementers` (alias `--team`) | One persistent implementer works through the chunks (or `--parallel` groups work side by side in isolated worktrees); every chunk is checked before it is committed, and a correction goes to the same agent, which keeps its context. Interactive only - refused with `--auto` | One per chunk, after it is accepted - by you, or by the team check under the gate policy you pick |

A flag pre-answers the picker; bare `/lets:execute` asks once. `/lets:execute <task-id>` or `<plan-path>` picks the plan explicitly, and `--status` shows where the current plan stands.

If reality diverges from the plan's approach mid-run — a tool behaving differently than assumed, a step that can't be done as written — Claude stops and asks instead of quietly re-planning in place (under `--auto` that is a hard stop). Use `/lets:commit` at natural commit points along the way.

### Delegated runs - review and correct

With **Implementers**, this session stops writing code and starts reviewing it. First you see how the plan splits: every task is either done here (steps that write no files, such as checks) or part of a *chunk* - the tasks up to the next commit point. The split is checked before anything starts: every path a task's steps touch must be in its `**Files:**` (a missing one refuses delegation, because the agent would have to step outside its allowlist), and a removed symbol still used later, a stale `file:line` anchor or a Verify path that does not exist shows as a warning. Each chunk carries a **Risk** - `high` when any of its tasks says `**Risk:** high` or its commit task says nothing (missing = high), else `low` - and the CI commands your repo runs go into every brief.

**One persistent implementer.** By default the run has ONE implementer (`impl-<RUN>`): after its first chunk is accepted it gets the next one as a new brief file, keeping everything it already knows. You pick its model at the first spawn (Opus by default, Fable for the hardest chunks, Sonnet or Haiku when the work is mechanical). When a report comes back you see the real diff under that agent's name and choose: accept it, send a correction (the same agent continues), or stop. Stopping an agent while it works ends it: the chunk then goes to a fresh agent that gets the brief and every correction so far, but not the first agent's memory. An agent's report is its claim and the diff is the fact: a plan step that leaves no trace in the diff or in a Verify output cannot be checked in review, so write delegated plans whose every step shows in one of them. A delegated run starts only on a clean working tree, and an interrupted run resumes from its record the next time you run `/lets:execute`.

**The team check.** Before any gate, every clean report is checked: an explorer compares the patch with the brief (every step done, nothing beyond it, CI output present and passing) on every chunk, and an Opus skeptic tries to refute it when Risk is high or missing. In a standing team's worktree the team's own explorer and skeptic do it; otherwise the run spawns its own.

**The launch plan and the gate policy.** Before Start, `/lets:execute` proposes how to run THIS plan - how many implementers and which chunks each owns, which run isolated, the order chunks land in, whether to pipeline, and the gate policy - each with a one-line reason, as the preview of the Start gate; you accept it or change any choice. The **gate policy** decides where you stop:

| Policy | Flag | You stop | Who accepts a chunk |
|---|---|---|---|
| `per-commit` (default) | `--gate per-commit` | at every chunk (your Review gate) | you |
| `high-only` | `--gate high-only` | at every Risk-high chunk, then a review of the finished run | you on Risk high, the team check on Risk low |
| `at-end` | `--gate at-end` | once, reviewing the finished run | the team check |

In every policy the explorer check always runs, the skeptic runs on Risk high, and a deviation, a blocked report, a failed tree check or a hard stop comes to you at once - never saved for the run review. Picking `high-only` or `at-end` at Start is your approval for the commits the lead makes without a gate; the record says, per chunk, who accepted and who committed. Under `high-only` and `at-end` the run ends in a **run review**: the commits of the run, each with its Risk and verdicts - accept the run, send a fix chunk, or stop (the commits stay, local and unpushed).

**Parallel groups** (`--parallel`). You declare which chunks each implementer owns; no file may belong to two groups ("file-disjoint is not independence" - a chunk that calls code another group adds belongs in the same group). Each group's implementer runs in its own harness-isolated worktree, switched to the task branch's HEAD once at spawn, and commits its chunk there. Each finished chunk lands in your tree through `lets integrate`: its commits are cherry-picked in a temporary worktree and applied here as ONE staged patch, verified against the picked tree - never a merge, never `reset --hard`; a conflict leaves your tree untouched and goes to the gate. A rejected, still uncommitted patch is taken back out with `lets integrate --revert`. At the end, an isolated group's worktree and branch are removed only when every one of its commits was integrated, and only on your yes.

**Pipelined** (`--pipelined`). The implementer commits each chunk locally and starts the next one before the previous is accepted; review then reads each commit by its sha, and a correction comes back as a `git commit --fixup`. Start is your approval for those local commits. At the end the fixups are folded in with a non-interactive `git rebase --autosquash` - only when `lets worktree pushed` finds none of the run's commits on any configured remote, and only on your yes.

Nothing in a delegated run pushes, opens a PR or touches the tracker - that stays `/lets:done`'s job.

## The full loop

```
/lets:start ─── /lets:plan ─── (review the plan) ─── /lets:execute ─── /lets:commit ─── /lets:done ─── /lets:end
```

If you want to check a plan before executing it, `/lets:check --plan` is a fast inline pass; `/lets:review --plan` is the full multi-agent version, with agents selected from signals in the plan content.

## See also

- **[workflow.md](workflow.md)** — where plan → execute fits in the day-to-day loop
- **[parallel-work.md](parallel-work.md)** — `/lets:team run` gives each of several tasks its own session running this flow
- **[code-review.md](code-review.md)** — reviewing a plan with `--plan`
- **[commands.md](commands.md)** — `/lets:plan`, `/lets:execute` flags
