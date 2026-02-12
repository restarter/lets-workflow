---
name: lets-review
description: Full code review with dynamic agent selection (up to 10 specialized agents). Analyzes changes first, selects relevant experts from .claude/agents/.
---

# Full Code Review

Comprehensive code review using expert agents from `.claude/agents/`. Analyzes changes, selects relevant experts, launches them in parallel. Confidence-based filtering (>= 80).

Works with:
- GitHub PRs (posts comment to PR)
- Local changes (saves to file)

## Usage

```bash
/lets-review                     # Interactive - asks what to review
/lets-review <PR-url-or-number>  # GitHub PR
/lets-review --local             # Uncommitted changes
/lets-review --staged            # Staged changes only
/lets-review --last-commit       # Last commit
```

## Step 1: Determine Review Mode

**If argument provided:**
- PR URL/number - GitHub PR mode
- `--local` / `--staged` / `--last-commit` - Local mode

**If no argument, ASK user:**

```
What are we reviewing?

1. GitHub PR (enter PR number or URL)
2. Local uncommitted changes
3. Staged changes only
4. Last commit

Choice:
```

Wait for user response before proceeding.

## Step 2: Get Changes

### For GitHub PR:

```bash
gh pr view <PR> --json state,isDraft,author,title,body,additions,deletions,changedFiles
```

**Skip if:** PR is closed, draft, trivial, or already reviewed.

```bash
gh pr diff <PR>
```

### For Local Changes:

```bash
# Uncommitted
git diff

# Staged only
git diff --staged

# Last commit
git diff HEAD~1
```

If no changes found, inform user and exit.

## Step 3: Gather Context

```bash
ROOT=$(git rev-parse --show-toplevel)
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -200
```

## Step 4: Analyze Changes & Select Agents

**BEFORE launching agents, analyze the diff to determine which experts are needed.**

### 4.1 Detect Change Types

Scan the diff for file patterns:

| Pattern | Change Type | Agents |
|---------|-------------|--------|
| `Dockerfile`, `docker-compose`, `Makefile` | Infrastructure | devops-expert, security-expert, docs-expert |
| `*.php` (Laravel/Yii) | Backend PHP | backend-expert, security-expert, architect |
| `*.py`, `*.go`, `*.java`, `*.kt` | Backend | backend-expert, security-expert, architect |
| `*.ts`, `*.tsx`, `*.vue`, `*.jsx` | Frontend | frontend-expert, security-expert |
| `*.swift` | Mobile | backend-expert, security-expert, architect |
| `config/*`, `.env*`, `*.yml` | Configuration | security-expert, devops-expert |
| `migrations/*`, `*.sql` | Database | database-expert, security-expert, architect |
| `tests/*`, `*.test.*`, `*.spec.*` | Tests | qa-expert |
| `docs/*`, `*.md`, `CLAUDE.md` | Documentation | docs-expert |
| `package.json`, `composer.json` | Dependencies | security-expert, devops-expert |

### 4.2 Agent Selection Rules

| Agent | When to Include | Skip If |
|-------|-----------------|---------|
| **compliance-expert** | ALWAYS | Never skip |
| **backend-expert** | Any backend code changes | Docs-only, config-only |
| **security-expert** | Code, config, or deps | Docs-only, test-only |
| **architect** | Code changes > 50 lines | Small fixes, docs |
| **git-historian** | Changes to existing code | New files only |
| **docs-expert** | Any non-trivial change | Tiny fixes |
| **devops-expert** | Docker, CI, Makefile, scripts | App code only |
| **database-expert** | Migrations, queries, schemas | No DB changes |
| **frontend-expert** | JS/TS/CSS/Vue changes | Backend only |
| **qa-expert** | Test file changes | No test changes |
| **pragmatist** | Changes > 200 lines | Small changes |

### 4.3 Output Selection

```
## Agent Selection

Changes detected:
- [x] PHP backend code (15 files)
- [x] Database migrations (2 files)
- [ ] Frontend code
- [x] Docker config (1 file)
- [ ] Documentation

Selected agents (6 of 11):
1. compliance-expert (always)
2. backend-expert (PHP code)
3. security-expert (PHP + DB)
4. architect (>50 lines changed)
5. database-expert (migrations detected)
6. devops-expert (Docker changes)

Skipped:
- frontend-expert (no frontend changes)
- qa-expert (no test files)
- git-historian (mostly new files)
- docs-expert (will check inline)
- pragmatist (< 200 lines)
```

