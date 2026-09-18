# Command reference

Every `/lets:*` command, grouped. For the day-to-day flow see **[workflow.md](workflow.md)**.

## Session & task

| Command | What it does |
|---------|--------------|
| `/lets:start` | Start a session — restore context, show tasks, claim one and prepare its branch. `/lets:start <task-id>` jumps to a task; `--continue` resumes the in-progress one; `--main` (alias `--assistant`) is the no-task project-assistant stance on the merge branch - triage, groom, route, no edits - and registers the session as an orchestrator, with `--scope "<part>"` naming the part of the repo it owns; `<task-id> --orc=<name>` binds a worker chat to that orchestrator. |
| `/lets:end` | End the session — one settlement pass: asks once about what is unsettled (uncommitted changes -> `/lets:commit`, an open task -> referred to `/lets:done`, unpushed commits -> push, a progress comment), then always writes a snapshot for next time. Silent when the session is tidy; it never finishes or closes a task itself. `--session` (aliases `--snapshot`, `--pre-compact`, `--compact`) writes only the snapshot and keeps the session going, whether or not a `/compact` follows. See [sessions.md](sessions.md). |
| `/lets:commit` | Commit changes — review, conventional commit message, task ID in the scope and a `Task:` footer. Staging rule: an already-staged set is committed as-is; otherwise only the reviewed files are staged by name - never `git add -A`. Use this instead of `git commit`. Also auto-triggers on "commit" in conversation. |
| `/lets:done` | Finish the task — checks the work against the task's scope (Fix first / Update scope / PR only, keep open), offers a CHANGELOG `[Unreleased]` entry, then pushes the branch and opens a PR (`github` via `gh`, `bitbucket` via `bbb`; the task stays open until the PR merges, and the PR is read back - a conflicting PR means CI will not run), or merges locally and closes the task (`local`). After a GitHub PR it offers **Merge & close**. In trunk-mode (on the merge branch) it pushes and closes, no PR. See [commands/done.md](commands/done.md). |
| `/lets:status` | Read-only orient snapshot — where you are, what's in flight, what's next (tracker-universal). Shows a Peers block when other sessions of the repo are live. |
| `/lets:orc` | Talk to this chat's orchestrator or a named peer session — `ask`, `ping`, `read`, `tell`, `who`; the only sender of peer messages. See [commands/orc.md](commands/orc.md). |
| `/lets:peer` | `/lets:peer <name> <ask\|ping\|read\|tell\|who> [text]` — the alias of `/lets:orc` with a named target. |
| `/lets:hub` | Orca addon (`LETS_LAUNCHER=orca`): lists the orchestrators of every project Orca knows, asks a stopped one a read-only question through a narrowed headless fork (plan mode, Read/Grep/Glob, no MCP), and wakes one in a visible Orca terminal for anything that needs its gates. |
| `/lets:note` | Add a note to the active task — a decision, gotcha, fact, or reference. `/lets:note <free text>` uses the text directly; bare `/lets:note` asks for the note type. `--session` (same aliases) writes the same recovery snapshot `/lets:end` does, without ending anything. |

See **[tasks.md](tasks.md)** for the task lifecycle.

## Planning & execution

