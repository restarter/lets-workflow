# Expert Agents Team - Design Document

**Task:** lets-plugin-claude-fbp
**Date:** 2026-02-12
**Status:** Approved

## Overview

10 universal expert agents that work like a real dev team. Same agent serves multiple contexts - review, opinion, direct consultation. Agent knows WHO it is, skill tells WHAT to do.

## Architecture

```
┌─────────────────────────────────────────────┐
│              Skills (Orchestrators)          │
│                                             │
│  /lets-review   /lets-opinion   /lets-ask   │
│       │              │              │       │
│   analyzes       analyzes       analyzes    │
│   diff           decision       question    │
│       │              │              │       │
│   selects        selects        routes to   │
│   agents +       agents +       1 agent +   │
│   model          model          model       │
└─────┬──────────────┬──────────────┬─────────┘
      │              │              │
      ▼              ▼              ▼
┌─────────────────────────────────────────────┐
│           agents/ (10 expert files)         │
│                                             │
│  architect.md      security-expert.md       │
│  backend-expert.md frontend-expert.md       │
│  database-expert.md devops-expert.md        │
│  qa-expert.md      docs-expert.md           │
│  compliance-expert.md git-historian.md      │
│                                             │
│  Each: base expertise + confidence scoring  │
│  Model: sonnet default, overridden by skill │
└─────────────────────────────────────────────┘
```

**Principle:** Agent file = expertise + personality + scoring format. Skill prompt = task + context (diff/decision/question) + output format.

## File Structure

```
agents/
  architect.md
  security-expert.md
  backend-expert.md
  frontend-expert.md
  database-expert.md
  devops-expert.md
  qa-expert.md
  docs-expert.md
  compliance-expert.md
  git-historian.md
skills/
  lets-review/SKILL.md    (updated - use Task tool with agents)
  lets-opinion/SKILL.md   (updated - launch agents for perspectives)
  lets-ask/SKILL.md       (new - direct consultation)
```

## Agent Roster

| # | File | Name | Tools | Expertise |
|---|------|------|-------|-----------|
| 1 | `architect.md` | architect | Read, Grep, Glob | SOLID, patterns, coupling, abstractions, system design |
| 2 | `security-expert.md` | security-expert | Read, Grep, Glob, Bash | OWASP Top 10, auth, crypto, secrets, input validation |
| 3 | `backend-expert.md` | backend-expert | Read, Grep, Glob, Bash | API design, business logic, error handling, performance |
| 4 | `frontend-expert.md` | frontend-expert | Read, Grep, Glob | React, Vue, TypeScript, a11y, CSS, bundle size, state management |
| 5 | `database-expert.md` | database-expert | Read, Grep, Glob, Bash | Schema design, migrations, queries, indexes, N+1, transactions |
| 6 | `devops-expert.md` | devops-expert | Read, Grep, Glob, Bash | Docker, CI/CD, nginx, shell scripts, monitoring, IaC |
| 7 | `qa-expert.md` | qa-expert | Read, Grep, Glob, Bash | Test strategy, coverage, assertions, mocking, TDD |
| 8 | `docs-expert.md` | docs-expert | Read, Grep, Glob | Documentation sync, API docs, README, CLAUDE.md, changelogs |
| 9 | `compliance-expert.md` | compliance-expert | Read, Grep, Glob | Project rules (CLAUDE.md), coding standards, conventions |
| 10 | `git-historian.md` | git-historian | Read, Grep, Glob, Bash | Git blame, history analysis, past decisions, context recovery |

**Tools logic:**
- All have Read, Grep, Glob (read code)
- Bash only for those who need commands (git log, docker, tests)
- No Edit/Write - experts analyze, don't modify code

## Agent File Format

```yaml
---
name: expert-name
description: When to delegate to this agent...
tools: Read, Grep, Glob
model: sonnet
---

Base expertise prompt. Who you are, what you know.
Confidence scoring rubric (0-100).
Output structure.
```

## Skill: /lets-review (Updated)

Steps 1-2 unchanged (determine mode, get diff).

### Step 3: Analyze + Select Agents + Select Model

```
Model selection:
  diff < 100 lines  -> haiku for most, sonnet for security
  diff 100-500      -> sonnet for all
  diff > 500        -> opus for architect/security, sonnet for rest

Exceptions:
  security-expert   -> minimum sonnet (never haiku)
  compliance-expert -> haiku always (rule matching)

Agent selection (same logic as current):
  Always: compliance-expert
  Code changes: backend-expert, security-expert, architect
  DB changes: database-expert
  Frontend: frontend-expert
  Docker/CI: devops-expert
  Tests: qa-expert
  Any non-trivial: docs-expert
  Existing code modified: git-historian
```

