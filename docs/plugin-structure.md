# Plugin Structure

## Directory Layout

```
lets-plugin-claude/
|-- .claude-plugin/
|   +-- plugin.json          # Plugin manifest
|-- agents/                   # Custom subagent types
|   +-- agent-name.md        # Each agent = markdown file with YAML frontmatter
|-- skills/                   # Skills (slash commands)
|   +-- skill-name/
|       +-- SKILL.md
|-- rules/                    # Rules loaded into every conversation
|   +-- rule-name.md
|-- hooks/                    # Lifecycle hooks
+-- docs/                     # Documentation (not part of plugin)
```

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

After installation, agents become available as `lets:agent-name`, skills as `lets:skill-name`.

## Installation

```bash
# From local directory
claude plugins install /path/to/lets-plugin-claude

# From git repo
claude plugins install github:user/lets-plugin-claude
```
