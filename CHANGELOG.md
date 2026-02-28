# Changelog

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

[0.2.0]: https://github.com/restarter/lets-workflow/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/restarter/lets-workflow/releases/tag/v0.1.0
