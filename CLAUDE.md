# lets-workflow

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

Monorepo layout: plugin source in `plugins/lets/` subdirectory, marketplace manifest at root pointing into it. Infrastructure scripts and docs stay at root, outside the plugin payload.

```
.claude-plugin/marketplace.json   # Marketplace manifest (source: ./plugins/lets)
plugins/lets/                     # Plugin payload (everything ${CLAUDE_PLUGIN_ROOT} resolves to)
├── .claude-plugin/plugin.json    #   Plugin manifest (Claude Code; .codex-plugin/ planned for multi-agent)
├── commands/                     #   Slash commands (/lets:start, /lets:done, /lets:review, etc.)
├── agents/                       #   15 expert agents (review/opinion/ask/plan/backlog/team; skeptic verifies review findings + research claims)
├── skills/                       #   Reusable skills (user-facing auto-triggered + internal referenced by commands)
├── rules/lets-rules.md           #   Workflow rules (frontmatter `version`; copied to .claude/rules/ by `lets init`)
└── hooks/                        #   SessionStart + PreCompact hooks (drift check + LETS Config only - no rules in stdout)
cli/                              # Go CLI - companion binary (Phase 2+, lets-7vtaw)
├── cmd/lets/main.go              #   Entry point (thin)
├── internal/cli/                 #   Cobra command factories (root.go, version.go, *_test.go)
├── internal/version/version.go   #   Version (var, ldflags-overridable from git tag)
├── go.mod, go.sum
└── .golangci.yml                 #   golangci-lint v2 config (default linters + misspell; gofmt/goimports as formatters)
Makefile                          # Repo-root build (build/test/vet/lint/fmt/install/clean)
.editorconfig                     # Editor whitespace/charset settings
.github/workflows/                # CI: ci.yml (Go build/vet/test/lint on PRs+pushes), verify-versions.yml (source-tree version coherence), release.yml (tag-driven goreleaser)
scripts/dev/                      # Dev workflow helpers (make dev / dev-tmux)
scripts/release/                  # Release tooling: bump-version.sh + verify-versions.sh (used by Makefile bump/release-tag)
scripts/remote/dolt/              # Dolt SQL server VPS deployment + ad-hoc backup (NOT plugin)
scripts/remote/beads-web/         # beads-web (Rust kanban board) VPS deployment (NOT plugin)
scripts/deprecated/               # Retired scripts kept for cleanup runbooks - gitignored, not tracked
docs/                             # Public-facing docs (installation.md, commands.md, workflow.md, agents.md, autonomous.md, commands/ per-command pages, …) + images/; planning notes / comment exports / KB live in gitignored docs-local/
reference/                        # Local-only reference materials (gitignored)
```

References that resolve via `${CLAUDE_PLUGIN_ROOT}` (e.g. `${CLAUDE_PLUGIN_ROOT}/skills/X/SKILL.md`) work as before - the env var points at `plugins/lets/`.

## Local Development

Testing the LETS plugin against unmerged changes — across parallel worktrees, in TMUX, without polluting the global installation:

- **`make dev`** in a worktree builds `cli/lets` with version `dev-<branch>-<sha>[-dirty]`, prepends `<worktree>/cli/` to PATH, and execs `claude --plugin-dir <worktree>/plugins/lets`. Each invocation is self-contained — no global install, no marketplace mutation.
- **`make dev-tmux`** auto-discovers `.worktrees/*/` and spawns a tmux session with one Claude pane per worktree. Pass `WORKTREES="name1 name2"` to limit. Threshold prompt at >10.
- Implementation: `scripts/dev/run.sh` (subcommands: `build|info|claude|tmux`). The Makefile targets are thin shims for discoverability via `make help`.

**Gotcha — Claude-inside-Claude.** `make dev` must run from a **host terminal**, not from a Bash tool inside an existing Claude session. The Bash tool spawns a subshell that `exec`s the inner `claude`; the outer Claude's tool harness captures stdin/stdout, so the inner Claude has no terminal to interact through and hangs. Symptom: the Bash tool times out with no visible Claude prompt. Use a fresh tmux pane or terminal window instead.

The remaining dev-binary gotchas — `LETS_ENV_VERSION` stamping, old worktrees, PATH-shadowing ("trust the branch"), and `IsDev()` semantics — live in `CONTRIBUTING.md` "### Dev binary: `make dev` / `make dev-tmux`".

## Key Concepts

