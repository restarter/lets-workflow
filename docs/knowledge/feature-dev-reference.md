# Feature-Dev Plugin - Internal Reference

How the official `feature-dev` plugin works. Used as reference when building our own skills and agents.

> Full source available in `reference/feature-dev/`

## File Structure

```
feature-dev/
├── .claude-plugin/
│   └── plugin.json
├── commands/
│   └── feature-dev.md           # 7-phase workflow command
└── agents/
    ├── code-explorer.md         # Codebase analysis (yellow, sonnet)
    ├── code-architect.md        # Architecture design (green, sonnet)
    └── code-reviewer.md         # Code review (red, sonnet)
```

## plugin.json

Minimal - just name, description, author. No version, hooks, or mcp.

## 7-Phase Workflow

| Phase | What | Agents |
|-------|------|--------|
| 1. Discovery | Parse request, create todo list | - |
| 2. Exploration | Deep codebase analysis | 2-3 code-explorer (parallel) |
| 3. Clarify | Fill gaps, ask questions | - |
| 4. Architecture | Design approaches | 2-3 code-architect (parallel) |
| 5. Implementation | Build (needs user approval) | - |
| 6. Review | Quality assurance | 3 code-reviewer (parallel) |
| 7. Summary | Wrap up, next steps | - |

## Agent Pattern

All agents share the same structure:
- Model: sonnet (cheaper, for helper tasks)
- Tools: read-only (Glob, Grep, Read, WebFetch, etc.) - no Edit/Write/Bash
- `color` field for UI differentiation
- Must return list of key files for main Claude to read

## Key Patterns Worth Adopting

1. **Parallel agents** - phases 2, 4, 6 all launch multiple agents simultaneously
2. **File list requirement** - agents must return key files, main Claude reads them after
3. **User checkpoints** - explicit approval needed at phases 3, 4, 5
4. **Confidence scoring** - reviewers filter noise with >= 80 threshold
5. **TodoWrite tracking** - progress visible throughout
6. **Generic agents** - not framework-specific, CLAUDE.md provides context

## Confidence Scoring

```
0:   false positive
25:  maybe real, maybe false positive
50:  real but might be nitpick
75:  highly confident, impacts functionality
100: absolutely certain
```

Only report issues with confidence >= 80.
