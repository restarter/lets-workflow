# Feature-Dev Plugin - Internal Reference

How the official `feature-dev` plugin works. Reference for building our own.

## Source Location

```
~/.claude/plugins/marketplaces/claude-plugins-official/plugins/feature-dev/
```

## File Structure

```
feature-dev/
├── .claude-plugin/
│   └── plugin.json              # {"name": "feature-dev", ...}
├── README.md
├── commands/
│   └── feature-dev.md           # Slash command /feature-dev (7-phase workflow)
└── agents/
    ├── code-explorer.md         # Codebase analysis (yellow, sonnet)
    ├── code-architect.md        # Architecture design (green, sonnet)
    └── code-reviewer.md         # Code review (red, sonnet)
```

## plugin.json - Minimal

```json
{
  "name": "feature-dev",
  "description": "Comprehensive feature development workflow with specialized agents for codebase exploration, architecture design, and quality review",
  "author": {
    "name": "Anthropic",
    "email": "support@anthropic.com"
  }
}
```

No `version`, no `hooks`, no `mcp` - minimal config.

## Command: feature-dev.md

### Frontmatter

```yaml
---
description: Guided feature development with codebase understanding and architecture focus
argument-hint: Optional feature description
---
```

Key fields:
- `description` - shown in /help list
- `argument-hint` - shown as placeholder
- `$ARGUMENTS` - replaced with whatever user typed after `/feature-dev`
- Also available: `allowed-tools` (pre-approve tools), `model` (override model)

### 7-Phase Workflow

**Core principles stated at top:**
- Ask clarifying questions, don't assume
- Understand before acting - read existing code first
- Read files identified by agents
- Simple and elegant code
- Use TodoWrite for progress tracking

**Phase 1: Discovery**
- Create todo list with all phases
- If feature unclear, ask: what problem? what should it do? constraints?
- Summarize understanding and confirm

**Phase 2: Codebase Exploration**
- Launch 2-3 code-explorer agents in PARALLEL
- Each targets different aspect (similar features, architecture, patterns)
- Each must include list of 5-10 key files to read
- After agents return: read ALL identified files
- Present summary

**Phase 3: Clarifying Questions** (CRITICAL - DO NOT SKIP)
- Review findings + original request
- Identify underspecified: edge cases, error handling, integration, scope, design prefs, backward compat, performance
- Present organized question list
- Wait for answers
- If user says "whatever you think" - give recommendation and get explicit confirmation

**Phase 4: Architecture Design**
- Launch 2-3 code-architect agents with different focuses:
  - Minimal changes (smallest change, maximum reuse)
  - Clean architecture (maintainability, elegant abstractions)
  - Pragmatic balance (speed + quality)
- Review all approaches, form opinion on best fit
- Present: summary of each, trade-offs comparison, recommendation with reasoning
- Ask user which approach they prefer

**Phase 5: Implementation**
- DO NOT START WITHOUT USER APPROVAL
- Read all relevant files
- Implement following chosen architecture
- Follow codebase conventions
- Update todos as progress

**Phase 6: Quality Review**
- Launch 3 code-reviewer agents in parallel:
  - Simplicity/DRY/Elegance
  - Bugs/Functional correctness
  - Conventions/Abstractions
- Consolidate findings, identify highest severity
- Present findings, ask: fix now, fix later, or proceed as-is

**Phase 7: Summary**
- Mark todos complete
- What was built, key decisions, files modified, next steps

## Agent Definitions

### Common Pattern

All agents have identical frontmatter structure:

```yaml
---
name: agent-name
description: What the agent does and when to delegate to it
tools: Glob, Grep, LS, Read, NotebookRead, WebFetch, TodoWrite, WebSearch, KillShell, BashOutput
model: sonnet
color: yellow/green/red
---
```

**Key observations:**
- All agents use `sonnet` (cheaper, for helper tasks)
- All have identical tool set (read-only + search + web)
- No Edit, Write, Bash - agents can't modify code
- `BashOutput` is read-only variant of Bash (can read output but not run destructive commands)
- `color` is just for UI differentiation

### code-explorer (yellow)

**Role:** Trace feature implementations across codebase

**System prompt structure:**
1. Feature Discovery - entry points, core files, boundaries
2. Code Flow Tracing - call chains, data transformations, dependencies, state changes
3. Architecture Analysis - layers, patterns, interfaces, cross-cutting concerns
4. Implementation Details - algorithms, data structures, error handling, edge cases, performance

**Must output:**
- Entry points with file:line references
- Execution flow with data transformations
- Key components and responsibilities
- Architecture insights
- Dependencies
- Observations
- **List of essential files**

### code-architect (green)

**Role:** Design implementation blueprints

**System prompt structure:**
1. Codebase Pattern Analysis - extract patterns, conventions, tech stack, module boundaries, CLAUDE.md guidelines
2. Architecture Design - make decisive choices, ensure integration, design for testability/performance/maintainability
3. Complete Implementation Blueprint - every file to create/modify, components, data flow, build phases

**Must output:**
- Patterns & Conventions Found (file:line, similar features, abstractions)
- Architecture Decision (chosen approach, rationale, trade-offs)
- Component Design (file path, responsibilities, dependencies, interfaces)
- Implementation Map (specific files with detailed changes)
- Data Flow (entry to output)
- Build Sequence (phased checklist)
- Critical Details (errors, state, testing, performance, security)

**Key directive:** Make confident choices, don't present multiple options.

### code-reviewer (red)

**Role:** Review code with confidence scoring

**Review scope:** By default, unstaged changes (`git diff`). User can specify different scope.

**System prompt structure:**
1. Project Guidelines Compliance - CLAUDE.md rules, conventions, patterns
2. Bug Detection - logic errors, null handling, race conditions, memory leaks, security, performance
3. Code Quality - duplication, error handling, accessibility, tests

**Confidence scoring (0-100):**
- 0: false positive
- 25: maybe real, maybe false positive
- 50: real but might be nitpick
- 75: highly confident, impacts functionality
- 100: absolutely certain

**ONLY report issues with confidence >= 80**

**Output:** Group by severity (Critical vs Important), include file:line, guideline reference, concrete fix.

## How Agents Are Invoked

In the command markdown, the instruction says "launch 2-3 code-explorer agents in parallel." Claude interprets this and uses the Task tool:

```
Task(
  subagent_type="feature-dev:code-explorer",
  prompt="Find features similar to [X] and trace through implementation...",
  description="Explore similar features"
)
```

The naming convention is `plugin-name:agent-name`. After plugin installation, agents are registered globally under this namespace.

## Key Patterns Worth Adopting

1. **Parallel agents** - phases 2, 4, 6 all launch multiple agents simultaneously
2. **File list requirement** - agents must return key files, main Claude reads them after
3. **User checkpoints** - explicit approval needed at phases 3, 4, 5
4. **Confidence scoring** - reviewers filter noise with >= 80 threshold
5. **TodoWrite tracking** - progress visible throughout
6. **Generic agents** - not framework-specific, CLAUDE.md provides context
