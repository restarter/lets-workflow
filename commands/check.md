---
description: Quick sanity check - code (inline 5-perspective) or plan (--plan).
argument-hint: "[--staged|--last-commit|--plan]"
---

# Quick Local Code Check

Fast inline sanity check of local changes from 5 perspectives.

## Usage

```bash
/lets:check              # uncommitted changes (default)
/lets:check --staged     # only staged changes
/lets:check --last-commit # last commit
/lets:check --plan       # quick plan sanity check
```

## When to Use

- Quick sanity check during development
- Before commit for significant changes
- When unsure if code is ready
- Spot check after refactoring

**For full review:** Use `/lets:review` (local or PR, up to 11 agents).

## Plan Mode (--plan)

If `--plan` flag detected, switch to plan review mode. Skip all code review steps below.

```bash
# ROOT = project-root from LETS Config
# Find latest plan
PLAN=$(ls -t "$ROOT/.lets/plans/"*.md 2>/dev/null | head -1)
# Or specific path: /lets:check --plan path/to/plan.md
```

If no plan found: "No plans found in `.lets/plans/`. Run `/lets:brainstorm` first."

Read the plan and review with 5 lenses (same confidence filter):

- **[Feasibility]** Can this be implemented as described? Missing steps, impossible constraints?
- **[Completeness]** Are all requirements covered? Edge cases? Error handling?
- **[Risk]** What could go wrong? Dependencies, breaking changes, migration risks?
- **[Scope]** Is the plan proportional to the problem? Overengineered? Underspecified?
- **[Clarity]** Can a developer follow this without guessing? Ambiguous steps?

Output same format as code check, then:

```
┌─ LETS ─────────────────────────┐
│  Full review?  /lets:review --plan  │
│  Execute?      /lets:execute        │
└─────────────────────────────────────┘
```

---

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

### Severity Filter

Classify each finding:
- **[BLOCKER]** - Bug, security issue, or rule violation - always report
- **[SUGGESTION]** - Concern experienced developers would flag - report
- **[NIT]** - Minor improvement - skip in quick check

**Only report [BLOCKER] and [SUGGESTION]. Max 5 issues.**

## Step 4: Present Results

### Output Format

```
## Quick Check: {N files changed}

### Verdict: {[OK] GOOD | [!] REVIEW | [X] FIX}

### Issues
{Only if found}
- [{TIER}][Tag] **file:line** {issue} - {fix suggestion}

### Looks Good
{1-2 positive notes}
```

### Verdict Logic

- No issues -> [OK] GOOD
- Minor issues only -> [!] REVIEW
- Security or critical bugs -> [X] FIX

## Step 5: Link to Active Task

If issues were found, record in beads:

Use the **detect-task** skill to find the active task (read `skills/detect-task/SKILL.md` and follow its detection flow).
If multiple tasks found, skip beads comment.
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
