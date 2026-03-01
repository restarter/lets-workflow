# Custom Subagents

## How Agents Work

A subagent is a markdown file with YAML frontmatter. When Claude delegates work via Task tool, it specifies `subagent_type` which maps to an agent file.

## Agent File Format

```markdown
---
name: my-agent
description: When Claude should delegate to this agent. Use proactively when...
tools: Read, Grep, Glob, Bash
model: sonnet
---

System prompt for the agent goes here.
The agent receives this as its base instructions.
```

## Frontmatter Fields

| Field | Required | Values | Description |
|-------|----------|--------|-------------|
| `name` | Yes | lowercase + hyphens | Unique identifier |
| `description` | Yes | text | When to use - Claude reads this to decide delegation |
| `tools` | No | comma-separated | Available tools (default: all) |
| `disallowedTools` | No | comma-separated | Tools to deny |
| `model` | No | `sonnet`, `opus`, `haiku`, `inherit` | Model for this agent (default: inherit from parent) |
| `permissionMode` | No | `default`, `acceptEdits`, `dontAsk`, `bypassPermissions`, `plan` | Permission level |
| `skills` | No | list | Skills to preload into agent context |
| ~~`hooks`~~ | ~~No~~ | ~~object~~ | NOT SUPPORTED in runtime (tested 2026-03-01, silently ignored). Use plugin hooks.json instead |
| `memory` | No | `user`, `project`, `local` | Persistent memory scope |

## Tool Options

Available tools for `tools` field:
- `Read` - read files
- `Edit` - edit files
- `Write` - create files
- `Bash` - run commands
- `Glob` - find files by pattern
- `Grep` - search file contents
- `WebFetch` - fetch URLs
- `WebSearch` - web search
- `Task` - launch sub-subagents
- `NotebookEdit` - edit Jupyter notebooks

## Where to Place Agents

| Location | Scope | Naming in Task tool |
|----------|-------|---------------------|
| `.claude/agents/` (project) | This project only | `agent-name` |
| `~/.claude/agents/` (user) | All your projects | `agent-name` |
| Plugin `agents/` dir | Anyone who installs plugin | `plugin-name:agent-name` |

**Important:** Only plugin agents register as `subagent_type` for the Task tool. Project-level agents (`.claude/agents/`) are available but not namespaced.

## Description Field

The `description` field determines WHEN Claude delegates to the agent. Write it as instructions:

- BAD: "Code reviewer"
- GOOD: "Reviews code for bugs, logic errors, and security vulnerabilities. Use proactively after code changes."

## Model Selection

| Model | When to use |
|-------|-------------|
| `haiku` | Quick, straightforward tasks (file search, simple checks) |
| `sonnet` | Most tasks (code review, analysis, implementation) |
| `opus` | Complex reasoning, architecture decisions |
| `inherit` | Same model as parent conversation (default) |
