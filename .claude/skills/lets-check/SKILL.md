---
name: lets-check
description: Quick code sanity check (~30s). Single agent, 4 perspectives.
---

# Quick Local Code Check

Fast sanity check of local changes from 4 perspectives.

## Usage

```bash
/lets-check              # uncommitted changes (default)
/lets-check --staged     # only staged changes
/lets-check --last       # last commit
```

## When to Use

- Quick sanity check during development
- Before commit for significant changes
- When unsure if code is ready
- Spot check after refactoring

**For full review:** Use `/lets-review` (local or PR).

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

## Step 2: Get Context (Quick)

```bash
# Just the essentials
git diff --stat
cat CLAUDE.md 2>/dev/null | head -100
```

## Step 3: Single Agent, 4 Perspectives

**NO sub-agents.** You (Claude) review directly from 4 angles.

Review the diff and provide quick feedback:

### [Arch] Architecture (10 sec)
- SOLID violations?
- High coupling?
- Code duplication?
- Fits existing patterns?

### [Sec] Security (10 sec)
- Obvious vulnerabilities?
- Input validation?
- Secrets exposed?
- SQL injection risk?

### [Ops] DevOps (5 sec)
- Config issues?
- Error handling?
- Logging adequate?

### [Quality] Code Quality (5 sec)
- Naming clear?
- Complexity ok?
- CLAUDE.md compliance?

## Step 4: Output Format

Keep it SHORT - this is a quick check, not full review.

```markdown
## Quick Check: {N files changed}

### Verdict: {[OK] GOOD | [!] REVIEW | [X] FIX}

### Issues
{Only if found - bullet points, max 5}

- [Arch] **[file:line]** {issue} - {fix}
- [Sec] **[file:line]** {issue} - {fix}

### Looks Good
{1-2 positive notes}

---
{verdict suggestion}
```

## Verdict Logic

| Condition | Verdict | Suggestion |
|-----------|---------|------------|
| No issues | [OK] GOOD | "Looks clean" |
| Minor issues only | [!] REVIEW | "Consider fixing, or proceed" |
| Security/critical issues | [X] FIX | "Fix these issues first" |

## Example Output

```markdown
## Quick Check: 3 files changed

### Verdict: [!] REVIEW

### Issues
- [Sec] **UserController.php:45** Raw SQL with user input - Use parameterized query
- [Arch] **helpers/Utils.php:12** Duplicates logic from StringHelper - Reuse existing

### Looks Good
- Error handling in ApiService
- Clear naming conventions

---
Consider fixing security issue before proceeding.
```

## What This Is NOT

- NOT a full code review (use `/lets-review` for PRs)
- NOT confidence scoring (that's for full review)
- NOT multiple agents (single pass only)
- NOT saved to file (console only)

## Workflow Integration

```
Work → /lets-check → /lets-commit → Push → PR → /lets-review
         ↑                                           ↓
    Quick local                              Full PR review
    ~30 seconds                              ~2-3 minutes
```

## Output

End with LETS box based on verdict:

**If GOOD or REVIEW:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets-commit  │
└────────────────────────┘
```

**If FIX:** No box. Say "Fix the issues above, then run `/lets-check` again."

## Notes

- Keep response under 20 lines
- Focus on actionable issues only
- No false positives - when in doubt, skip it
- This is a helper, not a blocker
