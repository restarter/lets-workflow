# Changelog

## [Unreleased]

### Added
- **Ref-file architecture: `.task-<slug>` validated-cache state file (lets-dsdmp).** Replaces the dual-purpose `.session-start-ref-<slug>` (one SHA silently overloaded as both task- and session-boundary) with a single per-branch `.lets/sessions/.task-<slug>` carrying three individually-governed fields: `task:` (identity), `start:` (task boundary), `session: <sha> <session-id>` (session boundary). Fixes the trunk-mode `/lets:done` boundary bug (it reads the task boundary now, not the session one), gives correct per-task ranges when several tasks are worked in one worktree, and adds bd-independent READ of task identity off the merge-branch (custom worktrees / attached branches); on the merge-branch the read is validated against live bd `in_progress` status. The file is a validated cache: each reader cross-checks its field against a live anchor - bd `in_progress` status (merge-branch only), git ancestry for `start:`, the Claude session-id for `session:` - and degrades loudly rather than trusting stale state. `/lets:start` heals it; the SessionStart hook proactively refreshes the session boundary on a new session (source=startup); detect-task precedence is now file-first (explicit-arg -> file `task:` -> branch-name -> bd). Cleanup is owned (`/lets:done` rewrites to `session:`-only on close; `lets worktree remove` deletes the file), with a one-release back-compat read of the legacy ref (session boundary only).
- **`/lets:execute` start-of-run mode picker (lets-hs4fj).** Bare `/lets:execute` now asks once how to run the approved plan: **Straight-through** (here, one approval then all tasks, auto-commit at plan points), **Step-by-step** (here, pause + review after each task, confirm each commit), **Auto** (`--auto` AUTO MODE, unattended; hard-stops + push/PR/close/external still gated; refuses on the merge-branch), or **Team** (parallel implementers in isolated worktrees via `/lets:team`, review at the end). Commit cadence is derived from the mode (not a separate question). Flag shortcuts pre-answer and skip the picker entirely: `--team`, `--step`/`--step-by-step`, `--straight`/`--straight-through`, `--auto`. A remembered `LETS_EXECUTE_MODE` default is a deferred follow-up.

### Changed
- **`/lets:update` — one-command, self-driving, no half-step (lets-rlue4).** `lets update` is now order-aware and self-driving instead of a static status dump. Order-aware: when the Claude Code plugin is behind (outdated vs latest, or locally older than the binary), the workflow-rules re-copy is **`deferred`** rather than synced to the stale plugin — killing the half-step where updating the binary first stamped rules at the OLD plugin's version (only a genuinely `outdated` installed copy defers; a *missing* one still installs, an *ahead* one keeps its reset). Self-driving: the Go binary computes a single ordered `next_action` (`init → binary → plugin → reload → done`) from the artifact statuses, and `/lets:update` renders exactly that one step and re-runs until `✓ Everything on vX.Y.Z` (no status matrix on a synced machine). When the binary is behind, `/lets:update` offers to run the official installer in-session (approval-gated even in AUTO MODE, one attempt per run, source + command shown; `next_action.command` is an execution-bound compile-time const, byte-equal-tested). `/lets:init` now leads with the one-time plugin auto-update recommendation that removes the manual plugin step. The `lets update --json` schema stays `2` (`next_action` + the `deferred` status are additive). Absorbs lets-ew17g.

### Fixed
- **`/lets:check --plan` finds trunk-mode plans (lets-u06sc).** In trunk-mode (on the merge-branch) the plan lookup used the branch slug (`main`), which never matched `plan.md`'s `<date>-<task-id>.md` save name — forcing a manual `--plan <path>` every time. It now calls detect-task in plan mode and derives the slug from the task-id (with a task-id fallback glob), mirroring `/lets:execute` and `/lets:review --plan` (which already did this).

## [0.6.4] - 2026-06-12

### Added
- **`/lets:plan` — Self-evaluation option at the architecture-eval gate (lets-4x5mg).** The Step 7 "Which experts should evaluate the architecture?" checkpoint gains a **Self-evaluation** choice between *Pragmatist only* and *Skip evaluation*: no expert subagents — the orchestrator critiques its own chosen architecture (risks / overengineering / trade-offs) in the same `## Expert Evaluation` shape, then proceeds to the Evaluation Results checkpoint. Unlike *Skip* it still produces an evaluation; unlike *Full panel* / *Pragmatist only* it spends no agent budget. Reuses the existing `--fast` self-eval block (now shared by both triggers), so it's a per-stage lean lever without committing the whole `--fast` flag.
- **`/lets:research` — web-first sourced-answer command (lets-ppetz).** Answers an external/technical question by decomposing it into 3-6 sub-questions, searching + fetching the web per sub-question (each returns its 2-5 strongest claims, with `evidence` quoting the actual source material), and synthesizing a CITED answer with a Sources list, an as-of date, and an overall confidence note. The differentiator vs a naive search dump is a verify pass: a per-claim `lets:skeptic` cross-check (new RESEARCH-VERIFY mode) that compares each claim against its siblings to flag contradictions, on top of deterministic single-source / low-confidence flags — additive, never silently drops, and a per-claim "unverified (cross-check errored)" flag when the skeptic can't run. Web-unavailable degrades to a `NO LIVE SOURCES — model knowledge as of <cutoff>` banner (never fabricated URLs); all fetched content is treated as untrusted reference data, never instructions. `--workflow` runs decompose→research fan-out→cross-check→synthesize off-context via the committed `research-workflow` asset (only the synthesis enters context); `--project` grounds findings against this repo while keeping the PROJECT_ROOT read boundary. Distinct from `/lets:opinion` (project-grounded judgment, no web) and `/lets:ask` (model-knowledge consult). Out of scope for v1: auto-creating bd tasks from findings, multi-page `--report` output, non-web sources.
- **First-class user-scope install (lets-wug9k).** `lets init --user` (offered by `/lets:init` when the plugin is installed at user scope) installs global workflow rules to `~/.claude/rules/lets-rules.md` and user-level defaults (`LETS_LANGUAGE`, `LETS_LAUNCHER`) to `~/.lets/.env` — one install covers every project on the machine. The SessionStart/PreCompact hook is scope-aware: no more "run /lets:init" nag in projects covered by global rules, project `.lets/.env` overlays user defaults (project wins per key), and `LETS_MERGE_BRANCH` falls back to the repo's origin default branch when unset anywhere. `/lets:update` gains an optional `user-rules` artifact (row omitted when the file is absent) that re-syncs the global copy — never overwriting a customized/newer (ahead) one, the documented per-project opt-out. The "rely on global rules" choice is persisted via `LETS_RULES_SCOPE` (`project`|`user`) in `.lets/.env`: `/lets:update` reports `delegated` instead of re-creating a skipped project copy, and the hook points at `lets init --user` when the global copy a project relies on goes missing. Also fixes a pre-existing gap: a customized `LETS_LAUNCHER` now survives init/update regens. README + installation docs drop the "avoid user-scope" warning; precedence and the no-per-project-opt-out limitation (Claude Code #8395) are documented.
- `/lets:plan-workflow --fast` (lets-tj5jd): the same autonomous off-context Dynamic Workflow planning chain on a minimal agent budget (~7 agents vs ~15-25) — 1 explorer over a merged area, 1 architect to propose approaches, 1 architect for the top rubric-ranked approach, 1 judge, 1 evaluator, 1 planner, plus the quick plan-check (+ refine only if needed); the heavy plan-review pass is skipped (`refinement_log.review_skipped = true`). Distinct from native `/lets:plan --fast` (orchestrator-only, no subagents, in-conversation): cheap workflow vs no workflow.
- **`/lets:backlog --fast` — quick no-agent backlog pulse (lets-942yz).** Orchestrator-only Fast mode for `/lets:backlog`: a fast context scan (bd stats/list, git log, TODO grep) and a direct conversation with no subagents — the lightweight sibling of the agent-backed Review, following the house `--fast` convention (no flag = subagents, `--fast` = orchestrator-only). This is the former `/lets:brainstorm` body, relocated to where backlog work lives; `--fast` wins over `--workflow` and is ignored with `cleanup`.

### Removed
- **`/lets:brainstorm` and `/lets:explore` removed (lets-942yz).** The ideation/thinking command family is consolidated to two surfaces: project-grounded judgment and ideas → `/lets:opinion` (subagents read your code, no web), external sourced answers → `/lets:research` (cited; `--project` grounds against the repo). `/lets:brainstorm`'s no-agent backlog pulse moves to `/lets:backlog --fast`; `/lets:explore` (multi-agent topic ideation, ~0 usage) is dropped along with its `explore-workflow` Dynamic Workflow asset — its niche is covered by `/lets:opinion` + `/lets:research --project`. Clean removal: the `brainstorm`/`explore` trigger words no longer fire anything.

### Fixed
- (lets-i85v9): plan-workflow architects no longer thrash on StructuredOutput validation when an approach creates no new files — `ARCH_SCHEMA` requires only `summary` (`files_create`/`files_modify` optional), and the architect prompt instructs `[]` over omission + terse strings against payload truncation.

## [0.6.3] - 2026-06-08

