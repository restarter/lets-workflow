# Changelog

## [Unreleased]

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

[Unreleased]: https://github.com/restarter/lets-workflow/compare/v0.5.3...HEAD
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
