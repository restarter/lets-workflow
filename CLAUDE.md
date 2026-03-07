# lets-plugin-claude

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

```
.claude-plugin/plugin.json   # Plugin manifest
commands/                     # Slash commands (/lets:start, /lets:done, /lets:review, etc.)
agents/                       # Expert agents (review, opinion, ask, plan, team implementation)
hooks/                        # SessionStart hook, workflow rules, config template
scripts/lets/                 # Statusline + init script (copied/run per-project by /lets:init)
reference/                    # Reference plugins for studying patterns (gitignored)
```

## Key Concepts

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = experts dispatched by commands. `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:team`, `/lets:brainstorm` use specialized agents
- **Orchestrators** = commands that delegate to other commands. `/lets:pr` orchestrates `/lets:review` for full PR lifecycle
- **Hooks** = SessionStart injects workflow rules
- **Statusline** = per-project `.lets/statusline.sh`, source in `scripts/lets/statusline.sh`, copied by `/lets:init`

## Architecture Decisions

- Agents define WHO (expertise, scoring, output format). Commands define WHAT to do (provide diff, context)
- `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:brainstorm` use `subagent_type: "lets:agent-name"` to dispatch agents via Task tool
- `/lets:pr` orchestrates `/lets:review` (delegates analysis) and handles GitHub posting, follow-up, respond, and approval directly via gh CLI
- `/lets:execute` uses EnterPlanMode for native plan mode execution with user approval gates. No subagents.
- `/lets:check` reviews inline (no subagent) for speed
- All agents are read-only (Read, Grep, Glob, optionally Bash). No Edit/Write. Exception: `agents/implementer.md` has Edit/Write/Bash for `/lets:team` parallel implementation in isolated worktrees.
- `/lets:team` uses Agent Teams (TeamCreate, Agent with isolation: worktree) for parallel implementation. All other commands use subagents for analysis.
- Agents with Bash have prompt-level read-only constraints (`## Constraints` section in agent `.md` files). `hooks/validate-readonly.sh.old` exists as a PreToolUse hook prototype (not yet registered - agent frontmatter hooks silently ignored)
- Interactive worktrees managed via `/lets:worktree` command. Hook prototypes `hooks/worktree-setup.sh.old` and `hooks/worktree-cleanup.sh.old` (deferred - caused agent auto-cleanup issues)
- Worktrees stored in `.worktrees/` at project root - `.lets/` symlinked for interactive sessions
- SessionStart hook injects rules from `hooks/rules-context.md`
- SessionStart hook reads `.lets/config.yaml` and injects settings into session context
- Statusline source in `scripts/lets/statusline.sh`, copied to `.lets/statusline.sh` per-project by `/lets:init`. Project `.claude/settings.json` uses `git rev-parse --show-toplevel` to locate it.

## File Storage

All plugin-generated files go to `.lets/` (gitignored). Never use `/tmp` or other external paths.
This includes hook debug logs, temp files, and any runtime artifacts.
**WARNING:** Always use `.lets/` (with dot prefix), never `lets/`. The dot is easy to miss in manual paths.

```
.lets/config.yaml        # Per-project settings (language, merge-branch)
.lets/sessions/          # Session summaries, session-start-ref
.lets/reviews/           # Saved review reports
.lets/plans/             # Implementation plans
.lets/execution/         # Execution state (PR review: pr-{number}/, team records: team-*.json)
.lets/cache/             # Cached data (usage stats)
# Worktrees (outside .lets/ to avoid circular symlinks):
# .worktrees/            # Interactive worktrees only (agent worktrees use native Claude Code behavior)
```

## Dependencies

- beads plugin (task tracking)

## When Adding/Modifying Commands

Update these files:

| File | What to update |
|------|----------------|
| `hooks/rules-context.md` | Skill Quick Reference table |
| `commands/install.md` | Essential Skills / Planning Skills tables |

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
- [ ] Updates Skill Quick Reference in `hooks/rules-context.md`
- [ ] Updates `/lets:install` and `/lets:init` tables
- [ ] Follows session flow (start -> work -> commit -> done -> end)
- [ ] Description is clear and actionable