### Added
- **Autonomous task pipeline + `lets cmux notify` (lets-m8ecy).** A near-autonomous spawn → plan → execute pipeline: `/lets:worktree create <id> --flow plan-workflow --auto` opens a worktree that claims the task (resolve-and-claim convention, AUTO-MODE-exempt entry claim) and lands in autonomous planning (`plan-workflow`, falling back to interactive `--flow plan` when the PREVIEW workflow is unavailable); the human answers a bounded up-front clarify gate and approves the plan file; `/lets:execute --auto` then runs the approved plan without per-step gates (hard-stops preserved — push / PR / `bd close` / external stay gated, and `--auto` REFUSES on the merge-branch rather than auto-entering trunk-mode). cmux notifications fire at the human gates via a new `lets cmux notify` subcommand (macOS-only, never hard-fails; the non-unix stub emits a parseable `ok=true` envelope), marker-gated by a per-task `.lets/cache/pipeline-state-<id>` file so only autonomous runs notify and parallel worktrees don't collide. `--flow` only swaps the launch `--command` (launcher-agnostic — terminal / cmux / tmux inherit). The statusline phase-row that renders the marker is a deferred follow-up.
- **`/lets:statusline` + `lets statusline config` — persist statusline appearance (lets-vpwvs).** The render flags (`--light`/`--compact`/`--no-tip`/`--no-dir`/`--no-task`) were render-only — running them just drew once, the choice never stuck without hand-editing settings. New `lets statusline config` persists the chosen appearance to your personal, gitignored `.claude/settings.local.json` (not the tracked `settings.json`, so `--light` is never forced onto collaborators); it rewrites only the `statusLine` key (preserving other local keys), refuses to clobber a foreign command without `--force`, and supports `--show` (read current), `--reset` (back to defaults), and `--json`. A zero-flag write with neither `--reset` nor `--show` is rejected to avoid an accidental reset. `/lets:statusline` is the interactive front door (AskUserQuestion → `lets statusline config … --json`). Restart Claude Code to apply (the statusLine command is read on session start). Backed by a new `statuslinecmd` Go package (own JSON envelope + typed exit codes, mirroring `worktreecmd`).
- **cmux session identity + autonomous launch (lets-nddb8).** `lets cmux open` gains `--description`, and `/lets:worktree create` now stamps each cmux workspace with `<task-id> · <title>` (the canonical beads id + full title, shown in cmux's `workspace list` / tooltip) so parallel sessions self-identify. A new `--auto` override launches the worktree session in `claude --permission-mode auto` (autonomous — still gates push / PR / `bd close` / external per AUTO MODE rules; never `bypassPermissions`), uniform across the terminal and cmux launchers. (cmux notifications are now wired by lets-m8ecy above — `lets cmux notify` driven at LETS gate points.)
- **Autonomous pipeline + plan-family discoverability (lets-0uabp).** The autonomous task pipeline (lets-m8ecy) and the plan-family variants are now discoverable to humans and the model: the workflow rules `## Session Flow` names the pipeline as a flow (`--flow plan-workflow --auto`) and lists it under "when to use which"; a new human-facing **[docs/autonomous.md](docs/autonomous.md)** maps the hands-off flows (Dynamic Workflows `--workflow` + the pipeline runbook + degradation / prerequisites, linking rather than duplicating the lifecycle / loop / review docs); and `/lets:start` offers a plan-family picker (`/lets:plan` / `--fast` / `/lets:plan-workflow` PREVIEW) for Medium/Large tasks. `docs/commands.md` gains the `--flow` / `execute --auto` / `plan-workflow` flags. Docs / rules only — no runtime change, no version bump.

### Changed
- **Statusline tips + `/lets:start --main` discoverability (lets-1f6z8).** The rotating statusline tips now cover the commands shipped in 0.6.2 — `/lets:backlog`, `/lets:review-round`, `/lets:start --main`, `/lets:explore`, and the `LETS_LAUNCHER=cmux` worktree launcher — and the stale "/lets:brainstorm reviews the backlog" tip was corrected (brainstorm is Quick-only now). `--main` was added to the `/lets:start` `argument-hint` so Claude Code's command autocomplete surfaces the project-assistant mode, and `LETS_LAUNCHER` was added to the two CLAUDE.md prose key-lists that had omitted it (the canonical config-keys table already listed it).

### Fixed
- **`lets worktree create` can attach a slash-bearing branch (lets-x5ucf).** A new `--branch <ref>` flag decouples the worktree directory name from the branch ref, so git-flow / Bitbucket branches like `feature/pwa-46696` can be attached (or created) even though the worktree dir name forbids `/`. Previously such a branch was unreachable — the name validator rejected the slash and there was no way to name the branch separately from the directory. The dir name keeps its strict allowlist; the explicit ref is validated by `git check-ref-format` (slashes allowed, leading `-` rejected) and used verbatim with no `worktree-` prefix. `/lets:worktree create` auto-routes a slash name to a sanitized dir name + `--branch`. No JSON schema bump — the envelope already distinguishes `worktree.name` (dir) from `worktree.branch` (ref).
- **Worktree symlinks no longer show as untracked in a worktree (lets-x5ucf).** `.lets` and `.beads/.env` surfaced as `?? ` noise inside a worktree because the tracked `.gitignore` the worktree checks out from its branch can lack the entry (or carry a directory-only `/.lets/` that can't match the `.lets` symlink). `lets worktree create` now also writes the narrow entries `.lets` and `.beads/.env` to the shared `.git/info/exclude` (effective in every worktree, untracked, never pushed) so the LETS-managed symlinks are reliably ignored — while leaving any other untracked `.beads/` content visible.
- **`lets statusline` no longer hangs when run interactively (lets-7frjs).** Running it by hand blocked forever on the stdin read (it expects Claude Code's JSON context). It now detects a character-device (TTY) stdin and prints a one-line wiring hint (preview command + `lets init`) and exits 0 instead of hanging — the natural first-touch UX no longer looks like a freeze.
- **Statusline box border no longer drifts on the location pill (lets-6md86).** The pill's `☰` mark (U+2630) was font-substituted to a 2-cell glyph on some terminals (cmux/Ghostty) while the box width math counted it as 1 cell, pushing the right border out by a column on the identity row. Swapped it for `»` (U+00BB), which is present in standard monospace fonts at 1 cell; the width-caveat docs now cover font substitution (not just East-Asian ambiguous width) and the `printf` ruler check.
- **Light-theme statusline pill nudged back to a visible label (lets-nl0ks).** The light-palette pill background shipped in 0.6.2 (`246,243,237`) blended too far into a white terminal — the location/worktree label all but disappeared. Moved it back toward visible (`246,243,237` → `238,234,225` / `#EEEAE1`) so the pill reads as a small but present label, without returning to the heavy `230,226,216` block. Dark palette unchanged.

## [0.6.2] - 2026-06-06

### Added
- **cmux as an optional macOS worktree launcher (lets-3jutw).** A new `LETS_LAUNCHER` config key (`terminal` default | `cmux`, asked by `/lets:init`) lets `/lets:worktree create` open the new worktree session inside a [cmux](https://github.com/manaflow-ai/cmux) workspace running `claude '/lets:start <id>'` — one command instead of opening a second terminal — with a readable workspace slug derived from the task title. Backed by a `lets cmux` Go subcommand (`open` + `rename`, `//go:build unix`, Windows stub) that wraps `cmux workspace create/list/rename`. **Strictly optional and never hard-fails:** it detects cmux on PATH + macOS and silently falls back to the `cd … && claude` terminal flow when absent, so non-macOS / no-cmux setups are unchanged. `open` carries a duplicate-session guard (refuses a second workspace for a worktree that already has one — `--force` overrides), and `lets cmux rename` lets a session relabel its own tab to disambiguate concurrent sessions. Override per-run with `/lets:worktree create <name> --cmux` / `--no-cmux`.
- **`/lets:backlog` — backlog review + cleanup command (lets-9r4at).** Extracted from `/lets:brainstorm`: a two-mode command (`/lets:backlog review` | `/lets:backlog cleanup`, or a menu when called bare). *Review* launches an explorer scout + parallel domain agents that ideate over the backlog and aggregate multi-perspective insights (the former brainstorm Heavy mode, moved verbatim); *Cleanup* is fast interactive triage of stale/duplicate/orphan/unassigned tasks (no agents). Continues the decomposition started when `/lets:explore` was split out for topic ideation.
- **`/lets:backlog review --workflow` — off-context Review fan-out (lets-9r4at).** Opt-in Dynamic Workflow variant of Review mode via the committed `backlog-workflow` asset: the explorer scout (Phase 1) and agent selection (Phase 2) stay in-context while the parallel domain-agent fan-out (Phase 3) + the semantic aggregate/cluster (Phase 4) run off-context — only the clustered, impact-sorted result returns. Unlike `/lets:explore --workflow` there is no Web Research stage: backlog review ideates over the project's own state profile, so omitting web keeps the `--workflow` path equivalent to the standard Task path (a pure performance lever, same findings). The interactive dialog + task capture stay in-context.
- **`/lets:review-round` — work through a RECEIVED review round (lets-lqdtz).** The inverse of `/lets:review`: instead of generating findings, it consumes a multi-comment review round on a spec/doc/PR. Triage each comment (accept/reject/defer/done), record decisions to the beads task as you go, keep the artifact FROZEN during triage, then apply all accepted edits in ONE final pass — cascading reframes change earlier items, so a half-edited artifact mid-round is inconsistent. Verifies falsifiable reviewer claims against the real code, surfaces premise-level reframes before nits, and for GitHub PRs hands the per-comment summary to `/lets:github-pr --respond`.
- **`/lets:start --main` — no-task project-assistant / PM mode (lets-jzimq).** A new `--main` (alias `--assistant`) entry-mode for `/lets:start` that skips the mandatory task-selection gate and enters a persistent project-assistant / PM stance: it stays on `$LETS_MERGE_BRANCH` in read + triage mode (backlog grooming via `/lets:backlog`, strategy, task creation, note capture) and routes you to the right `/lets:*` command when concrete work starts. Code edits still require claiming a task — the merge-branch boundary is unchanged, so on edit-intent the session hands off to `take-task` / `create-task` instead of refusing. The persona is hardcoded inline for v1 (persona registry / custom source / `--as` review persona are a deferred follow-up). Rules carve-outs name the mode in Task Selection, Boundaries, and Session Flow.

### Changed
- **`/lets:brainstorm` is now Quick-only ideation (lets-9r4at).** Slimmed to a single fast flow — a quick context scan (no agents) then a direct conversation — after Review-backlog and Cleanup moved to the new `/lets:backlog`. The "specific topic → `/lets:explore`" handoff is preserved (new Step Q0); the Step 0 mode menu is gone.

### Fixed
- **Plan filenames are date-prefixed + plan lookups are slug-scoped (lets-fe788).** `/lets:plan` now saves plans as `.lets/plans/YYYY-MM-DD-HHMM-<slug>.md` (same convention as `.lets/sessions/`), so a second plan on the same branch no longer overwrites the first — plan history is preserved. The three readers — `/lets:execute`, `/lets:review --plan`, `/lets:check --plan` — resolve the latest plan slug-scoped (`ls -t .lets/plans/*<slug>*.md | head -1`) instead of a global latest, fixing cross-worktree shadowing where the symlink-shared `.lets/plans/` made `ls -t *.md` grab another worktree session's plan. Each reader has an empty-slug guard (prevents the glob collapsing back to global latest), a task-id fallback, and `/lets:review --plan` now calls detect-task in plan mode so the fallback resolves. Legacy bare-name plans still resolve (wildcard matches both formats).
- **`/lets:update` artifact table no longer reads as self-contradictory (lets-kaw72).** `.env` and `rules` now report a distinct `in-sync` status (they track a *local* source — the `lets` binary and the plugin respectively) instead of `up-to-date` (reserved for `binary`/`plugin` vs the *latest release*), so two in-sync rows at different versions stop looking contradictory; each row names what it tracks and appends "itself behind latest v…" when that source is itself outdated (covers the uniformly-behind case the `consistent` warning missed). A re-copied `rules` row now carries a past-tense detail ("was missing" / "was outdated (v…)") instead of the pre-install imperative ("Run /lets:init") that contradicted its own `updated` status. `lets update --json` is now `schema_version=2`.
- **Light-theme statusline pill softened (lets-nl0ks).** The light-palette location/worktree pill used a beige-grey fill that read as a heavy block on white terminals; lightened it (`230,226,216` → `246,243,237`) so the pill stays subtle. Dark palette unchanged.

## [0.6.1] - 2026-06-05

### Added
- **`/lets:plan-workflow` — autonomous planning Dynamic Workflow, PREVIEW (lets-y3aic).** A standalone preview command that runs whole-command planning off-context: you give a goal + a rubric up front, and the workflow explores, proposes approaches, architects each, judges them against your rubric, evaluates the winner, and writes a plan — you approve at the end (steer-by-rubric, not gate-each-step). Shipped standalone to dogfood across projects before it folds into native `/lets:plan --workflow` (lets-jsw00); experimental, expect rough edges. The interactive native `/lets:plan` is untouched.

## [0.6.0] - 2026-06-05

### Added
- **`/lets:explore` — topic-exploration command + `--workflow` (lets-odo4o).** Extracted from `/lets:brainstorm` into its own command: a scout gathers project context, an always-on Web Research stage pulls CURRENT community standards via WebSearch/WebFetch, then parallel domain agents surface insights, open questions, and approaches grounded in that brief. `--workflow` runs the fan-out off-context via a Dynamic Workflow (web research → ideate → semantic cluster); `--no-web` skips the web stage; an off-project guard handles topics unrelated to the repo. The cluster stage semantically merges convergent ideas across agents (real multi-agent attribution, title-only fallback). `/lets:brainstorm` is now backlog-only (3 modes).
- **`/lets:review --workflow` + adversarial finding-verification (lets-odo4o).** Opt-in off-context Dynamic Workflow variant; every `[BLOCKER]`/`[SUGGESTION]` is refuted by `lets:skeptic` agents before being reported (asymmetric drop rule). Verification runs in both standard and `--workflow` modes — `--workflow` is a pure off-context performance lever.
- **`/lets:opinion --workflow` + conditional challenge (lets-odo4o).** Opt-in off-context variant; adds an adversarial-challenge stage that fires only on weak consensus, reusing the selected experts as cross-critics.
- **Rich statusline is now the default (lets-ds6bc).** `lets statusline` renders a boxed multi-line statusline; `--compact` falls back to the legacy 2-line output (kept for terminals where the box misbehaves). Flags: `--light` (default dark), `--no-tip` (env `LETS_STATUSLINE_TIP=off`) hides the bottom tip line, `--no-dir` (env `LETS_STATUSLINE_DIR=off`) hides the location pill, `--no-task` (env `LETS_STATUSLINE_TASK=off`) hides the task line and skips its background `bd` refresh; `--rich` is a hidden accepted no-op. Two `COLUMNS` tiers — **Full** (≥ 72) and **Compact** (< 72), each in a closed box (`┌─┐ │ ├─┤ └─┘`) that is cell-accurate so the right border aligns; below 90 cols the box fills the window, above it hugs the content (capped at 120), with a small right gutter so it never overflows Claude Code's render area. Lines: identity (`⚘` brand + version, a `☰` location pill showing the project/worktree folder name — the git top-level basename, stable across `cd` into subdirs — or the literal word `worktree` inside a worktree, branch shown verbatim, diff, PR), budget (model with its dimmed `(… context)` suffix + color-graded effort + `window % (used/total) · 5h % (Δ) · 7d % (Δ)`), task (id + title that clips first, keeping `N comments`/age/`← /lets:note`), and a rotating tip. In the **Compact** tier the model name and effort stay (only the `(… context)` paren is shed and `window`→`w`); the location pill and PR drop. **All glyphs are monochrome 1-cell text** (`⚘ ☰ ✦ ✓ ⎇ ⇄ ←`, tip `*`) that take the theme accent via ANSI — no emoji, so `wideRunes` is empty and the border never drifts. Separators are a single neutral gray middot `·` throughout. Usage gauges color the whole `window 44%` pair by threshold (green < 50, amber 50-74, red ≥ 75). The task line reads a `.lets/cache/task-status` cache the binary self-refreshes in a detached `bd show` subprocess (90s TTL); rate limits come straight from the Claude Code payload when present, with the Keychain/usage-API fetch as a now-gated fallback. Every externally-sourced rendered field (folder/branch/worktree name, model, PR state, effort, bd title) is scrubbed of terminal control bytes (C0/ESC/DEL/C1) before rendering — escape-injection defense. Resilient to Claude Code payload quirks: `workspace.git_worktree` sent as a string and `rate_limits.*.resets_at` sent as a number both decode without blanking the bar. (The session-growth emoji ladder is implemented but parked behind the static `⚘` mark.)
- **`/lets:note --pre-compact` — snapshot the session before compaction (lets-lwut3).** A new `--pre-compact` (alias `--resume`) mode on `/lets:note` writes a single recovery-grade `## RESUME` comment to the active task — where things live (branch@sha, key `file:line`, external sources + recovery commands), state (committed/uncommitted, frozen SHAs), decisions already made, and the explicit next step — so a future session or another agent can fully reconstruct the working context after `/compact` summarizes the conversation. Falls back to a `.lets/sessions/` file when there is no active task.
- **Statusline tips now surface the new features (lets-h63lo).** The rotating bottom-line hints in `lets statusline` gained coverage for `--workflow` (off-context fan-out + skeptic finding-verification on `/lets:review` / `/lets:opinion` / `/lets:explore`), `/lets:explore` and its `--no-web`, the `--no-task` / `--no-dir` / `--no-tip` trim flags, and `/lets:note --pre-compact`.
- **`/lets:end --pre-compact` + shared `pre-compact-note` skill (lets-jvbft).** `/lets:end` gains a `--pre-compact` (alias `--resume`) mode: a pre-compaction snapshot that writes the recovery-grade `## RESUME` comment to the active task plus a session-summary file, then returns WITHOUT ending the session (no push, no task-status prompt) so you can `/compact` and keep working in the same window. The resume-snapshot step is extracted into a shared internal `pre-compact-note` skill that `/lets:note --pre-compact` now delegates to as well — single source of truth, no template drift.

### Fixed
- **Commit skill stages context-aware (lets-vdbhz).** `/lets:commit` no longer runs a blanket `git add -A`, which could sweep in unrelated changes, untracked cruft, or secrets (`.env`, keys) and clobber a curated staged set. Step 5 now respects an already-staged set as-is, otherwise stages the reviewed files by name — consistent with `.claude/rules/git.md`.

## [0.5.5] - 2026-05-27

### Added
- **`/lets:check --branch` and `/lets:review --branch` (lets-id3d6).** Full-branch review against `$LETS_MERGE_BRANCH` using a three-dot diff (`git diff main...HEAD` — same shape GitHub renders for a PR), so you can do a PR-equivalent review locally before pushing without needing a draft PR. Guards: refuses on the merge branch itself, refuses if `$LETS_MERGE_BRANCH` is unset/empty (mirrors `/lets:done`'s pattern), refuses if the merge ref is missing locally (with a `git fetch origin MB:MB` hint that creates the local branch, not just the remote ref), refuses if zero commits ahead. JSON `mode` values: `check-branch` (preserves check.md's `check-*` prefix) and `branch-review` (preserves review.md's `{kind}-review` naming consistent with `local-review` / `plan-review`). Branch reviews save to a dedicated artifact `.lets/reviews/{date}-branch-review.md` (PR-equivalent diff deserves its own file). Workflow Option A in `review.md` documents `--branch` as an optional final pass for multi-commit branches before pushing.
- New `## AskUserQuestion Conventions` section in `lets-rules.md` codifies 8 rules for `AskUserQuestion` calls: header chip style (4-12 chars descriptive, `"LETS"` forbidden), question wording, label/description format, `multiSelect` use, `preview` constraints, follow-through (invoke via Skill tool), and skip-when-one-action. Includes a worked example. CLAUDE.md "When Adding/Modifying" table and "Command Checklist" point at it. (lets-uffs7)
- Auto-execute follow-through: when an `AskUserQuestion` option's `label` or `description` names a `/lets:*` command (e.g. `/lets:done`'s "End session" option, whose description contains "Run /lets:end - save context and wrap up"), the model now invokes that command via the `Skill` tool (`Skill(skill: "lets:<name>", args: "<args>")`) instead of narrating "now run /lets:end". The invoked command's own approval gates still apply (push, close, external-facing ops). Exceptions for prose-only patterns (qualifiers like `later`/`if needed`, cross-terminal hints, `/clear`-chained workflows) are documented in Rule 7. (lets-sc15i + lets-5pkvh)
- **`lets worktree create/remove/list/info` Go subcommand (lets-rqep4).** New package `cli/internal/worktreecmd/` (build constraint `//go:build unix`) owns all filesystem/git operations for interactive worktrees: validate name (positive allowlist + `git check-ref-format`), guard not-inside-worktree, attach vs new-branch auto-detect, `.gitignore` hardening (now race-safe via flock + integrity check in `initcmd.EnsureGitignore`), atomic `git worktree add`, `.lets/` wholesale symlink, `.beads/.env` targeted symlink + chmod 0o600/0o700 (no `.beads/redirect`), verify, and rollback (residual paths surfaced in JSON envelope; no log file). `lets worktree remove` blocks on unpushed commits as well as uncommitted changes (`ExitUnpushedCommits=21` + `error.kind=unpushed_commits` — parity with the pre-rewrite markdown safety net). JSON envelope (`schema_version=1`), typed exit codes 10..21 (22..29 reserved for `adopt`) routed through `main.go`'s `exitCoder` interface so scripts can branch on failure kind, `--print-cd` flag for shell composition (`tmux new-window -c "$(lets worktree create foo --print-cd)" claude`). Windows ships a no-op stub (`cli/internal/cli/worktree_stub.go`, `//go:build !unix`) returning a structured "not yet supported" error for each of the 4 subcommands; full Windows port (requires replacing `syscall.Flock` with `LockFileEx`) tracked in lets-rqep4 backlog.
- **Local dev workflow: `make dev` / `make dev-tmux` (lets-rqep4).** New `scripts/dev/run.sh` orchestrates per-worktree development of the LETS plugin without polluting the global install: builds `cli/lets` with `dev-<branch>-<sha>[-dirty]` ldflag stamping, prepends `<worktree>/cli/` to PATH, and execs `claude --plugin-dir <worktree>/plugins/lets` (or a tmux session with one pane per worktree). Production `lets` at `~/.local/bin/lets` is untouched. CLAUDE.md "Local Development" section covers the workflow + 4 gotchas (LETS_ENV_VERSION stamping, Claude-inside-Claude tool-harness deadlock, old worktrees missing the tooling, PATH-shadowing trust caveat).
- **Trunk-mode — `take-task` picker option "Stay on current branch" (lets-3o9d7).** A third workspace option in `take-task` Step 4 — skips `git checkout -b` and stays on the current branch (often `$LETS_MERGE_BRANCH`, but works for any pre-existing branch). Downstream commands detect trunk-mode at runtime via `HEAD == $LETS_MERGE_BRANCH` (HEAD-based, no state file, no marker — survives context compaction). In trunk-mode: `/lets:done` pushes (upstream-aware: `rev-parse --abbrev-ref @{u}` guard + `git push -u origin` for first-push, no silent skip when upstream is missing) and `bd close` instead of creating a PR (same-source-target is not a valid PR — GitHub returns `422 No commits between ...`); `/lets:plan` derives plan slug from task-id (avoids `.lets/plans/main.md` collisions across tasks); `/lets:execute` soft-gates with AskUserQuestion ("Take a branch" / "Continue here" / "Cancel") instead of hard-refusing on the merge-branch. `lets-rules.md` line 32 gains a sub-paragraph exception clause for trunk-mode opt-in; new `Trunk-mode` line in Session Flow for discoverability. Mutually exclusive with worktree (picker doesn't fire inside worktrees). Bonus pre-existing fix: `/lets:done` PR body and bd-comment templates `main..HEAD` → `{LETS_MERGE_BRANCH}..HEAD`. CLAUDE.md `## Architecture Decisions` gained a matching bullet.

### Changed
- **`/lets:worktree` markdown skill rewritten as thin dispatcher (lets-rqep4).** Shrunk from 455 → 317 lines; shells out to `lets worktree --json` and renders the result. Adds explicit attach-existing-branch mode (auto-detected; `--attach`/`--new-branch` to force), opt-in `--switch-main-if-needed` for attaching the branch checked out in main (requires clean tree), and a credential-sharing threat-model note. Drops dependency on `bd worktree create/remove`. Migration recipe for legacy `.beads/redirect`-bearing worktrees included.
- **`version.IsDev()` recognizes `dev-<metadata>` stamps (lets-rqep4).** Previously only the literal `"dev"` sentinel was treated as a dev build; `scripts/dev/run.sh` now stamps richer values like `dev-feat-abc1234[-dirty]`. The widened predicate (extracted into `version.IsDevString(s)` for parameter-style callers) flows through statusline (no `v` prefix), `lets update`'s `.env` regen skip, and the same command's binary/plugin/consistency checks — a dev binary no longer silently reports as "up to date with latest" via invalid-semver compare collapsing to 0.

### Changed (workflow / docs)
- **`/lets:review` interactive menu — `Staged` dropped, `Branch` added (lets-id3d6).** The 4-option AskUserQuestion menu (Claude Code schema cap) now offers `Local changes / Branch / Plan / Last commit`. `--staged` keeps working as a flag — `/lets:review --staged` is the explicit path. Rationale: `Branch` covers the more common "review what I'm about to push" workflow; `Staged` was the rarer pick (typically reached via `git diff --staged` before commit). This is the first time an option has been removed from the menu in `review.md`'s history.
- Response language rule is now MANDATORY with explicit slash-command disambiguation. Slash commands (`/lets:start`, `/lets:done`, etc.) are command syntax — they do NOT override `$LETS_LANGUAGE`. A fresh session whose first user message is a slash command now responds in `$LETS_LANGUAGE`, not English. The SessionStart hook's `LETS_LANGUAGE` description trimmed to factual metadata with a pointer to the rule (single source of truth). (lets-00sdu)
- `header: "LETS"` brand chip replaced with descriptive 4-12 char topic chips across 48 sites in 16 files (`"Uncommitted"`, `"Next step"`, `"PR flow"`, `"Language"`, `"Finish"`, `"Confirm"`, `"Architecture"`, `"Experts"`, etc.) per the new conventions and the AskUserQuestion tool spec. The orchestrator translates `question` / option `description` strings to `$LETS_LANGUAGE` at call time as usual. (lets-uffs7)
- **Skill tool migration — 17 pre-existing Read+follow sites (lets-5pkvh).** Replaced all pre-existing command-to-skill / command-to-command invocations with `Skill(skill: "lets:<name>", args: "...")`: `detect-task` ×12 (across 10 commands + `skills/commit`), `actor-fetch-personality` ×4 (`opinion`/`review`/`brainstorm`/`ask`), `take-task` delegation in `start.md`. Skill tool properly substitutes nested `${CLAUDE_PLUGIN_ROOT}` references where Read tool returned literal placeholders.
- **`commands/commit.md` removed (lets-5pkvh).** The 3-line delegation shim is superseded — `skills/commit/SKILL.md` is user-invocable by default and its description lists `/lets:commit` as a trigger, so the slash command resolves directly to the skill.
- **Rule 7 rewrite + doc sync (lets-5pkvh).** Rule 7 in `plugins/lets/rules/lets-rules.md` rewritten (~370 → ~125 words) — the Skill tool handles namespace resolution, so path-resolution boilerplate is no longer needed. Worked example, CLAUDE.md surfaces (architectural lines, "When Adding/Modifying" table row, internal-invocation note, Command Checklist item) all switched from "Read tool" to "Skill tool" wording in lockstep.
- **Model now silently ignores `TaskCreate`/`TodoWrite` system-reminders (lets-imi3b).** beads (`bd`) is the sole task tracker for LETS projects, so Claude Code's harness reminders about its internal task list were conversational noise. Codified in a new `### Task Tracker` subsection of `lets-rules.md`'s `## Beads Best Practices`.
- **English-only enforcement for written artifacts (lets-dmsnw).** Rule 2 in `lets-rules.md` rewritten as a principle ("Written artifacts MUST be in English regardless of conversation language or `$LETS_LANGUAGE`") with examples spanning code, commits, docs, beads tasks (titles/descriptions/labels/comments), plan documents (`.lets/plans/`), PR titles/descriptions, and external API posts (GitHub, Linear, Slack) — closes the gap that let Ukrainian beads task content be created during a Ukrainian conversation. The `create-task` skill gained a prominent `## IMPORTANT: Language` block before `## Why This Exists`, its frontmatter description now mentions "enforces English-only task content" for trigger-search visibility, and Step 3 (Present for Approval) was upgraded from free-text "wait for approval" to an explicit `AskUserQuestion` (Create / Cancel) for parity with `take-task` and `commit`.
- **`/lets:done` survives an already-merged branch + no-hard-wrap rule (lets-mzeys).** `/lets:done` gained an Already-Merged Guard - when the feature branch is already merged (e.g. the PR was created and merged in a parallel session), it skips push + `gh pr create` and routes to a compact close-and-sync path instead of crashing with `GraphQL: No commits between ...`. Separately, `lets-rules.md` `## Language & Communication` gained a "No hard-wrapping in prose" rule - markdown paragraphs are written as one continuous line, never column-wrapped.

### Removed
- **`make dev-shell` (lets-vxz1z).** Removed the subcommand: PATH prepend was silently wiped by user `.zshrc` (path=(...) reassembly on macOS+zsh), so the subshell ran the production `lets` instead of the dev-stamp. Not part of the test flow (`make dev` / `make dev-tmux` `exec claude` directly, no shell rc). Removed rather than hardened — convenience nobody used.

### Fixed
- `(Recommended)` marker placement — was inconsistently put in `description` in two sites (`init.md` beads option, `plan.md` expert-selection preset); now uniformly in `label` first, per tool spec. (lets-uffs7)
- `Handle response` blocks for AskUserQuestion options naming a `/lets:*` follow-up — model previously narrated "suggest /lets:X" / "run /lets:X" instead of executing. Now the spec explicitly tells the model to invoke the target via the `Skill` tool. Affects `done.md` (5 spots), `end.md` (2), `skills/take-task/SKILL.md` (1), `execute.md` (1, fix-up). Cross-terminal hints ("Switch to main repo terminal and run …") kept as prose. (lets-sc15i + lets-5pkvh)
- **`lets worktree create --print-cd` help text (lets-vxz1z).** Promised "JSON to stderr" unconditionally. Actual contract (per `worktree.go:20-21` + `TestWorktreeCreate_PrintCD_PathOnly`): stderr empty by default; stderr=JSON only with `--json`; stderr=human prose only with `--verbose`. Help text updated to match.
- **`not_in_repo` error output unified across worktree subcommands (lets-vxz1z).** Three `Error` sites (`info`/`list`/`remove`) had `Kind` set but no `Message`, so stderr printed `not_in_repo: ` (empty after colon). `info` additionally double-printed: `RenderInfo` emitted "Error: ..." on stdout while `main.go` emitted "kind: " on stderr. Added `Message` at the 3 sites; `info` RunE now skips `RenderInfo` when `!res.OK` so `main.go` owns the single error print. All 4 subcommands now emit `not_in_repo: not inside a git repository` (exit=10) consistently.
- **`lets worktree --json` envelope on RunE early-returns (lets-vxz1z).** `create --attach --new-branch --json` (`flag_conflict`), `{create,remove,list} --json` from outside a repo (`not_in_repo`), and `info --json` after `getwd()` failure (`getwd_failed`) emitted plain-text stderr instead of structured JSON on stdout — scripts that `JSON.parse(stdout)` crashed. Added `worktreecmd.NewErrorEnvelope` helper + `cli.emitErrorEnvelope` shim; all 5 early-return sites now emit a populated envelope. `create --print-cd --json` correctly routes the envelope to stderr to keep the stdout-is-the-path contract.
- **`make dev-tmux` works from inside a worktree (lets-vxz1z).** `do_tmux` resolved `$REPO = git rev-parse --show-toplevel`, which is the worktree path when invoked from a worktree; discovery searched `<worktree>/.worktrees/` (does not exist) and aborted. Now resolves the main repo via `git rev-parse --absolute-git-dir` and strips `/.git*` — works for both main (`<main>/.git` → `<main>`) and worktree (`<main>/.git/worktrees/<n>` → `<main>`).
- **AskUserQuestion descriptions show substituted LETS_* values, not literals (lets-7jise).** Users running `/lets:done` previously saw option strings like `"Switch to $LETS_MERGE_BRANCH, pick another task"` rendered literally instead of `"Switch to main"`. Root cause: `$LETS_FOO` marker had double duty (orchestrator-read reference in prose, substitute-and-display in AskUserQuestion) — the ambiguity caused orchestrators to copy spec strings verbatim instead of substituting. Fix: disambiguate by marker — `{LETS_FOO}` curly now denotes "substitute and display" (inherits bash-block substitution discipline that already works reliably), `$LETS_FOO` dollar is reserved for orchestrator-read prose / section headers / code comments (never displayed). New Rule 9 in `lets-rules.md` `## AskUserQuestion Conventions` makes the substitution requirement MANDATORY with a BAD/GOOD anti-pattern. `CLAUDE.md` Surface Forms split into 2 crisp rows; Command Checklist gains a Rule-9 reference. 8 user-facing AskUserQuestion sites migrated (`done.md` ×6, `review.md` ×1, `take-task` SKILL ×1). Audit confirms 0 `$LETS_*` left inside any AskUserQuestion block project-wide.

## [0.5.4] - 2026-05-13

### Fixed
- `/lets:update`'s binary-upgrade hint no longer points at a 404 — dropped `brew upgrade lets` (no Homebrew tap yet) and corrected the install URL to `https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh` (it was missing `scripts/`). Note: the broken hint is baked into the v0.5.2 / v0.5.3 `lets` binaries; this fix takes effect once you're on a newer binary. (lets-4fmgu)
- A rules-drift `## LETS Notice` from the SessionStart hook now reliably reaches the user even mid-slash-command: the hook appends a "surface this to the user" instruction to the block, and `/lets:start` was taught to put the notice first in its output (`start.md` previously said nothing about it, so the orchestrator relied only on the small `## LETS Notice` rule in `lets-rules.md` and tended to forget). (lets-4fmgu)
- The "lets binary not found" hint in `/lets:init` / `/lets:update` points at the curl install one-liner (run it in the prompt with `!`, or in a terminal) instead of `make install` — the contributor path. (lets-4fmgu)

### Changed
- `/lets:update` plugin-update guidance is the in-Claude-Code path now: `/plugin marketplace update lets-workflow`, then `/reload-plugins` (dropped the fragile `claude plugin update --scope` advice — `/plugin marketplace update` for a marketplace-tracked install plus a reload picks up the new version). The binary hint is shown both as `! curl …` (run it in the prompt) and the plain terminal command. `/lets:init` Step 1b and the README / `docs/installation.md` gained an "enable plugin auto-update" tip (`/plugin` → Marketplaces → lets-workflow → Enable auto-update). (lets-4fmgu)
- Doc freshen: the README install step spells out the exact `/plugin install` prompt labels and the local-clone note says `/reload-plugins` after editing; `docs/installation.md` covers install scope, auto-update, and that `/lets:init` also runs `bd init`; `cli/README.md` and `commands/install-deprecated.md` stop listing the already-shipped curl installer as "future"; `docs/installation.md`'s sudo note shows the real `curl … | sudo bash` invocation. (lets-4fmgu)

## [0.5.3] - 2026-05-13

### Added
- Community health files: `SECURITY.md` (private vulnerability reporting via the Security tab), `CONTRIBUTING.md`, GitHub issue/PR templates, `.github/dependabot.yml` (gomod + github-actions), and `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1; conduct reports via a private GitHub security advisory).
- CI: `.github/workflows/ci.yml` runs `make build` / `make vet` / `make test` / `golangci-lint` on every PR to `main` and every push to `main`, added as required status checks alongside `Verify version coherence`. (`verify-versions.yml` only checks source-tree version coherence — it doesn't compile, test, or lint the code.)
- `docs/` manual — `workflow.md`, `plan-execute.md`, `code-review.md`, `agents.md`, `parallel-work.md`, `tasks.md`, `commands.md`, `configuration.md`, plus an index. The exhaustive reference now lives here; the README links to each page.

### Changed
- README restructured as a landing-page tour (Why LETS? → Quick Start → Commands → Expert Agents → Using LETS → reference tail). The full reference moved to `docs/`; each section points to its deep-dive page; refreshed screenshot; pruned unused images.
- Repository made public — removed the "repo is private during testing" notices from `README.md` and `docs/installation.md`; the `docs/installation.md` manual-download example no longer hardcodes a version number.
- `cli/.golangci.yml` migrated to golangci-lint v2; 16 pre-existing `errcheck`/`staticcheck` findings fixed (mechanical, no behavior change). Bumped `golang.org/x/mod` to 0.36.0 (Dependabot).
- `.gitignore`: anchored the `AGENTS.md` pattern to `/AGENTS.md` so it no longer also ignores `docs/agents.md` on case-insensitive filesystems.
- Doc/reality drift fixes: `CLAUDE.md` `docs/` description now matches the actual tree (`installation.md` + images, plus `.github/workflows/`); `RELEASING.md` recovery steps account for release immutability + the `Protect release tags` ruleset, and dropped the stale "install.sh script (future)" line that already shipped.

## [0.5.2] - 2026-05-12

### Added
- `/lets:update` command + `lets update` Go subcommand — syncs a project with the current LETS release: self-heals `.lets/.env` (header refresh when `LETS_ENV_VERSION` is stale, user values preserved) and `.claude/rules/lets-rules.md` (re-copy when outdated/missing), and reports version status for the `lets` binary and the Claude Code plugin vs the latest GitHub release (it can't self-replace those, so it prints the upgrade command). GitHub `releases/latest` lookup is cached for 1h at `.lets/cache/update-check.json` with a stale-cache fallback; `--offline` skips the network, `--refresh-cache` bypasses the cache, `--json` emits a machine-readable envelope (`schema_version=1`). Also reports `consistent` (binary == plugin == installed-rules version) to flag partial upgrades. New `cli/internal/updatecmd/` package (lets-hdrdr.3)

### Changed
- Rules-drift Notice messages (SessionStart hook + `lets init --json` output) now point at `/lets:update` for the `unknown`/`outdated`/`ahead` states; `/lets:init` is kept only for `missing` (first-time install — no rules file yet). `lets update` re-copies the plugin rules on any detected drift, including `ahead`, so no `--force` flag is needed (lets-hdrdr.3)
- `lets init` re-run (and `/lets:update`) now restores a `LETS_*` key that was hand-deleted from `.lets/.env` to its canonical default, instead of rewriting it as an empty value the SessionStart hook would then inject into model context (lets-hdrdr.3)
- `/lets:commit` commit subject carries the `(task-id)` scope when an active task is detected (`feat(lets-abc): ...`), matching `.claude/rules/git.md`; the skill description and a `lets-rules.md` Commit rule now prefer `/lets:commit` over generic commit skills like `commit-commands:commit` (lets-4w2sc)
- Internal skills `detect-task` and `actor-fetch-personality` are hidden from the `/lets:` autocomplete via `user-invocable: false` frontmatter — the model (and commands that `Read` them by path) still use them (lets-4w2sc)
- `LETS_LANGUAGE` is documented everywhere as an English language name (`English`, `Russian`, `Japanese`, ...) like the other `LETS_*` values — the `.lets/.env` comment (via `letsconfig.Keys`), `/lets:init`'s "Other" free-text guidance (normalises native-script input), and a `lets-rules.md` Language bullet — so the model honours it regardless of the script the name itself is written in (lets-ha7sk)
- CLAUDE.md: new "Unreleased-features rule" (don't document `[Unreleased]`-only features as already shipped — tag them `(ships in vX.Y)` until released); expanded "Claude Code session-id channels" passage documenting `${CLAUDE_SESSION_ID}` (template substitution) vs `$CLAUDE_CODE_SESSION_ID` (Bash env var) with usage guidance and the `_CODE_` naming gotcha; the internal-skills note now mentions the `user-invocable: false` mechanism (lets-4w2sc)
- README: new "Setup order: `lets init` and `bd init`" section (what each tool touches in `.gitignore` / `.claude/settings.json` / project root, the duplicated PreCompact hook, the `bd init --server` auto-create-DB caveat). `scripts/remote/dolt/README.md`: new "Project Identity Mismatch (cross-checkout safety)" section explaining the `PROJECT IDENTITY MISMATCH` error; the example `BEADS_DOLT_SERVER_HOST` value scrubbed to a TEST-NET-3 reserved-for-docs IP (lets-4w2sc)

### Fixed
- `lets init` no longer writes `.beads/` to `.gitignore` — only its own paths (`.lets/`, `.worktrees/`); `bd init` manages the beads entries itself. Fixes the duplicated / messy `.gitignore` on bd-init-first ordering (lets-4w2sc)
- `lets init` step output distinguishes a freshly `git init`'d repo with no commits ("git (no commits yet)") from a true detached HEAD ("git (detached HEAD)") — new `gitutil.HasCommits` helper (lets-4w2sc)
- `/lets:start` Step 2 no longer fails with a scary `fatal:` on a repo that has no commits yet — `git log` is guarded with a graceful "no commits yet" message instead (lets-4w2sc)
- `/lets:done` Step 7 + `/lets:end` Steps 3 & 5: session-id capture in bash contexts moved from `${CLAUDE_SESSION_ID}` template substitution to the `$CLAUDE_CODE_SESSION_ID` Bash subprocess env var — the `Claude session:` line was sometimes dropped from bd comments when the placeholder sat inside a multiline argument; plain-markdown contexts (the session-summary template) keep `${CLAUDE_SESSION_ID}` (lets-4w2sc, lets-bdkvd)
- `/lets:init` Step 2b checks `git remote -v` alongside `gh auth status` — the GitHub PR-flow option's description is now conditional and warns when the repo has no git remote yet (so `/lets:done`'s push won't surprise-fail later) (lets-4w2sc)

## [0.5.1] - 2026-05-11

### Added
- README "Recommended scope" note in Install section: project scope is preferred for team-shared repos so teammates inherit `lets` without re-install (lets-i5ayk)
- `/lets:init` Step 1b: detect plugin install scope from `~/.claude/plugins/installed_plugins.json`; one-time informational notice when scope is `user`, suggesting re-install at project scope. No notice for `project` (best case), `local` (deliberate choice), or `unknown` (dev mode / missing file) (lets-i5ayk)
- **Pattern Recognition** section in `lets-rules.md` with 6 patterns the orchestrator should surface: 3+ recurring topic in same area, `bd search` before `bd create`, repeated blocker (root-cause not patch), branch kitchen-sink (split PRs), long unresolved debate (suggest `/lets:opinion`), periodic reflection in long sessions. Includes non-pushy discipline (one mention per pattern, drop on dismissal, don't fabricate) (lets-eb8zc)
- **AUTO MODE** section in `lets-rules.md` documenting stop conditions: always-approval ops (bd state changes, git push/PR, destructive, external-facing, new task creation), hard stops (3+ same failure, fabrication, scope drift), soft stops (decision points, new large scope), escape hatch (user interrupt = full stop, no auto-resume). Clarifies AUTO MODE system-reminders are default, not override (lets-vlw2k)
- **Local Config explainer** embedded in SessionStart hook output (via `//go:embed local_config_explainer.md`). Values + their per-key usage docs (semantics, bash-block rule for `LETS_PROJECT_ROOT`, fallbacks, prompt-injection defense) now travel together. Hook output grew 155 B → ~2 KB, still well under 10 K cap (lets-q9bx7)
- New `Boundaries` bullet: never edit installed `lets-*` rules files in `.claude/rules/`. Surfaces installed-copy doctrine at runtime (was previously only in CLAUDE.md + bd remember) (lets-q9bx7, lets-0okn8)
- 2 new bullets in `Architecture Mindset`: "Smallest change that solves the problem" (anti-scope-creep) and "Plan for breaking changes" (migration / back-compat discipline) (lets-kzne5 partial)
- `Phase Detection & LETS Boxes`: explicit exception "internal invocation = no box" — when one `/lets:*` command invokes another programmatically (e.g. `/lets:review --json` from `/lets:github-pr`), inner command's LETS box is waived to avoid duplicate next-step suggestions in one response (lets-t6c27)
- CLAUDE.md doctrine: `.claude/rules/lets-rules.md` (installed copy) is never edited directly — only the canonical source `plugins/lets/rules/lets-rules.md`, refreshed via `/lets:init`. Dogfoods drift detection live (lets-0okn8)
- `.claude/rules/git.md` updated with scope (task-id) format `feat(lets-sds): subject`, examples for tracked/ad-hoc commits, body-WHY discipline, and `/lets:commit` skill pointer (lets-vlw2k)
- `/lets:plan --fast` flag — orchestrator-only planning that skips the three subagent-dispatch phases (Step 4 explorers, Step 6 architects, Step 7 experts) and replaces them with in-session work (read files yourself, draft approaches inline, self-evaluate risks). Clarifying questions, checkpoints, discussion, plan format, and beads recording are unchanged; the Plan Ready output records which mode was used. Combinable with a task-id (lets-lq1ud)
- `/lets:check` parameter parity with `/lets:review` — accepts `<PR-url-or-number>`, `--local` (explicit), `--file <path>`, and `--json` in addition to `--staged`/`--last-commit`/`--plan`; `argument-hint` is byte-identical to `/lets:review`. PR mode warns when the PR is large (suggests full `/lets:review`); `--file` mode reviews full file content; `--json` output mirrors `/lets:review --json` (same `verdict`/`tier`/`findings[]` shape, `agent: "check"` + per-finding `lens`, `summary` object). No subagent dispatch in any mode — that's the only thing that stays inline vs `/lets:review`. End-of-run LETS box offers the matching `/lets:review --<flag>` upgrade path (lets-qadj0)
- `/lets:done` Step 5 — CHANGELOG update step: when `CHANGELOG.md` exists at the project root and the task touched user-visible files, offers (AskUserQuestion: Add / Edit / Skip) to add a `[Unreleased]` entry drafted from the task title + commits and commit it via `/lets:commit` so it lands in the same PR; skips silently and says so when there's no `CHANGELOG.md` (lets-v1rm1.1)
- `/lets:start` Step 7 — suggests `/rename {slug}` after a task is claimed so the statusline reflects the active task. Presented as a paste-ready hint (`/rename` is user-invoked; the assistant can't run it), not a gate (lets-gzsho)
- **Plan-visibility gate** in `lets-rules.md` AUTO MODE section — present the plan (per task/file, what changes, in what order) before any edits, even in AUTO MODE; one approval covers a multi-task batch. "Execute immediately" now means "run the next step of an approved plan", not "skip showing the plan"; "let's think about how to do X" / "проаналізуй" is a request for a plan, not a green light to edit. Plus a matching Soft stop line (lets-sjtiy.1)
- CLAUDE.md "Command Output Requirements": "Which shortcuts to offer" guidance for LETS boxes (most-likely next step + lighter/faster alternative where one exists + escape hatch; same-width-within-a-file rule made explicit) and a Command Checklist line for it (lets-ne36e)

### Changed
- **Discovery Logging** rewritten: orchestrator now proactively suggests `/lets:note` (approval gate) instead of autonomous `bd comments add`. Aligned with broader "never act without user approval" principle. Trigger list expanded with user-driven moments (accepted decisions, shared facts, external context). Removed artificial length cap on recorded content — future recovery beats brevity (lets-0okn8)
- **Context Window Management** section: percentage-based table (50%/70% thresholds) replaced with honest soft heuristic. Orchestrator has no programmatic token count; speculation was misleading. Now: tell user `/context`, suggest finishing current task and `/lets:end` late in long sessions before starting new scope (lets-0okn8)
- `Skill Quick Reference` table: removed outdated `/lets:install` row (command renamed to `/lets:install-deprecated`; global setup now via Claude Code marketplace `/plugin install`) (lets-t6c27)
- `Mid-Session Task Switch`: delegated explicitly to `take-task` skill instead of inline branch-creation description (lets-t6c27)
- `Task References` examples trimmed from 5 to 3 (Flowing text + Report rows + Bad) — redundant patterns removed (lets-t6c27)
- `Beads Task Creation`: deferred to `create-task` skill (was duplicating skill's required-fields contract) (lets-t6c27)
- `Architecture Mindset` bullets reworded with bold keywords for scanability (lets-kzne5 partial)
- **`/lets:pr` renamed to `/lets:github-pr`** — the PR review lifecycle command supports GitHub only (via `gh` CLI); Bitbucket and local-merge PR flows are not implemented (those finish tasks via `/lets:done`). Command file `pr.md` → `github-pr.md`; all `/lets:pr` references updated across commands, skills, `lets-rules.md`, CLAUDE.md, README; H1 and intro now state "GitHub only" (lets-v1rm1.2)
- LETS-box audit across all 17 command files — box widths now consistent within each file (brainstorm/check/execute/github-pr/review/team/worktree had mixed widths; worktree boxes had `/lets:worktree` commands touching the right border with no padding); added the lighter `/lets:check` alternative wherever `/lets:review` was offered alone (execute, team, review); `/lets:plan` Plan Ready box gains `Check plan? /lets:check --plan` (lets-ne36e)

### Removed
- **Local Config** section from `lets-rules.md` — content moved into hook output explainer (lets-q9bx7)
- **Key Principles** section from `lets-rules.md` — all 6 points were restatements of earlier sections (Task Selection MANDATORY, Task Size Assessment, Discovery Logging, Git Conventions, LETS Box rule). No new facts (lets-t6c27)
- **Git Conventions** section from `lets-rules.md` — commit format / subject length are project preference (teams differ), `git status` ritual is workflow opinion covered by `/lets:commit` skill, `Task:` footer is enforced programmatically by skill. Project-specific git rules live in user-owned `.claude/rules/git.md` (lets-kzne5 partial)
- spurious `> **IMPORTANT:**` deferred-tool callout from `commands/check.md` — the command invokes no `AskUserQuestion`; the brief now notes "no AskUserQuestion gates" instead (lets-qadj0)

### Fixed
- `bd comments list <id> [--limit N]` corrected to `bd comments <id>` in `/lets:start`, `/lets:note`, `/lets:team`, and the `take-task` skill — `bd comments` has no `list` subcommand or `--limit` flag (default already lists all), and the bad syntax made `/lets:start <task-id>` fail against bd v1.0.2. Those flows now also instruct reading the FULL task description and ALL comments, not a truncated slice (lets-8ptef)

### Closed tasks
- lets-0okn8 — Enhance rules injection (Discovery Logging + Context Window + installed-copy doctrine)
- lets-q9bx7 — SessionStart hook self-documenting (Local Config explainer in hook output)
- lets-q20gz — Cross-platform hooks (superseded by Go CLI from lets-7vtaw)
- lets-eb8zc — Proactive pattern recognition
- lets-t6c27 — Context window / rules audit pass
- lets-vlw2k — AUTO MODE stop conditions
- lets-8ptef — `bd comments` syntax bug in 4 plugin files
- lets-gzsho — Auto-rename session on `/lets:start` (as a paste-ready suggestion)
- lets-v1rm1.1 — CHANGELOG update step in `/lets:done`
- lets-v1rm1.2 — Rename `/lets:pr` → `/lets:github-pr` (GitHub-only flow)
- lets-qadj0 — Align `/lets:check` parameter surface with `/lets:review`
- lets-lq1ud — `/lets:plan --fast` flag
- lets-ne36e — LETS-box audit + shortcut-selection guidance
- lets-sjtiy.1 — Plan-visibility gate in AUTO MODE rules
- lets-afwa4 — Refine ultrathink (already shipped in PR #24; closed as superseded)
- lets-ae0wt — Standardize branch parsing comments (resolved by `detect-task` skill extraction)

### Tracked follow-ups
- lets-93mbk [P0] — Route commands through internal task skills (create-task, take-task, detect-task)
- lets-kzne5 [P0] — Optional rules system: opt-in modules (git, auto-mode, …) copied by `/lets:init`

## [0.5.0] - 2026-05-10

### Added (Go CLI port - lets-7vtaw, Phases 1-4b)
- Go CLI binary `lets` with subcommands: `lets version`, `lets hook session-start`, `lets hook precompact`, `lets statusline`, `lets init`. Cross-compiles for darwin/arm64, linux/amd64, windows/amd64. Module path: `github.com/restarter/lets-workflow/cli`
- Monorepo layout: plugin payload moved to `plugins/lets/` subdir; new `cli/` parallel directory; root `Makefile` for `make build/test/vet/lint/fmt/install`
- `plugins/lets/rules/lets-rules.md` workflow rules file with frontmatter `version` for SessionStart drift detection (replaces `hooks/rules-context.md`)
- Hook size guard: SessionStart/PreCompact stdout capped well under Claude Code's 10K cap (closes lets-q9bx7 17KB truncation bug)
- Shared Go packages: `cli/internal/envfile/` (.env parser), `cli/internal/frontmatter/` (semver drift check via `golang.org/x/mod/semver`), `cli/internal/initcmd/` (init orchestration + migrations + JSON merge), `cli/internal/statusline/` (render + cache + OAuth fetch with build-tag splits darwin/other and unix/windows)

### Added (lets-hdrdr.2 - pre-PR fixes)
- `LETS_ENV_VERSION` first key in `.lets/.env` records the binary version that wrote the file. `lets init` regenerates `.env` when version mismatches the running binary or when CLI prefs flags are passed with new values; refreshes header and adds any new canonical keys.
- `RegenerateEnv` reusable function in `cli/internal/initcmd/env.go` — handles fresh creation, version-mismatch refresh, and prefs-change overrides. Preserves user values + foreign keys (latter under `# User-added keys` separator). Imported by future `/lets:update` (lets-hdrdr.3).
- "Slash Command Discipline" section in `plugins/lets/rules/lets-rules.md` — global rule against bypassing Step bash blocks in slash commands.
- MANDATORY callout in `/lets:init` Step 1 reinforces literal execution of the dotfile pre-check.

### Changed (Go CLI port)
- SessionStart + PreCompact hooks ported from bash to Go (`lets hook session-start`, `lets hook precompact`); workflow rules now live in `<project>/.claude/rules/lets-rules.md` (copied by `lets init`) instead of injected via hook stdout
- Statusline ported from bash to Go (`lets statusline`); per-project `.lets/statusline.sh` becomes optional thin shim (legacy backward-compat only)
- `lets init` Go subcommand: drift-aware rules copy, value-match statusLine detection, atomic JSON merge with `.bak`, yaml→env migration, idempotent re-runs
- Plugin source files moved from repo root to `plugins/lets/` subdir; `marketplace.json` `source` updated to `./plugins/lets`
- Plugin version bumped 0.3.1 → 0.4.0 (lockstep with CLI)

### Changed (lets-hdrdr.2 - pre-PR fixes)
- statusLine detection in `.claude/settings.json` uses pure value-match against canonical command (no provenance marker). Existing installs' `_letsManaged.statusLine` keys are harmless residue.
- `lets init` drops `--force-env` flag — semantics now driven by raw cobra flag values (empty = preserve existing, non-empty = regen with new value).
- `--language/--merge-branch/--pr-flow` no longer cobra-required. Required-ness moved to runtime; only fails when `.env` needs fresh creation and no flags supplied.
- `bd init` workspace-detection switched from filesystem-layout sniffing (`.beads/dolt/`) to authoritative `bd status` exit code. Resilient to upstream bd internal layout changes.
- `MigrateYamlToEnv` deletes `config.yaml` outright (was: rename to `.deprecated`). Also handles orphan case (`.env` and `config.yaml` coexist) by removing the yaml.
- `lets init` recomputes drift state after rules install; JSON output reflects post-action state (no more "installed" + "missing" contradictions).

### Removed (Go CLI port)
- `hooks/session-start.sh` (replaced by `lets hook session-start`; archived in `scripts/deprecated/lets/`)
- `hooks/rules-context.md` (content migrated to `plugins/lets/rules/lets-rules.md`; rules now copied to `<project>/.claude/rules/lets-rules.md` by `lets init` instead of injected via hook stdout)
- yaml→env auto-migration from SessionStart hook (now only triggered by `lets init`; closes lets-p732a)

### Removed (lets-hdrdr.2 - pre-PR fixes)
- `_letsManaged.statusLine` provenance marker write logic — single-string field doesn't need provenance, value-match is sufficient.
- `UpdateEnvKeys` (replaced by `RegenerateEnv`).
- `os.RemoveAll(.beads)` auto-cleanup branch from `runBeadsInit` — destructive recovery shouldn't be automatic.
- `--force-env` cobra flag.

### Deprecated (Go CLI port)
- `plugins/lets/scripts/lets/init.sh` and `plugins/lets/scripts/lets/statusline.sh` source files - active until lets-8ilsl rewrites `commands/init.md` to invoke `lets init` directly

### Added
- `/lets:end` and `/lets:done` capture Claude Code session UUID into beads comments and session summaries for transcript traceability. Session summary now includes `### Claude Session` block with resolved transcript path
- Skill architecture (`skills/` directory) with two skill types: user-facing (auto-triggered) and internal (read by commands)
- 4 skills: `commit`, `create-task`, `take-task` (user-facing), `detect-task` (internal)
- Auto-triggered Skills table in `plugins/lets/rules/lets-rules.md` and `commands/install.md`
- beads-web Docker kanban board (`scripts/beads-web/`) - Rust binary (Shybko/beads-web fork v0.11.0), default port 3008
- `scripts/dolt/backup-remote.sh` for ad-hoc Dolt server snapshots before risky operations
- Branch return prompt after worktree creation
- Actor meta-agent (`agents/actor.md`) for dynamic personality loading from URL or local file
- `actor-fetch-personality` internal skill for personality fetching via curl/Read
- Actor integration with all 5 dispatching commands (ask, opinion, review, brainstorm, plan)

### Changed
- All 13 agent prompts upgraded to v2: tiered scoring ([BLOCKER]/[SUGGESTION]/[NIT]), self-contained behavioral modes, color assignments
- Moved `scripts/beads-ui/` and legacy `scripts/beads/setup-beads-remote.sh` to `scripts/deprecated/`; superseded by `scripts/beads-web/` (Rust kanban board) and Direct SQL mode respectively
- `scripts/beads-web/setup-remote.sh`: default port 9090 → 3008 (avoids Prometheus collision; matches upstream binary's native default), Dockerfile heredoc fixed (was unintentionally host-shell-substituted)
- `scripts/dolt/README.md`: replaced broken cron-based backup snippet (tarred only `dolt-home/`) with ad-hoc `backup-remote.sh` script + 2-layer model (VPS-snap + application-aware)
- `/lets:brainstorm` redesigned with 4 modes: review backlog, explore idea, quick brainstorm, cleanup
- Commands thinned - shared logic extracted to skills (`commit.md` 208->11 lines, `start.md` -120 lines)
- CLAUDE.md architecture decisions updated: agents define WHO+HOW, commands define WHAT+WHEN
- Mandatory context tables removed from 5 commands (review, opinion, ask, plan, brainstorm) - now in agents
- Dynamic agent scaling across all dispatching commands - LLM decides agent count based on context, no hardcoded caps
- `/lets:review --plan`: dynamic agent selection based on plan content signals (replaces hardcoded 2 agents)
- `/lets:review --file`: skip git-historian, adjust pragmatist threshold, skip systemic pattern check
- `/lets:plan` exploration: LLM-driven focus areas replace 3-tier size heuristic (Single/Medium/Full), confirmation gate at 10+ explorers
- `/lets:opinion`: dynamic expert count replaces hardcoded 3-5 preset table
- `/lets:brainstorm`: signal-driven agent count, no cap (was capped at 5)
- `/lets:team`: dynamic teammate count, no cap (was capped at 5)
- `/lets:opinion`, `/lets:brainstorm`, `/lets:review` show Agent Panel with cost note before launch
- `/lets:check` expanded to 6 lenses (added [Docs] documentation sync)
- `/lets:review` docs agent now ALWAYS included (was conditional)

### Removed
- RESPONSE LANGUAGE directive from all agent prompt templates - agents respond in English, orchestrator localizes
- Hardcoded agent count caps from opinion (3-5), brainstorm (3-5), team (3-5), plan explorers (1/2/3)

### Fixed
- beads-ui: input validation, firewall rules, deduplication (PR #35 review)

## [0.3.1] - 2026-03-08

Marketplace release - plugin available via Claude Code marketplace.

### Added
- `.claude-plugin/marketplace.json` for marketplace distribution
- `LICENSE` file (MIT)
- Marketplace and local install paths in README

### Changed
- `plugin.json`: added homepage, fixed repository URL to `restarter/lets-workflow`, bumped version
- `CLAUDE.md`: fixed header from old repo name
- README Quick Start rewritten with marketplace install instructions
- README version badge now dynamic (from GitHub tags)
- `scripts/release.sh`: syncs marketplace.json version on release

## [0.3.0] - 2026-03-07

README rewrite, install/init split, and interactive planning.

### Added
- `/lets:init` command - per-project setup (creates .lets/, config, statusline, beads)
- `scripts/lets/lets_init.sh` - bash script for project initialization
- Interactive exploration phase in `/lets:plan`, `/lets:brainstorm`, `/lets:opinion`
- AskUserQuestion with markdown preview for architecture choices

### Changed
- `/lets:install` rewritten - global setup only, delegates per-project work to `/lets:init`
- `scripts/lets/install.sh` replaced by `lets_init.sh`
- README fully rewritten: marketing intro, workflow modes diagram, section emojis, updated screenshots
- CLAUDE.md references updated for install/init split

### Removed
- `scripts/lets/install.sh` (replaced by `lets_init.sh`)

## [0.2.4] - 2026-03-06

Quality, consistency, and planning restructure.

### Added
- `/lets:plan` - structured planning with explorer/architect agents (extracted from brainstorm)
- `/lets:check --plan` - quick plan sanity check (5 lenses: feasibility, completeness, risk, scope, clarity)
- `/lets:review --file <path>` - review existing file quality (not just diffs)
- `Merge & close` option in `/lets:done` post-PR menu
- Discovery Logging rule in workflow rules - capture insights via `bd comments add` as they happen
- Implementer agent Bash Security with explicit ALLOWED/FORBIDDEN lists
- PROJECT ROOT boundary enforcement in agent prompts (ask, brainstorm, opinion, review)
- RESPONSE LANGUAGE propagation to agent prompts
- `.gitignore` safety net for `lets/` (without dot prefix)
- Per-project statusline install script (`scripts/lets/install.sh`)
- `bd config set hash_length 5` in `/lets:install`

### Changed
- `/lets:brainstorm` - repurposed as interactive backlog helper (review tasks, priorities, patterns)
- `/lets:execute` - rewritten to use native Claude Code plan mode (EnterPlanMode)
- Session files: single dated file with branch slug (no more `last-summary` files)
- `/lets:start` reads recent sessions via `ls -t` glob, supports `<task-id>` and `--continue` args
- Statusline moved from `hooks/` to `scripts/lets/`, install via standalone script
- Language rule standardized to "Respond in user's language" across all 17 commands
- Branch parsing updated to handle `worktree-<id>-<slug>` format in 9 commands
- `git rev-parse --show-toplevel` replaced with LETS Config `project-root` in worktree.md
- Usage fetcher removed (replaced by statusline install script)

### Fixed
- Statusline emoji spacing for Linux compatibility
- Stale `last-summary.md` references in worktree guide
- `status.md` missing language rule

### Migration
- `/lets:brainstorm` is now a backlog helper, not a planning command
- Use `/lets:plan` for what was previously `/lets:brainstorm` (architecture + implementation plan)
- `/lets:execute` now enters native plan mode instead of custom batch execution
- Statusline: re-run `/lets:install` to get per-project install

## [0.2.3] - 2026-03-04

Ultrathink and review quality improvements.

### Added
- `ultrathink` keyword in 5 commands: review, opinion, brainstorm, ask, check (high-effort reasoning)

### Changed
- Worktree open recommendation: single-line `cd && claude` format
- Explorer and implementer agents excluded from ultrathink (speed > depth)

## [0.2.2] - 2026-03-01

Worktree support for parallel Claude Code sessions.

### Added
- `/lets:worktree` command - create/list/remove/info for interactive worktrees
- Worktree detection in `/lets:start` - skips branch creation, uses worktree branch as-is
- Per-branch session refs - parallel sessions don't collide
- Worktree section in workflow rules (rules-context.md)
- Agent sandboxing prototype (`validate-readonly.sh`) + prompt-level constraints
- Worktree & agent teams research guide (docs/knowledge/)
- `/lets:execute` - plan execution with batch checkpoints (from 0.2.1, now documented)
- `/lets:pr` - full PR review lifecycle (from 0.2.1, now documented)

### Changed
- CLAUDE.md architecture decisions updated to reflect actual enforcement state
- Boundaries rule updated: `worktree-<name>` recognized as valid working branch
- Session start ref now per-branch (`.session-start-ref-<branch-slug>`)
- Session end reads per-branch ref with backwards compatibility fallback
- Commit task ID parsing handles worktree branch formats

### Fixed
- CLAUDE.md no longer claims hook-based enforcement that wasn't registered
- Hook prototypes (worktree-setup.sh, worktree-cleanup.sh) archived as .old

## [0.2.1] - 2026-03-01

LETS-branded statusline with usage stats.

### Added
- Statusline with warm flow design (gold branding, coral branch, sand metrics)
- OAuth usage fetcher with macOS Keychain + credentials.json fallback
- Context window and 5h/7d usage stats with color-coded thresholds and reset timers
- Background cache refresh (5min TTL) - never blocks rendering
- Project-scoped statusline config in `.claude/settings.json`

### Fixed
- Token cache restricted to owner-only permissions (umask 077)
- Keychain parsing uses `security -w` instead of fragile grep/sed
- Cache timestamp validation before shell command use

### Changed
- Usage cache moved from `/tmp` to `.lets/cache/`
- CLAUDE.md updated with hooks and cache documentation

## [0.2.0] - 2026-02-28

Full workflow with PR lifecycle, planning skills, and structured task tracking.

### Added
- `/lets:brainstorm` - interactive planning with explorer agents and architecture design
- `/lets:execute` - plan execution with batch checkpoints and context recovery
- `/lets:pr` - full PR lifecycle: analyze, discuss, post inline comments, follow-up, respond, approve
- GitHub integration (`github: true` in config) - PR workflow across all commands
- Local config system (`.lets/config.yaml`) with language, merge-branch, github settings
- Plugin storage folder (`.lets/`) for sessions, reviews, plans, execution state
- Beads deep integration - task tracking linked to all 10 commands
- AskUserQuestion interactive confirmations across all commands
- Scope verification step in `/lets:done`
- Task progress check in `/lets:commit`
- Mid-session task switch rules
- Directed search vs exploration rule for agent dispatch
- Beads best practices in workflow rules (epic lifecycle, dependency rules)
- Beads Dolt backend with GitHub remote sync

### Changed
- `/lets:check` inlined (removed quick-reviewer agent) for speed
- `/lets:status` rewritten with interactive views
- Workflow rules consolidated into SessionStart hook
- Agent colors assigned by semantic role
- Brainstorm asks clarifying questions before launching agents

### Task tracking
- 4 epics: Plugin Quality (17), Distribution (5), Feature Ideas (9), Agent Quality (9)
- 44 tracked issues total

## [0.1.0] - 2026-02-12

Initial release with expert agents team.

### Added
- 11 expert agents: architect, security, backend, database, frontend, devops, qa, docs, compliance, git-historian, pragmatist
- `/lets:start` - session start with task selection
- `/lets:end` - session end with summary
- `/lets:commit` - conventional commit with task linking
- `/lets:done` - task completion with merge/PR
- `/lets:check` - quick code review
- `/lets:review` - full deep review with agent dispatch
- `/lets:opinion` - technical decision analysis (3-5 agents)
- `/lets:ask` - single expert consultation
- `/lets:status` - task overview
- `/lets:install` - first-time setup
- SessionStart hook injecting workflow rules
- Plugin structure: commands, agents, hooks

[Unreleased]: https://github.com/restarter/lets-workflow/compare/v0.6.4...HEAD
[0.6.4]: https://github.com/restarter/lets-workflow/compare/v0.6.3...v0.6.4
[0.6.3]: https://github.com/restarter/lets-workflow/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/restarter/lets-workflow/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/restarter/lets-workflow/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/restarter/lets-workflow/compare/v0.5.5...v0.6.0
[0.5.5]: https://github.com/restarter/lets-workflow/compare/v0.5.4...v0.5.5
[0.5.4]: https://github.com/restarter/lets-workflow/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/restarter/lets-workflow/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/restarter/lets-workflow/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/restarter/lets-workflow/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/restarter/lets-workflow/compare/v0.3.0...v0.5.0
[0.3.1]: https://github.com/restarter/lets-workflow/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/restarter/lets-workflow/compare/v0.2.4...v0.3.0
[0.2.4]: https://github.com/restarter/lets-workflow/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/restarter/lets-workflow/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/restarter/lets-workflow/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/restarter/lets-workflow/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/restarter/lets-workflow/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/restarter/lets-workflow/releases/tag/v0.1.0
