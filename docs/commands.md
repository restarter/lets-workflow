# Command reference

Every `/lets:*` command, grouped. For the day-to-day flow see **[workflow.md](workflow.md)**.

## Session & task

| Command | What it does |
|---------|--------------|
| `/lets:start` | Start a session — restore context, show tasks, create a feature branch. `/lets:start <task-id>` jumps to a task; `/lets:start --continue` resumes the in-progress one. |
| `/lets:end` | End the session — save progress, sync the task tracker, write a snapshot for next time. `--session` (alias `--snapshot`) writes only the snapshot and keeps the session going; `--pre-compact` does the same, branded as a record written before a `/compact`. |
| `/lets:commit` | Commit changes — review, conventional commit message, task ID in the scope and a `Task:` footer. Use this instead of `git commit`. Also auto-triggers on "commit" in conversation. |
| `/lets:done` | Finish the task — push the branch and open a PR (GitHub mode), or merge locally and close the task (local/bitbucket). |
| `/lets:status` | Read-only orient snapshot — where you are, what's in flight, what's next (tracker-universal). |
| `/lets:note` | Add a note to the active task — a decision, gotcha, fact, or reference. `--session` and `--pre-compact` write the same recovery snapshot `/lets:end` does, without ending anything. |

See **[tasks.md](tasks.md)** for the task lifecycle.

## Planning & execution

| Command | What it does |
|---------|--------------|
| `/lets:backlog` | Backlog management — multi-agent review (`review`), a quick no-agent pulse (`--fast`), or interactive cleanup triage (`cleanup`). The keyword/flag skips the menu; `review --workflow` runs the fan-out off-context. |
| `/lets:plan` | Structured planning — codebase exploration with scaled explorer agents, then architecture design with expert evaluation, then a written plan in `.lets/plans/`. `--fast` skips the subagent phases and plans in-conversation. |
| `/lets:plan-workflow` | PREVIEW — autonomous planning via a Dynamic Workflow (goal + rubric up front, off-context, approve at the end). `--fast` = lean budget (~7 agents, still off-context) — distinct from `/lets:plan --fast` (no subagents, in-conversation). |
| `/lets:execute` | Execute the plan from `/lets:plan` in native plan mode, with your approval at each step. `--auto` runs an approved plan without per-step gates (push/PR/`bd close`/external still gated). |
| `/lets:team` | Parallel implementation with Agent Teams — `run` (pick tasks, spawn teammates), `status`, `stop`. |
| `/lets:worktree` | Create and manage worktrees for parallel sessions — `create <name>`, `list`, `remove <name>`. `create <id> --flow plan-workflow --auto` spawns the autonomous task pipeline. |

See **[plan-execute.md](plan-execute.md)**, **[parallel-work.md](parallel-work.md)**, and **[autonomous.md](autonomous.md)**.

## Review & analysis

| Command | What it does |
|---------|--------------|
| `/lets:check` | Inline sanity check — 6 perspectives, the orchestrator alone, no subagents. Targets: working tree, staged, last commit, full branch (`--branch`, three-dot vs `$LETS_MERGE_BRANCH`), a PR, `--file <path>`, `--plan`. |
| `/lets:review` | Full code review — dynamic expert-agent selection, then an adversarial verify pass. Same targets as `/lets:check`; `--local` for local changes, `--branch` for the whole branch (PR-equivalent), `<PR>` for a PR, `--plan` for a plan. `--workflow` runs the fan-out off-context. |
| `/lets:github-pr` | GitHub PR review lifecycle — `<PR>` to analyze and discuss, then post inline; `--follow-up` to check fixes; `--approve` to approve; `--respond <PR>` for the PR author. |
| `/lets:review-round` | Work through a RECEIVED review round — triage each comment (accept/reject/defer/done), record decisions to the task, keep the artifact frozen, then apply all edits in one final pass. The inverse of `/lets:review`. |
| `/lets:review-handoff` | Hand the current state OUT — one self-contained brief for an agent with no context (a fresh session, Codex, any external reviewer), covering a plan, a branch, uncommitted or staged work, the last N commits, a range, a PR, or a file. Same target selectors as `/lets:review`, plus handoff-only `--commits <N>` / `--range <a>..<b>`; reviews nothing itself and writes no file. Works in a repo with no LETS installed. |
| `/lets:opinion` | Technical decision analyzed by expert agents in parallel, with a recommendation. Dynamic agent count. `--workflow` runs it off-context. |
| `/lets:ask` | Quick consultation with a single expert agent — like pinging a colleague. |
| `/lets:research` | Answer an external/technical question with a CITED web synthesis — decompose, search + fetch sources, cross-check flags weak/contradicted claims, then a sourced answer + Sources list + as-of date. `--workflow` off-context; `--project` grounds against this repo. Deep dive: **[commands/research.md](commands/research.md)**. |

See **[code-review.md](code-review.md)** and **[agents.md](agents.md)**.

## Setup

| Command | What it does |
|---------|--------------|
| `/lets:init` | Initialize LETS in the current project — creates `.lets/`, writes `.lets/.env` with defaults, copies the workflow rules to `.claude/rules/lets-rules.md`, wires up the statusline, and runs `bd init` if beads is installed. Re-run anytime to self-heal drift or change config. With a user-scope plugin install it also offers `lets init --user` — global rules to `~/.claude/rules/` + personal defaults to `~/.lets/.env`. |
| `/lets:update` | Sync the project with the current release — a single self-driving loop: run it, do the one thing it says, re-run until `✓ Everything on vX.Y.Z`. Self-heals `.lets/.env` and the rules file (plus the user-level global rules when installed — never overwriting a customized copy); rules sync is **deferred** while the plugin is behind (no stale half-step). The binary step installs in-session on approval; the plugin step is a Claude Code slash command. |

See **[installation.md](installation.md)** and **[configuration.md](configuration.md)**.

## Lighter alternatives

Some commands have a faster, lower-cost counterpart — reach for the light one first:

| Heavy | Lighter |
|-------|---------|
| `/lets:review` | `/lets:check` |
| `/lets:review --plan` | `/lets:check --plan` |
| `/lets:opinion` | `/lets:ask` |
