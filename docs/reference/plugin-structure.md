# Plugin Structure

## Current Structure

```
lets-plugin-claude/
├── .claude-plugin/
│   └── plugin.json         # Plugin manifest
├── .claude/
│   └── rules/              # Always-on project instructions (5 files)
├── commands/               # 12 slash commands (/lets:start, /lets:done, etc.)
├── agents/                 # 12 agents (11 experts + quick-reviewer)
├── hooks/                  # SessionStart hook - injects workflow rules
│   └── rules-context.md
├── .lets/                  # Plugin-generated files (gitignored)
│   ├── sessions/           # Session summaries
│   ├── reviews/            # Saved review reports
│   └── plans/              # Implementation plans
├── reference/              # Reference plugins for study (gitignored)
└── docs/                   # Documentation
```

Note: `rules/` is NOT supported in plugins - only project-level (`.claude/rules/`). The plugin uses a SessionStart hook to inject workflow rules instead.

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