| Command | What it does |
|---------|--------------|
| `/lets:backlog` | Backlog management — multi-agent review (`review`), a quick no-agent pulse (`--fast`), or interactive cleanup triage (`cleanup`). The keyword/flag skips the menu; `review --workflow` runs the fan-out off-context. |
| `/lets:plan` | Structured planning — codebase exploration with scaled explorer agents, then architecture design with expert evaluation, then a written plan in `.lets/plans/`. `--fast` skips the subagent phases and plans in-conversation. `--idea` writes a concept document instead (the wish, triggers, constraints, open questions; no code) that `/lets:execute` never runs and a later `/lets:plan` reads as input. |
| `/lets:plan-workflow` | PREVIEW — autonomous planning via a Dynamic Workflow (goal + rubric up front, off-context, approve at the end). `--fast` = lean budget (~7 agents, still off-context) — distinct from `/lets:plan --fast` (no subagents, in-conversation). Before launch a budget panel shows the agents per stage and the model; lower any stage or pick another model (cuts are listed with the plan). |
| `/lets:execute` | Execute the plan from `/lets:plan` in native plan mode: `/lets:execute [task-id\|plan-path]`, one approval, then a run-mode picker - straight-through (default), step-by-step, auto or team. `--straight` / `--step` / `--auto` / `--team` pre-answer the picker; commit cadence follows the mode (step-by-step confirms each commit, straight-through and auto commit at the plan's commit points, team commits once per agent). `--auto` runs unattended (push/PR/`bd close`/external still gated; refused on the merge branch). `--status` shows progress on the current plan. |
| `/lets:team` | Parallel implementation with Agent Teams — `run` (pick tasks, spawn teammates; `run --tasks A,B,C` names them up front), `status`, `stop`. `--backend orca\|agents`: `orca` (the Orca addon, offered only with `LETS_LAUNCHER=orca` and the app running) runs each task as a visible LETS session in an Orca child worktree under Orca's supervised orchestration. |
| `/lets:worktree` | Create and manage worktrees for parallel sessions — `create <name>`, `list`, `info`, `remove <name>`. `create <name> --branch <ref>` attaches an existing branch (slash refs like `feature/x` work); attach vs new branch is auto-detected, `--attach` / `--new-branch` force it. `create <id> --flow plan\|plan-workflow` picks the command the new session lands in, `--auto` launches it with `--permission-mode auto`; together they spawn the autonomous task pipeline. `--orca` / `--no-orca` (like `--cmux` / `--tmux`) override the launcher for one run; `remove` stops on uncommitted changes or unpushed commits (force only after you confirm) and offers to sweep task branches already merged into `origin/<merge-branch>`. |

See **[plan-execute.md](plan-execute.md)**, **[parallel-work.md](parallel-work.md)**, and **[autonomous.md](autonomous.md)**.

## Review & analysis

| Command | What it does |
|---------|--------------|
| `/lets:check` | Inline sanity check — 6 perspectives, the orchestrator alone, no subagents. Targets: `--local` (working tree, the default), `--staged`, `--last-commit`, `--branch` (three-dot vs `origin/<merge-branch>`; the local branch only without an origin copy), `<PR>`, `--file <path>`, `--plan [<path>]`. `--spec <path>\|<task-id>\|none` names what to judge against; `--json` prints structured output. |
| `/lets:review` | Full code review — dynamic expert-agent selection, then an adversarial verify pass. Same targets as `/lets:check` (`--local`, `--staged`, `--last-commit`, `--branch`, `<PR>` on GitHub or Bitbucket, `--file <path>`, `--plan [<path>]`); bare `/lets:review` asks. `--spec <path>\|<task-id>\|none`, `--json`, and `--workflow` to run the fan-out off-context with the same verified findings. |
| `/lets:github-pr` | GitHub PR review lifecycle — `<PR>` to analyze and discuss, then post inline; no argument resumes the saved review; `--follow-up` to check fixes; `--approve` to approve; `--merge` to merge; `--respond [PR]` for the PR author; `--status` shows the review state; `--cancel` drops it. |
| `/lets:review-round` | Work through a RECEIVED review round — sources: inline `<!-- REVIEW` markers in an annotated copy, a review file, or a PR's threads (for a GitHub PR the per-comment summary is handed to `/lets:github-pr --respond` to post the replies). Triage each comment (accept/reject/defer/done), record decisions to the task, keep the artifact frozen, then apply all edits in one final pass. The inverse of `/lets:review`. |
| `/lets:handoff` | Hand the current state OUT — one self-contained brief for an agent with no context (a fresh session, Codex, Antigravity, any external reviewer), covering a plan, a branch, uncommitted or staged work, the last N commits, a range, a PR, or a file. Same target selectors as `/lets:review`, plus handoff-only `--commits <N>` / `--range <a>..<b>`. Without a delivery flag it prints the brief, reviews nothing and writes no file, and works in a repo with no LETS installed. `--codex` runs the brief through Codex headless (read-only sandbox); `--send [<tab>]` types a one-line pointer to it into an agent tab of this worktree (Orca). Either way the agent's final report is relayed whole as UNVERIFIED and each finding is checked against the code. The old name `/lets:review-handoff` still works as a deprecated alias and will be removed in a future release. |
| `/lets:opinion` | Technical decision analyzed by expert agents in parallel, with a recommendation. Dynamic agent count. `--workflow` runs it off-context. |
| `/lets:ask` | Quick consultation with a single expert agent — like pinging a colleague. `/lets:ask <expert> <question>` goes straight to one (`security`, `architect`, `docs`, ...); bare `/lets:ask` offers the most relevant four. |
| `/lets:research` | Answer an external/technical question with a CITED web synthesis — decompose, search + fetch sources, cross-check flags weak/contradicted claims, then a sourced answer + Sources list + as-of date. `--workflow` off-context; `--project` grounds against this repo. Deep dive: **[commands/research.md](commands/research.md)**. |

See **[code-review.md](code-review.md)** and **[agents.md](agents.md)**.

## Setup

| Command | What it does |
|---------|--------------|
| `/lets:init` | Initialize LETS in the current project — creates `.lets/`, writes `.lets/.env` with defaults, copies the workflow rules to `.claude/rules/lets-rules.md`, wires up the statusline, and runs `bd init` if beads is installed. Re-run anytime to self-heal drift or change config. With a user-scope plugin install it also offers `lets init --user` — global rules to `~/.claude/rules/` + personal defaults to `~/.lets/.env`. |
| `/lets:update` | Sync the project with the current release — a single self-driving loop: run it, do the one thing it says, re-run until `✓ Everything on vX.Y.Z`. Self-heals `.lets/.env` and the rules file (plus the user-level global rules when installed — never overwriting a customized copy); rules sync is **deferred** while the plugin is behind (no stale half-step). The binary step installs in-session on approval; the plugin step is a Claude Code slash command. |
| `/lets:statusline` | Choose and persist the statusline look — light/dark, compact, which rows to show. `show` prints the saved appearance, `reset` returns to the defaults. Writes your personal `.claude/settings.local.json`; restart Claude Code to apply. See [statusline.md](statusline.md). |

See **[installation.md](installation.md)** and **[configuration.md](configuration.md)**.

## Lighter alternatives

Some commands have a faster, lower-cost counterpart — reach for the light one first:

| Heavy | Lighter |
|-------|---------|
| `/lets:review` | `/lets:check` |
| `/lets:review --plan` | `/lets:check --plan` |
| `/lets:opinion` | `/lets:ask` |
