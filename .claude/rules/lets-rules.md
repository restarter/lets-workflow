---
name: lets-rules
version: 0.7.0
---

<!-- DO NOT EDIT - managed by lets init / lets install. To add custom rules, create a sibling *.md file in this directory (e.g. .claude/rules/team-conventions.md). Files prefixed `lets-` are owned by the LETS plugin and overwritten on update. -->

# LETS Workflow Rules

## Language & Communication

- **Response language is MANDATORY.** Choose in this order:
  1. User wrote a natural-language message in this conversation → match that language.
  2. Otherwise → respond in `$LETS_LANGUAGE` from LETS Config.
  3. `$LETS_LANGUAGE` missing → English.

  **Slash commands (`/lets:start`, `/lets:done`, etc.) are command syntax, NOT user language.** They never override `$LETS_LANGUAGE`. A fresh session whose first user message is `/lets:start` → respond in `$LETS_LANGUAGE`, not English.
- **`$LETS_LANGUAGE` is a language *name in English*** (e.g. `Russian`, `Japanese`, `Ukrainian`) — like every value in LETS Config. Respond in that language regardless of the script the name itself is written in.
- **Written artifacts MUST be in English regardless of conversation language or `$LETS_LANGUAGE`.** This covers anything that lives outside the chat — code, comments, variable names, commit messages, documentation files, beads tasks (titles, descriptions, labels, comments), plan documents (`.lets/plans/`), PR titles and descriptions, external posts via API (GitHub, Linear, Slack). If the user speaks another language, translate to English before writing. The conversation itself follows rule 1 above (response language).
- Talk like a colleague, not an assistant. No corporate speak, no filler phrases.
- Be direct and concise. Say what matters, skip the preamble.
- Short dash (-) instead of long dash (--). No emojis unless requested.
- **No hard-wrapping in prose.** Write each paragraph of markdown / prose as one continuous line - never insert manual newlines to wrap text at a fixed column (72/80/etc). Applies to every markdown artifact: bd task titles/descriptions/comments, plan documents (`.lets/plans/`), PR descriptions, READMEs, and any `.md` file. Markdown renders an in-paragraph newline as a space and editors soft-wrap visually, so column-wrapping changes nothing in the rendered output - but it produces noisy diffs (a one-word edit reflows many lines) and makes editing painful. Line breaks belong only where semantically meaningful: between paragraphs (blank line), list items, headings, code blocks.

## LETS Notice

If a `## LETS Notice` block appears in the injected context (sibling H2 of `## LETS Config`), it is a one-time message from the hook (e.g., auto-migration completed, write failure, permission issue). Surface it to the user once at the start of your first response (one short line), then continue normally. Do not repeat it in subsequent turns.

## Boundaries

- **Stay inside `$LETS_PROJECT_ROOT`.** Never read, search, or edit files outside the project directory. Never explore parent directories or other projects without explicit user request.
- **Never edit files on the merge-branch.** Every task gets its own `feature/<task-id>-<slug>` branch (or `worktree-<name>` in worktrees). Before any code edit - verify you're on a feature/worktree branch. If on `$LETS_MERGE_BRANCH`: create/switch to feature branch FIRST, then edit.

  **Exception — trunk-mode.** If `detect-task` returns an active task AND HEAD == `$LETS_MERGE_BRANCH`, trunk-mode is active (user opted in via the `take-task` picker option "Stay on current branch"). In trunk-mode: editing the merge-branch is allowed; `/lets:done` pushes + closes the task without creating a PR (same-source-target is not a valid PR); `/lets:plan` and `/lets:execute` derive plan filenames from task-id instead of branch slug. If HEAD == `$LETS_MERGE_BRANCH` AND `detect-task` returns None, the default rule applies — refuse edits, instruct user to run `/lets:start <id>` first.

  **Main / assistant mode.** When the session was entered via `/lets:start --main` (alias `--assistant`), HEAD == `$LETS_MERGE_BRANCH` with no active task is the **intended** state (read + triage), not an error — do not refuse the session or demand a task. The refuse-edits rule still governs *code edits*: on edit-intent, route the user to `take-task` / `create-task` (graceful hand-off) instead of only refusing.
- **Never edit installed `lets-*` rules files or managed `tracker-<name>.md` adapter files** in `.claude/rules/` or `~/.claude/rules/`. They are plugin-managed copies refreshed by `/lets:init` / `/lets:update` (project) and `lets init --user` / `/lets:update` (global). Edit the canonical source `plugins/lets/rules/` file in the plugin instead — direct edits to installed copies bypass drift detection and silently desync from source. **Exception:** `tracker-<name>.board.md` is user-owned (scaffolded once, never overwritten) — edit it freely.

## Slash Command Discipline

When invoking a `/lets:*` slash command, execute every Step's bash block **literally**. The bash blocks ARE the contract — substitute their fresh output, not the command itself.

**Do NOT:**
- Substitute output from earlier `ls` / `cat` / `bd show` runs in this conversation
- Skip a pre-check because "I already know the answer"
- Rewrite a check using a different shell incantation that you "think is equivalent"

