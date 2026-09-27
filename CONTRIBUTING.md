# Contributing to LETS Workflow

Thanks for taking the time. This repo is a Claude Code plugin (`plugins/lets/`) plus its companion Go CLI (`cli/`) and some infrastructure scripts. It's small and opinionated — read this and `CLAUDE.md` before a non-trivial change.

## Repo layout

Monorepo layout:

- `plugins/lets/` — the plugin payload. `commands/` (slash commands), `agents/` (expert subagents), `skills/` (reusable + internal), `rules/lets-rules.md` (workflow rules, frontmatter-versioned), `hooks/` (SessionStart + PreCompact).
- `cli/` — the Go CLI (`lets`). Go module root is `cli/`, not the repo root — all `go` commands run from there (or via the repo-root `Makefile`).
- `scripts/release/` — release tooling (`bump-version.sh`, `verify-versions.sh`).
- `scripts/remote/` — VPS deployment for the shared task-tracker backend (not part of the plugin).
- `docs/` — `installation.md` + images.
- `CLAUDE.md` — the authoritative architecture/conventions doc. **Read it.**

## Local development

From the repo root:

```bash
make build    # build the lets binary
make test     # run the Go test suite
make vet      # go vet
make lint     # golangci-lint (config in cli/.golangci.yml)
make fmt      # gofmt + goimports
make install  # build + install to /usr/local/bin or ~/.local/bin
```

CLI changes **must** keep `make test` and `make build` green, and update testdata goldens when behaviour changes (see `cli/internal/initcmd/testdata/`).

To run the plugin from a local checkout in Claude Code: `/plugin marketplace add ./lets-workflow` then `/plugin install lets`.

### Dev binary: `make dev` / `make dev-tmux`

`make dev` in a worktree builds `cli/lets` (version `dev-<branch>-<sha>[-dirty]`), prepends `<worktree>/cli/` to PATH, and execs `claude --plugin-dir <worktree>/plugins/lets` — self-contained, no global install, no marketplace mutation. `make dev-tmux` auto-discovers `.worktrees/*/` and spawns one Claude pane per worktree (`WORKTREES="a b"` to limit). Implementation: `scripts/dev/run.sh`. The most important gotcha (run `make dev` from a host terminal, never from a Bash tool inside Claude) is in `CLAUDE.md` "## Local Development"; the rest:

**`LETS_ENV_VERSION` stamping.** Running `lets init` from a dev binary writes `LETS_ENV_VERSION=dev-<branch>-<sha>[-dirty]` into `.lets/.env` (because `initcmd/render.go` writes `version.Version` literally). Reversible by running prod `lets init`, which restores the proper semver stamp. If you don't want to churn `.env`, skip `lets init` on the dev binary.

**Old worktrees.** Both `make dev` and `make dev-tmux` require `scripts/dev/run.sh` plus the corresponding Makefile targets to exist in the worktree's branch HEAD. Worktrees created before this tooling shipped (or on feature branches that haven't pulled main) will fail with `bash: scripts/dev/run.sh: No such file or directory` or `make: *** No rule to make target 'dev'`. Three fixes (cheapest first): (a) `git checkout main -- Makefile scripts/dev/` from inside the affected worktree — fast but leaves the working tree dirty (uncommitted staged files), so plan to `git restore --staged Makefile scripts/dev/` or commit the migration on the worktree's branch; (b) rebase the worktree's branch onto main; (c) `lets worktree remove <name> && lets worktree create <name>`. `make dev-tmux` is the more dangerous case — it opens panes in ALL worktrees, any of which might be stuck on an old branch. Use `make dev-tmux WORKTREES="up-to-date-only"` to limit, or migrate all worktrees first.

**Production install unaffected.** The production `lets` at `~/.local/bin/lets` is untouched — the dev binary lives at `<worktree>/cli/lets` and only wins on PATH for the `make dev` exec'd process.

