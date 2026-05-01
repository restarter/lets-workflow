# lets-workflow

Claude Code plugin for development workflow with session management, code review, and task tracking.

## Structure

```
.claude-plugin/plugin.json   # Plugin manifest
commands/                     # Slash commands (/lets:start, /lets:done, /lets:review, etc.)
agents/                       # 14 expert agents (architect, security, qa, actor, etc.) dispatched by review/opinion/ask/plan/brainstorm/team
skills/                       # Reusable skills (user-facing auto-triggered + internal referenced by commands)
hooks/                        # SessionStart hook, workflow rules, config template
scripts/lets/                 # Statusline + init script (copied/run per-project by /lets:init)
docs/                         # Plans, knowledge base, reference docs, comment exports
reference/                    # Reference plugins for studying patterns (gitignored)
```

## Key Concepts

- **Commands** = user-initiated workflows (sessions, commits, reviews)
- **Agents** = experts dispatched by commands. `/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:brainstorm` dispatch via subagents. `/lets:team` dispatches via Agent Teams (parallel, worktree isolation). `actor` is a meta-agent that loads external personalities (URL or file) and adapts them to LETS modes
- **Orchestrators** = commands that delegate to other commands. `/lets:pr` orchestrates `/lets:review` for full PR lifecycle
- **Hooks** = SessionStart injects workflow rules
- **Statusline** = per-project `.lets/statusline.sh`, source in `scripts/lets/statusline.sh`, copied by `/lets:init`
- **Skills** = reusable actions in `skills/<name>/SKILL.md`. Two types: user-facing (auto-discovered, triggered via description match or Skill tool) and internal (not auto-discovered, read by commands via Read tool when needed). Examples: `create-task`, `commit`, `take-task` (user-facing), `detect-task`, `actor-fetch-personality` (internal)

## Architecture Decisions

- Agents define WHO and HOW (expertise, behavioral modes, tiered scoring, output format). Commands define WHAT and WHEN (provide content, select agents, pass mode name)
- Agent frontmatter fields: `name`, `description`, `tools`, `color` (terminal output: red/blue/green/yellow/purple/orange/pink/cyan), optional `model` (default inherits from parent, `opus` for complex analysis), `memory: project` (persistent cross-session learning). All agents use tiered scoring ([BLOCKER]/[SUGGESTION]/[NIT]), self-contained Modes, MANDATORY rules in `Output Format` sections, and domain-specific `Memory (after output)` sections that subordinate memory persistence to the primary text response
- Actor meta-agent loads personalities at runtime via internal skill `actor-fetch-personality`. Command fetches personality content (curl for URLs, Read for files), user confirms via review gate, actor receives it in prompt as `PERSONALITY:` block. Fallback "generalist" identity when no personality provided
- Agent selection: each command owns its detection/selection logic (different semantics per context). Multi-agent dispatching commands (review, opinion, brainstorm, plan) show selection panel with cost note before launch. Most agents have explicit PLAN mode for plan review.
- Agents always respond in English. Commands localize output to user's language via LETS Config and Rules.
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
- User-facing skills: auto-discovered by Claude Code, appear in skill list, trigger on description match. Frontmatter description must NOT use YAML quotes.
- Internal skills: NOT auto-discovered. Commands reference with "use the X skill" and read the SKILL.md via Read tool at `` `${CLAUDE_PLUGIN_ROOT}/skills/X/SKILL.md` `` - the env var ensures the path resolves correctly whether plugin is loaded via marketplace install or `--plugin-dir` dev mode (relative `skills/...` paths break in foreign projects). No accidental triggering, no context cost until needed.
- Commands define WHAT to do and orchestrate the flow. User-facing skills define full reusable flows (steps, user gates) that auto-trigger on natural language. Internal skills define shared procedures read by commands on demand. Commands delegate to skills for shared operations.
- Gate for new skills: extract only if (a) user-facing with standalone trigger value, or (b) internal logic duplicated in 3+ commands.

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

## When Adding/Modifying Commands or Skills

Update these files:

| File | What to update |
|------|----------------|
| `hooks/rules-context.md` | Skill Quick Reference table |
| `commands/install.md` | Essential Skills / Planning Skills tables |
| `CLAUDE.md` Key Concepts | If adding a new skill |
| `README.md` | Agent table, feature descriptions |
| Agent prompts (`agents/*.md`) + Task templates in `commands/{review,plan,opinion}.md` | MANDATORY block + Memory opener: keep wording synced across all 22 occurrences (`grep -n "MANDATORY:" agents/ commands/` to find them) |

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
- [ ] Updates `/lets:install` Essential Skills / Planning Skills tables
- [ ] Follows session flow (start -> work -> commit -> done -> end)
- [ ] Description is clear and actionable
- [ ] **If file invokes any deferred tool** (`AskUserQuestion`, `EnterPlanMode`, `WebFetch`, etc.), include the `> **IMPORTANT:**` deferred-tool callout right after the file's brief description, before the first `## Step` (or first major section). Wording: see existing commands/skills for the standard block (search for `IMPORTANT:** If the spec below`)
