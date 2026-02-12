# Plugin Architecture Research Findings

Research for task do3: study reference plugins and form architecture for lets-plugin-claude.

## Sources Studied

1. **Official docs**: [Create plugins](https://code.claude.com/docs/en/plugins), [Plugins reference](https://code.claude.com/docs/en/plugins-reference), [Skills](https://code.claude.com/docs/en/skills), [Subagents](https://code.claude.com/docs/en/sub-agents)
2. **Superpowers plugin** (v4.2.0) - 14 skills, 3 commands, 1 agent, hooks, lib/
3. **Feature-dev plugin** (v1.0.0) - 3 agents, 1 command (7-phase workflow)
4. **BMad Method** (v6.0.0) - 10 agents, 17+ workflows, CLI installer (NPM-based, not a standard plugin)

---

## 1. Plugin Structure Conventions

### Standard Layout (all 3 reference plugins agree)

```
plugin-name/
├── .claude-plugin/
│   └── plugin.json        # Minimal manifest (name required, rest optional)
├── skills/                # Auto-triggered by Claude based on description
│   └── skill-name/
│       ├── SKILL.md       # Required entry point
│       └── *.md           # Optional supporting files
├── commands/              # User-invoked slash commands
│   └── command-name.md
├── agents/                # Subagent definitions
│   └── agent-name.md
├── hooks/                 # Event handlers
│   └── hooks.json
├── .mcp.json              # MCP server configs (optional)
├── .lsp.json              # LSP server configs (optional)
└── scripts/               # Helper scripts for hooks
```

### Key Rules
- `.claude-plugin/` contains ONLY `plugin.json` - never put components there
- All component dirs at plugin root level
- `rules/` NOT supported in plugins - only at project (`.claude/rules/`) or user (`~/.claude/rules/`) level
- Components auto-discovered from default directories
- Custom paths in plugin.json SUPPLEMENT defaults, don't replace them
- All custom paths must be relative and start with `./`
- `${CLAUDE_PLUGIN_ROOT}` env var for absolute paths in hooks/scripts

### plugin.json
Minimal - only `name` required:
```json
{
  "name": "lets",
  "description": "Development workflow with session management, code review, and task tracking",
  "version": "1.0.0",
  "author": { "name": "..." },
  "homepage": "...",
  "repository": "...",
  "license": "MIT",
  "keywords": ["workflow", "review", "sessions"]
}
```

### Namespacing After Installation
- Commands: `/lets:start`, `/lets:commit`
- Agents: `lets:architect`, `lets:security-expert`
- Skills: `lets:workflow-rules` (auto-invoked)

---

## 2. Skills: Patterns & Best Practices

### Two Types of Skills

| Type | Location | Trigger | Use Case |
|------|----------|---------|----------|
| **Skills** | `skills/name/SKILL.md` | Auto-triggered by Claude based on description | Background knowledge, auto-guidance |
| **Commands** | `commands/name.md` | User types `/plugin:name` | Explicit workflows, user-initiated actions |

### SKILL.md Frontmatter (Full Schema)

```yaml
---
name: skill-name              # Optional (defaults to dir name), kebab-case
description: Use when...       # CRITICAL for auto-triggering
disable-model-invocation: true # Prevent Claude auto-invocation
user-invocable: false          # Hide from / menu
allowed-tools: Read, Grep      # Restrict tool access
model: sonnet                  # Override model
context: fork                  # Run in subagent
agent: Explore                 # Which subagent type for context:fork
argument-hint: [issue-number]  # Autocomplete hint
hooks: {}                      # Scoped lifecycle hooks
---
```

### Claude Search Optimization (CSO) - CRITICAL

From superpowers research - the most important finding:

**Description = triggering conditions ONLY. NEVER summarize the workflow.**

Why: Testing proved that when a description summarizes the skill's workflow, Claude follows the description as a shortcut instead of reading the full skill content. A description saying "code review between tasks" caused Claude to do ONE review, even though the skill specified TWO reviews.

```yaml
# BAD: Summarizes workflow - Claude may follow this instead of reading skill
description: Use when executing plans - dispatches subagent per task with code review between tasks

# BAD: Too much process detail
description: Use for TDD - write test first, watch it fail, write minimal code, refactor

# GOOD: Just triggering conditions
description: Use when executing implementation plans with independent tasks in the current session

# GOOD: Triggering conditions only
description: Use when implementing any feature or bugfix, before writing implementation code
```

Rules:
- Start with "Use when..."
- Describe the PROBLEM or SITUATION, not the process
- Keep under 500 characters
- Technology-agnostic unless skill is technology-specific
- Third person (injected into system prompt)

### Commands as Skill Wrappers (superpowers pattern)

Superpowers uses commands as lightweight wrappers:
```markdown
---
description: "You MUST use this before any creative work..."
disable-model-invocation: true
---

Invoke the superpowers:brainstorming skill and follow it exactly as presented to you
```

This pattern:
- Command = user entry point (`/brainstorm`)
- Skill = full workflow logic
- `disable-model-invocation: true` prevents model from elaborating

### Who Invokes What

| Frontmatter | User Can | Claude Can | Context Loading |
|-------------|----------|------------|-----------------|
| (default) | Yes | Yes | Description always in context, full on invoke |
| `disable-model-invocation: true` | Yes | No | Description NOT in context |
| `user-invocable: false` | No | Yes | Description always in context |

### Supporting Files

```
skill-name/
├── SKILL.md           # Main instructions (required, keep <500 lines)
├── reference.md       # Heavy docs (100+ lines)
├── examples.md        # Usage examples
└── scripts/
    └── helper.py      # Executable scripts
```

Reference from SKILL.md so Claude loads on demand.

---

## 3. Agents: Patterns & Best Practices

### Agent Frontmatter

```yaml
---
name: code-reviewer
description: Reviews code for bugs, logic errors, security vulnerabilities...
tools: Read, Grep, Glob, Bash
model: sonnet
color: red
permissionMode: default
maxTurns: 20
skills:
  - api-conventions
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "./scripts/validate.sh"
memory: user
---

System prompt content here...
```

### Description Pattern (from feature-dev)

All feature-dev agents follow same structure:
```
[Action verb]: [Detailed description of what it does] by [how it does it], [what deliverables it provides]
```

Examples:
- "Deeply analyzes existing codebase features by tracing execution paths, mapping architecture layers..."
- "Designs feature architectures by analyzing existing codebase patterns and conventions, then providing comprehensive implementation blueprints..."
- "Reviews code for bugs, logic errors, security vulnerabilities, code quality issues, and adherence to project conventions, using confidence-based filtering..."

### System Prompt Structure (consistent across reference plugins)

1. **Role declaration** - "You are a [expert type] who [responsibility]"
2. **Core mission** - Clear statement of what to accomplish
3. **Methodology** - Step-by-step approach with numbered phases
4. **Output format** - What to deliver, how to format
5. **Quality standards** - Thresholds, filters, scoring

### Color Coding (feature-dev pattern)
- Yellow = exploration/analysis
- Green = design/architecture
- Red = review/quality

### Tool Allocation
Feature-dev gives all agents same 10 tools (differentiation via system prompt).
Superpowers has 1 agent with `model: inherit`.

### Model Selection
| Model | When |
|-------|------|
| `haiku` | Quick tasks, file search |
| `sonnet` | Most tasks (review, analysis) |
| `opus` | Complex reasoning |
| `inherit` | Same as parent (default) |

### Confidence-Based Filtering (feature-dev code-reviewer)
```
0: false positive / pre-existing
25: maybe
50: moderate
75: highly confident
100: absolutely certain

ONLY report issues with confidence >= 80
```

---

## 4. Hooks: Patterns

### hooks.json Structure

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh",
            "async": true
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          { "type": "command", "command": "..." }
        ]
      }
    ]
  }
}
```

### Available Hook Events
- `PreToolUse` / `PostToolUse` / `PostToolUseFailure` - tool lifecycle
- `SessionStart` / `SessionEnd` - session lifecycle
- `SubagentStart` / `SubagentStop` - subagent lifecycle
- `UserPromptSubmit` - user input
- `PreCompact` - before compaction
- `Stop` - when Claude stops
- `Notification`, `PermissionRequest`, `TeammateIdle`, `TaskCompleted`

### Hook Types
- `command` - execute shell script
- `prompt` - evaluate with LLM
- `agent` - run agentic verifier

### Superpowers SessionStart Hook
Runs on startup/resume/clear/compact to set up context, check for skill updates.

---

## 5. Rules Workaround for Plugins

Since `rules/` is NOT supported in plugins, superpowers uses a meta-skill:

```yaml
---
name: using-superpowers
description: Use when starting any conversation - establishes how to find and use skills
---