**Trust the branch.** Because `make dev` prepends `<worktree>/cli/` to PATH, any executable in `<worktree>/cli/` — including a malicious `cli/git`, `cli/curl`, etc. shipped by an untrusted branch — would shadow the system binary for the duration of the inner Claude session. The exposure is limited to the dev process (the PATH change isn't exported), but treat `make dev` like `make` on the branch: only run it on branches you'd run arbitrary code from.

**`IsDev()` semantics.** `version.IsDev()` returns true for the literal string `"dev"` AND for any `"dev-<non-empty>"` form. Statusline + `lets update` already consume `IsDev()` correctly — dev-stamped binaries render without a `v` prefix and skip env regeneration as expected.

## Editing rules — the one thing that bites people

**Never edit `.claude/rules/lets-rules.md`.** That's the *installed copy*, plugin-managed; it's regenerated from the canonical source by `/lets:init` / `/lets:update`. Edit `plugins/lets/rules/lets-rules.md` instead. Editing the installed copy bypasses our own drift detection and silently desyncs.

Likewise: the **frontmatter `version`** in `lets-rules.md` (and `plugin.json`, `marketplace.json`) is bumped **once per release** at ceremony time (`scripts/release/bump-version.sh`) — not per change. A rules edit on a feature branch accumulates under the current target version. Don't bump it in your PR.

## Rules layers

LETS instructions live in five layers. Only the first one is loaded on every turn and into every session; everything else is loaded when a step needs it, or never (this file). Before lets-nobb5 the whole rule set (62 KB) plus the tracker adapter (~6 KB) rode along on every turn, next to the project's own CLAUDE.md and rules.

| layer | where | loaded | holds |
|---|---|---|---|
| core | `plugins/lets/rules/lets-rules.md` -> installed to `.claude/rules/` (or `~/.claude/rules/`) | every turn (project instructions) | what applies in free conversation, outside any command: language, boundaries, approval gates, AUTO MODE, peer trust, task references and tracker essentials, worktree essentials, the flow table, the response footer, AskUserQuestion essentials, context-window rules |
| protocol skills | `plugins/lets/skills/protocol-tracker/`, `plugins/lets/skills/protocol-orchestrator-offer/` (internal, `user-invocable: false`) | by a command or skill via `Skill(...)` right before the step that needs it, "unless its text is in your current context" | tracker verb resolution (adapter lookup, neutral read shape, bodies, degradation, preflight, trust); the orchestrator offer (select, resolve once, Act / Nav shapes) |
| protocol files | `plugins/lets/protocol/{worktrees,peers,handoff-lane,agents}.md` | Read via `${CLAUDE_PLUGIN_ROOT}/protocol/<topic>.md` at the step that needs it, same condition | worktree detail (adopt / release, task-state file, standing teams, remove nets, lifecycles, vanishing worktrees); boundary carve-outs and the team link; the handoff lane; agent dispatch, reports and members |
| tracker adapter | `plugins/lets/rules/tracker-<name>.md` (+ user-owned `.board.md`) | today auto-loaded from `.claude/rules/`; after the adapter move, Read by `lets:protocol-tracker` | the verb bindings of one tracker |
| CONTRIBUTING | this file | never by the model | rationale, worked examples, the full flow diagrams, the Skill Quick Reference table |

The load condition is the context, not the session: after `/compact` or `/clear` a protocol loaded earlier is gone, so every load point says "unless its text is in your current context". `cli/internal/cli/protocol_lint_test.go` keeps the wiring honest (a `lets-tracker` block loads `lets:protocol-tracker` first; every `protocol/<x>.md` reference resolves and every file is referenced; no pointer to a section that left core; every remaining `lets-rules` heading pointer names a heading core still has). `orc_lint_test.go` asserts the orchestrator-offer load.

### Budgets

| file | budget |
|---|---|
| core `lets-rules.md` | 16,384 B |
| tracker adapter `tracker-<name>.md` | 8,192 B |
| board profile `tracker-<name>.board.md` | 4,096 B |

**Why the core is 16k, not the 10k first targeted.** The first target (8-10 KB) assumed the response footer and the AskUserQuestion conventions could leave core. They cannot: both apply to every response, including free conversation outside any command. A complete dense draft of the always-on rows then measured 15,779 B: the safety and approval sections (boundaries, slash-command discipline, development-workflow gates, AUTO MODE, peer trust, tracker and worktree essentials) alone are 8,478 B, and the always-on non-safety sections (preamble, language, notice, agents and search, discovery, patterns, architecture, flow table + footer, AskUserQuestion, context window) 7,301 B. Dropping Discovery / Pattern / Architecture entirely would have saved only 1,318 B, and safety wording is never compressed to meet a number - so the owner raised the pin to 16,384 B. The core is still a quarter of the old 62 KB. Raising it again is an owner decision recorded here.

### Where does a new rule go?

1. Does it apply in free conversation, outside any command, or on every response (a gate, a boundary, a trust rule, an output format)? -> **core**, as one dense line or table row. Safety wording is never cut for size.
2. Does it apply only while a command runs a specific step? -> a **protocol skill** when it is a procedure several commands share and it carries structured calls (verbs, gates), a **protocol file** when it is reference detail; add the load at each step that needs it (the lint enforces it).
3. Does it apply to one command only? -> that **command** or its **skill**.
4. Does it bind a tracker? -> the **adapter**.
5. Is it an explanation, a history, an example? -> **here**.

### Rationale behind core rules

- **Artifacts in English (L5).** Everything the project stores and searches - code, commits, tracker content, `.lets/` plans, reviews and briefs, PR descriptions - is read and grepped long after the conversation by people and agents who do not share the author's language. A PR description lands in history beside the commit message, which is why it is an artifact and not a reply.
- **Reply in kind (L7).** Answering a person in a language they did not choose is worse service, not better hygiene. What decides is the text being answered, not a config value, so nothing has to be configured.
- **No hard-wrap (L11).** Markdown renders an in-paragraph newline as a space and editors soft-wrap visually, so column-wrapping changes nothing in the rendered output - but it produces noisy diffs (a one-word edit reflows many lines) and makes editing painful.
- **Installed copies (B9).** The project copy is refreshed by `/lets:init` / `/lets:update`; the global `~/.claude/rules/lets-rules.md` is refreshed by the session hook at every start, and a hand-edited global copy is saved to `.bak[-N]` before it is replaced - so own rules go in a separate `.md`.
- **Slash command discipline (D3).** State changes between commands (files appear and disappear, dotfiles are invisible to plain `ls`, sessions span editor and filesystem). The pre-checks exist because shortcutting them produced wrong branches and wrong outputs.
- **Pattern examples (P9).** Typical one-line surfacings: "Це 3-тя річ про X сьогодні - варто винести в окремий таск або epic?"; "Це 3-й раз на цей блокер - давай розберемось чому, замість обходити."; "На гілці зараз X + Y + Z - split на окремі PR'и?".
- **Directed search vs exploration (R8).** Sequential direct reads burn context-window tokens; one agent call returns a focused summary. Examples: directed - find a function definition, check a config value, read a specific file; exploration - understand how a feature works across files, compare patterns, find all places affected by a change.
- **Discovery suggestion (G4).** One line, e.g. "Це варто зафіксувати в задачі - `/lets:note`?". The note itself records full context (decision + reasoning, `file:line`, links, nuances) with no length limit - that is `/lets:note`'s job.
- **Tracker adapter trust (K11), for adapter authors.** An adapter file is trusted instruction; its binding cells execute as written. Installing a third-party adapter equals running its code - review every binding. The contract test pins table shape, not binding safety. A token belongs only in the transport's own config (the MCP server env, a gitignored 0600 file) - never in `tracker-*.md`, a `.board.md` or `.lets/.env`.

### AskUserQuestion - worked examples

Substitution (core: "substitute every `{LETS_FOO}` before the call"):

```
❌ BAD:  description: "Switch to $LETS_MERGE_BRANCH, pick another task"  →  user sees literal "$LETS_MERGE_BRANCH" (broken)
✅ GOOD: description: "Switch to {LETS_MERGE_BRANCH}, pick another task"  →  user sees "Switch to main, pick another task"
```

Worked example with Rule 7 follow-through:

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

### Session Flow - full diagrams

Core keeps one flow table (one row per phase). The full diagrams and the review guidance it replaced:

```
$LETS_PR_FLOW=local   /lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done (merge) -> /lets:end
$LETS_PR_FLOW=github  /lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done (push + PR) -> /lets:end
$LETS_PR_FLOW=bitbucket  /lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done (push + PR via bbb) -> /lets:end

Trunk-mode (any $LETS_PR_FLOW): /lets:start (pick "Stay on current branch") -> Work -> /lets:check -> /lets:commit -> /lets:done (push + close, no PR) -> /lets:end

Main mode (no task):  /lets:start --main -> triage / groom / route (no edits) -> /lets:start <id> when coding starts -> /lets:end

Worktree:  /lets:worktree create -> `cd .worktrees/<name>/ && claude` -> /lets:start -> Work -> /lets:done -> /lets:end -> /lets:worktree remove (main repo)
Orca:      /lets:worktree create (LETS_LAUNCHER=orca) -> Orca pane runs /lets:start <id> (adopt already linked it) -> Work -> /lets:done -> /lets:end -> archive in Orca (lets worktree release)

Team:      /lets:team run [--tasks A,B] -> one visible worker session per task on any launcher, bound via /lets:orc -> each worker /lets:start ... /lets:done   (orca: an Orca child worktree per task; --backend agents is refused)
Standing team:  /lets:team create [<callsign>] --area <a> (from the main checkout) -> the lead <callsign>-lead opens in team_<callsign> -> /lets:start <id> claims the lead and switches the branch (uncommitted work: commit or park, never stash) -> work -> /lets:done -> next task -> /lets:team disband <callsign>

Orchestrators:  /lets:start --main [--scope "<part>"] (several per repo, unique per session name) -> /lets:worktree create <id> binds each spawned worker (--orc) -> a worker chat opened by hand: /lets:start <id> --orc=<name> -> worker and orchestrator talk via /lets:orc

Auto-pipeline:  /lets:worktree create <id> --flow plan-workflow --auto -> [GATE1 clarify] -> auto-plan (plan-workflow) -> [GATE2 approve] -> /lets:execute --auto -> stop at push/PR -> /lets:done

PR review:  /lets:github-pr <PR> -> discuss -> post -> /lets:github-pr --follow-up -> /lets:github-pr --approve
PR respond: /lets:github-pr --respond <PR> -> triage -> fix -> reply
```

If a plan exists from `/lets:plan`, the user runs `/lets:execute` to implement it here, or `/lets:handoff --execute --send [<tab>]` to have an agent tab of this worktree implement it - nothing else starts implementation, and the model never starts it on its own. Execute runs inline in native plan mode (its approval is the code-write gate), or delegates to implementer agents (its Start gate is, and each chunk is committed only after it is accepted - by you, or by the team check under the gate policy you pick at Start: `per-commit` | `high-only` | `at-end`; the one exception is `--pipelined`, where Start approves the implementer's local commits); use `/lets:commit` at natural commit points.

Two separate lifecycles:
- **Session:** `/lets:start` ... `/lets:end` (one conversation)
- **Task:** picked at start ... `/lets:done` (may span multiple sessions)

**Review options:**
- `/lets:check` - the orchestrator reviews inline, no subagents; same target flags as `/lets:review` - before any commit, or a fast first pass on a PR
- `/lets:review` - selected expert subagents review, then an adversarial verify pass; works locally OR on GitHub PR

**When to use which:**
- Small change -> `/lets:check` -> commit
- Significant change -> `/lets:check` -> `/lets:review --local` -> fix -> commit -> PR
- PR already exists -> `/lets:review <PR>` -> comment on PR
- Full PR lifecycle -> `/lets:github-pr <PR>` -> discuss -> post inline -> follow-up -> approve
- Existing file quality -> `/lets:review --file <path>`
- Quick plan check -> `/lets:check --plan`
- Autonomous task (spawn + plan + execute, you gate twice) -> `/lets:worktree create <id> --flow plan-workflow --auto` (PREVIEW; see docs/autonomous.md)

### Skill Quick Reference

The maintained per-command table (core keeps only the flow table and the auto-triggered skill names; each command's own frontmatter `description` is what the model sees in the skill list). Update it when a command is added, renamed or removed.

| Skill | Category | When |
|-------|----------|------|
| `/lets:start` | Session | Beginning of session; `--orc=<name>` binds a worker chat to an orchestrator, `--main --scope "<part>"` registers one |
| `/lets:end` | Session | End of session - settlement pass (commit / push / progress / snapshot, auto-skips when tidy). It REFERS an open task to `/lets:done` and never finishes one itself. `--session` (aliases `--snapshot`, `--pre-compact`, `--compact`) skips settlement and only writes the shared snapshot, keeping the session going |
| `/lets:done` | Task | Task is complete |
| `/lets:commit` | Code | Ready to commit (also auto-triggers on "commit", "закоміть") |
| `/lets:check` | Code | Inline 6-lens reviewed by the orchestrator alone, no subagents; same targets as `/lets:review` (local/staged/last-commit/branch/PR/`--file`/`--plan`/`--json`); `--fix` verifies each finding inline and applies the fixes when nothing needs deciding |
| `/lets:review` | Code | Expert subagents review, then an adversarial verify pass; `<PR>` offers a `gh pr checkout` so agents read the real tree - it stashes on a dirty tree and restores the branch at the end; `--fix` applies the verified fixes when nothing needs deciding |
| `/lets:github-pr` | Code | GitHub PR review lifecycle (review, respond, follow-up, approve) |
| `/lets:review-round` | Code | Work through a RECEIVED review round - triage N comments, decisions->task, artifact FROZEN, one final edit-pass (inverse of `/lets:review`) |
| `/lets:handoff` | Code | Hand the current state OUT - one self-contained brief another agent (fresh session, Codex, Antigravity, external reviewer) can act on with no context; same target selectors as `/lets:review`, plus handoff-only `--commits` / `--range`. `--send` types it into an agent's Orca tab, `--open` opens a new Codex tab for it, `--codex` runs it through Codex headless; the report comes back UNVERIFIED and is checked against the code. `--execute` hands an approved plan to an open agent tab to implement - its commits come back UNVERIFIED for `/lets:review --branch`. `--fix` applies the report's verified fixes here when nothing needs deciding. Deprecated alias: `/lets:review-handoff` |
| `/lets:opinion` | Expert | Technical decision (dynamic agent count; `--workflow` = off-context fan-out + adversarial challenge) |
| `/lets:ask` | Expert | Quick expert consultation (1 agent) |
| `/lets:research` | Expert | Web-sourced CITED answer to an external/technical question; cross-check pass flags single-source/contradicted/stale claims (`--workflow` = off-context; `--project` = repo-grounded) |
| `/lets:backlog` | Planning | Backlog review (multi-agent; `--workflow` = off-context) + `--fast` quick no-agent pulse + interactive cleanup triage |
| `/lets:plan` | Planning | Structured planning with agents - architecture + implementation plan (`--fast` = orchestrator-only, skips explorer/architect/expert subagents; `--idea` = a concept document, no code exploration, never executed) |
| `/lets:plan-workflow` | Planning | **PREVIEW** - autonomous planning via a Dynamic Workflow (goal + rubric up front, off-context, approve at end); folds into native `/lets:plan` later (lets-jsw00); `--fast` = lean budget (~7 agents, still off-context, heavy review pass skipped, quick plan-check kept) - distinct from `/lets:plan --fast` (orchestrator-only, no subagents) |
| `/lets:execute` | Planning | Execute plan from /lets:plan - inline in native plan mode, or delegated (`--implementers`): one persistent implementer by default, `--parallel` isolated groups joined by `lets integrate`, a gate policy picked at Start |
| `/lets:status` | Utility | Read-only orient snapshot - where you are, what's in flight, what's next (tracker-universal) |
| `/lets:worktree` | Utility | Create/manage interactive worktrees for parallel work |
| `/lets:orc` | Utility | Talk to this chat's orchestrator or a named peer session - `ask` / `ping` / `read` / `tell` / `who`; the only sender of peer messages |
| `/lets:peer` | Utility | Alias: `/lets:peer <name> <verb> [text]` = `/lets:orc` with a target |
| `/lets:hub` | Utility | Orca addon (needs `LETS_LAUNCHER=orca`): every project's orchestrators, a read-only answer from a stopped one, wake one for gated work |
| `/lets:statusline` | Utility | Manage & persist statusline appearance - light/dark, compact, hidden rows (writes personal `.claude/settings.local.json`) |
| `/lets:team` | Utility | Team management - `run` (one visible session per task, any launcher), `create` / `disband` a standing team, `spawn` / `dismiss` / `roster` its members, `status`, `stop` |
| `/lets:note` | Utility | Add note to active task (`--session`, aliases `--snapshot` / `--pre-compact` / `--compact` = resume snapshot on request, one path) |
| `/lets:init`    | Setup | Per-project initialization. Re-run for self-heal (drift fix) or to change config; offers the user-scope global-rules install (`lets init --user`) when the plugin is user-scoped |
| `/lets:update`  | Setup | Sync project with the current release - `.lets/.env` + rules self-heal, plus version status for the `lets` binary and the plugin; the global rules are only reported (the session hook keeps them current) |

#### Auto-triggered Skills

These skills fire automatically when you describe the action in conversation:

| Skill | Triggers on |
|-------|-------------|
| `create-task` | "create task", "new task", "bd create" and variations |
| `commit` | "commit", "закоміть", "git commit" and variations |
| `take-task` | "take task X", "візьми таск", "work on X", "claim task" and variations |
| `orc` | "ask the orchestrator", "спитай у оркестратора", "message <name>", "who is working" and variations |

## Audience of plugin source

`commands/`, `skills/`, `agents/`, `rules/` are read by Claude (the model), never by humans. Write for the model: terse, structured, parseable; tables and bullets over prose; `MANDATORY` / `NEVER` / `IMPORTANT` markers where a constraint must be locked onto. Match the existing style. Human-facing docs live in `README.md` and `CLAUDE.md`.

When adding a config key, command, skill, or agent, follow the checklists in "## Editing commands, skills, agents, config keys" below — they list every file that needs to stay in sync. `CLAUDE.md` keeps the canonical *decisions*; the step-by-step *procedures* live here.

## Editing commands, skills, agents, config keys

> Paths below are relative to `plugins/lets/` (the plugin root) unless they start with `cli/`, `scripts/`, `docs/`, or are repo-root files (`README.md`, `CLAUDE.md`).

Update these files:

| File | What to update |
|------|----------------|
| `plugins/lets/rules/lets-rules.md` | The core flow table (`## Session Flow`) when a command changes a phase - the per-command Skill Quick Reference table now lives in "## Rules layers" above; keep the core within its budget. Edit ONLY here, never the installed `.claude/rules/lets-rules.md`. **Do NOT bump frontmatter `version` per change** — it's bumped once per release at ceremony time (`scripts/release/bump-version.sh`; see `RELEASING.md`). A rules edit on a feature branch accumulates under the current target version. |
| `commands/install-deprecated.md` | Essential Skills / Planning Skills tables |
| `CLAUDE.md` Key Concepts | If adding a new skill |
| `docs/commands.md` (+ the relevant `docs/<topic>.md`) | A new command, a renamed command, or a **new user-facing flag**. This row exists because it was missing: `--pre-compact`, `--session` and `/lets:start --main` all shipped while the human-facing reference kept claiming those commands take no flags. The plugin-source rows above do not cover `docs/` — nothing else does either. |
| `README.md` | Agent table, feature descriptions |
| All `agents/*.md` `## Constraints` / `## Report` sections | If changing the read-only Bash allowlist, the constraint wording or the report instructions, sync the identical `## Report` section and Constraints lines across all 14 analyst agents - pinned by `TestAgentReport` (quick check: `grep -h "You are read-only" agents/*.md \| sort -u` returns exactly one line) |
| `commands/end.md` + `commands/done.md` + `skills/session-snapshot/SKILL.md` session-id references | Channel per context (the rule): **inside a bash command** → `$CLAUDE_CODE_SESSION_ID` (Bash subprocess env var; bash expands it at runtime) — used by `done.md` Step 7, `end.md` Step 3b progress-comment heredoc (body-filed via the tracker `comment-add`), and the `session-snapshot` skill Step 2 (`SID=$CLAUDE_CODE_SESSION_ID` + `find ... "${CLAUDE_CODE_SESSION_ID}.jsonl"`) which writes the snapshot's `- ID:` line from that single bash channel. **The `${CLAUDE_SESSION_ID}` command-load-time template channel is intentionally NOT used in the snapshot skill** (fragile inside a multiline Write arg — lets-bdkvd QA #13). No `SESSION_ID=` alias anywhere. Verify with `grep -rn "SESSION_ID" plugins/lets/commands/ plugins/lets/skills/`. Background + remaining adoption scope (subagents, statusline, `/lets:team` records): see `lets-bdkvd`. |
| `cli/internal/initcmd/reviewspec_test.go` | If touching `commands/review.md`, `commands/check.md` or `skills/review-workflow/review.workflow.js`. Go tests read that markdown from `../../../plugins/` and pin the SPEC blocks, the `--workflow` args wiring, the one-shell PR switch (which they also EXECUTE against a stub `gh`), the restore fence and the no-worktree boundary. **Run with `-count=1`** — Go's test cache does not track files reached through `../../../plugins/`, so a markdown-only edit serves a stale PASS. Same caveat applies to `trackerbodies_test.go` / `trackerrules_test.go`. |
| `cli/internal/cli/<name>.go` + register in `cli/internal/cli/root.go` | If adding a Go subcommand. Add `<name>_test.go` (`package cli_test`). Use `cmd.OutOrStdout()`. Domain logic goes in `cli/internal/<name>/` (see `initcmd/`, `updatecmd/`, `worktreecmd/`, `sessionstart/`, `statusline/`, `frontmatter/` for patterns). If the package needs platform-specific primitives (`syscall.Flock` etc.), gate with `//go:build unix` and add a `<name>_stub.go` (`//go:build !unix`) per the `worktreecmd` pattern so cross-platform builds keep working. Update `cli/README.md` "Adding a subcommand" recipe if pattern changes. |
| Any `commands/*.md` or `skills/*/SKILL.md` invoking `AskUserQuestion` | Follow `## AskUserQuestion Conventions` in `plugins/lets/rules/lets-rules.md` (header chip 4-12 chars descriptive, `(Recommended)` in label, `multiSelect` per rule, follow-through via `Skill` tool per Rule 7, `{LETS_FOO}` substitution). Spec strings hardcoded in English; orchestrator translates to `$LETS_LANGUAGE` at runtime. |
| Any `commands/*.md` or `skills/*/SKILL.md` that **offers the orchestrator** | Loads `Skill(skill: "lets:protocol-orchestrator-offer")` at the offer (see "## Rules layers") and follows its Act shape (an "Ask orchestrator" option + a `Skill(lets:orc, verb=ask footer=none)` handler + re-show the gate) or its Nav shape (a `- **Orchestrator offer (Nav)**` handle line; `/lets:orc ask` only in a LETS box or on a handle line) - never a restated rule, never a fifth option. `TestOrcLint` (`cli/internal/cli/orc_lint_test.go`) enforces it; a file that only REFERENCES `/lets:orc` goes into its `orcOfferExempt` list. |
| Any `commands/*.md` or `skills/*/SKILL.md` that **runs a tracker verb or needs protocol detail** | Loads `Skill(skill: "lets:protocol-tracker")` before its first ```` ```lets-tracker ```` block, and Reads `${CLAUDE_PLUGIN_ROOT}/protocol/<topic>.md` at the step that needs worktree / peers / handoff / agent-dispatch detail - each "unless its text is in your current context" (see "## Rules layers"). `TestProtocolLint` (`cli/internal/cli/protocol_lint_test.go`) enforces it. |
| Any `commands/*.md` or `skills/*/SKILL.md` that **writes a file under `.lets/plans`, `.lets/reviews`, `.lets/sessions` or `.lets/handoffs`** | Resolve the path via `Skill(skill: "lets:artifact-path", args: "kind=<kind> ext=<ext> [task=<id>]")` and write to the echoed `ARTIFACT_FILE` VERBATIM; register a new `<kind>` in `skills/artifact-path/SKILL.md`'s kinds table. Never `date`+`ls` a path by hand - `.lets/` is one symlinked dir shared by every worktree, and a hand-built name overwrote another session's file (lets-05c4s). Go-owned state files (`.task-*`, `peers/*.role`, `cache/released-*`) are exempt: they are not artifacts, and markdown writes them only through their owner's CLI. So are transient `.lets/cache/peer-msg/<msgid>.txt` handoffs (0700 directory): created by `lets peers frame`, written by the orc skill or `/lets:hub` (its ask-ro prompts use the same handoffs), deleted by `lets peers tell` / `ask-ro`. |
| Adding a **Dynamic Workflow asset** (a `Workflow`-tool script) | Follow `## Dynamic Workflow Assets (authoring standard)` in `CLAUDE.md`. Ship as `skills/<name>-workflow/` (`<name>.workflow.js` + `user-invocable: false` `SKILL.md`); invoke from the command via `Workflow({scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/<name>-workflow/<name>.workflow.js", args})`; obey the 6 conventions + runtime must-obey list; validate via a live smoke test (no committed unit test — runtime-blocked); degrade gracefully when an `agentType` can't resolve. `skills/review-workflow/` is the reference example. |

### Command output requirements

Every command response ends with exactly ONE footer of the right type - the runtime rule is `plugins/lets/rules/lets-rules.md` `### Response Footer` (Act = AskUserQuestion / Nav = LETS box / Close = prose or nothing; never mix; Nav content is state-driven). This section covers authoring the **Nav** box.

```
┌─ LETS ─────────────────┐
│  [action]? [command]   │
└────────────────────────┘
```

**Box format:**
- Header `┌─ LETS ─` + `─` padding + `┐`; lines `│  ` + content + padding + ` │`; footer `└─` + `─` padding + `┘`. Min width 25.
- **All boxes in one file MUST be the same width**, measured in DISPLAY columns = Unicode code points, NOT bytes (the box-drawing glyphs are 3 bytes each, so a byte count like `awk length` lies). Verify:
  ```bash
  python3 -c "import sys;[print(len(l.rstrip())) for l in open(sys.argv[1]) if l.rstrip().endswith(('┐','│','┘'))]" <file> | sort -u | wc -l   # expect 1 (matches box lines by their right edge; skips tree diagrams)
  ```

**Content:**
- Short action word + `?` (e.g. "Commit?", "Fix?"), then a `/lets:*` command. **ONLY `/lets:*`** (exception: `git push` after `/lets:done`/`/lets:end`). If the next step is NOT a `/lets:*` command (a shell command, "restart to apply", etc.) → no box, use a Close prose line.
- **No command / internal invocation → no footer** (a `/lets:*` called by another command, or a Rule-7 follow-through — see `lets-rules.md` `## AskUserQuestion Conventions` Rule 7). Only the outermost user-invoked command emits one.
- **State-driven** (per the runtime rule): task active → NEVER `/lets:start` (it is only a no-task / bootstrap escape hatch); "reset context, keep the task" → `/clear` + the mid-task command, never `/lets:start`.

**Which shortcuts (pick in order):** (1) most-likely next step in the loop; (2) a lighter alt if one exists (`/lets:check` for `/lets:review`, `/lets:ask` for `/lets:opinion` — pass the matching flag); (3) one escape hatch (`/lets:start` / `/lets:end` / `/lets:status`). ≤4 lines; one line is correct when there is a single sensible step.

### Command checklist

- [ ] Response ends with exactly one footer of the right type (Act/Nav/Close - see the Response Footer rule)
- [ ] Nav-box shortcuts follow the guidance above (next step + lighter alt + escape hatch; no `/lets:start` mid-task); all boxes in the file are the same display-column width (python recipe)
- [ ] Updates the Skill Quick Reference in `CONTRIBUTING.md` "## Rules layers", and the core flow table in `plugins/lets/rules/lets-rules.md` when a phase changes (do NOT bump frontmatter `version` per change — once per release at ceremony, see the version-coherence rule above)
- [ ] Updates `/lets:install` Essential Skills / Planning Skills tables
- [ ] Follows session flow (start -> work -> commit -> done -> end)
- [ ] Description is clear and actionable
- [ ] **Dispatches agents?** Use the `agent-report` protocol (`REPORT_FILE` per agent, `op=collect`, READ EVERY REPORT IN FULL) and register the dispatch -> consumer pair in `reportPhases` - `TestAgentReport` fails on an unregistered `subagent_type=`
- [ ] **If file invokes any deferred tool** (`AskUserQuestion`, `EnterPlanMode`, `WebFetch`, etc.), include the `> **IMPORTANT:**` deferred-tool callout right after the file's brief description, before the first `## Step` (or first major section). Wording: see existing commands/skills for the standard block (search for `IMPORTANT:** If the spec below`)
- [ ] **If file invokes `AskUserQuestion`**, follow `## AskUserQuestion Conventions` in `plugins/lets/rules/lets-rules.md` — header chip 4-12 chars descriptive (never `"LETS"`; command name is OK when it names the topic), `(Recommended)` in label not description, follow-through via `Skill` tool (Rule 7) when an option names a `/lets:*` command, **substitute `{LETS_FOO}` placeholders before tool call** — never use `$LETS_FOO` inside `label`/`description`/`question` strings

### Adding a new config key

Single source of truth for canonical metadata: `cli/internal/letsconfig/keys.go::Keys`. Single source of truth for Prefs↔Key wiring: `cli/internal/initcmd/render.go::Prefs.AsValues()`.

Required edits:

1. Append `Key{Name, Comment, Default}` entry to `letsconfig.Keys`. Name MUST start with `LETS_`.
2. Add field to `Prefs` struct in `cli/internal/initcmd/render.go` AND add ONE entry to `Prefs.AsValues()` map (one-line addition right below the field).
3. Regenerate the env goldens (`go test ./internal/initcmd -run Golden -update`) and bump any hardcoded key-count assertions (`keys_test.go`, sessionstart tests, the "N canonical keys" prose in `CLAUDE.md` / `cli/README.md`). Do **NOT** bump the `lets-rules.md` frontmatter `version` per change — it's bumped once per release at ceremony time (see the version-coherence rule above); the SessionStart drift check picks the new key up on the next release.

If the key is exposed via the `/lets:init` slash command (most are):

4. Add a `--<key>` cobra flag in `cli/internal/cli/init.go` and wire it through `flagOrDefault(flag<X>, defaults["LETS_X"])` in prefs construction.
5. Add an AskUserQuestion in `plugins/lets/commands/init.md` (Step 2 first-time path + Step 3d "Keep current" option in change-config path).

Auto-derived (no edit needed):
- `.lets/.env` content (renderEnv → renderTemplate(Header, p.AsValues()))
- `.lets/.env.example` content (renderEnvExample → renderTemplate(ExampleHeader, Defaults()))
- SessionStart hook env-injection whitelist (sessionstart imports `letsconfig.Names()`)
- Regenerate wiring (`RegenerateEnv` uses `p.AsValues()`, iterates `letsconfig.Keys`)
- Future `/lets:doctor` validation + display

Then document in CLAUDE.md's "LETS Config keys" table + `README.md` Configuration block, and add consuming logic in the relevant commands.

### Add a tracker adapter

A **tracker adapter** binds the neutral task-tracker verbs to one concrete transport. `LETS_TRACKER` names the adapter; `lets init` installs exactly one `plugins/lets/rules/tracker-<name>.md` into the project's `.claude/rules/` (same drift-tracked, frontmatter-versioned mechanism as `lets-rules.md`). Adding an adapter is authoring one markdown file - no command forks, usually no Go.

1. Copy `plugins/lets/rules/tracker-TEMPLATE.md` to `tracker-<name>.md`. Set frontmatter `name: tracker-<name>` + `version:` matching the current plugin version (bumped only at release ceremony, like `lets-rules.md`). The release scripts (`scripts/release/bump-version.sh` + `verify-versions.sh`) **glob** `tracker-*.md` (excluding `tracker-TEMPLATE.md` and `*.board.md`), so your adapter is version-bumped, staged, and drift-checked automatically - no release-script edit needed. (Corollary: never name a real adapter `tracker-TEMPLATE.md` or end it `.board.md` - both are deliberately excluded.)
2. Fill the **Capabilities + bindings** table. Header is PINNED (`| verb | tier | supported | binding |`) - the contract test parses it; don't reorder columns. Bind the 5 CORE verbs (`create`, `show`, `comment-add`, `set-status`, `close`) - they MUST be `supported = yes`. OPTIONAL verbs may be `absent` (the command degrades gracefully). **Each binding cell is EXECUTED as written** when a command invokes its verb (command bodies carry neutral ` ```lets-tracker ` blocks, not inline `bd`) - keep bindings to the tracker's own transport, never a destructive/exfiltrating command; installing a shared adapter runs its bindings (the contract test pins table shape, not binding safety).
3. **Normalize output:** `show`/`list-by-status` MUST return status as a NEUTRAL name (`open`/`in_progress`/`closed`/...) so consuming commands stay adapter-agnostic. The adapter owns native↔neutral translation. `close` returns the status the task ended in - `closed` when it really closed, another neutral status when this board's terminal is gated and the legal move was an advance (the caller then reports a handoff, never a close).
4. **Declare your fields:** `create` and `set-field` open with `accepts:`, `show` with `returns:`, in NEUTRAL field names (the vocabulary is listed in the TEMPLATE's declaration bullet; a native rename is written `priority`→`severity`). **Close each declaration with a period** - that is the terminator, and without it the transport call's own backticks get read as field names. A verb marked unsupported still writes the marker with nothing in it (`accepts: nothing - absent`). A supported `show` must declare `id`/`title`/`status`/`description` because commands read them unconditionally; `url` only if your tracker really has per-task links. Keep the lists minimal - every declared field multiplies a read verb's payload.
5. **Prune `## Neutral statuses` to your board.** It is a declaration, not a vocabulary listing: naming `in_review` there is what authorizes `/lets:done` to move a task into review after opening a PR. Copying the TEMPLATE line unedited will push tasks into a column you do not have.
6. **Mark what you have not exercised** - `[ASSUMED]` / `[UNVERIFIED]` inline, `[VERIFIED <date>]` once confirmed. Scoped to claims you took from documentation or inferred; a binding you have actually run needs no marker. The file is auto-loaded instruction, and the agent acts on whatever it states flatly.
7. **Degradation:** OPTIONAL absent → continue with a message; a CORE verb unresolvable at runtime → HARD-FAIL loud (never a phantom success, esp. under AUTO MODE).
8. **Secrets:** never in `.lets/.env` (644, injected) or a `.board.md` (auto-loaded + git-shareable). A direct-API adapter reads its token from a gitignored chmod-0600 file (`.lets/trackers/<name>/.env`, written `0o600` + dir `0o700`); a transport that owns its own creds (an MCP server) keeps them there.
9. Optionally ship a `tracker-<name>.board.md` template (project-specific status-id map / transitions / principles) - `lets init` scaffolds it once and NEVER overwrites it.
10. **Declare `## Worktree`** (see the TEMPLATE section): `links:` names the store paths every worktree shares with the main checkout (beads: `.beads/.env` (0600); `links: nothing.` for a tracker with no local store) - each link is a symlink to a credential, so declare only what the transport needs and never a path outside the repo. Then the task id and branch convention: `id:` (one backticked RE2 fragment, or `nothing.` when no id can be read off a name), `branch:` / `worktree-branch:` (the names LETS creates, templates with `{id}` and `{slug}`) and optionally `accept:` (shapes only `lets worktree adopt` recognizes, e.g. Orca's `{id}-{slug}`; adopt records them as unconfirmed `origin:` candidates). A team overrides any convention key - never `links:` - in its `tracker-<name>.board.md` (e.g. ``id: `PWA-[0-9]+`.``). Go owns parsing and rendering (`cli/internal/trackeradapter`), so no markdown matches the regex by eye.
11. The adapter contract test (`cli/internal/.../trackerrules_test.go`) auto-covers the new file (valid frontmatter + pinned header + 5 CORE rows each marked `supported = yes` (the `none` null adapter excepted) + `## Degradation` + `accepts:` on `create`/`set-field` and `returns:` on `show`, declared in neutral field names). No allowed-values list to edit - `lets init` validates `LETS_TRACKER` against `^[a-z0-9][a-z0-9-]*$` and skips with a warning if `tracker-<name>.md` is absent.

Command bodies invoke verbs via ` ```lets-tracker ` blocks (never inline `bd`); the orchestrator resolves them through the loaded adapter. The "no adapter file loaded" fallback lives in the rule, NOT the adapter file (an unloaded file can't instruct): `beads`/unset resolves via `tracker-beads.md`; a non-beads name with no file loaded behaves as `none` (no tracker ops, never `bd`) and nudges `/lets:update`.

### Add a worktree launcher

A launcher is a Go package plus a cobra subcommand, not a file on disk: `cli/internal/<name>cmd/` (copy `tmuxcmd`: JSON envelope, typed exit codes, never hard-fails, a `//go:build unix` implementation and a stub), registered in `cli/internal/cli/root.go`. Add the value to `letsconfig.ShippedLaunchers` AND to the reverse list in `TestShippedLaunchers_MatchSubcommands` (the test pins both directions), add a `case` to `notifycmd` so gate notifications reach it, and give `/lets:worktree create` its step (C3.5, or an earlier step when the launcher creates the worktree itself, like Orca's C0). An optional launcher stays opt-in: nothing looks for its binary unless `LETS_LAUNCHER` names it.

### Exit-code ranges per package

A `--json` subcommand package owns its typed exit codes (`exit.go`, with a `TestResult_SchemaContract` for its envelope). Ranges never overlap, so a code in a log names its package:

| Package | Command | Range |
|---|---|---|
| `worktreecmd` | `lets worktree` | 10-26; 27-34 reserved for the standing-team verbs |
| `memberscmd` | `lets members` | 40-49 |
| `integratecmd` | `lets integrate` | 50-59 (full: a new failure class reuses 55 with its own `error.kind`) |

`0` ok, `1` generic and `2` usage are shared. A new package takes the next free decade and adds a row here.

### Spawning an agent that writes or persists

An implementer, or any member of a standing team, is spawned, messaged and dismissed ONLY through `skills/member-run/SKILL.md`, with `lets members` as the registry of who is live. A command never calls the `Agent` tool for such a member itself (`TestMemberRun`, `cli/internal/initcmd/memberrun_test.go`, pins the contract; `TestOrcLint` exempts only member-run's and execute.md's own `{agent}` recipient from the peer-send rule). One-shot analysis subagents (`/lets:review`, `/lets:opinion`, ...) are not members.

### Leaf packages and import direction

`trackeradapter`, `taskid`, `fsutil`, `peername`, `taskstate`, `redact`, `ccregistry`, `agentrun`, `teamfile`, `memberscmd`, `integratecmd` and `gitutil` are leaves (the test's table is authoritative): they import the standard library or another leaf, nothing else, so any command package can use them without an import cycle. `TestLeafPackages` (`cli/internal/cli/leaf_packages_test.go`) holds the table - append a directory when a new leaf lands - and `TestDeniedImports` forbids the edges that would compile but invert the layering (`worktreecmd` never imports `orcacmd` or `peerscmd`; `orcacmd` never imports `peerscmd`). A package that owns a file format (`taskstate` owns `.task-<branch-slug>`) is the only writer; markdown writes through its CLI.

## Commits & PRs

- **Conventional commits**: `feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`. Subject under 50 chars, imperative mood ("add" not "added"). Optional `(task-id)` scope when the work is tracked. See `.claude/rules/git.md`.
- Branch off `main`; open a PR. The `Verify version coherence` check must pass (source-tree version coherence), one approval, conversations resolved.
- Keep PRs focused — one theme per branch. Surgical changes are easier to review and revert.
- Update `CHANGELOG.md` under `## [Unreleased]` for user-visible changes.
- Don't bump versions (see above).

## Task tracking

Maintainers track work in [beads](https://github.com/steveyegge/beads) with a shared [Dolt](https://github.com/dolthub/dolt) remote. As an external contributor you don't need any of that — just describe your change in the PR body. If you're filing a bug or proposing a feature, use the issue templates.

## Where to ask

- **Bug / feature request** → open an issue (templates provided).
- **Security vulnerability** → see `SECURITY.md` (private reporting, *not* a public issue).
- **Design questions** → open an issue; reference the relevant section of `CLAUDE.md`.
