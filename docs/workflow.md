# Workflow

LETS gives Claude Code a structure to work inside: every session starts with a task, every commit links to it, and context survives across sessions and conversation compaction. This page walks through the day-to-day loop.

## The session loop

```
/lets:start ─── choose how to work ─── /lets:commit ─── /lets:done ─── /lets:end
```

**`/lets:start`** — restores context from your last session (what you did, what's next), shows the tasks you can pick up, and creates a feature branch for the one you choose. You always work on a task; if you don't pick one, you'll be asked to. Branches are named `feature/<task-id>-<slug>`.

**Choose how to work** — see [the three ways to work](#three-ways-to-work) below.

**`/lets:commit`** — reviews your changes and creates a conventional commit (`feat:`, `fix:`, `docs:`, …) with the task ID in the scope and a `Task:` footer. Use this instead of `git commit` directly — it keeps the format consistent and links the commit to the task.

**`/lets:done`** — finishes the task. In GitHub or Bitbucket mode it pushes the branch and opens a PR - via `gh` or `bbb` - and the task stays open until the PR merges. Before shipping it checks the work against the task's scope and offers a CHANGELOG entry; afterwards it reads the PR back. Details in **[commands/done.md](commands/done.md)**. In local mode it merges to your merge branch and closes the task.

**`/lets:end`** — settles the session: it asks once about what is left unsettled (uncommitted changes, unpushed commits, a progress comment, an open task - which it refers to `/lets:done`, never finishing it itself), stays silent when everything is tidy, and always saves a session snapshot so the next conversation picks up where you left off. Details in **[sessions.md](sessions.md)**.

> Two separate lifecycles: a **session** is one conversation (`/lets:start` … `/lets:end`); a **task** is picked at start and finished with `/lets:done` — it can span several sessions. When you return to an unfinished task, `/lets:start` restores its context from the latest session snapshot file first, then the task's comments. The snapshot is local (`.lets/sessions/`, gitignored), so a teammate on another machine gets the comments only.

## Three ways to work

Once you've started a session, pick the approach that fits the task.

### You write the code

Write code with Claude in the conversation. Helpers along the way:

| Command | What it does |
|---------|--------------|
| `/lets:opinion` | Technical decision analyzed by expert agents in parallel, with a recommendation |
| `/lets:ask` | Quick question to a single expert agent |
| `/lets:check` | Sanity check before a commit — 6 perspectives, inline, no subagents |
| `/lets:review` | Full multi-agent code review + an adversarial verify pass |

### You plan, Claude builds

For anything non-trivial — design first, then implement:

| Command | What it does |
|---------|--------------|
| `/lets:backlog` | Backlog review (multi-agent), `--fast` quick no-agent pulse, + cleanup triage |
| `/lets:plan` | Design how to build it — codebase exploration, architecture, a written plan |
| `/lets:execute` | Claude implements the plan after your approval - straight through by default, or pausing after each task if you pick step-by-step |

See **[plan-execute.md](plan-execute.md)** for the full flow.

### Agents work in parallel

| Command | What it does |
|---------|--------------|
| `/lets:team` | Run several tasks at once - one worktree and one visible LETS session per task, on any launcher - and manage a standing team's members |
| `/lets:worktree` | Open parallel sessions in separate terminals — you drive each one |

See **[parallel-work.md](parallel-work.md)**.

For the hands-off version — autonomous spawn → plan → execute, and off-context `--workflow` runs — see **[autonomous.md](autonomous.md)**.

### An orchestrator and its workers

Several chats on one repo can coordinate without you carrying messages. Start one or more orchestrators with `/lets:start --main` (give each session a name with `/rename`, and optionally `--scope "<part of the repo>"`). Worker chats are bound to one of them: `/lets:worktree create <id>` run from an orchestrator binds every worker it spawns, and a chat you open by hand takes `/lets:start <id> --orc=<name>`. From then on a worker uses `/lets:orc ask` / `ping` / `read`, `/lets:done` offers to ping the orchestrator about the PR, and an execute deviation or an undecided `/lets:opinion` offers to ask it. See **[commands/orc.md](commands/orc.md)**, and **[messaging.md](messaging.md)** for all three ways to reach another session or agent - peers, other projects' orchestrators, and hand-off briefs for Codex or Antigravity.

## How LETS keeps Claude on track

**SessionStart + PreCompact hooks.** On every Claude Code conversation, `lets hook session-start` (and `lets hook precompact`) runs and emits a small `## LETS Config` block — plus a notice if your rules file is out of date. The workflow rules themselves live in `<project>/.claude/rules/lets-rules.md` (copied there by `/lets:init`, re-synced by `/lets:update` when a new release ships), which Claude Code loads as project instructions. SessionStart fires on new, resumed, cleared, and compacted sessions; PreCompact makes sure the rules survive when a long session gets compacted. This is what makes Claude follow the workflow without you having to remind it.

**LETS boxes.** After key actions, Claude shows a small box with the most likely next steps, so you always know what to do next:

```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

**One footer per response.** Every response ends with exactly one of three things, chosen by whose move is next: a **question** with options when the next step is Claude's and there is a real choice (you pick, Claude runs it); a **LETS box** when the next step is yours to start; or a single **prose line** (or nothing) when the step is obvious or the work is finished. Picking an option that names a `/lets:*` command runs that command right away - with that command's own approval gates still in place.

## What Claude does on its own - and never does

The rules file (`.claude/rules/lets-rules.md`) is the source of truth; this is the short version of what it makes Claude do.

| Claude will | Claude never |
|-------------|--------------|
| Show the plan - per file, what changes, in what order - before editing anything you have not already approved | Treats "ok" on a plan, an analysis or a review verdict as permission to write code - a plan is only entered through `/lets:execute` |
| Stop and lay out the options when an approach fails or reality diverges from the plan | Silently switches approaches, or deletes / comments out / "simplifies" code you did not ask about |
| Suggest `/lets:note` when a decision, fact or gotcha is worth keeping | Writes to the tracker on its own |
| Mention a pattern once - the third time a topic or blocker recurs, a branch collecting unrelated themes, a debate running past five turns | Repeats the nudge after you dismiss it, or invents a pattern to look observant |
| Tell you to run `/context` when you ask how full the context is | Guesses a percentage, or pushes you to wrap up because the session "feels long" |
| Ask a concrete question with a recommended option first when there are two real choices | Asks when only one sensible action exists - it does it and tells you |

**AUTO MODE does not lift the gates.** Under `/lets:execute --auto`, `/loop`, a team run or any autonomous mode, these still need your explicit approval: tracker state changes (claim, status, close, sync), `git push` and every PR operation, destructive operations (`rm`, `git reset --hard`, force push, branch or worktree removal), anything posted to an external service, messages to peer sessions, and new task creation. An autonomous run halts on three failures of the same command in a row, on a reference to something that does not exist, on scope drifting outside the task, on a deviation from the approved plan, and refuses outright to run on the merge branch.

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
- **[messaging.md](messaging.md)** — talking to other sessions and agents