EXTREMELY-IMPORTANT:
If there is even a 1% chance a skill might apply, you ABSOLUTELY MUST invoke the skill.
```

This auto-triggers at conversation start and establishes behavior rules.

Alternative: Use a `SessionStart` hook that injects context.

---

## 6. Cross-Plugin Comparison

| Aspect | Superpowers | Feature-dev | BMad Method |
|--------|------------|-------------|-------------|
| Focus | Process skills (TDD, debugging, planning) | Feature development workflow | Full agile framework |
| Skills | 14 | 0 (uses commands) | 4 (.claude only) |
| Commands | 3 (wrappers) | 1 (full workflow) | 0 |
| Agents | 1 (code-reviewer) | 3 (explorer, architect, reviewer) | 10 (YAML, non-standard) |
| Hooks | SessionStart | None | None |
| Model choice | inherit | sonnet (all agents) | N/A |
| Workflow type | Composable (skills chain) | Sequential 7-phase | Phase-based (4 phases) |
| User gates | Skill-level | 4 explicit approval points | Step-by-step |
| Rules approach | Meta-skill (using-superpowers) | None | Critical actions in YAML |
| Key pattern | CSO descriptions, self-enforcing | Confidence filtering, parallel agents | Step-file architecture, personas |

---

## 7. Architecture Decisions for lets-plugin-claude

Based on research findings, here are the key decisions needed:

### A. What Goes in the Plugin vs Project-Level

| Component | Current Location | Plugin? | Reasoning |
|-----------|-----------------|---------|-----------|
| Skills (lets-*) | `.claude/skills/` | YES | Core plugin functionality |
| Agents (experts) | `.claude/agents/` (fbp branch) | YES | Plugin agents for review/opinion |
| Rules | `.claude/rules/` | NO* | Not supported in plugins |
| Hooks (beads) | Project settings | MAYBE | Could provide SessionStart |

*Rules stay project-level. Consider a "workflow-rules" skill as workaround for portable rules.

### B. Skills vs Commands Decision

Current lets-* functions are all user-invoked (`/lets-start`, `/lets-commit`, etc.). In plugin terms:

**Option 1: Commands only** (simple, like feature-dev)
- All lets-* become `commands/lets-start.md`, `commands/lets-commit.md`
- User invokes via `/lets:start`, `/lets:commit`
- No auto-triggering

**Option 2: Skills with disable-model-invocation** (like superpowers wrappers)
- Skills in `skills/lets-start/SKILL.md` with `disable-model-invocation: true`
- Same user experience but supports supporting files

**Option 3: Mixed** (recommended)
- User-invoked workflows -> `commands/` (lets-start, lets-commit, lets-end, etc.)
- Auto-triggered guidance -> `skills/` (workflow-rules, beads-awareness)
- Expert agents -> `agents/`

### C. Rules Workaround

Create a skill that auto-triggers to inject workflow rules:
```yaml
---
name: workflow-rules
description: Use when starting any conversation, making code changes, or reviewing code. Provides LETS workflow conventions.
user-invocable: false
---