### Step 4: Launch Agents

```
Task(
  subagent_type="lets:security-expert",
  model="sonnet",
  prompt="REVIEW MODE. Review this diff for security vulnerabilities.

CLAUDE.MD RULES:
{claude_md}

DIFF:
{diff}

Rate each issue confidence 0-100. Only report >= 80.
Output format: ## Security Review ..."
)
```

Steps 5-9 unchanged (filter >= 80, dedup, verdict, save, output).

## Skill: /lets-opinion (Updated)

### Step 1: Frame the Problem - unchanged

### Step 2: Analyze Context + Select Experts

```
Decision about...        -> Agents
--------------------------------------------------
Auth/tokens/encryption   -> security, architect, backend
DB schema/migrations     -> database, architect, backend
Docker/CI/deploy         -> devops, security, architect
API design               -> architect, backend, security
UI/UX/components         -> frontend, architect, qa
Testing strategy         -> qa, backend, architect
Performance              -> backend, database, devops
General architecture     -> architect, security, backend
```

Minimum 3, maximum 5 agents. Architect always included (tech lead role).
All agents on sonnet. --deep flag upgrades architect/security to opus.

### Step 3: Launch Agents in Parallel

```
Task(
  subagent_type="lets:architect",
  model="sonnet",
  prompt="OPINION MODE. Evaluate this technical decision.

DECISION: {what needs to be decided}
OPTIONS: A) ... B) ... C) ...
CONSTRAINTS: {context}

Give your expert perspective in 2-3 sentences.
State which option you recommend and why.
Flag risks from your area of expertise."
)
```

### Step 4: Aggregate

Claude collects responses and forms:
- Quick Verdict (TL;DR)
- Expert opinions (from each agent)
- Comparison table
- Final recommendation

## Skill: /lets-ask (New)

### Usage

```bash
/lets-ask                          # Interactive - asks who and what
/lets-ask security "is this safe?" # Direct to expert with question
/lets-ask architect                # Opens dialog, asks question after
```

### Difference from /lets-opinion

| | /lets-ask | /lets-opinion |
|---|---|---|
| Purpose | Ask one colleague | Team meeting |
| Experts | Always 1 | 3-5 in parallel |
| Input | Question | Decision + options |
| Output | Direct answer | Comparison table + recommendation |
| Analogy | Slack ping | 30 min meeting |

### Step 1: Determine Expert

If specified - use it. Otherwise show picker:

```
Who do you want to ask?

 1. architect     - System design, patterns, SOLID
 2. security      - Vulnerabilities, auth, crypto
 3. backend       - API, logic, performance
 4. frontend      - UI, React/Vue, a11y
 5. database      - Schema, queries, migrations
 6. devops        - Docker, CI/CD, infrastructure
 7. qa            - Testing, coverage, strategy
 8. docs          - Documentation, API docs
 9. compliance    - Project rules, standards
10. git-historian - History, past decisions, blame

Expert (name or number):
```

### Step 2: Determine Question

If passed as argument - use it. Otherwise ask.

### Step 3: Model Selection

```
Keywords "what is", "show", "example"       -> haiku
Keywords "how best", "compare", "why"       -> sonnet
Keywords "design", "architecture", "plan"   -> opus

Fallback -> sonnet
```

### Step 4: Launch

```
Task(
  subagent_type="lets:architect",
  model="sonnet",
  prompt="ASK MODE. Answer this developer's question.

PROJECT CONTEXT:
{claude_md summary}

QUESTION: {user_question}

Give a clear, actionable answer. Use code examples if relevant.
Reference project patterns from CLAUDE.md where applicable."
)
```

### Step 5: Output

```
## Architect says:
{answer}

---
LETS box with follow-up suggestions
```

## Model Selection Summary

| Context | Default | --deep |
|---------|---------|--------|
| Review (small diff) | haiku | sonnet |
| Review (medium diff) | sonnet | opus |
| Review (large diff) | opus | opus |
| Review security | min sonnet | opus |
| Review compliance | haiku | haiku |
| Opinion | sonnet | opus for architect/security |
| Ask (simple) | haiku | sonnet |
| Ask (analysis) | sonnet | opus |
| Ask (design) | opus | opus |

## Implementation Order

1. Create 10 agent files in `agents/`
2. Create `/lets-ask` skill
3. Update `/lets-review` - replace inline prompts with Task calls
4. Update `/lets-opinion` - launch agents instead of single-pass
5. Update CLAUDE.md with new capabilities
6. Test each skill manually
