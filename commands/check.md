---
description: Quick code sanity check. Inline 5-perspective review.
argument-hint: "[--staged|--last-commit]"
---

# Quick Local Code Check

Fast inline sanity check of local changes from 5 perspectives.

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
# ROOT = project-root from LETS Config
git diff --stat
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -100
```

## Step 3: Review with 5 Lenses

ultrathink

Review the diff directly using these 5 perspectives. Think like a senior dev doing a quick PR scan - catch real issues, skip noise.

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

### [Compliance] Project Rules & Docs
- CLAUDE.md violations (quote the rule)
- Breaks existing patterns in codebase
- Inconsistent with project conventions
- Code changed but related docs/README not updated

### Review Focus

Ask yourself:
- Will this break in production?
- Can this be exploited?
- Does this violate project rules?
- Will the next developer be confused?

### Confidence Filter

Rate each finding 0-100:
- **90-100**: Bug, security issue, or rule violation - report it
- **70-89**: Concern that experienced developers would flag - report it
- **Below 70**: Skip it

**Only report findings with confidence >= 70. Max 5 issues.**

## Step 4: Present Results

### Output Format

```
## Quick Check: {N files changed}

### Verdict: {[OK] GOOD | [!] REVIEW | [X] FIX}

### Issues
{Only if found}
- [Tag] **file:line** {issue} - {fix suggestion}
  Confidence: {score}

### Looks Good
{1-2 positive notes}
```

### Verdict Logic

- No issues -> [OK] GOOD
- Minor issues only -> [!] REVIEW
- Security or critical bugs -> [X] FIX

## Step 5: Link to Active Task

If issues were found, record in beads:

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Example: feature/ji2-beads-deep-integration -> lets-plugin-claude-ji2

# Fallback: bd list --status=in_progress
```

If multiple in-progress tasks found via fallback, skip beads comment.
If active task found AND issues detected:

```bash
bd comments add <task-id> "Quick check: {verdict}. {N} issues found."
```

If clean (no issues) - skip, don't add noise to the task.

## Output

**If GOOD or REVIEW:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets:commit  │
└────────────────────────┘
```

**If FIX:** No box. Say "Fix the issues above, then run `/lets:check` again."

## Rules

- Respond in user's language

## What This Is NOT

- NOT a full code review (use `/lets:review`)
- NOT multi-agent (no subagents, inline review only)
- NOT saved to file (console only)

## Workflow Integration

```
Work -> /lets:check -> /lets:commit -> Push -> PR -> /lets:review
         ^                                            |
    Quick inline check                         Full PR review
    5 perspectives                             up to 11 agents
```

## Notes

- Inline review - no subagent overhead, fast feedback
- Focus on actionable issues only - this is a helper, not a blocker
- No false positives - when in doubt, skip it
- Be direct, no hedging
- Reference specific lines from the diff
