---
description: Full code review with dynamic agent selection (up to 11 specialized agents). Analyzes changes first, selects relevant experts.
argument-hint: "[PR-url-or-number|--local|--staged|--last-commit]"
---

# Full Code Review

Comprehensive code review with dynamic agent selection based on change types. Up to 11 specialized agents, confidence scoring. Works with:
- GitHub PRs (posts comment to PR)
- Local changes (saves to file)

## Usage

```bash
/lets:review                     # Interactive - asks what to review
/lets:review <PR-url-or-number>  # GitHub PR
/lets:review --local             # Uncommitted changes
/lets:review --staged            # Staged changes only
/lets:review --last-commit       # Last commit
```

## Step 1: Determine Review Mode

**If argument provided:**
- PR URL/number -> GitHub PR mode
- `--local` / `--staged` / `--last-commit` -> Local mode

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

### For GitHub PR:

```bash
# Get CLAUDE.md files
cat CLAUDE.md 2>/dev/null | head -200

# Check for CLAUDE.md in modified directories
gh pr diff <PR> --name-only | xargs -I{} dirname {} | sort -u | xargs -I{} cat {}/CLAUDE.md 2>/dev/null

# Get PR summary
gh pr view <PR> --json title,body,commits
```

### For Local Changes:

```bash
# Get CLAUDE.md
cat CLAUDE.md 2>/dev/null | head -200

# Get changed file list
git diff --name-only  # (or --staged, HEAD~1)
```

## Step 4: Analyze Changes & Select Agents

**BEFORE launching agents, analyze the diff to determine which experts are needed.**

### 4.1 Detect Change Types

Scan the diff for file patterns:

| Pattern | Change Type | Relevant Agents |
|---------|-------------|-----------------|
| `Dockerfile`, `docker-compose`, `Makefile` | Infrastructure | devops-expert, security-expert, docs-expert |
| `*.php` (Laravel/Yii) | Backend PHP | backend-expert, security-expert, architect |
| `*.py`, `*.go`, `*.java`, `*.rb` | Backend | backend-expert, security-expert, architect |
| `*.ts`, `*.tsx`, `*.vue`, `*.jsx` | Frontend | frontend-expert, security-expert, docs-expert |
| `*.kt`, `*.swift` | Mobile | backend-expert, security-expert, architect |
| `config/*`, `.env*`, `*.yml` | Configuration | security-expert, devops-expert, docs-expert |
| `migrations/*`, `*.sql` | Database | database-expert, security-expert, architect |
| `tests/*`, `*.test.*`, `*.spec.*` | Tests | qa-expert, architect |
| `docs/*`, `*.md`, `CLAUDE.md` | Documentation | docs-expert only |
| `package.json`, `composer.json` | Dependencies | security-expert, devops-expert |

### 4.2 Agent Catalog

| Agent (subagent_type) | When to Include | Skip If |
|----------------------|-----------------|---------|
| `lets:compliance-expert` | ALWAYS | Never skip |
| `lets:backend-expert` | Any code changes | Docs-only changes |
| `lets:security-expert` | Code, config, deps | Docs-only, test-only |
| `lets:architect` | Code changes > 50 lines | Small fixes, docs |
| `lets:git-historian` | Changes to existing code | New files only |
| `lets:docs-expert` | Any non-trivial change | Tiny fixes |
| `lets:devops-expert` | Docker, CI, Makefile, scripts | App code only |
| `lets:database-expert` | Migrations, queries, ORM | No DB changes |
| `lets:frontend-expert` | JS/TS/CSS changes | Backend only |
| `lets:qa-expert` | Test file changes | No test changes |
| `lets:pragmatist` | Large changes (> 200 lines) | Small changes |

### 4.3 Select Agents

Based on analysis, output:

```
## Agent Selection

Changes detected:
- [x] PHP backend code (15 files)
- [x] Database migrations (2 files)
- [ ] Frontend code
- [x] Docker config (1 file)
- [ ] Documentation

Selected agents (7 of 11):
1. compliance-expert (always)
2. backend-expert (PHP code + bug scanning)
3. security-expert (PHP + DB + Docker)
4. architect (>50 lines changed)
5. database-expert (migrations detected)
6. devops-expert (Docker changes)
7. pragmatist (>200 lines total)

Skipped:
- frontend-expert (no frontend changes)
- qa-expert (no test files)
- git-historian (mostly new files)
- docs-expert (will check inline)
```

