# lets-develop Command - Design Document

> **STATUS: SUPERSEDED** - This design was partially replaced by the Expert Agents Team approach (see `expert-agents-team-design.md`). The universal expert agents model replaced framework-specific agents like `codebase-explorer` and `solution-architect`. Kept for historical context.

## Overview

`/lets-develop` - guided feature development command. Inspired by feature-dev but with LETS-specific additions: AskUserQuestion for structured choices, LETS boxes, beads integration.

## Phase Structure

| # | Phase | Agents | User Interaction |
|---|-------|--------|-----------------|
| 1 | Understand | - | AskUserQuestion if unclear |
| 2 | Explore | 2-3 codebase-explorer | - |
| 3 | Clarify | - | AskUserQuestion with options |
| 4 | Design | 2 solution-architect | AskUserQuestion to pick |
| 5 | Build | - | Explicit approval required |
| 6 | Review | 2-3 code-reviewer | AskUserQuestion: fix/skip |
| 7 | Wrap | - | LETS box |

## What Changed

The Expert Agents Team design (task fbp) took a different direction:
- Instead of task-specific agents (codebase-explorer, solution-architect, code-reviewer), we built 11 universal expert agents (architect, security-expert, backend-expert, etc.)
- Same agent serves review, opinion, and direct consultation modes
- Skills orchestrate which agents to launch based on context

## Ideas Still Relevant

- AskUserQuestion for structured choices (not yet implemented in current skills)
- LETS boxes between phases
- Beads task awareness in feature development flow
- 2-architect comparison (minimal vs clean approach)
