# Plugin Capabilities

What a Claude Code plugin can and cannot do.

## Supported Directories

| Directory | Purpose | Example |
|-----------|---------|---------|
| `.claude-plugin/` | Plugin manifest (plugin.json only) | `{"name": "lets", ...}` |
| `commands/` | Slash commands (user-invoked) | `/lets:develop` |
| `agents/` | Custom subagents (Task tool) | `lets:architect` |
| `skills/` | Agent skills (model-invoked, auto-triggered) | Claude invokes based on context |
| `hooks/` | Event handlers (hooks.json) | Run linter after file edit |
| `.mcp.json` | MCP server configs | External tool integrations |
| `.lsp.json` | LSP server configs | Code intelligence |

## NOT Supported in Plugins

- **`rules/`** - rules only work at project level (`.claude/rules/`) or user level (`~/.claude/rules/`)
- No way to inject always-on instructions from a plugin

## Commands vs Skills

| Aspect | commands/ | skills/ |
|--------|-----------|---------|
| Trigger | User types `/plugin:command` | Claude auto-invokes based on description |
| Naming | `commands/hello.md` -> `/plugin:hello` | `skills/hello/SKILL.md` -> auto |
| Context | Gets `$ARGUMENTS` from user | Gets full conversation context |
| Use case | Explicit workflows | Background knowledge, auto-triggered guidance |

## Workaround: "Rules" via Skills

Since `rules/` is not supported in plugins, use a skill with a broad description to simulate always-on rules:

```yaml
---
name: workflow-rules
description: Use when starting any conversation, making code changes, or reviewing code. Provides LETS workflow conventions and code standards.
---
```

Claude will auto-invoke this skill when the description matches the current context. Not guaranteed to trigger every time (unlike real rules), but close enough.

## Plugin Naming

After installation, all components are namespaced with the `name` from plugin.json:

| Component | Project-level | Plugin |
|-----------|--------------|--------|
| Command | N/A | `/lets:develop` |
| Agent | `architect` | `lets:architect` |
| Skill | `lets-review` | `lets:lets-review` |

## Testing Locally

```bash
# Load plugin without installing
claude --plugin-dir ./lets-plugin-claude

# Load multiple plugins
claude --plugin-dir ./plugin-one --plugin-dir ./plugin-two
```

Restart Claude Code after changes to pick up updates.

## Installation

```bash
# From local path
claude plugins add /path/to/lets-plugin-claude

# From git repo
claude plugins add github:user/lets-plugin-claude
```
