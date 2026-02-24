---
description: Quick code sanity check. Single agent, 5 perspectives.
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
  subagent_type="lets:quick-reviewer",
  prompt="CHECK MODE. Quick code sanity check.

PROJECT CONTEXT:
{CLAUDE.md summary}

CHANGED FILES:
{git diff --stat output}

DIFF:
{diff content}

Be concise - this is a quick check, not a full review."
)
```

## Step 4: Present Results

Show the agent's response directly.

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

## What This Is NOT

- NOT a full code review (use `/lets:review`)
- NOT confidence scoring (that's for full review)
- NOT multi-agent (uses single quick-reviewer, not a panel of lets: experts)
- NOT saved to file (console only)

## Workflow Integration

```
Work -> /lets:check -> /lets:commit -> Push -> PR -> /lets:review
         ^                                            |
    Quick check                               Full PR review
    single agent                              up to 11 agents
```

## Notes

- Uses lets:quick-reviewer agent (opus) for thorough single-pass analysis
- Agent keeps diff out of main context window
- Focus on actionable issues only - this is a helper, not a blocker
