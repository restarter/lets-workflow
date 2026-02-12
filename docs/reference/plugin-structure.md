# Plugin Structure

## Current State

Project is NOT yet packaged as a plugin. All components live under `.claude/` as project-level config:

```
lets-plugin-claude/
├── .claude/
│   ├── rules/              # Always-on instructions (5 files)
│   ├── skills/             # Slash commands (9 skills)
│   │   ├── lets-start/
│   │   ├── lets-end/
│   │   ├── lets-commit/
│   │   ├── lets-check/
│   │   ├── lets-review/
│   │   ├── lets-opinion/
│   │   ├── lets-install/
│   │   ├── lets-beads-finish/
│   │   └── lets-beads-status/
│   └── agents/             # Expert agents (11 files, on fbp branch)
├── reference/              # Copies of reference plugins for study
│   ├── feature-dev/
│   ├── superpowers/
│   └── bmad-method/
└── docs/                   # Documentation
```

## Target Plugin Structure

When packaged as a plugin, components move to plugin-standard directories:

```
lets-plugin-claude/
├── .claude-plugin/
│   └── plugin.json         # Plugin manifest
├── agents/                 # Custom subagent types
│   └── agent-name.md
├── skills/                 # Auto-triggered skills
│   └── skill-name/
│       └── SKILL.md
├── commands/               # User-invoked slash commands
│   └── command-name.md
├── hooks/                  # Lifecycle hooks
│   └── hooks.json
└── .mcp.json               # MCP server configs (optional)
```

Key difference: `rules/` is NOT supported in plugins. Only project-level (`.claude/rules/`) or user-level (`~/.claude/rules/`). Workaround: use a skill with a broad description to simulate always-on rules.

## plugin.json

```json
{
  "name": "lets",
  "description": "Development workflow plugin with session management, code review, and task tracking",
  "version": "1.0.0",
  "author": {
    "name": "Your Name"
  }
}
```

After installation, components are namespaced: agents become `lets:agent-name`, skills become `lets:skill-name`.

## Installation

```bash
# From local directory
claude plugins add /path/to/lets-plugin-claude

# From git repo
claude plugins add github:user/lets-plugin-claude

# Test locally without installing
claude --plugin-dir ./lets-plugin-claude
```
