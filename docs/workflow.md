# Workflow

LETS gives Claude Code a structure to work inside: every session starts with a task, every commit links to it, and context survives across sessions and conversation compaction. This page walks through the day-to-day loop.

## The session loop

```
/lets:start ─── choose how to work ─── /lets:commit ─── /lets:done ─── /lets:end
```

**`/lets:start`** — restores context from your last session (what you did, what's next), shows the tasks you can pick up, and creates a feature branch for the one you choose. You always work on a task; if you don't pick one, you'll be asked to. Branches are named `feature/<task-id>-<slug>`.

**Choose how to work** — see [the three ways to work](#three-ways-to-work) below.

**`/lets:commit`** — reviews your changes and creates a conventional commit (`feat:`, `fix:`, `docs:`, …) with the task ID in the scope and a `Task:` footer. Use this instead of `git commit` directly — it keeps the format consistent and links the commit to the task.

**`/lets:done`** — finishes the task. In GitHub mode it pushes the branch and opens a PR (the task stays open until the PR merges). In local/bitbucket mode it merges to your merge branch and closes the task.

**`/lets:end`** — saves a session summary so the next conversation picks up where you left off.

> Two separate lifecycles: a **session** is one conversation (`/lets:start` … `/lets:end`); a **task** is picked at start and finished with `/lets:done` — it can span several sessions. When you return to an unfinished task, `/lets:start` restores its context from the task's comments.

## Three ways to work

Once you've started a session, pick the approach that fits the task.

### You write the code

Write code with Claude in the conversation. Helpers along the way:

| Command | What it does |
|---------|--------------|
| `/lets:opinion` | Technical decision analyzed by expert agents in parallel, with a recommendation |
| `/lets:ask` | Quick question to a single expert agent |
| `/lets:check` | Quick sanity check before a commit — 6 perspectives, inline, ~30s |
| `/lets:review` | Full multi-agent code review (~2-3 min) |

### You plan, Claude builds

For anything non-trivial — design first, then implement:

| Command | What it does |
|---------|--------------|
| `/lets:brainstorm` | Quick ideation on a topic — fast context scan, no agents |
| `/lets:backlog` | Backlog review (multi-agent) + cleanup triage |
| `/lets:plan` | Design how to build it — codebase exploration, architecture, a written plan |
| `/lets:execute` | Claude implements the plan, with your approval at each step |

See **[plan-execute.md](plan-execute.md)** for the full flow.

### Agents work in parallel

| Command | What it does |
|---------|--------------|
| `/lets:team` | Spawn autonomous agents that implement several tasks at once, each in its own worktree |
| `/lets:worktree` | Open parallel sessions in separate terminals — you drive each one |

See **[parallel-work.md](parallel-work.md)**.

## How LETS keeps Claude on track

**SessionStart + PreCompact hooks.** On every Claude Code conversation, `lets hook session-start` (and `lets hook precompact`) runs and emits a small `## LETS Config` block — plus a notice if your rules file is out of date. The workflow rules themselves live in `<project>/.claude/rules/lets-rules.md` (copied there by `/lets:init`, re-synced by `/lets:update` when a new release ships), which Claude Code loads as project instructions. SessionStart fires on new, resumed, cleared, and compacted sessions; PreCompact makes sure the rules survive when a long session gets compacted. This is what makes Claude follow the workflow without you having to remind it.

**LETS boxes.** After key actions, Claude shows a small box with the most likely next steps, so you always know what to do next:

```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

## When to use which review

| Change | Path |
|--------|------|
| Small | `/lets:check` → `/lets:commit` |
| Significant | `/lets:check` → `/lets:review --local` → fix → `/lets:commit` → PR |
| PR already exists | `/lets:review <PR>`, or the full `/lets:github-pr <PR>` lifecycle |
| Quick plan check | `/lets:check --plan` |

More in **[code-review.md](code-review.md)**.

## See also

- **[commands.md](commands.md)** — every `/lets:*` command in one place
- **[tasks.md](tasks.md)** — how task tracking works
- **[plan-execute.md](plan-execute.md)** — the plan → execute flow
- **[configuration.md](configuration.md)** — settings and file layout