[Content from current .claude/rules/ files, consolidated]
```

### D. Naming Convention

After installation, everything gets `lets:` prefix:
- `/lets:start`, `/lets:commit`, `/lets:review`
- `lets:architect`, `lets:security-expert`
- Skills auto-namespaced to `lets:workflow-rules`

Note: Current skills use `lets-` prefix. Plugin namespacing adds `lets:`. This creates `lets:lets-start` which is redundant. Options:
1. Rename to just `start`, `commit`, `review` -> becomes `lets:start`, `lets:commit`
2. Keep `lets-` prefix -> becomes `lets:lets-start` (ugly)

**Recommendation: Drop `lets-` prefix from component names.**

### E. SessionStart Hook

Provide a SessionStart hook for context restoration (like superpowers):
```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh"
          }
        ]
      }
    ]
  }
}
```

### F. Agent Architecture

Keep the expert agents pattern from fbp branch:
- Each agent = focused expert with system prompt
- `model: sonnet` for most, `inherit` for complex
- Consistent tool set per role type (read-only for reviewers, full for implementers)
- Color coding for UI differentiation
- Confidence-based filtering for review agents (feature-dev pattern)

---

## 8. Proposed Plugin Structure

```
lets-plugin-claude/
├── .claude-plugin/
│   └── plugin.json
├── commands/                    # User-invoked workflows
│   ├── start.md                 # /lets:start
│   ├── end.md                   # /lets:end
│   ├── commit.md                # /lets:commit
│   ├── check.md                 # /lets:check
│   ├── review.md                # /lets:review
│   ├── opinion.md               # /lets:opinion
│   ├── install.md               # /lets:install
│   ├── beads-finish.md          # /lets:beads-finish
│   └── beads-status.md          # /lets:beads-status
├── skills/                      # Auto-triggered guidance
│   └── workflow-rules/
│       └── SKILL.md             # Rules workaround
├── agents/                      # Expert subagents
│   ├── architect.md
│   ├── security-expert.md
│   ├── qa-expert.md
│   ├── pragmatist.md
│   ├── compliance-expert.md
│   ├── database-expert.md
│   ├── backend-expert.md
│   ├── performance-expert.md
│   ├── frontend-expert.md
│   ├── devops-expert.md
│   └── ux-expert.md
├── hooks/
│   ├── hooks.json
│   └── session-start.sh
├── scripts/                     # Helper scripts
└── docs/                        # Internal docs (not shipped)
```

---

## 9. Migration Checklist

From current `.claude/` structure to plugin:

1. [ ] Create `.claude-plugin/plugin.json`
2. [ ] Move `.claude/skills/lets-*` to `commands/` (drop `lets-` prefix)
3. [ ] Move `.claude/agents/*` to `agents/`
4. [ ] Create `skills/workflow-rules/SKILL.md` from `.claude/rules/` content
5. [ ] Create `hooks/hooks.json` with SessionStart
6. [ ] Create `hooks/session-start.sh`
7. [ ] Update all internal skill references (cross-references between skills)
8. [ ] Test with `claude --plugin-dir .`
9. [ ] Keep `.claude/rules/` for project-level use during development
10. [ ] Update CLAUDE.md to reflect plugin structure

---

## 10. Open Questions

1. **Beads integration**: Should the plugin ship beads commands or assume beads is installed separately? (beads is already a separate plugin)
2. **Rules portability**: How much of `.claude/rules/` should go into the workflow-rules skill vs stay project-level?
3. **Command vs skill for lets-review**: Since review auto-selects experts, should it be a skill that Claude can auto-trigger after code changes?
4. **Plugin name**: `lets` (short, clean namespace) vs `lets-workflow` (more descriptive)?
5. **Dual-mode**: Support both plugin and project-level installation during transition?
