# lets-workflow

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

Monorepo layout (beads-style): plugin source in `plugins/lets/` subdirectory, marketplace manifest at root pointing into it. Infrastructure scripts and docs stay at root, outside the plugin payload.

```
.claude-plugin/marketplace.json   # Marketplace manifest (source: ./plugins/lets)
plugins/lets/                     # Plugin payload (everything ${CLAUDE_PLUGIN_ROOT} resolves to)
├── .claude-plugin/plugin.json    #   Plugin manifest (Claude Code; .codex-plugin/ planned for multi-agent)
├── commands/                     #   Slash commands (/lets:start, /lets:done, /lets:review, etc.)
├── agents/                       #   14 expert agents dispatched by review/opinion/ask/plan/brainstorm/team
├── skills/                       #   Reusable skills (user-facing auto-triggered + internal referenced by commands)
├── rules/lets-rules.md           #   Workflow rules (frontmatter `version`; copied to .claude/rules/ by `lets init`)
└── hooks/                        #   SessionStart + PreCompact hooks (drift check + LETS Config only - no rules in stdout)
cli/                              # Go CLI - companion binary (Phase 2+, lets-7vtaw)
├── cmd/lets/main.go              #   Entry point (thin)
├── internal/cli/                 #   Cobra command factories (root.go, version.go, *_test.go)
├── internal/version/version.go   #   Version (var, ldflags-overridable from git tag)
├── go.mod, go.sum
└── .golangci.yml                 #   Linter config (default + gofmt/goimports/misspell)
Makefile                          # Repo-root build (build/test/vet/lint/fmt/install/clean)
.editorconfig                     # Editor whitespace/charset settings
scripts/release/                  # Release tooling: bump-version.sh + verify-versions.sh (used by Makefile bump/release-tag)
scripts/remote/dolt/              # Dolt SQL server VPS deployment + ad-hoc backup (NOT plugin)
scripts/remote/beads-web/         # beads-web (Rust kanban board) VPS deployment (NOT plugin)
scripts/deprecated/               # Retired scripts kept for cleanup runbooks - gitignored, not tracked
docs/                             # Plans, knowledge base, reference docs, comment exports
reference/                        # Reference plugins for studying patterns (gitignored)
```

References that resolve via `${CLAUDE_PLUGIN_ROOT}` (e.g. `${CLAUDE_PLUGIN_ROOT}/skills/X/SKILL.md`) work as before - the env var points at `plugins/lets/`.

## Key Concepts

> Path convention: paths like `commands/`, `skills/`, `rules/lets-rules.md` in this doc are **relative to `plugins/lets/`** (the plugin root, also exposed as `${CLAUDE_PLUGIN_ROOT}` at runtime). Paths starting with `scripts/release/`, `scripts/remote/`, `docs/`, `reference/` are relative to the **repo root** (outside the plugin payload).
>
> Go CLI source paths (`cli/cmd/lets/main.go`, `cli/internal/...`) are relative to the **repo root**. The Go module root is `cli/` - all `go` commands operate from there (or via the repo-root `Makefile`).

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = experts dispatched by commands. `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:brainstorm` dispatch via subagents. `/lets:team` dispatches via Agent Teams (parallel, worktree isolation). `actor` is a meta-agent that loads external personalities (URL or file) and adapts them to LETS modes
- **Orchestrators** = commands that delegate to other commands. `/lets:pr` orchestrates `/lets:review` for full PR lifecycle
- **Hooks** = SessionStart + PreCompact inject workflow rules (PreCompact preserves rules across context compaction in long sessions)
- **Statusline** = `lets statusline` Go subcommand. Project `.claude/settings.json` invokes it directly. `lets init` detects the canonical command via value-match against `"lets statusline"`; foreign user-customized commands are left alone. Legacy bash shim `plugins/lets/scripts/lets/statusline.sh` removed in lets-8ilsl. Byte-equal detection of pre-deletion installs (.lets/statusline.sh) handled via the frozen `cli/internal/initcmd/embedded_statusline_shim.sh` snapshot — `MigrateStatuslineSh` deletes matching legacy shims and triggers `SetStatusLine` to point settings.json at `lets statusline`.
- **`.env` versioning** — first key `LETS_ENV_VERSION` records which `lets` binary version last regenerated the file. `lets init` regenerates when version mismatches the running binary OR when CLI prefs flags are passed with new values. `RegenerateEnv` (`cli/internal/initcmd/env.go`) is the canonical writer; preserves user values + foreign keys (under `# User-added keys` separator), refreshes header. NOT in `letsconfig.Keys` whitelist (metadata, not user-config) — hook session injection skips it. Reusable for future `/lets:update` (lets-hdrdr.3).
- **Skills** = reusable actions in `skills/<name>/SKILL.md`. Two types: user-facing (auto-discovered, triggered via description match or Skill tool) and internal (not auto-discovered, read by commands via Read tool when needed). Examples: `create-task`, `commit`, `take-task` (user-facing), `detect-task`, `actor-fetch-personality` (internal)