**RULE: Default is INCLUDE. Only skip if clearly irrelevant.**

## Step 5: Launch Selected Agents (Parallel)

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent, use the Task tool:

```
Task(
  subagent_type="{agent-name}",
  model="sonnet",
  prompt="REVIEW MODE. You are reviewing code changes as part of a code review.

CLAUDE.MD RULES:
{claude_md_content}

DIFF:
{diff_content}

INSTRUCTIONS:
- Review the diff from YOUR area of expertise
- Rate each finding with confidence 0-100
- Only report findings with confidence >= 80
- For each finding: what, where (file:line), why it matters, how to fix
- If no issues found in your area, say so briefly
- Be specific - reference exact lines from the diff"
)
```

**Note:** Each agent already has its expertise, confidence scoring rubric, and output format defined in its `.claude/agents/` file. The REVIEW MODE prompt adds the review context (diff, rules) on top.

## Step 6: Filter & Aggregate Results

Wait for all agents, then:

1. **Filter:** Keep only issues with confidence >= 80
2. **Dedupe:** Remove duplicate issues found by multiple agents
3. **Prioritize:** Sort by confidence (highest first)
4. **Count:** Tally issues by category

## Step 7: Determine Verdict

| Condition | Verdict |
|-----------|---------|
| 0 critical security, <= 2 total issues | APPROVED |
| 1-3 issues, no critical | APPROVED WITH SUGGESTIONS |
| > 3 issues OR any critical security | CHANGES REQUESTED |

## Step 8: Save Review (BEFORE output)

**CRITICAL: Save first, then show results.**

```bash
mkdir -p .claude/sessions/reviews
```

Save to:
- PR mode: `.claude/sessions/reviews/{date}-PR-{number}.md`
- Local mode: `.claude/sessions/reviews/{date}-local-review.md`

Content: Full review report with all issues, verdict, and summary.

## Step 9: Output Results

### For GitHub PR Mode:

Post comment to PR:

```bash
gh pr comment <PR> --body "$(cat <<'EOF'
### Code Review

**Verdict:** {APPROVED | APPROVED WITH SUGGESTIONS | CHANGES REQUESTED}

Found {N} issues (filtered from {M} with confidence >= 80):

1. **{issue title}** [Confidence: {X}]

   {file link with full SHA}

   {description and fix suggestion}

2. ...

---

**Review Summary:**
- Compliance: {pass/N issues}
- Backend: {pass/N issues}
- Security: {pass/N issues}
- Architecture: {pass/N issues}
- Database: {pass/N issues}
- Infrastructure: {pass/N issues}

Generated with Claude Code
EOF
)"
```

**Link format:** `https://github.com/owner/repo/blob/{full-sha}/path/file.ext#L10-L15`

### For Local Mode:

Display full report in console.

**Always end with:** `Saved to: .claude/sessions/reviews/{filename}`

## Step 10: Update Beads (if task linked)

If PR description, branch, or current task contains task ID:

```bash
bd comments add <task-id> "Code review ({PR #X | local}): {verdict}. {N} issues found."
```

## Notes

- Use `gh` CLI for GitHub operations
- Always use full git SHA in links
- Filter to >= 80 confidence to reduce noise
- For PR: post comment even if no issues (confirms review happened)
- For local: show full report in console + save to file

## Workflow Integration

### Option A: Review before PR (recommended for significant changes)
```
Work - /lets-review --local - Fix issues - /lets-commit - Push - PR
```

### Option B: Review after PR
```
Work - /lets-commit - Push - PR - /lets-review <PR> - Comment on PR
```

### Quick check vs Full review
```
/lets-check  = Quick sanity check (~30 sec), before any commit
/lets-review = Full deep review (~2-3 min), before PR or on PR
```

## Output

**If no issues or approved:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets-commit  │
└────────────────────────┘
```

**If changes requested:** No box. List issues to fix first.
