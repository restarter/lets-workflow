---
description: Quick code sanity check. Single agent (opus), 5 perspectives.
argument-hint: "[--staged|--last-commit]"
---

# Quick Local Code Check

Fast sanity check of local changes via single agent, 5 perspectives.

## Usage

```bash
/lets:check              # uncommitted changes (default)
/lets:check --staged     # only staged changes
/lets:check --last-commit # last commit
```

## When to Use

- Quick sanity check during development
- Before commit for significant changes
- When unsure if code is ready
- Spot check after refactoring

**For full review:** Use `/lets:review` (local or PR, up to 11 agents).

## Step 1: Get Changes

```bash
# Default: uncommitted changes
git diff

# Or staged only
git diff --staged

# Or last commit
git diff HEAD~1
```

If no changes, inform user and exit.

## Step 2: Gather Context

```bash
ROOT=$(git rev-parse --show-toplevel)
git diff --stat
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

## Step 3: Launch Agent

Launch a single agent via Task tool:

```
Task(
  subagent_type="general-purpose",
  model="opus",
  prompt="CHECK MODE. Quick code sanity check from 5 perspectives.

PROJECT CONTEXT:
{CLAUDE.md summary}

CHANGED FILES:
{git diff --stat output}

DIFF:
{diff content}

INSTRUCTIONS:
Review this diff quickly from 5 angles. Be concise - this is a quick check, not a full review.

### [Bug] Bugs & Logic
- Logic errors, off-by-one, edge cases
- Null/undefined access, missing checks
- Broken control flow, unreachable code
- Incorrect conditions or comparisons

### [Sec] Security
- Secrets or credentials exposed
- SQL injection, XSS, command injection
- Missing input validation at boundaries
- Auth/permission issues

### [Perf] Performance
- N+1 queries, unnecessary DB calls
- Expensive operations in loops
- Missing pagination, unbounded queries
- Memory leaks, large object copies

### [Quality] Code Quality
- Unclear naming, high complexity
- Code duplication (3+ similar blocks)
- Dead code, unused imports
- Readability issues

### [Compliance] Project Rules
- CLAUDE.md violations (quote the rule)
- Breaks existing patterns in codebase
- Inconsistent with project conventions

OUTPUT FORMAT:
Keep it SHORT - max 5 issues, only actionable findings.

## Quick Check: {N files changed}

### Verdict: {[OK] GOOD | [!] REVIEW | [X] FIX}

### Issues
{Only if found}
- [Tag] **file:line** {issue} - {fix suggestion}

### Looks Good
{1-2 positive notes}

---
{verdict suggestion}

VERDICT LOGIC:
- No issues -> [OK] GOOD
- Minor issues only -> [!] REVIEW
- Security or critical bugs -> [X] FIX

RULES:
- Max 5 issues - prioritize by severity
- No false positives - when in doubt, skip it
- Be direct, no hedging
- Reference specific lines from the diff"
)
```

## Step 4: Present Results

Show the agent's response directly. Then add LETS box:

**If GOOD or REVIEW:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets:commit  │
└────────────────────────┘
```

**If FIX:** No box. Say "Fix the issues above, then run `/lets:check` again."

## What This Is NOT

- NOT a full code review (use `/lets:review`)
- NOT confidence scoring (that's for full review)
- NOT specialized agents (uses single general-purpose agent, not lets: experts)
- NOT saved to file (console only)

## Workflow Integration

```
Work -> /lets:check -> /lets:commit -> Push -> PR -> /lets:review
         ^                                            |
    Quick check                               Full PR review
    single agent                              up to 11 agents
```

## Notes

- Uses opus for thorough single-pass analysis
- Agent keeps diff out of main context window
- Focus on actionable issues only - this is a helper, not a blocker
