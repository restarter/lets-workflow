# lets-develop Command - Design Document

## Overview

`/lets-develop` - guided feature development command for the `lets-workflow` plugin. Inspired by feature-dev but with LETS-specific additions: AskUserQuestion for structured choices, LETS boxes, beads integration.

## Command Invocation

```
/lets-develop Add rate limiter to mobile API
/lets-develop Fix proxy connection timeout handling
/lets-develop Refactor payment gateway to use service layer
```

## Phase Structure

| # | Phase | Agents | User Interaction |
|---|-------|--------|-----------------|
| 1 | **Understand** | - | AskUserQuestion if unclear |
| 2 | **Explore** | 2-3 codebase-explorer | - |
| 3 | **Clarify** | - | AskUserQuestion with options |
| 4 | **Design** | 2 solution-architect | AskUserQuestion to pick |
| 5 | **Build** | - | Explicit approval required |
| 6 | **Review** | 2-3 code-reviewer | AskUserQuestion: fix/skip |
| 7 | **Wrap** | - | LETS box |

## Differences from feature-dev

| Aspect | feature-dev | lets-develop |
|--------|-------------|-------------|
| User questions | Free text questions | AskUserQuestion with clickable options |
| Progress | TodoWrite only | TodoWrite + LETS boxes between phases |
| Task tracking | None | Beads awareness (active task) |
| Architect agents | 3 (minimal/clean/pragmatic) | 2 (minimal/clean) - simpler choice |
| Agent naming | `feature-dev:code-explorer` | `lets-workflow:codebase-explorer` |
| Phase names | Discovery/Exploration/etc | Understand/Explore/Clarify/Design/Build/Review/Wrap |

## Phase Details

### Phase 1: Understand

```
Goal: Parse request, confirm understanding
```

- Read `$ARGUMENTS`
- Check for active beads task: `bd list --status=in_progress`
- If linked to task, read task details for context
- If description clear: summarize understanding, ask user to confirm
- If unclear: use AskUserQuestion
  - "What type of work?" [New feature / Bug fix / Refactor / Enhancement]
  - Follow up on problem, scope, constraints

### Phase 2: Explore

```
Goal: Deep codebase analysis
```

- Launch 2-3 `lets-workflow:codebase-explorer` agents in parallel
- Agent 1: "Find similar features/patterns in the codebase related to [feature]"
- Agent 2: "Map the architecture for [area], trace data flow through all layers"
- Agent 3 (if applicable): "Analyze existing implementation of [related feature]"
- Each agent MUST return list of 5-10 key files
- After agents complete: read ALL identified files
- Present findings summary

### Phase 3: Clarify

```
Goal: Fill all gaps before designing
```

- Review exploration findings + requirements
- Identify underspecified aspects
- Use AskUserQuestion with structured options for each decision point:
  - Error handling approach
  - Scope boundaries
  - Edge cases
  - Integration preferences
- Wait for ALL answers before proceeding
- If user says "your call" - recommend and confirm

### Phase 4: Design

```
Goal: Choose implementation architecture
```

- Launch 2 `lets-workflow:solution-architect` agents:
  - **Minimal approach**: smallest change, maximum reuse
  - **Clean approach**: better architecture, more maintainable
- After agents return: review and compare
- Present:
  - Comparison table (complexity, files changed, reusability, risk)
  - Your recommendation with reasoning
- Use AskUserQuestion: "Which approach?" [Minimal / Clean / Custom blend]
- Wait for choice

### Phase 5: Build

```
Goal: Implement the feature
```

- **DO NOT START WITHOUT EXPLICIT USER APPROVAL**
- Read all relevant files from previous phases
- Implement step by step
- Track progress with TodoWrite
- Follow codebase conventions (CLAUDE.md)
- Show LETS box mid-implementation for check:
  ```
  +- LETS --------------------+
  |  Check?  /lets-check      |
  +---------------------------+
  ```

### Phase 6: Review

```
Goal: Quality assurance
```

- Launch 2-3 `lets-workflow:code-reviewer` agents in parallel:
  - Focus 1: Bugs + functional correctness
  - Focus 2: Conventions + CLAUDE.md compliance
  - Focus 3: Architecture + DRY (if many files changed)
- Only report confidence >= 80
- Group by severity (Critical / Important)
- Use AskUserQuestion: "What to do?" [Fix all / Fix critical only / Proceed as-is]
- Fix based on choice

### Phase 7: Wrap

```
Goal: Summarize and suggest next steps
```

- Mark all todos complete
- Summary:
  - What was built
  - Key decisions made
  - Files created/modified
  - Suggested follow-up work
- Show LETS box:
  ```
  +- LETS --------------------------+
  |  Document?  /lets-beads-finish  |
  |  Commit?    /lets-commit        |
  +----------------------------------+
  ```

## Agent Specifications

### codebase-explorer

```yaml
name: codebase-explorer
model: sonnet
color: yellow
tools: Glob, Grep, LS, Read, NotebookRead, WebFetch, TodoWrite, WebSearch, KillShell, BashOutput
```

System prompt additions vs feature-dev:
- Read CLAUDE.md first for project context
- Understand MVC/service layer patterns
- Trace data flow through all layers
- Note migration/schema implications
- Return list of essential files with file:line

### solution-architect

```yaml
name: solution-architect
model: sonnet
color: green
tools: Glob, Grep, LS, Read, NotebookRead, WebFetch, TodoWrite, WebSearch, KillShell, BashOutput
```

System prompt additions:
- Read CLAUDE.md for architecture guidelines
- Follow existing patterns in codebase
- Provide file-by-file implementation map
- Build sequence as numbered checklist
- Make confident choices (don't hedge)

### code-reviewer

```yaml
name: code-reviewer
model: sonnet
color: red
tools: Glob, Grep, LS, Read, NotebookRead, WebFetch, TodoWrite, WebSearch, KillShell, BashOutput
```

System prompt additions:
- Check CLAUDE.md rules compliance first
- Confidence scoring 0-100, report >= 80 only
- Group by severity (Critical / Important)
- Concrete fix suggestions with file:line

## Plugin Structure

```
lets-plugin-claude/
+-- .claude-plugin/
|   +-- plugin.json
+-- commands/
|   +-- lets-develop.md
+-- agents/
|   +-- codebase-explorer.md
|   +-- solution-architect.md
|   +-- code-reviewer.md
+-- README.md
+-- docs/
    +-- feature-dev-reference.md
    +-- lets-develop-design.md
    +-- plugin-structure.md
    +-- agents.md
    +-- skills-vs-agents.md
    +-- gotchas.md
```

## Key Design Decisions

| Decision | Choice | Why |
|----------|--------|-----|
| commands/ not skills/ | Slash command | User explicitly invokes, like feature-dev |
| Generic agents | Not framework-specific | CLAUDE.md provides context, works with any project |
| 2 architect agents | Minimal vs Clean | Simpler choice than 3 options |
| AskUserQuestion | Structured options | Better UX than free-text |
| Confidence >= 80 | Filter noise | Proven threshold from feature-dev |
| sonnet for agents | Cost efficiency | Agents do analysis, main Claude orchestrates on opus |

## Installation & Testing

```bash
# Install
claude plugins add /path/to/lets-plugin-claude

# Verify
claude plugins list  # should show lets-workflow

# Test
/lets-develop Add rate limiter to mobile API
```

Verification:
- [ ] `/lets-develop` available
- [ ] Phase 2 launches `lets-workflow:codebase-explorer` agents
- [ ] Phase 4 shows AskUserQuestion with options
- [ ] Phase 6 shows confidence scores
- [ ] Phase 7 shows LETS box