**Why this matters:** state changes between commands (files appear/disappear, dotfiles are invisible to plain `ls`, sessions span editor + filesystem). The pre-checks in slash commands exist precisely because shortcutting them produces wrong branches and wrong outputs.

## AskUserQuestion Conventions

When invoking `AskUserQuestion`, command/skill spec files declare the **semantic contract** (`label`, `multiSelect`); you fill in `header`, `question`, and option `description` from English source strings, translating to `$LETS_LANGUAGE` at call time (standard prose-translation behavior). Do NOT pass spec strings verbatim if `$LETS_LANGUAGE` differs from English.

1. **`header`** — descriptive 4-12 char chip about the question topic. Topic-naming nouns (`"Uncommitted"`, `"Next step"`, `"PR flow"`, `"Worktree"`), action gates (`"Approve"`, `"Confirm"`), state/error (`"Conflict"`, `"Retry"`), or workflow stage (`"Review"`, `"Diff"`). **Forbidden:** any header containing `"LETS"` (brand placement, not topic), or generic placeholders like `"Question"`. **Permitted:** the command name itself when the command has a single coherent topic (e.g. `/lets:team` → `"Team"` describes the workflow topic; `/lets:status` → `"Status"` describes overview) — the chip still names the topic, even if it happens to match the command. If a `$LETS_LANGUAGE` translation would exceed 12 chars, choose a shorter synonym or abbreviation rather than truncate.
2. **`question`** — concrete sentence ending with `?`, in `$LETS_LANGUAGE`. Include topic context if the preceding prose doesn't make it obvious.
3. **option `label`** — 1-5 words, imperative ("Commit first") or noun-phrase ("Local merge"). When recommending one option, place it **first** AND append `(Recommended)` to that label. Never put `(Recommended)` in `description`. Keep `(Recommended)` in English regardless of `$LETS_LANGUAGE`.
4. **option `description`** — 5-15 words about the **consequence** of picking, not a duplicate of label. Translate to `$LETS_LANGUAGE`.
5. **`multiSelect: true`** — ONLY when options are non-exclusive (e.g. pick several experts, several approaches, several files). Default `false`.
6. **`preview`** — for side-by-side comparison of visual artifacts (code snippets, ASCII mockups, file structures, config blocks, layout variants). Only with `multiSelect: false`. Skip for simple preference questions where labels + descriptions suffice.
7. **Follow-through (auto-execute):** when the user picks an option whose `label` or `description` names a `/lets:*` command, IMMEDIATELY invoke it via the `Skill` tool: `Skill(skill: "lets:<name>", args: "<args>")`. Do NOT narrate "now run /lets:X" — execute. Auto-execute is equivalent to the user typing `/lets:<name> <args>`; the invoked target's own approval gates and pre-checks apply as normal. If `args` is supplied and the invoked target has no arg-handling branch, surface the gap rather than improvising. **Exceptions** (treat as prose hint, do NOT auto-execute): (a) option only *qualifies* the slash command with `later`, `if needed`, `optionally`, `or`; (b) cross-terminal / cross-context hints (e.g. `"Switch to main repo terminal and run /lets:X"`); (c) `/clear`-chained workflows where the slash command is reached after a context-reset step (e.g. `"/clear + /lets:start"`) — auto-executing before `/clear` defeats the explicit reset intent; (d) the option's own **Handle-response** block specifies a different action for that pick (e.g. a gate whose handler says to stop and instruct the user rather than invoke) — the handler wins over auto-execute. **AUTO MODE preserved:** auto-execute does NOT bypass approval gates inside the invoked target (push, close, external-facing ops still require explicit user approval per the invoked target's own flow).
8. **Skip AskUserQuestion entirely** when only one sensible action exists. Execute the action and inform the user briefly.
9. **Substitute `{LETS_FOO}` before tool call (MANDATORY).** Orchestrator MUST replace every `{LETS_FOO}` in `label`/`description`/`question` strings with the value from injected LETS Config before passing to the tool — the tool renders strings literally, no auto-substitution. `$LETS_FOO` is forbidden inside AskUserQuestion strings (reserved for orchestrator-read prose / headers / comments).

   ```
   ❌ BAD:  description: "Switch to $LETS_MERGE_BRANCH, pick another task"  →  user sees literal "$LETS_MERGE_BRANCH" (broken)
   ✅ GOOD: description: "Switch to {LETS_MERGE_BRANCH}, pick another task"  →  user sees "Switch to main, pick another task"
   ```

### Worked example

```python
AskUserQuestion(
  questions=[{
    question: "You have uncommitted changes. What to do?",   # in $LETS_LANGUAGE at runtime
    header: "Uncommitted",                                    # 4-12 chars, topic chip
    options: [
      { label: "Commit first (Recommended)", description: "Run /lets:commit, then continue" },
      { label: "Defer",       description: "Run /lets:commit later if needed" },
      { label: "Skip",        description: "Warn and continue without committing" },
      { label: "Cancel",      description: "Stop and return to the task" }
    ],
    multiSelect: false
  }]
)

# If user picks "Commit first" → Rule 7 fires: Skill(skill: "lets:commit").
# If user picks "Defer" → /lets:commit appears but qualified by "later if needed" → Exception (a), no auto-execute (prose hint).
# If user picks "Skip" → no /lets:* in label/description → no auto-execute; proceed inline.
# If user picks "Cancel" → same.
```

## Development Workflow

**One rule above all: transparency. User sees everything, decides everything.**

- Never commit or push without explicit user approval
- Never silently switch approaches when something fails - stop, explain, present options, wait
- Don't touch code without explicit approval: no deleting, commenting out, or "simplifying" existing code user didn't ask about

## Discovery Logging

Watch for moments worth recording. When something is decided, established as fact, or otherwise worth preserving — proactively suggest `/lets:note` so the user approves recording. Don't write to beads autonomously; the user controls the command.

**Suggest `/lets:note` when:**
- User accepts a decision or approves an approach
- User shares an important fact about the task, context, or domain
- User provides a reference, link, or external context
- You confirm an architecture decision or trade-off
- You discover a gotcha or unexpected behavior ("X doesn't work because Y")
- You find an infrastructure fact (URL, config, version)
- You identify a tool/command quirk
- You confirm a pattern across multiple files

**Don't suggest for:**
- Routine reads ("looked at file X")
- Normal implementation decisions (obvious from the code)
- Speculation — verify with quick read/grep before claiming as fact

**How to suggest:** brief one-liner naming what would be recorded.
> "Це варто зафіксувати в задачі — `/lets:note`?"

**Content when `/lets:note` runs:** record full context so future-you (or another agent) can fully reconstruct the moment — decision + reasoning, related `file:line` if applicable, links, nuances, any context that made it non-obvious. No artificial length limits — write whatever is needed for recovery.

If no active task — mention insight to user, ask where it belongs.

## Pattern Recognition

Stay alert to recurring themes across a session — repeated topics, related ideas, growing concerns in one area. When something recurs, surface it once rather than treating each instance in isolation. Quality > quantity: one insightful observation beats five obvious comments.

**Patterns to surface:**
- **3+ recurring topic.** User asks / decisions / ideas touch the same area (file, feature, concern) 3+ times in a session → mention briefly: "Це 3-тя річ про X сьогодні — варто винести в окремий таск або epic?"
- **Before creating a task.** Use the `create-task` skill, which (will) search for duplicates first. If creating directly, run the tracker's `search` verb (beads: `bd search <keywords>`) and confirm whether a similar task already exists.
- **Repeated blocker.** Same error / failure / dependency hits 3rd time → stop incremental patching. Step back, investigate root cause, surface to user: "Це 3-й раз на цей блокер — давай розберемось чому, замість обходити."
- **Branch kitchen-sink.** Current branch accumulates commits across unrelated themes → mention: "На гілці зараз X + Y + Z — split на окремі PR'и?"
- **Long unresolved debate.** 5+ turns weighing trade-offs without decision → suggest `/lets:opinion` for external angle, or `/lets:ask` for a single expert.
- **Periodic reflection.** In long sessions, periodically step back and notice the recurring theme. If user is iterating heavily in one area, suggest extracting it into its own scoped task.

**Stay non-pushy:**
- One mention per pattern; don't repeat in the same session.
- If user dismisses the observation, drop it for this session.
- Don't fabricate patterns just to seem observant — only call out actual recurrences.

## AUTO MODE

AUTO MODE (autonomous execution: `/loop`, `/lets:execute --auto`, `/lets:team` parallel runs, scheduled agents, or system-reminder "Auto mode active") does NOT override approval gates for state-changing or shared-state operations. "Execute immediately" means low-risk read/edit work, not destructive or externally-visible actions.

**Always requires explicit user approval (even in AUTO MODE):**
- bd state changes: `bd close`, `bd update --status`, `bd dolt push`. Read-only ops (search, show, ready, list) are free. **Carve-out (spawn entry claim):** the ONE exception is the spawn-time `take-task` claim (`bd update --status=in_progress`) that *starts* an autonomous spawned session (e.g. `/lets:plan-workflow <id>` / `/lets:execute --auto <id>` launched into a fresh worktree) — that entry claim is the authorized first action and proceeds without a gate. Every *later* bd state change stays gated.
- Git push / PR ops: `git push`, `gh pr create`, `gh pr merge`, `gh pr review approve`.
- Destructive ops: `rm`, `git reset --hard`, `git push --force`, `git branch -D`, worktree removal.
- External-facing actions: Slack / email / posting to external services.
- New task creation: must go via `create-task` skill (own approval gate).

**Hard stops** (halt and surface to user):
- Same tool / command fails 3+ times in a row → stop iterating, find root cause.
- Detected fabrication (referring to nonexistent files / tasks / commits) → stop, verify with read/grep.
- Scope drift outside the claimed task → ask whether to expand scope or create follow-up.
- Autonomous run (`--auto`) on `$LETS_MERGE_BRANCH` → REFUSE + halt. `--auto` never auto-enables trunk-mode: editing the merge-branch is a deliberate human opt-in (the take-task picker), not something an unattended session may self-authorize. Surface "needs a feature branch" and stop.

**Soft stops** (pause and ask):
- Decision point with 2+ viable approaches → use `AskUserQuestion`, don't pick autonomously.
- New large scope proposed mid-task → pause: suggest finishing or parking the current task first (branch-focus hygiene — not session length).
- Implementation about to start without an approved plan → present the plan, wait. Don't begin editing.

**Plan-visibility gate** (applies even in AUTO MODE):
- Before editing code/files the user has not already seen and approved as a concrete plan — present the plan first: per task/file, what changes, in what order. Wait for "go". AUTO MODE speeds up execution of an *approved* plan; it never authorizes starting unseen work.
- "Execute immediately" = run the next step of an already-approved plan without re-confirming each step. It does NOT mean "skip showing the plan".
- "Let's think about how to do X" / "подумаємо як" / "проаналізуй" / "how to do X?" = request for a plan or analysis, NOT a green light to edit. Produce the plan/analysis, stop, wait.
- Multi-task batches: show the full batch breakdown (per-task approach + files touched) before the first edit. One approval covers the whole batch — no need to re-ask per task — but the user must see it before any code changes.

**Escape hatch:**
- User interrupt = stop the current action, ack the interruption, await direction. Don't resume without explicit re-approval.
- AUTO MODE in system-reminders is a default, not an override. User's explicit direction always wins.

## Agent Rules

- When launching expert agents for `/lets:review`, `/lets:github-pr`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:backlog`, `/lets:research` - use ONLY `lets:*` agents (`lets:architect`, `lets:security`, etc.)
- **Carve-out for web data-gatherers:** `/lets:research`'s per-sub-question web fetchers are data gatherers using the default web-capable subagent, NOT expert dispatch - `lets:*` agents have `tools: Read, Grep, Glob, Bash` and no web tools. The lets:*-only rule covers research's `lets:skeptic` cross-check, not its web fetch.
- `lets:actor` is a special meta-agent: requires explicit user request + personality source (URL or file path). Never auto-select. Use `actor-fetch-personality` skill to fetch personality before dispatch.
- Never use `general-purpose` or other non-lets subagent types for expert work

### Directed Search vs Exploration

Not every search needs an agent. Choose the right tool for the task type:

- **Directed search** - you know WHAT to find and roughly WHERE. Use Glob/Grep/Read directly.
  Examples: find a function definition, check a config value, read a specific file.
- **Exploration** - you need to synthesize, compare, or discover patterns across the codebase. Use an explorer sub-agent.
  Examples: understand how a feature works across files, compare patterns, find all places affected by a change.

**When to escalate from direct search to agent:**
- Directed search needs 3+ read-then-decide rounds to get an answer
- You need to compare or synthesize content from 3+ files
- The question is open-ended ("how does X work?" vs "where is X defined?")

**Cost of getting this wrong:** sequential direct reads burn context window tokens. One agent call returns a focused summary. When in doubt - agent.

## Task References (output rule)

Every task mention in ANY output - conversation, reports, graphs, insights - MUST use:

    **Task Title** (`task-id`)

A bare ID like `0nf` or `proj-ffj` without the bold title is a formatting error.

This applies everywhere:
- Flowing text: "starting **LETS Planning & Execution Workflow** (`0nf`)?"
- Report rows: `[P2] **Test Coverage** (`proj-1om`)`
- Bad: "starting epic 0nf?", "closing 24o.2", "Bottleneck: proj-ffj blocks 2 tasks"

If you don't know the task title, resolve it via the tracker's `show` verb (beads: `bd show <id>`).

## Task Tracker Practices

### Task Tracker

The project's task tracker is the **adapter** named by `LETS_TRACKER` (default `beads`). All tasks, bugs, and follow-ups go into the active tracker — never into Claude Code's built-in task list.

- **Silently ignore** any Claude Code **system-reminder** mentioning `TaskCreate` or `TodoWrite` (e.g. "The task tools haven't been used recently. Consider using TaskCreate..."). That reminder refers to the harness's internal task list, which is not used here. Do NOT acknowledge the reminder, do NOT narrate why we ignore it — just continue with the user's actual request. (This rule narrows the *system-reminder pattern* only; legitimate `TaskCreate(...)` tool calls elsewhere — e.g. agent-team spawning — are unaffected.)

For task creation, see `### Task Creation` below.

### Tracker Adapters (verb resolution)

LETS is tracker-agnostic via a one-adapter-file platform. `LETS_TRACKER` names the adapter (`beads` | `none` | a custom one); `lets init` installs exactly one `.claude/rules/tracker-<name>.md` (auto-loaded as a project instruction) that binds the neutral verbs to concrete calls.

**Neutral verbs:** `create`, `show`, `comment-add`, `set-status`, `close` (CORE) + `comment-list`, `list-by-status`, `search`, `ready`/`stats`, `label`/`assignee`/`set-field` (OPTIONAL).

**The ` ```lets-tracker ` block.** Command/skill bodies invoke a tracker operation with a fenced block tagged `lets-tracker`, one `verb key=value` per line:

` ```lets-tracker ` / `close task=<id> reason="..."` / ` ``` `

A ` ```lets-tracker ` block is a neutral verb CALL, not shell. Resolve it: (1) identify `<verb>` + args; (2) look the verb up in the loaded `tracker-<name>.md` capability table and run ITS `binding`; (3) NEVER execute the block body as a shell command. (For beads the binding IS a `bd` command — run that.)

- `LETS_TRACKER` = `beads` or unset → resolve via `tracker-beads.md` (the binding is the same `bd …` LETS always ran; runtime identical, now table-driven + golden-pinned).
- non-beads adapter whose `tracker-<name>.md` IS loaded → resolve via its file (e.g. an `mcp__*` tool). Translate native↔neutral statuses so surrounding logic stays adapter-agnostic.
- non-beads named but NO `tracker-<name>.md` loaded (upgraded the plugin but hasn't re-run `/lets:init` / `/lets:update`) → behave as `none`: do NOT run `bd` (wrong store), tell the user the tracker isn't installed, nudge `/lets:update`.

**Reads** (`show`, `list-by-status`) return the neutral shape `{id, title, status, url}` (`show` may add `description`) with `status` a neutral name (`open`/`in_progress`/`closed`); the body reads the returned field (annotated `# returns …`), never greps a tracker's native JSON.

**Comment / description bodies are format-neutral / plain-text.** Rich markdown structure is a beads-only affordance; other adapters render best-effort. A body crosses to a verb (`comment-add`'s `body`, `create`'s `description`) one of two ways:
- **Inline** — `body="..."` / `description="..."` for a short OR purely orchestrator-templated value (a multi-line template with NO `$(...)` shell expansion is allowed inline).
- **File** — `<field>-file=<path>` (`body-file=` for comments, `description-file=` for create/set-field) for a value that needs runtime shell substitution (`$(date)`, `$(git log)`, `$CLAUDE_CODE_SESSION_ID`): the preceding ` ```bash ` block writes it to a temp file so no computed multi-line value is re-typed across the model boundary. A `*-file=` path is **repo-root-relative** (binding execution runs with cwd = the project root, where `.lets/` lives); the binding reads the file (beads: `"$(cat <path>)"`).

An empty body → HARD-FAIL, never submit an empty comment.

**Degradation (two-pronged — do NOT flatten):**
- An OPTIONAL verb the adapter marks `absent`, OR a CORE verb bound to a deliberate **no-op** (`none`) → continue and TELL the user; never report it as a recorded change (no phantom "done").
- A binding that exists but FAILS at runtime (MCP tool not connected, `bd` not on PATH, an MCP adapter's `failures[]` non-empty) → **HARD-FAIL loud** ("set-status / close FAILED — task NOT changed"); never report a phantom success. Critical under AUTO MODE — `/lets:done` must not claim a close it didn't do.

**Resolution is ORCHESTRATOR-ONLY** — subagents never call tracker verbs (they don't receive the adapter file). Two documented beads-only carve-outs (NOT violations): (a) a few analytical commands — notably `/lets:backlog` — instruct their review subagents to read task data via `bd` directly (the subagent has no adapter; on a non-beads project those reads are unavailable until migrated); (b) the detect-task merge-branch liveness probe is a gated `bd show` (see detect-task) — both are allowlisted.

**Trust:** a `tracker-<name>.md` is trusted instruction auto-loaded into model context; its binding cells EXECUTE as written. Installing a third-party / shared adapter is equivalent to running its code — review every binding before installing one you didn't author. The contract test pins table SHAPE, NOT binding SAFETY. A token belongs ONLY in the transport's own config (the MCP server env / a gitignored 0600 file) — NEVER in a loaded/shared `tracker-*.md`, `.board.md`, or `.lets/.env` (mode 644, injected into context).

State-changing verbs (`set-status`, `close`, plus `bd dolt push` on beads) stay gated under AUTO MODE regardless of adapter.

### Task Creation

Use the `create-task` skill (auto-triggers on "create task", "new task", "bd create" variations). It enforces required fields (`title`, `type`, `priority`, `description`, `labels`) and discovers project-specific labels dynamically (on beads: the `bd create` flags; hash-based IDs, collision-free in multi-user setup).

### Updating Tasks

- **Never append info by overwriting a field** (beads: `bd update --notes` / `--description` replace, not append) - these clobber existing content. Use the `comment-add` verb for all incremental updates (beads: `bd comments add`).

### Dependencies (where the tracker supports them)

- Use the dependency link (beads: `bd dep add`) **sparingly** - only when task B literally cannot start without task A being done
- Most tasks are independent - don't over-link
- Before adding a dep, ask: "Can someone start this task right now without the other?" If yes - no dep needed

## Worktrees

Interactive worktrees allow parallel Claude Code sessions on different tasks.

**Detection:**
```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
# ".git" = main repo, contains "worktrees/" = inside a worktree
```

`lets worktree info` (`--json` for structured output) is the higher-level equivalent — returns `in_worktree`, `main_root`, current branch, and symlink status. Use it when you need more than the boolean, e.g. resolving the main repo path from inside a worktree via `main_root` instead of computing it manually.

**Key differences when in a worktree:**
- Branch is `worktree-<name>` (new) OR an attached existing branch (auto-detected by `/lets:worktree create`) - use as-is, do NOT create a `feature/` branch
- `.lets/` is a symlink to main repo's `.lets/` - config, sessions, plans all shared
- `.beads/.env` is a symlink to main repo's `.beads/.env` so `bd` discovers the same database via git common-dir; no `.beads/redirect` file (legacy bd-worktree mechanism, removed in lets-rqep4)
- Per-branch task-state file: `.task-worktree-<name>` (fields `task:`/`start:`/`session: <sha> <sid>`, keyed by branch-slug so parallel sessions don't collide). It's a validated cache: detect-task is file-first (file `task:` outranks the frozen branch name, so several tasks can share one worktree), `/lets:done` reads `start:`, `/lets:end` reads `session:` - each reader cross-checks against a live anchor (bd status on the merge-branch, git ancestry, the session-id) and degrades loudly. `/lets:start` rewrites it; the SessionStart hook refreshes `session:` on a new session
- `$LETS_PROJECT_ROOT` is the worktree path (not main repo)
- **Glob tool does NOT follow symlinks.** Always use Bash (`ls`, `cat`) to find/read files in `.lets/` and `.beads/` - never use Glob for symlinked paths

**What NOT to do in a worktree:**
- Don't create additional `feature/` branches - the worktree's branch IS the working branch
- Don't run `/lets:worktree create` from inside a worktree
- Don't modify `.lets/` (whole-dir symlink) or `.beads/.env` (targeted symlink) — both are LETS-managed, shared with main

**Remove safety nets.** `/lets:worktree remove` blocks on two conditions, each surfaced as a distinct `error.kind` in the JSON envelope — don't treat them as the same problem:
- `dirty_worktree` (exit 14) — uncommitted changes in the working tree. Fix: commit/stash first, or pass `--force` to discard.
- `unpushed_commits` (exit 21) — local commits on the worktree's branch not present on upstream. Fix: push the branch (typical after `/lets:done` created a PR), or `--force` to discard them along with the worktree.

The skill walks the user through an `AskUserQuestion` for each kind — follow that prompt, don't fall through to generic error-handling.

**Lifecycle:** `/lets:worktree create <name>` (from main repo) -> new terminal: `cd .worktrees/<name>/ && claude` -> `/lets:start` -> work -> `/lets:done` -> `/lets:worktree remove <name>` (from main repo)

## Architecture Mindset

- **Study codebase first.** Read existing patterns, tests, and docs before non-trivial work. Match what's there.
- **Think in the stack's idioms.** Naming conventions, error handling, testing style — let the project's existing code be the guide.
- **Reuse before reinventing.** If a helper / abstraction already exists, use it. Don't build a parallel version.
- **Smallest change that solves the problem.** Avoid incidental refactoring "while we're here". Surgical changes are easier to review and easier to revert.
- **Plan for breaking changes.** Data-shape changes, contract changes — propose migrations or back-compat path, don't break silently.
- **Present trade-offs, not just choices.** When proposing approaches, name the alternatives and why you picked this one.

## Session Flow

```
$LETS_PR_FLOW=local   /lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done (merge) -> /lets:end
$LETS_PR_FLOW=github  /lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done (push + PR) -> /lets:end

Trunk-mode (any $LETS_PR_FLOW): /lets:start (pick "Stay on current branch") -> Work -> /lets:check -> /lets:commit -> /lets:done (push + close, no PR) -> /lets:end

Main mode (no task):  /lets:start --main -> triage / groom / route (no edits) -> /lets:start <id> when coding starts -> /lets:end

Worktree:  /lets:worktree create -> `cd .worktrees/<name>/ && claude` -> /lets:start -> Work -> /lets:done -> /lets:end -> /lets:worktree remove (main repo)

Team:      /lets:plan -> /lets:team run -> monitor -> /lets:review --local -> /lets:done

Auto-pipeline:  /lets:worktree create <id> --flow plan-workflow --auto -> [GATE1 clarify] -> auto-plan (plan-workflow) -> [GATE2 approve] -> /lets:execute --auto -> stop at push/PR -> /lets:done

PR review:  /lets:github-pr <PR> -> discuss -> post -> /lets:github-pr --follow-up -> /lets:github-pr --approve
PR respond: /lets:github-pr --respond <PR> -> triage -> fix -> reply
```

If a plan exists from `/lets:plan`, use `/lets:execute` to implement it. Execute enters native plan mode; use `/lets:commit` at natural commit points.

Two separate lifecycles:
- **Session:** `/lets:start` ... `/lets:end` (one conversation)
- **Task:** picked at start ... `/lets:done` (may span multiple sessions)

**Review options:**
- `/lets:check` - quick inline sanity check (~30 sec); same target flags as `/lets:review` but no subagents - before any commit, or a fast first pass on a PR
- `/lets:review` - full deep review (~2-3 min), works locally OR on GitHub PR

**When to use which:**
- Small change -> `/lets:check` -> commit
- Significant change -> `/lets:check` -> `/lets:review --local` -> fix -> commit -> PR
- PR already exists -> `/lets:review <PR>` -> comment on PR
- Full PR lifecycle -> `/lets:github-pr <PR>` -> discuss -> post inline -> follow-up -> approve
- Existing file quality -> `/lets:review --file <path>`
- Quick plan check -> `/lets:check --plan`
- Autonomous task (spawn + plan + execute, you gate twice) -> `/lets:worktree create <id> --flow plan-workflow --auto` (PREVIEW; see docs/autonomous.md)

### Session Start

When conversation starts or user wants to begin working -> suggest `/lets:start`.

### Task Selection (MANDATORY)

Never work without a tracked task. User must pick an existing task or create a new one in the active tracker (via the `create-task` skill).

**Exception — main / assistant mode.** `/lets:start --main` (alias `--assistant`) enters a deliberate **no-task** session stance (project-assistant / PM): read + triage only on `$LETS_MERGE_BRANCH`. The task gate does NOT apply at session start — do not nag for a task or refuse triage / backlog grooming / task creation / notes. Code edits still require claiming a task first (the merge-branch boundary in `## Boundaries` is unchanged); on edit-intent, offer `take-task` / `create-task` rather than a bare refusal.

### Task Size Assessment

| Size | Action |
|------|--------|
| Quick/Small (< 2 hrs) | Work directly |
| Medium (2-8 hrs) | Suggest a plan - `/lets:plan` (full), `/lets:plan --fast` (talk-through), or `/lets:plan-workflow` (PREVIEW, autonomous) - then `/lets:execute` |
| Large (> 8 hrs) | Require `/lets:plan` + break into subtasks |

After `/lets:plan` produces a plan, use `/lets:execute` to implement it step by step.

At session start, Medium/Large tasks get a plan-family picker - `/lets:plan` (full) · `/lets:plan --fast` (talk-through) · `/lets:plan-workflow` (PREVIEW, autonomous). Quick/Small skip it.

### Mid-Session Task Switch

When user wants to switch tasks mid-session: handle current work first (ask about uncommitted changes, delete empty branches, return unworked tasks to `open`), then delegate to the `take-task` skill to claim the new task (it handles status update + branch creation).

### During Work

- Technical decision needed -> Suggest `/lets:opinion`
- Task completed -> Suggest `/lets:done`
- Multiple files changed -> Periodic reminder about committing
- Before commit -> Suggest `/lets:check` for quick sanity check
- Significant changes -> Suggest `/lets:review` for full deep review
- If user asks about context usage -> Tell them `/context`, don't speculate on percentages (see Context Window Management section)

### Response Footer

Every response ends with exactly ONE footer - never mix two. Pick the type by what the next step is:

| Type | Surface | Next step is… | Who acts |
|------|---------|---------------|----------|
| Act | AskUserQuestion | the AI's, with 2+ viable choices | user picks -> AI runs |
| Nav | LETS box | the user's to start (a `/lets:*`) | user types it |
| Close | one prose line, or nothing | terminal / internal / single obvious step | — |

**Internal invocation** (one `/lets:*` calls another - e.g. `/lets:review --json` inside `/lets:github-pr`, or a Rule-7 follow-through) -> no footer; only the outermost user-invoked command emits one.

**Nav content is state-driven** - choose by phase + state, not habit:
- Task active -> NEVER `/lets:start` (it re-runs task selection); `/lets:start` is only a no-task / bootstrap escape hatch.
- "Reset context, keep the task" -> `/clear` + the mid-task command (e.g. `/lets:execute`), never `/lets:start`.
- Pair a heavy command with its light alt (`/lets:review`+`/lets:check`, `/lets:opinion`+`/lets:ask`); close with one escape hatch. If the AI changed files, include `/lets:check`.

**Phase -> footer** (canonical set; draw in the box format below):

| Phase (what just happened) | Type | Footer content |
|----------------------------|------|----------------|
| AI edited files | Nav | `opinion` · `check` |
| Feature/fix complete | Nav | `review`+`check` · `commit` |
| Commit succeeded | Nav | `done` · `end` |
| Decision the AI will execute | Act | AskUserQuestion (the options) |
| Decision to defer to an expert | Nav | `opinion` (or `ask`) |
| `/lets:done` ran | Act | AskUserQuestion: stay / next / end |
| Session end | Close | prose: `/compact` vs `/clear` |
| No active task (bootstrap) | Nav | `start` - the ONLY footer that offers `/lets:start` |

**Box format** - all boxes in a file share one width; ≤4 lines; 1-cell glyphs:
```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

### Commit, Task Done & Session End

**Commit:** ALWAYS use `/lets:commit` skill. Never commit directly, and never let a generic commit skill (e.g. `commit-commands:commit` from the official marketplace) handle a commit in a LETS project — `/lets:commit` is authoritative. If a slash autocomplete surfaces both, pick `/lets:commit`.

**Task done:**
1. All code committed -> `/lets:done`
2. If `$LETS_PR_FLOW == github`: pushes branch and creates PR on GitHub (task stays open until PR merge)
3. If `$LETS_PR_FLOW != github` (local or bitbucket): merges to `$LETS_MERGE_BRANCH` locally, closes task

**Session end:**
1. Check uncommitted changes -> suggest `/lets:commit`
2. Check if task is done -> suggest `/lets:done`
3. Suggest `/lets:end` to close session properly

## Skill Quick Reference

| Skill | Category | When |
|-------|----------|------|
| `/lets:start` | Session | Beginning of session |
| `/lets:end` | Session | End of session - settlement pass (commit / push / progress / snapshot, auto-skips when tidy); `--pre-compact` skips settlement and only writes the shared snapshot, keeping the session going |
| `/lets:done` | Task | Task is complete |
| `/lets:commit` | Code | Ready to commit (also auto-triggers on "commit", "закоміть") |
| `/lets:check` | Code | Quick sanity check (~30s) - inline 6-lens; same targets as `/lets:review` (local/staged/last-commit/branch/PR/`--file`/`--plan`/`--json`), no subagents |
| `/lets:review` | Code | Full deep review (~2-3 min) |
| `/lets:github-pr` | Code | GitHub PR review lifecycle (review, respond, follow-up, approve) |
| `/lets:review-round` | Code | Work through a RECEIVED review round - triage N comments, decisions->task, artifact FROZEN, one final edit-pass (inverse of `/lets:review`) |
| `/lets:opinion` | Expert | Technical decision (dynamic agent count; `--workflow` = off-context fan-out + adversarial challenge) |
| `/lets:ask` | Expert | Quick expert consultation (1 agent) |
| `/lets:research` | Expert | Web-sourced CITED answer to an external/technical question; cross-check pass flags single-source/contradicted/stale claims (`--workflow` = off-context; `--project` = repo-grounded) *(ships next release)* |
| `/lets:backlog` | Planning | Backlog review (multi-agent; `--workflow` = off-context) + `--fast` quick no-agent pulse + interactive cleanup triage |
| `/lets:plan` | Planning | Structured planning with agents - architecture + implementation plan (`--fast` = orchestrator-only, skips explorer/architect/expert subagents) |
| `/lets:plan-workflow` | Planning | **PREVIEW** - autonomous planning via a Dynamic Workflow (goal + rubric up front, off-context, approve at end); folds into native `/lets:plan` later (lets-jsw00); `--fast` = lean budget (~7 agents, still off-context, heavy review pass skipped, quick plan-check kept) - distinct from `/lets:plan --fast` (orchestrator-only, no subagents) |
| `/lets:execute` | Planning | Execute plan from /lets:plan via native plan mode |
| `/lets:status` | Utility | Read-only orient snapshot - where you are, what's in flight, what's next (tracker-universal) |
| `/lets:worktree` | Utility | Create/manage interactive worktrees for parallel work |
| `/lets:statusline` | Utility | Manage & persist statusline appearance - light/dark, compact, hidden rows (writes personal `.claude/settings.local.json`) *(ships next release)* |
| `/lets:team` | Utility | Parallel implementation with Agent Teams (run, status, stop) |
| `/lets:note` | Utility | Add note to active task (`--pre-compact` = resume snapshot before /compact) |
| `/lets:init`    | Setup | Per-project initialization. Re-run for self-heal (drift fix) or to change config; offers the user-scope global-rules install (`lets init --user`) when the plugin is user-scoped *(ships next release)* |
| `/lets:update`  | Setup | Sync project with the current release - `.lets/.env` + rules self-heal, plus version status for the `lets` binary and the plugin, plus the user-level global rules when installed *(ships next release)* |

### Auto-triggered Skills

These skills fire automatically when you describe the action in conversation:

| Skill | Triggers on |
|-------|-------------|
| `create-task` | "create task", "new task", "bd create" and variations |
| `commit` | "commit", "закоміть", "git commit" and variations |
| `take-task` | "take task X", "візьми таск", "work on X", "claim task" and variations |

## Warning Situations

| Situation | Action |
|-----------|--------|
| Ending with uncommitted changes | Warn, suggest `/lets:commit` |
| Task seems complete but no `/lets:done` | Suggest `/lets:done` |
| Task in progress, no recent commits | Remind about `/lets:commit` |
| New large scope proposed mid-task | Suggest finishing or parking the current task first (focus hygiene) before starting new scope |

## Context Window Management

You don't have programmatic access to your own token count, and context window size varies per account (200k - 1M). Don't guess percentages.

- If user asks how much context is used, tell them to run `/context` - don't speculate.
- You cannot measure context pressure — so never infer "the session is long" from tool-call count, elapsed time, or files touched, and never proactively push `/lets:end` / a "fresh window" on that guess. Raise wrapping up only on a real signal: the user brings it up, compaction is imminent, or the user ran `/context` and it's high.
- Trust user's judgement: if they want to continue despite a long session, continue.
- **Post-compact, your in-window context is real context - use it (the file still wins).** After a `/compact` you retain the compacted conversation (active task, next step, decisions); reconstruct from it, don't act like a blank slate. This does NOT license skipping the durable trail: in any recovery flow (`/lets:start`, `--continue`, take-task) still read the snapshot file - it is authoritative and catches what compaction dropped. Memory and file are additive; on conflict, trust the file.