> Path convention: paths like `commands/`, `skills/`, `rules/lets-rules.md` in this doc are **relative to `plugins/lets/`** (the plugin root, also exposed as `${CLAUDE_PLUGIN_ROOT}` at runtime). Paths starting with `scripts/release/`, `scripts/remote/`, `docs/`, `reference/` are relative to the **repo root** (outside the plugin payload).
>
> Go CLI source paths (`cli/cmd/lets/main.go`, `cli/internal/...`) are relative to the **repo root**. The Go module root is `cli/` - all `go` commands operate from there (or via the repo-root `Makefile`).

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = experts dispatched by commands. `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:backlog`, `/lets:research` dispatch via subagents. `/lets:team` dispatches via Agent Teams (parallel, worktree isolation). `actor` is a meta-agent that loads external personalities (URL or file) and adapts them to LETS modes
- **Orchestrators** = commands that delegate to other commands. `/lets:github-pr` orchestrates `/lets:review` for full PR lifecycle
- **review-round** = the inverse of `/lets:review`: `/lets:review-round` CONSUMES a received multi-comment review (spec/doc/PR) - triage each item, decisions->beads task, artifact FROZEN during triage, ALL edits in ONE final pass. `commands/review-round.md`.
- **Hooks** = SessionStart + PreCompact inject workflow rules (PreCompact preserves rules across context compaction in long sessions)
- **Statusline** = `lets statusline` Go subcommand; `.claude/settings.json` invokes it directly, `lets init` wires it via value-match against `"lets statusline"` (foreign user-customized commands left alone). **The rich multi-line box (`cli/internal/statusline/rich.go`) is the DEFAULT** (identity / budget / task / rotating-tip; the Full tier adds a location pill); flags `--compact` / `--light` / `--no-tip` / `--no-dir` / `--no-task`. Glyphs are 1-cell text (no emoji), so `wideRunes` is empty — **the border drifts on (1) an unregistered 2-cell glyph, or (2) a glyph absent from the monospace face that font-substitutes to 2 cells while `cellWidth` counts 1** (the `lets-6md86` cause: `glyphFolder` changed `☰`→`»` because cmux/Ghostty rendered `☰` 2-cell; check a glyph with `printf '%s\n' 'ref |X|' 'g |<G>|'`). Run by hand `lets statusline` no longer hangs — a TTY-stdin guard prints a wiring hint and exits 0 (`lets-7frjs`). The task line reads `.lets/cache/task-status`, self-refreshed by a detached `--fetch-task-only` subprocess — no bd/network on the render path. Appearance is persisted (not just rendered) by `lets statusline config` / `/lets:statusline` to personal `.claude/settings.local.json`. Flags, `COLUMNS` tiers, width math, payload-robustness, escape-injection defense, the TTY guard, `lets statusline config`, and legacy-shim migration: `cli/README.md` "## lets statusline" / "## lets statusline config".
- **`.env` versioning** — the first key `LETS_ENV_VERSION` records which `lets` binary version last regenerated `.lets/.env`. `RegenerateEnv` (`cli/internal/initcmd/env.go`) is the canonical writer (preserves user values + foreign keys under `# User-added keys`; restores a hand-deleted `LETS_*` key to its `letsconfig.Defaults()` value). It is NOT in the `letsconfig.Keys` whitelist (metadata, not user-config), so the hook's session injection skips it. `lets init` regenerates on version mismatch or new prefs; `/lets:update` reuses it to refresh the header. Detail: `cli/README.md` "## lets init" / "## lets update".
- **`lets worktree create/remove/list/info`** — Go subcommand (`cli/internal/worktreecmd/`, `//go:build unix`) owns the filesystem/git work for interactive worktrees: name validation, not-inside-worktree guard, attach-vs-new-branch auto-detect, race-safe `.gitignore` hardening, atomic `git worktree add`, `.lets/` whole-dir symlink + `.beads/.env` targeted symlink, verify, and rollback. `commands/worktree.md` is a thin `--json` dispatcher (captures intent via `AskUserQuestion`); Windows gets a no-op stub (`lets-rqep4`). JSON envelope, typed exit codes, `--print-cd`, and the rollback contract: `cli/README.md` "## lets worktree".
- **`lets cmux open`** — optional, macOS-only worktree launcher (`cli/internal/cmuxcmd/`, `//go:build unix`). Wraps `cmux workspace create --cwd <path>` (manaflow-ai/cmux) to open a worktree session running `claude '/lets:start <id>'`. **Strictly optional, never hard-fails:** detects cmux on PATH + `runtime.GOOS=="darwin"`; on absence/error returns `OK=true` + `launch.launched=false` + a `fallback_command`. `commands/worktree.md` Step C3.5 decides terminal-vs-cmux from `$LETS_LAUNCHER` (+ `--cmux`/`--no-cmux` override) and renders the result. **Duplicate-session guard:** `open` refuses to spawn a second workspace for a path that already has one (`reason=already_open`; `--force` overrides) — "one live session per worktree" at the launcher level. **Identity stamp at create:** `open --name <slug>` sets the short tab label and `open --description "<task-id> · <title>"` stamps the canonical beads id + full title (cmux stores it, exposes it in `workspace list --json`), so each of N parallel sessions self-identifies its task — C3.5 wires both from the task. **`lets cmux rename`** relabels a workspace tab post-hoc (resolve by `--ref`/`--cwd`/active; invoked on-demand, not auto-wired) — identity is normally stamped at create now, so rename is the fallback, not the primary path. **`--auto`** (at the `/lets:worktree create` level, a `--command` string change — no Go/cmux change) launches `claude --permission-mode auto '/lets:start <id>'` across terminal + cmux; maps ONLY to `--permission-mode auto`, never `bypassPermissions` (autonomous impl that still gates push/PR/`bd close`/external per AUTO MODE rules). **`lets cmux notify`** (lets-m8ecy) — the gate-notification sink: wraps `cmux notify --workspace <ref> --title/--subtitle/--body` (resolve by `--ref`/`--cwd`/active), same never-hard-fail contract as open/rename (`OK=true` + `notified=false` + reason off-macOS/cmux-absent/no-match; `Notified=true` means ENQUEUED, not confirmed-seen). The non-unix stub emits a parseable `ok=true,reason=not_macos` envelope (exit 0) — unlike open/rename's hard-error stub — so `--json` gate snippets degrade cleanly cross-platform. Driven at LETS gate points by the autonomous pipeline (below). Windows gets a stub. Detail: `cli/README.md` "## lets cmux" (`notify` bullet).
- **Autonomous task pipeline** (lets-m8ecy) — near-autonomous spawn → plan → execute so N parallel sessions are manageable: `/lets:worktree create <id> --flow plan-workflow --auto` spawns a worktree that claims the task (resolve-and-claim convention in `detect-task`, AUTO-MODE-exempt entry claim) and lands in autonomous `plan-workflow` (gated on its PREVIEW availability, falls back to `--flow plan`); the human answers a bounded up-front clarify gate (GATE 1) and approves the plan file (GATE 2); `/lets:execute --auto` then runs the approved plan without per-step gates (hard-stops preserved; REFUSES on the merge-branch). cmux notifications fire at the gates (+ execute-blocked), marker-gated by the per-task `.lets/cache/pipeline-state-<id>` file so only autonomous runs notify. The statusline phase-row that renders the marker is a deferred follow-up. `--flow` only swaps the launch `--command` (launcher-agnostic — terminal/cmux/tmux inherit). Design boundary held: filesystem/process/git → Go (`cmuxcmd`); orchestration/gates → markdown.
- **`/lets:update`** — sync a project with the current release. `commands/update.md` shells `lets update --json` (mirrors `/lets:init → lets init`); the Go subcommand (`cli/internal/updatecmd/`) auto-syncs `.lets/.env` (via `RegenerateEnv`, skipped on a `dev` binary) and `.claude/rules/lets-rules.md` (via `drift.Check` + re-copy), then version-checks the `lets` binary + the plugin against the latest GitHub release (report-only — can't self-replace). Does NOT prompt for config or touch `settings.json`/beads — that's `lets init`'s job (init = setup; update = sync). Rules-drift Notices point here for `unknown`/`outdated`/`ahead`; `/lets:init` only for `missing` (global-rules drift points at `/lets:update` or `lets init --user` via `drift.MessageUser`). Scope-aware (`LETS_RULES_SCOPE`, merged project>user env): `scope=user` + missing project copy = `delegated` (rules from `~/.claude/rules`, never re-created - kills the boomerang) or `not-initialized` when nothing covers the project. The 4-artifact table (+ the optional 5th `user-rules` artifact - omitted when `~/.claude/rules/lets-rules.md` is absent; ahead = never overwritten) + caching detail: `cli/README.md` "## lets update" (lets-hdrdr.3).
- **Skills** = reusable actions in `skills/<name>/SKILL.md`. Two types: user-facing (auto-discovered, triggered via description match; can also be invoked via the `Skill` tool) and internal (not auto-discovered, invoked by commands via the `Skill` tool when needed). Examples: `create-task`, `commit`, `take-task` (user-facing), `detect-task`, `actor-fetch-personality` (internal)

## Architecture Decisions

- **Adversarial verification + Dynamic Workflows (opt-in `--workflow`).** `/lets:review` refutes every `[BLOCKER]`/`[SUGGESTION]` with `lets:skeptic` agents before reporting (asymmetric drop: SUGGESTION on a simple-majority `real=false`; BLOCKER only on near-unanimous high-confidence refute, downgraded on a simple-majority, else kept). Runs in BOTH standard (Task, capped) and `--workflow` (off-context) modes — `--workflow` is a pure performance lever, same verified findings. `/lets:review`, `/lets:opinion`, `/lets:backlog`, `/lets:research` ship committed workflow skeletons (`skills/<name>-workflow/<name>.workflow.js`); authoring standard + runtime constraints in **Dynamic Workflow Assets** below.
- **Audience for plugin source** — `commands/`, `skills/`, `agents/`, `rules/lets-rules.md` are read by the model (orchestrator + subagents), never by humans: write terse, structured, marker-driven (`MANDATORY`/`NEVER`/`IMPORTANT`), tables over prose. Human-facing docs are `README.md` + `CLAUDE.md`. Full guidance: `CONTRIBUTING.md` "## Audience of plugin source".
- **Claude Code session-id channels (two of them — pick the right one).**
  - `${CLAUDE_SESSION_ID}` — **template substitution** in command/skill markdown: Claude Code rewrites it to the literal session UUID before the model reads the spec. Available since Claude Code v2.1.9. Use when a top-level orchestrator markdown needs the value pre-rendered (e.g. inside a markdown template the model later writes via the Write tool).
  - `$CLAUDE_CODE_SESSION_ID` — **Bash subprocess env var** Claude Code injects into every Bash tool invocation. Use inside bash commands — `bash` expands it at runtime, no template/model magic needed. **Preferred** for bash-only contexts (e.g. `bd comments add "... $CLAUDE_CODE_SESSION_ID ..."`) because it sidesteps the placeholder-skip fragility QA found in template-substitution-inside-multiline-args (`lets-bdkvd` QA #13). Also the only channel subagents and external scripts can read.
  - **Naming gotcha:** template has no `_CODE_`, env var has `_CODE_`. They are NOT aliases.
  - As of this branch, `/lets:end` + `/lets:done` use `$CLAUDE_CODE_SESSION_ID`; broader adoption (subagents stamping session_id, statusline, `/lets:team` records) tracked in `lets-bdkvd`.
- **Other Claude Code template variables in command/skill markdown.** `${CLAUDE_PLUGIN_ROOT}` (plugin install path) is substituted at command/skill load time — immune to context compaction. `${CLAUDE_PROJECT_DIR}` is NOT substituted; use `git rev-parse --show-toplevel` instead.
- Agents define WHO and HOW (expertise, behavioral modes, tiered scoring, output format). Commands define WHAT and WHEN (provide content, select agents, pass mode name)
- Agent frontmatter fields: `name`, `description`, `tools`, `color` (terminal output: red/blue/green/yellow/purple/orange/pink/cyan), optional `model` (default inherits from parent, `opus` for complex analysis). All agents use tiered scoring ([BLOCKER]/[SUGGESTION]/[NIT]), self-contained Modes, and Output Format sections.
- Agent memory (`memory: project` frontmatter) is **currently disabled** across all agents as a workaround for upstream Claude Code issue [#55648](https://github.com/anthropics/claude-code/issues/55648). Subagents writing memory skip the assigned task and return only a memory-write confirmation. Disabled via [PR #47](https://github.com/restarter/lets-workflow/pull/47). Tracked via bd tasks `lets-erx1c` (this disable) and `lets-rqwdg` (restore memory when Anthropic fixes the bug).
- Actor meta-agent loads personalities at runtime via internal skill `actor-fetch-personality`. Command fetches personality content (curl for URLs, Read for files), user confirms via review gate, actor receives it in prompt as `PERSONALITY:` block. Fallback "generalist" identity when no personality provided
- Agent selection: each command owns its detection/selection logic (different semantics per context). Multi-agent dispatching commands (review, opinion, backlog, plan) show selection panel with cost note before launch. Most agents have explicit PLAN mode for plan review.
- Agents always respond in English. Commands localize output to user's language via LETS Config and Rules.
- `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:backlog`, `/lets:research` use `subagent_type: "lets:agent-name"` to dispatch agents via Task tool (research dispatches `lets:skeptic` for its cross-check; its web fetchers are the default subagent, not `lets:*`)
- `/lets:github-pr` orchestrates `/lets:review` (delegates analysis) and handles GitHub posting, follow-up, respond, and approval directly via gh CLI
- `/lets:execute` uses EnterPlanMode for native plan mode execution with user approval gates. No subagents.
- `/lets:check` reviews inline (no subagent) for speed
- All analyst agents are read-only with uniform tools: `Read, Grep, Glob, Bash`. No `Edit/Write`. Exception: `agents/implementer.md` adds `Edit/Write` for `/lets:team` parallel implementation in isolated worktrees.
- `/lets:team` uses Agent Teams (TeamCreate, Agent with isolation: worktree) for parallel implementation. All other commands use subagents for analysis.
- All analyst agents have prompt-level read-only Bash constraints in their `## Constraints` section (identical 1-line allowlist across all 14). `hooks/validate-readonly.sh.old` exists as a PreToolUse hook prototype (not yet registered - agent frontmatter hooks silently ignored)
- **JSON envelope (`--json` subcommands).** Every `lets <sub> --json` emits a single JSON object, valid even on `ok=false` (partial-completion: `steps[]` + a typed `error`). **`SchemaVersion` is per-package** — `initcmd`/`updatecmd`/`worktreecmd` each own a `const SchemaVersion`, guarded by a `TestResult_SchemaContract` test; new `--json` packages copy `worktreecmd`'s shape. Mechanics + per-subcommand contracts: `cli/README.md` "## JSON envelope conventions".
- Interactive worktrees managed via `/lets:worktree` command. Hook prototypes `hooks/worktree-setup.sh.old` and `hooks/worktree-cleanup.sh.old` (deferred - caused agent auto-cleanup issues)
- Worktrees stored in `.worktrees/` at project root - `.lets/` symlinked for interactive sessions
- **Trunk-mode** — per-task opt-in via the `take-task` picker ("Stay on current branch"), detected at runtime by `HEAD == $LETS_MERGE_BRANCH` (no state file — survives compaction). In trunk-mode: `/lets:done` pushes + closes the task without a PR (same-source-target isn't a valid PR); `/lets:plan` derives the plan slug from task-id (avoids `.lets/plans/main.md` collisions); `/lets:execute` soft-gates instead of hard-refusing on `$LETS_MERGE_BRANCH`. An exception to the "never edit the merge-branch" rule (see `plugins/lets/rules/lets-rules.md`). For quick docs/spec/small-fix work; mutually exclusive with worktree. Implementation: `lets-3o9d7`.
- **SessionStart + PreCompact hooks** invoke `lets hook session-start|precompact`, emitting an optional `## LETS Notice` (rules-drift check) + a `## LETS Config` block. The workflow rules themselves do NOT travel through the hook — they live in `.claude/rules/lets-rules.md` (the uncapped project-instructions channel), copied there by `lets init` and frontmatter-version-tracked. Output mechanics, drift messages, and project-root detection: `cli/README.md` "## lets hook".
- The bash `session-start.sh` was deleted along with its yaml→env migration block - lets-p732a closed.
- **Dual-hook pattern** — same output on both events keeps workflow rules present across context compaction: SessionStart on a `compact` source re-injects them post-compaction; PreCompact seeds the pre-compaction context the auto-summary is built from. Prevents workflow drift in long sessions.
- User-facing skills: auto-discovered by Claude Code, appear in skill list, trigger on description match. Frontmatter description must NOT use YAML quotes.
- Internal skills: hidden from the user-facing `/lets:` autocomplete via `user-invocable: false` in the frontmatter (Claude Code supports this — see `code.claude.com/docs` skills frontmatter). The model can still use them when commands explicitly ask; commands invoke them via `Skill(skill: "lets:<name>", args: "...")`. Claude Code resolves the `lets:` namespace regardless of whether the plugin is loaded via marketplace install or `--plugin-dir` dev mode (no relative-path fragility). No accidental user triggering, no context cost until invoked. Current internal skills: `detect-task`, `actor-fetch-personality`, `pre-compact-note` (shared resume-snapshot for `/lets:note --pre-compact` and `/lets:end --pre-compact`).
- Commands define WHAT to do and orchestrate the flow. User-facing skills define full reusable flows (steps, user gates) that auto-trigger on natural language. Internal skills define shared procedures read by commands on demand. Commands delegate to skills for shared operations.
- Gate for new skills: extract only if (a) user-facing with standalone trigger value, or (b) internal logic duplicated in 3+ commands.

## Dynamic Workflow Assets (authoring standard)

A LETS command that orchestrates many agents can run them inside a Claude Code **Dynamic Workflow** (the `Workflow` tool) instead of the Task tool, so per-agent output stays off-context and only the aggregate enters the conversation. `skills/review-workflow/` is the **reference example**; `/lets:review --workflow` is the first consumer, `/lets:opinion --workflow` (the `opinion-workflow` skill) the second (CONDITIONAL adversarial-challenge stage; reuses the selected `lets:*` experts as cross-critics), and `/lets:backlog review --workflow` (the `backlog-workflow` skill) the third - Review-mode Phase 3 fan-out + Phase 4 aggregate off-context, NO Web Research stage (backlog review grounds in the project's own profile, so omitting web keeps the `--workflow` path equivalent to the standard Task path), and `/lets:research --workflow` (the `research-workflow` skill) the fourth - in-context decompose, then off-context per-sub-question web research (DEFAULT web subagent, not a `lets:*` agent - they lack web tools) → per-claim `lets:skeptic` cross-check → synthesize; the citations ARE the output. Mirrors review's find→verify (per-claim skeptic, additive flag-never-drop); the skeptic pass compares each claim against its siblings to flag contradictions (no separate deterministic pass). All opt-in; review/opinion/backlog/research `--workflow` live. `/lets:plan-workflow` ships the autonomous planning chain as a standalone **PREVIEW** command (dogfood across projects before folding into native `/lets:plan --workflow` - lets-jsw00). A `--fast` lever runs it lean (~7 agents vs ~15-25; collapse upstream panels + skip the heavy plan-review pass, quick plan-check kept), distinct from native `/lets:plan --fast` (orchestrator-only, no subagents). All multi-agent fan-out review/ideation commands (review/opinion/backlog/research) now have a `--workflow` path; `/lets:plan`'s equivalent is the standalone `/lets:plan-workflow` PREVIEW above.

**When a workflow is worth it.** A single fan-out that immediately hands back to a user gate is barely more than Task subagents - the workflow earns its keep only with a **multi-stage off-context chain** (fan-out -> reduce -> verify/judge -> aggregate) with NO user checkpoint between stages. Every checkpoint forces intermediate results back into context and breaks the off-context win. So: want to steer each step -> use Task subagents; want autonomous multi-stage off-context -> use a workflow.

**Layout & naming.** A workflow ships as a skill folder: `plugins/lets/skills/<name>-workflow/` holding `<name>.workflow.js` (the `.workflow.js` suffix is the marker) + `SKILL.md` (`user-invocable: false`; documents the `args` contract and states "this is a workflow asset invoked via `scriptPath`, not auto-triggered"). It is NOT a conversational skill - it is never `Skill()`-invoked.

**Invocation contract.** The command builds `args`, then `Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/<name>-workflow/<name>.workflow.js", args })`, then persists the returned aggregate. `${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time (the markdown carries the literal path). Script = static logic; `args` = all per-run data. The workflow runs in the **background** (returns a `runId`; the command flow resumes on the completion `<task-notification>`).

**The 6 file sections** (canonical order): META (`export const meta`, pure literal) · ARGS (defensive parse) · SCHEMAS · pure logic · prompts · orchestration -> `return`.

**6 conventions** (every workflow file):
1. Defensive `args` parse first line: `const input = typeof args === 'string' ? JSON.parse(args) : (args || {})` (the runtime may deliver `args` as a JSON string).
2. `agentType: "lets:<name>"` to reuse plugin agents - never duplicate an agent's logic in the script.
3. `schema` wherever structured output feeds aggregation (forces StructuredOutput even when the agent's own format is markdown).
4. args = dynamic data, script = static logic.
5. `meta.phases` titles match the `phase()` calls.
6. Keep-in-sync comment vs the command's prose spec - logic that exists in BOTH a no-workflow path (prose) and the JS lives twice; mark it.

**Runtime must-obey** (empirically verified - lets-odo4o):
- No filesystem - the script returns data; the command persists files.
- No sibling `import`/`require` - all logic stays inline (`await import('./x.js')` fails).
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used (the runtime wraps the body) - so the file is NOT Node-importable and has no clean unit test. Validate behavior with a live smoke test; verify pure-logic correctness by a deterministic check of a copy if needed.
- `agent({agentType})` resolves against the agents loaded at **session start** - a newly-added plugin agent is unavailable until the next session/reload. Make the script degrade gracefully when an agent can't resolve (e.g. a failed verifier -> keep the finding, flag it; never crash, never silently treat as done).

**`scriptPath` confirmed** to load an arbitrary plugin-path file (not only session-dir scripts). Reusable runtime facts also live in `bd memories` (key `claude-code-workflow-runtime-constraints-*`).

## Release Flow

Two-phase tag-driven pipeline (bump → review → tag → distribute). The full maintainer ceremony — phase-by-phase commands, prerelease handling, recovery, and the design rationale (why two phases / bash / goreleaser, prerelease CHANGELOG handling) — lives in `RELEASING.md` (see its `## Rationale` for the "why").

**Source-tree version invariant:** a single semver string spans `plugin.json`, `marketplace.json`, and `lets-rules.md` frontmatter (the binary version derives from the git tag via ldflags). Drift between any of these fails `verify-versions.yml`. Bumped once per release at ceremony time — never per change.

## File Storage

All plugin-generated files go to `.lets/` (gitignored). Never use `/tmp` or other external paths.
This includes hook debug logs, temp files, and any runtime artifacts.
**WARNING:** Always use `.lets/` (with dot prefix), never `lets/`. The dot is easy to miss in manual paths.

```
.lets/.env               # Per-project settings (LETS_LANGUAGE, LETS_MERGE_BRANCH, LETS_PR_FLOW, LETS_TRACKER, LETS_LAUNCHER)
.lets/.env.example       # Reference defaults — generated each `lets init` from canonical letsconfig.Keys defaults via renderEnvExample(). Not used by the hook; it's a user-facing template
.lets/.env.bak           # Single backup written by `RegenerateEnv` before mutation. Plugin-owned: user-created files at this path are silently overwritten — copy elsewhere for permanent backup
.lets/sessions/          # Session summaries, session-start-ref
.lets/reviews/           # Saved review reports
.lets/plans/             # Implementation plans
.lets/execution/         # Execution state (PR review: pr-{number}/, team records: team-*.json)
.lets/cache/             # Cached data (usage stats; update-check.json — /lets:update latest-release lookup, 1h TTL; task-status — rich statusline task cache id|title|notes|iso, self-refreshed via detached bd show, 90s TTL; pipeline-state-<task-id> — autonomous-pipeline phase marker <id>|<phase>|<iso>, per-task so parallel worktrees don't clobber, written by plan-workflow/execute --auto)
# Worktrees (outside .lets/ to avoid circular symlinks):
# .worktrees/            # Interactive worktrees only (agent worktrees use native Claude Code behavior)
```

Workflow rules live OUTSIDE `.lets/` because they belong to Claude Code's project-instructions channel:

```
.claude/rules/lets-rules.md  # Workflow rules (copied from plugin by `lets init`, frontmatter-versioned, customizable - tracked in git per project's choice)
```

User-scope install adds two machine-global files:

```
~/.claude/rules/lets-rules.md  # Global rules — written by `lets init --user`, synced by /lets:update's user-rules artifact; a customized (ahead) copy is reported, never overwritten
~/.lets/.env                   # User-level defaults (LETS_LANGUAGE + LETS_LAUNCHER only); project .lets/.env overrides per key; + ~/.lets/.env.bak
```

**NEVER edit `.claude/rules/lets-rules.md` (or `~/.claude/rules/lets-rules.md`) directly.** They are **installed copies**, plugin-managed. Only the canonical source `plugins/lets/rules/lets-rules.md` is edited. The installed copies are rewritten by `/lets:init` / `lets init --user` / `/lets:update` (and only via those plugin-managed paths) when the plugin's frontmatter `version` is bumped — this dogfoods drift detection live. Workflow: edit source -> bump source `version` (at release ceremony) -> commit -> release -> end user (or maintainer) runs `/lets:update` (or `/lets:init`) -> installed copy refreshed. Editing an installed copy directly bypasses drift testing and silently desyncs from source.

## Naming Convention: `LETS_*`

All plugin configuration uses the `LETS_*` prefix (UPPER_SNAKE_CASE). The prefix removes ambiguity in command instructions - `LETS_PR_FLOW=github` is unambiguously a parameter, while `github` could be the platform name.

### LETS Config keys

| Key | Source | Purpose |
|---|---|---|
| `LETS_PROJECT_ROOT` | Auto-injected by SessionStart hook | Absolute path to project root |
| `LETS_LANGUAGE` | `.lets/.env` (falls back to `~/.lets/.env`) | Default response language |
| `LETS_MERGE_BRANCH` | `.lets/.env` (falls back to the repo's origin default branch, else `main`, when unset) | Target branch for merges and PRs |
| `LETS_PR_FLOW` | `.lets/.env` | `github` \| `bitbucket` \| `local` - which PR workflow to use |
| `LETS_TRACKER` | `.lets/.env` | Task tracker. Currently `beads`. **Schema reserved** - no command branches on this yet; all task ops call `bd` regardless. Future: Linear/Jira (lets-nwwkj) |
| `LETS_LAUNCHER` | `.lets/.env` (falls back to `~/.lets/.env`) | Worktree launcher: `terminal` (default - print `cd … && claude`) \| `cmux` (open in a cmux workspace via `lets cmux`, macOS only). A preference, not a guarantee - `cmux` falls back to `terminal` when cmux/macOS is absent |
| `LETS_RULES_SCOPE` | `.lets/.env` | Where this project's workflow rules come from: `project` (own `.claude/rules` copy - default) \| `user` (deliberately no project copy; rules from the global `~/.claude/rules`). `/lets:update` reports `delegated` instead of re-creating a `user`-scoped project's missing copy. Fail-safe: any value other than `user` is treated as `project`. NOT injected into subagent context (project-only, like the others) |

User-level fallbacks: the hook overlays `~/.lets/.env` UNDER the project `.lets/.env` (project wins per key; only non-empty project values mask).

### Surface forms - where to use which form

The same `LETS_FOO` value appears in different surface forms depending on context. The orchestrator (model) handles substitution; subagents do NOT receive LETS Config injection and need explicit substitution before Task() launch.

| Surface | Form | Resolved by | Applies to |
|---|---|---|---|
| Bash block (model executes via Bash tool) | `LETS_FOO=$(...)` then `$LETS_FOO` | local shell - assign at top of every block (each Bash call is a fresh shell) | **only `LETS_PROJECT_ROOT`** (computable in-shell via `git rev-parse`) |
| Bash command snippet inside markdown (template the model runs) | `{LETS_FOO}` placeholder | orchestrator substitutes literal value before running | all keys EXCEPT `LETS_PROJECT_ROOT` (which uses bash assignment instead) |
| AskUserQuestion option `label`/`description`/`question` text | `{LETS_FOO}` placeholder | orchestrator substitutes literal value before tool call | all keys |
| Orchestrator prose, section headings, code comments | `$LETS_FOO` | orchestrator reads from injected LETS Config (reference only, never displayed) | all keys |
| Subagent prompt template | `{LETS_FOO from LETS Config}` | orchestrator substitutes literal before Task() call | all keys |

**Important note on `LETS_PROJECT_ROOT`:** the value injected in `## LETS Config` is for prompt-text reference and orchestrator substitution - it is NOT a shell variable available in Bash tool calls. Every bash block that uses the project root path must assign locally:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
```

`LETS_PROJECT_ROOT` is the **only** key computable in-shell (via `git rev-parse`). Other `LETS_*` keys (`LETS_MERGE_BRANCH`, `LETS_PR_FLOW`, `LETS_LANGUAGE`, `LETS_TRACKER`, `LETS_LAUNCHER`) have no shell-side derivation - inside bash snippets, use the `{LETS_FOO}` template form so the orchestrator substitutes the literal value before running. Trying to use `$LETS_FOO` in a bash block (without local assignment) yields empty string - silently wrong commands.

### User config file

`.lets/.env` is `KEY=VALUE` format with comments **above** keys (NOT inline - inline comments would pollute the model's context after the hook strips full-line comments). Migrated from legacy `config.yaml` to `.env` by `lets init` (`cli/internal/initcmd/migrate.go::MigrateYamlToEnv`). The yaml→env auto-migration in the SessionStart hook has been removed (lets-p732a closed). Legacy yaml deprecation removal tracked in `lets-q8xtk`.

**NOT FOR SECRETS.** The SessionStart hook injects the file's whitelisted `LETS_*` values into model context every session. Tokens, passwords, and API keys belong elsewhere: `gh auth login` for GitHub, OS keychain for general secrets, tool-specific credential files (e.g. `.beads/.env` for beads). The whitelist filter in `cli/internal/hook/sessionstart/sessionstart.go` calls `letsconfig.Names()` to get the canonical `LETS_*` keys, so unknown keys are filtered - but file mode is 644 (world-readable on disk), so secrets in `.env` would still be exposed locally. Do not add secret keys. The same applies with a strictly larger blast radius to the user-level `~/.lets/.env` - it is injected in EVERY project the user opens.

### Adding a new config key

Single source of truth: `cli/internal/letsconfig/keys.go::Keys` (canonical metadata) and `cli/internal/initcmd/render.go::Prefs.AsValues()` (Prefs↔Key wiring). The full step-by-step recipe — `Keys` entry, `Prefs` field, cobra flag, `init.md` AskUserQuestion, what's auto-derived, what to document — lives in `CONTRIBUTING.md` ("### Adding a new config key").

## Dependencies

- beads plugin (task tracking)

## When Adding/Modifying Commands, Skills, or Agents

**Rules-file rule:** edits go ONLY to `plugins/lets/rules/lets-rules.md` (the canonical source). NEVER touch `.claude/rules/lets-rules.md` (installed copy) — that file is refreshed exclusively via `/lets:init` or `/lets:update` after a source bump. This is intentional: we eat our own dogfood for drift detection.

**Unreleased-features rule:** don't document `[Unreleased]`-only features in this file as if they already shipped. If you describe a feature that isn't in a tagged release yet, tag it `(ships in vX.Y)` (or `(merged; ships next release)`); drop the tag at the release ceremony when the version moves forward. This avoids the trap QA hit on v0.5.0 — descriptions of features that lived only in `[Unreleased]` CHANGELOG read as available, sending testers down dead-end paths.

**Full file-sync checklist, the LETS box output spec, and the per-command checklist live in `CONTRIBUTING.md` ("## Editing commands, skills, agents, config keys").** Modifying a command/skill/agent? Read it — it names every file that must stay in sync: the rules `Skill Quick Reference`, every `agents/*.md` `## Constraints` line, the `/lets:install` skill tables, `README.md`, the `end.md`/`done.md` session-id channels, and the Go-subcommand wiring.