## Architecture Decisions

- **Audience for plugin source.** Everything in `commands/`, `skills/`, `agents/`, `rules/lets-rules.md` is read by Claude (orchestrator and subagents) - never by humans. Write for the model: direct, structured, parseable. No rhetorical flourishes, no human-onboarding tone. Use markers (`MANDATORY`, `NEVER`, `IMPORTANT`) where the model needs to lock onto a constraint. Tables and bullet lists beat prose. Examples are templates the model imitates - keep them precise. Human-facing docs live in `README.md` and `CLAUDE.md` (this file).
- **Claude Code template variables in command/skill markdown.** `${CLAUDE_PLUGIN_ROOT}` (plugin install path) and `${CLAUDE_SESSION_ID}` (session UUID, available since Claude Code v2.1.9) are substituted by Claude Code before the model reads the markdown - immune to context compaction (substitution happens at command/skill load time, not at session start). `${CLAUDE_SESSION_ID}` is documented for skill prompt text; empirically verified to work in command markdown too. Used by `/lets:end` and `/lets:done` to anchor session identity in beads comments and session summaries for transcript traceability. `${CLAUDE_PROJECT_DIR}` is NOT substituted - use `git rev-parse --show-toplevel` instead.
- Agents define WHO and HOW (expertise, behavioral modes, tiered scoring, output format). Commands define WHAT and WHEN (provide content, select agents, pass mode name)
- Agent frontmatter fields: `name`, `description`, `tools`, `color` (terminal output: red/blue/green/yellow/purple/orange/pink/cyan), optional `model` (default inherits from parent, `opus` for complex analysis). All agents use tiered scoring ([BLOCKER]/[SUGGESTION]/[NIT]), self-contained Modes, and Output Format sections.
- Agent memory (`memory: project` frontmatter) is **currently disabled** across all agents as a workaround for upstream Claude Code issue [#55648](https://github.com/anthropics/claude-code/issues/55648). Subagents writing memory skip the assigned task and return only a memory-write confirmation. Disabled via [PR #47](https://github.com/restarter/lets-workflow/pull/47). Tracked via bd tasks `lets-erx1c` (this disable) and `lets-rqwdg` (restore memory when Anthropic fixes the bug).
- Actor meta-agent loads personalities at runtime via internal skill `actor-fetch-personality`. Command fetches personality content (curl for URLs, Read for files), user confirms via review gate, actor receives it in prompt as `PERSONALITY:` block. Fallback "generalist" identity when no personality provided
- Agent selection: each command owns its detection/selection logic (different semantics per context). Multi-agent dispatching commands (review, opinion, brainstorm, plan) show selection panel with cost note before launch. Most agents have explicit PLAN mode for plan review.
- Agents always respond in English. Commands localize output to user's language via LETS Config and Rules.
- `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:brainstorm` use `subagent_type: "lets:agent-name"` to dispatch agents via Task tool
- `/lets:pr` orchestrates `/lets:review` (delegates analysis) and handles GitHub posting, follow-up, respond, and approval directly via gh CLI
- `/lets:execute` uses EnterPlanMode for native plan mode execution with user approval gates. No subagents.
- `/lets:check` reviews inline (no subagent) for speed
- All analyst agents are read-only with uniform tools: `Read, Grep, Glob, Bash`. No `Edit/Write`. Exception: `agents/implementer.md` adds `Edit/Write` for `/lets:team` parallel implementation in isolated worktrees.
- `/lets:team` uses Agent Teams (TeamCreate, Agent with isolation: worktree) for parallel implementation. All other commands use subagents for analysis.
- All analyst agents have prompt-level read-only Bash constraints in their `## Constraints` section (identical 1-line allowlist across all 13). `hooks/validate-readonly.sh.old` exists as a PreToolUse hook prototype (not yet registered - agent frontmatter hooks silently ignored)
- Interactive worktrees managed via `/lets:worktree` command. Hook prototypes `hooks/worktree-setup.sh.old` and `hooks/worktree-cleanup.sh.old` (deferred - caused agent auto-cleanup issues)
- Worktrees stored in `.worktrees/` at project root - `.lets/` symlinked for interactive sessions
- **SessionStart hook** invokes the Go subcommand `lets hook session-start --rules=${CLAUDE_PLUGIN_ROOT}/rules/lets-rules.md`. It emits ONLY: optional `## LETS Notice` (drift check via `frontmatter.ReadVersion` on plugin vs `.claude/rules/lets-rules.md`) + `## LETS Config` block. Total output is <500 chars - well under Claude Code's 10K hook output cap (lets-q9bx7). Workflow rules themselves live in the project's `.claude/rules/lets-rules.md` (uncapped project-instructions channel), copied there by `lets init` and version-tracked via frontmatter. Project root detected via `git rev-parse --show-toplevel` with `os.Getwd()` fallback.
- **PreCompact hook** invokes `lets hook precompact --rules=${CLAUDE_PLUGIN_ROOT}/rules/lets-rules.md` (separate cobra subcommand, currently shares output behavior with `session-start` via `sessionstart.Run()`). Distinct subcommand kept for future divergence (e.g. context snapshotting before compaction).
- The bash `session-start.sh` was deleted along with its yaml→env migration block - lets-p732a closed.
- Dual-hook pattern (same effective output on both events) follows beads precedent: [issue #486](https://github.com/gastownhall/beads/issues/486) and [PR #297](https://github.com/gastownhall/beads/pull/297). SessionStart on `compact` source re-injects rules into the post-compaction context; PreCompact ensures rules are in the pre-compaction context that the auto-summary is generated from - prevents workflow drift after compaction in long sessions.
- User-facing skills: auto-discovered by Claude Code, appear in skill list, trigger on description match. Frontmatter description must NOT use YAML quotes.
- Internal skills: NOT auto-discovered. Commands reference with "use the X skill" and read the SKILL.md via Read tool at `` `${CLAUDE_PLUGIN_ROOT}/skills/X/SKILL.md` `` - the env var ensures the path resolves correctly whether plugin is loaded via marketplace install or `--plugin-dir` dev mode (relative `skills/...` paths break in foreign projects). No accidental triggering, no context cost until needed.
- Commands define WHAT to do and orchestrate the flow. User-facing skills define full reusable flows (steps, user gates) that auto-trigger on natural language. Internal skills define shared procedures read by commands on demand. Commands delegate to skills for shared operations.
- Gate for new skills: extract only if (a) user-facing with standalone trigger value, or (b) internal logic duplicated in 3+ commands.

## Release Flow

Two-phase tag-driven pipeline. See `RELEASING.md` for the maintainer ceremony.

```
Phase 1: Bump (manual, on release/X.Y.Z branch)
  scripts/release/bump-version.sh: edits source-tree version + (stable only) CHANGELOG,
                                   runs gates, commits. Does NOT push, does NOT tag.
                                   Stable: 4 files (3 source-tree + CHANGELOG promote)
                                   Prerelease (X.Y.Z-rc.N): 3 files only, CHANGELOG intact
Phase 2: Review (PR to main)
  .github/workflows/verify-versions.yml: PR-time source-tree coherence check
Phase 3: Tag (manual, on main)
  make release-tag VERSION=X.Y.Z: tags merge commit + pushes tag
Phase 4: Distribute (automated, on tag push)
  .github/workflows/release.yml:
    - guard job:   verify-versions.sh --against-tag
    - release job: goreleaser builds 5 archives + checksums, uploads to GH Releases
                   release notes from CHANGELOG [X.Y.Z] (stable) or [Unreleased] (prerelease)
```

**Why two phases** — bump is reviewable (can revert), tag is reproducible (same tag → same binaries). Mixing them means tag commits to changes that haven't been reviewed.

**Why bash for orchestration** — bump-version.sh + verify-versions.sh are file-edits + gates, natural for bash + jq + awk. No Go binary involvement; CI doesn't need `setup-go` for verify.

**Why goreleaser** — single declarative config builds 5 platforms in parallel, handles archives + checksums + GH Release creation + prerelease detection (semver suffix `-rc.1` etc.). Battle-tested in beads and similar Go CLIs.

**Source-tree version invariants** — single semver string across `plugin.json`, `marketplace.json`, `lets-rules.md` frontmatter (binary version derives from git tag via ldflags). Drift between any of these fails `verify-versions.yml`.

**Prereleases skip CHANGELOG mutation** — rc/beta/alpha tags exist as validation snapshots; the full release entry is reserved for the stable tag. release.yml synthesizes prerelease notes from `[Unreleased]`. PREV_TAG (used to compute compare-link bottom of CHANGELOG) filters prereleases so stable releases compare against the previous **stable** tag.

## File Storage

All plugin-generated files go to `.lets/` (gitignored). Never use `/tmp` or other external paths.
This includes hook debug logs, temp files, and any runtime artifacts.
**WARNING:** Always use `.lets/` (with dot prefix), never `lets/`. The dot is easy to miss in manual paths.

```
.lets/.env               # Per-project settings (LETS_LANGUAGE, LETS_MERGE_BRANCH, LETS_PR_FLOW, LETS_TRACKER)
.lets/.env.example       # Reference defaults — generated each `lets init` from canonical letsconfig.Keys defaults via renderEnvExample(). Not used by the hook; it's a user-facing template
.lets/.env.bak           # Single backup written by `RegenerateEnv` before mutation. Plugin-owned: user-created files at this path are silently overwritten — copy elsewhere for permanent backup
.lets/sessions/          # Session summaries, session-start-ref
.lets/reviews/           # Saved review reports
.lets/plans/             # Implementation plans
.lets/execution/         # Execution state (PR review: pr-{number}/, team records: team-*.json)
.lets/cache/             # Cached data (usage stats)
# Worktrees (outside .lets/ to avoid circular symlinks):
# .worktrees/            # Interactive worktrees only (agent worktrees use native Claude Code behavior)
```

Workflow rules live OUTSIDE `.lets/` because they belong to Claude Code's project-instructions channel:

```
.claude/rules/lets-rules.md  # Workflow rules (copied from plugin by `lets init`, frontmatter-versioned, customizable - tracked in git per project's choice)
```

**NEVER edit `.claude/rules/lets-rules.md` directly.** It is the **installed copy**, plugin-managed. Only the canonical source `plugins/lets/rules/lets-rules.md` is edited. The installed copy is rewritten by `/lets:init` (and only via that path) when the plugin's frontmatter `version` is bumped — this dogfoods drift detection live. Workflow: edit source -> bump source `version` -> commit -> release -> end user (or maintainer) runs `/lets:init` -> installed copy refreshed. Editing the installed copy directly bypasses drift testing and silently desyncs from source.

## Naming Convention: `LETS_*`

All plugin configuration uses the `LETS_*` prefix (UPPER_SNAKE_CASE). The prefix removes ambiguity in command instructions - `LETS_PR_FLOW=github` is unambiguously a parameter, while `github` could be the platform name.

### LETS Config keys

| Key | Source | Purpose |
|---|---|---|
| `LETS_PROJECT_ROOT` | Auto-injected by SessionStart hook | Absolute path to project root |
| `LETS_LANGUAGE` | `.lets/.env` | Default response language |
| `LETS_MERGE_BRANCH` | `.lets/.env` | Target branch for merges and PRs |
| `LETS_PR_FLOW` | `.lets/.env` | `github` \| `bitbucket` \| `local` - which PR workflow to use |
| `LETS_TRACKER` | `.lets/.env` | Task tracker. Currently `beads`. **Schema reserved** - no command branches on this yet; all task ops call `bd` regardless. Future: Linear/Jira (lets-nwwkj) |

### Surface forms - where to use which form

The same `LETS_FOO` value appears in different surface forms depending on context. The orchestrator (model) handles substitution; subagents do NOT receive LETS Config injection and need explicit substitution before Task() launch.

| Surface | Form | Resolved by | Applies to |
|---|---|---|---|
| Bash block (model executes via Bash tool) | `LETS_FOO=$(...)` then `$LETS_FOO` | local shell - assign at top of every block (each Bash call is a fresh shell) | **only `LETS_PROJECT_ROOT`** (computable in-shell via `git rev-parse`) |
| Bash command snippet inside markdown (template the model runs) | `{LETS_FOO}` placeholder | orchestrator substitutes literal value before running | all keys EXCEPT `LETS_PROJECT_ROOT` (which uses bash assignment instead) |
| Orchestrator prose, AskUserQuestion descriptions | `$LETS_FOO` | orchestrator reads from injected LETS Config | all keys |
| Subagent prompt template | `{LETS_FOO from LETS Config}` | orchestrator substitutes literal before Task() call | all keys |

**Important note on `LETS_PROJECT_ROOT`:** the value injected in `## LETS Config` is for prompt-text reference and orchestrator substitution - it is NOT a shell variable available in Bash tool calls. Every bash block that uses the project root path must assign locally:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
```

`LETS_PROJECT_ROOT` is the **only** key computable in-shell (via `git rev-parse`). Other `LETS_*` keys (`LETS_MERGE_BRANCH`, `LETS_PR_FLOW`, `LETS_LANGUAGE`, `LETS_TRACKER`) have no shell-side derivation - inside bash snippets, use the `{LETS_FOO}` template form so the orchestrator substitutes the literal value before running. Trying to use `$LETS_FOO` in a bash block (without local assignment) yields empty string - silently wrong commands.

### User config file

`.lets/.env` is `KEY=VALUE` format with comments **above** keys (NOT inline - inline comments would pollute the model's context after the hook strips full-line comments). Migrated from legacy `config.yaml` to `.env` by `lets init` (`cli/internal/initcmd/migrate.go::MigrateYamlToEnv`). The yaml→env auto-migration in the SessionStart hook has been removed (lets-p732a closed). Legacy yaml deprecation removal tracked in `lets-q8xtk`.

**NOT FOR SECRETS.** The SessionStart hook injects the file's whitelisted `LETS_*` values into model context every session. Tokens, passwords, and API keys belong elsewhere: `gh auth login` for GitHub, OS keychain for general secrets, tool-specific credential files (e.g. `.beads/.env` for beads). The whitelist filter in `cli/internal/hook/sessionstart/sessionstart.go` calls `letsconfig.Names()` to get the 4 canonical keys, so unknown keys are filtered - but file mode is 644 (world-readable on disk), so secrets in `.env` would still be exposed locally. Do not add secret keys.

### Adding a new config key

Single source of truth for canonical metadata: `cli/internal/letsconfig/keys.go::Keys`.
Single source of truth for Prefs↔Key wiring: `cli/internal/initcmd/render.go::Prefs.AsValues()`.

Required edits:

1. Append `Key{Name, Comment, Default}` entry to `letsconfig.Keys`. Name MUST start with `LETS_`.
2. Add field to `Prefs` struct in `cli/internal/initcmd/render.go` AND add ONE entry to `Prefs.AsValues()` map (one-line addition right below the field).
3. Bump frontmatter `version` in `plugins/lets/rules/lets-rules.md` so SessionStart drift check fires for installed users on next session.

If the key is exposed via the `/lets:init` slash command (most are):

4. Add a `--<key>` cobra flag in `cli/internal/cli/init.go` and wire it through `flagOrDefault(flag<X>, defaults["LETS_X"])` in prefs construction.
5. Add an AskUserQuestion in `plugins/lets/commands/init.md` (Step 2 first-time path + Step 3d "Keep current" option in change-config path).

Auto-derived (no edit needed):
- `.lets/.env` content (renderEnv → renderTemplate(Header, p.AsValues()))
- `.lets/.env.example` content (renderEnvExample → renderTemplate(ExampleHeader, Defaults()))
- SessionStart hook env-injection whitelist (sessionstart imports `letsconfig.Names()`)
- Regenerate wiring (`RegenerateEnv` uses `p.AsValues()`, iterates `letsconfig.Keys`)
- Future `/lets:doctor` validation + display

Then document in this CLAUDE.md "LETS Config keys" table + `README.md` Configuration block, and add consuming logic in the relevant commands.

## Dependencies

- beads plugin (task tracking)

## When Adding/Modifying Commands, Skills, or Agents

**Rules-file rule:** edits go ONLY to `plugins/lets/rules/lets-rules.md` (the canonical source). NEVER touch `.claude/rules/lets-rules.md` (installed copy) — that file is refreshed exclusively via `/lets:init` after a source bump. This is intentional: we eat our own dogfood for drift detection.

Update these files:

| File | What to update |
|------|----------------|
| `plugins/lets/rules/lets-rules.md` | Skill Quick Reference table (frontmatter `version` bump on any change so SessionStart drift check fires for installed users). Edit ONLY here, never the installed `.claude/rules/lets-rules.md`. |
| `commands/install.md` | Essential Skills / Planning Skills tables |
| `CLAUDE.md` Key Concepts | If adding a new skill |
| `README.md` | Agent table, feature descriptions |
| All `agents/*.md` `## Constraints` sections | If changing the read-only Bash allowlist or constraint wording, sync the identical 1-line text across all 13 analyst agents (verify with `grep -h "You are read-only" agents/*.md \| sort -u` returning exactly one line) |
| `commands/end.md` + `commands/done.md` `${CLAUDE_SESSION_ID}` references | If changing the session-id capture wording, sync across all occurrences (Step 3 progress comment + Step 5 summary block in `end.md`, Step 6 completion comment in `done.md`). Verify with `grep -n "CLAUDE_SESSION_ID" commands/` |
| `cli/internal/cli/<name>.go` + register in `cli/internal/cli/root.go` | If adding a Go subcommand. Add `<name>_test.go` (`package cli_test`). Use `cmd.OutOrStdout()`. Domain logic goes in `cli/internal/<name>/` (see `initcmd/`, `sessionstart/`, `statusline/`, `frontmatter/` for patterns). Update `cli/README.md` "Adding a subcommand" recipe if pattern changes. |

### Command Output Requirements

Every lets:* command MUST end with branded LETS box:

```
┌─ LETS ─────────────────┐
│  [action]? [command]   │
└────────────────────────┘
```

**Box format:**
- Header: `┌─ LETS ─` + padding with `─` + `┐`
- Lines: `│  ` + content + padding + ` │`
- Footer: `└─` + padding with `─` + `┘`
- Min width: 25 chars

**Content guidelines:**
- Short action word + `?` (e.g., "Commit?", "Next?", "Fix?")
- **ONLY `/lets:*` commands** - never raw commands like `bd sync`, `bd update`
- **Exception:** `git push` allowed after `/lets:done` or `/lets:end`
- **No command = no box** - if next step isn't a /lets:* command, just ask in plain text
- **Internal invocation = no box** - when a command is invoked programmatically by another command (e.g., `/lets:review --json` called by `/lets:pr`), the LETS box is waived

### Command Checklist

- [ ] Has LETS box in output section
- [ ] Updates Skill Quick Reference in `plugins/lets/rules/lets-rules.md` (and bump frontmatter `version`)
- [ ] Updates `/lets:install` Essential Skills / Planning Skills tables
- [ ] Follows session flow (start -> work -> commit -> done -> end)
- [ ] Description is clear and actionable
- [ ] **If file invokes any deferred tool** (`AskUserQuestion`, `EnterPlanMode`, `WebFetch`, etc.), include the `> **IMPORTANT:**` deferred-tool callout right after the file's brief description, before the first `## Step` (or first major section). Wording: see existing commands/skills for the standard block (search for `IMPORTANT:** If the spec below`)