**RULE: Default is INCLUDE. Only skip if clearly irrelevant.**

## Step 5: Launch Selected Agents (Parallel)

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent, use the Task tool with:

- **subagent_type**: The agent identifier from the catalog (e.g., `lets:architect`)
- **prompt**: Provide review context (see below)
- Agents use their own model from frontmatter (opus for architect/security/backend, session model for others)

### Task Prompt Template

Each agent receives this context in their task prompt. Agents define their own expertise, confidence scoring, and output format in `agents/*.md` - do NOT duplicate those in the prompt.

```
Review the following code changes from your expert perspective.
Use your confidence scoring system. Only report findings >= 80 confidence.
Follow your output format as defined in your system prompt.

CLAUDE.MD RULES:
{claude_md_content}

CHANGED FILES:
{list of modified files with stats}

DIFF:
{diff_content}

{mandatory context - see table below}
```

### Mandatory Agent Context

These instructions are **required** for agents that need project-specific context to function correctly. Always include them.

| Agent | Why | Instruction |
|-------|-----|-------------|
| `lets:compliance-expert` | Needs rules to check against | "Only flag violations EXPLICITLY mentioned in CLAUDE.md. Quote the rule being violated." |
| `lets:git-historian` | Needs to access project history | "Use git blame and git log to check historical context of modified files." |
| `lets:docs-expert` | Needs to know what docs exist | "Check CLAUDE.md sync, docs/ sync, beads tracking, README/config docs." |
| `lets:pragmatist` | Specific review lens | "Assess if the solution is proportional to the problem. Flag overengineering." |

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
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$ROOT/.lets/reviews"
```

Save to:
- PR mode: `.lets/reviews/{date}-PR-{number}.md`
- Local mode: `.lets/reviews/{date}-local-review.md`

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
- Backend/Bugs: {pass/N issues}
- Security: {pass/N issues}
- Architecture: {pass/N issues}
- Git History: {pass/N issues}
- Documentation: {pass/N issues}
- Infrastructure: {pass/N issues}
- Database: {pass/N issues}
- Frontend: {pass/N issues}
- Tests: {pass/N issues}
- Pragmatism: {pass/N issues}

Generated with Claude Code
EOF
)"
```

**Link format:** `https://github.com/owner/repo/blob/{full-sha}/path/file.ext#L10-L15`

### For Local Mode:

Display full report in console.

**Always end with:** `Saved to: .lets/reviews/{filename}`

## Step 10: Link Review to Active Task

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Example: feature/ji2-beads-deep-integration -> lets-plugin-claude-ji2

# Fallback: bd list --status=in_progress
```

If multiple in-progress tasks found via fallback, skip beads comment.
If active task found:

```bash
bd comments add <task-id> "Code review ({PR #X | local}): {verdict}. {N} issues found."
```

## Notes

- Use `gh` CLI for GitHub operations
- Always use full git SHA in links
- Filter to >= 80 confidence to reduce noise
- For PR: post comment even if no issues (confirms review happened)
- For local: show full report in console + save to file
- Agents define their own expertise, scoring, and output format in agents/*.md
- The review command only provides context (diff, CLAUDE.md) and orchestrates

## Workflow Integration

### Option A: Review before PR (recommended for significant changes)
```
Work -> /lets:review --local -> Fix issues -> /lets:commit -> Push -> PR
```

### Option B: Review after PR
```
Work -> /lets:commit -> Push -> PR -> /lets:review <PR>
                                        |
                                   Comment on PR
```

### Quick check vs Full review
```
/lets:check  = Quick sanity check (~30 sec), before any commit
/lets:review = Full deep review (~2-3 min), before PR or on PR
```

## Output

**If no issues or approved:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets:commit  │
└────────────────────────┘
```

**If changes requested:** No box. List issues to fix first.
