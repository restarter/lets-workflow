# Changelog

## [Unreleased]

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

[0.2.4]: https://github.com/restarter/lets-workflow/compare/v0.2.3...v0.2.4
[0.2.3]: https://github.com/restarter/lets-workflow/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/restarter/lets-workflow/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/restarter/lets-workflow/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/restarter/lets-workflow/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/restarter/lets-workflow/releases/tag/v0.1.0
