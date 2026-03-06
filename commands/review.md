---
description: Full code review with dynamic agent selection (up to 11 specialized agents). Analyzes changes first, selects relevant experts. Also reviews implementation plans.
argument-hint: "[PR-url-or-number|--local|--staged|--last-commit|--plan] [--json]"
---

# Full Code Review

Comprehensive code review with dynamic agent selection based on change types. Up to 11 specialized agents, confidence scoring. Works with:
- GitHub PRs (posts comment to PR)
- Local changes (saves to file)
- Implementation plans (reviews `.lets/plans/` files)

## Usage

```bash
/lets:review                     # Interactive - asks what to review
/lets:review <PR-url-or-number>  # GitHub PR
/lets:review --local             # Uncommitted changes
/lets:review --staged            # Staged changes only
/lets:review --last-commit       # Last commit
/lets:review --plan              # Review latest plan in .lets/plans/
/lets:review --plan <path>       # Review specific plan file
```

## Step 1: Determine Review Mode

**If argument provided:**
- PR URL/number -> GitHub PR mode
- `--local` / `--staged` / `--last-commit` -> Local mode
- `--plan` / `--plan <path>` -> **Plan review mode** (skip to Plan Review section below)

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What are we reviewing?",
    header: "LETS",
    options: [
      { label: "Local changes", description: "Uncommitted changes in working tree" },
      { label: "Staged", description: "Only staged changes (git diff --staged)" },
      { label: "Last commit", description: "Review the most recent commit" },
      { label: "Plan", description: "Review implementation plan from .lets/plans/" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Local changes** -> local mode with `git diff`
- **Staged** -> local mode with `git diff --staged`
- **Last commit** -> local mode with `git diff HEAD~1`
- **Plan** -> skip to Plan Review section
- **Other** (free text) -> treat as PR number or URL, use GitHub PR mode

**If plan mode selected:** skip to **Plan Review** section below.

### JSON output flag

If `--json` is present alongside any mode:
- Save review output as structured JSON instead of markdown
- File: `.lets/reviews/{date}-{mode}.json` (e.g., `2026-02-26-PR-42.json`, `2026-02-26-local-review.json`)
- Skip markdown report generation (Step 8)
- Skip GitHub PR comment posting (Step 9) - JSON mode implies the caller handles output

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
| `Dockerfile`, `docker-compose`, `Makefile` | Infrastructure | devops, security, docs |
| `*.php` (Laravel/Yii) | Backend PHP | backend, security, architect |
| `*.py`, `*.go`, `*.java`, `*.rb` | Backend | backend, security, architect |
| `*.ts`, `*.tsx`, `*.vue`, `*.jsx` | Frontend | frontend, security, docs |
| `*.kt`, `*.swift` | Mobile | backend, security, architect |
| `config/*`, `.env*`, `*.yml` | Configuration | security, devops, docs |
| `migrations/*`, `*.sql` | Database | database, security, architect |
| `tests/*`, `*.test.*`, `*.spec.*` | Tests | qa, architect |
| `commands/*.md`, `agents/*.md`, `hooks/*.md` | Skill/Command | compliance, docs, pragmatist |
| `docs/*`, `*.md`, `CLAUDE.md` | Documentation | docs only |
| `package.json`, `composer.json` | Dependencies | security, devops |

### 4.2 Agent Catalog

| Agent (subagent_type) | When to Include | Skip If |
|----------------------|-----------------|---------|
| `lets:compliance` | ALWAYS | Never skip |
| `lets:backend` | Any code changes | Docs-only changes |
| `lets:security` | Code, config, deps | Docs-only, test-only |
| `lets:architect` | Code changes > 50 lines | Small fixes, docs |
| `lets:git-historian` | Changes to existing code | New files only |
| `lets:docs` | Any non-trivial change | Tiny fixes |
| `lets:devops` | Docker, CI, Makefile, scripts | App code only |
| `lets:database` | Migrations, queries, ORM | No DB changes |
| `lets:frontend` | JS/TS/CSS changes | Backend only |
| `lets:qa` | Test file changes | No test changes |
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
1. compliance (always)
2. backend (PHP code + bug scanning)
3. security (PHP + DB + Docker)
4. architect (>50 lines changed)
5. database (migrations detected)
6. devops (Docker changes)
7. pragmatist (>200 lines total)

Skipped:
- frontend (no frontend changes)
- qa (no test files)
- git-historian (mostly new files)
- docs (will check inline)
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
ultrathink

RESPONSE LANGUAGE: {language from LETS Config, e.g. "English"}
PROJECT ROOT: {project-root from LETS Config}. Do NOT read or search files outside this directory.

Review the following code changes from your expert perspective.
Use your confidence scoring system. Only report findings >= 80 confidence.
Follow your output format as defined in your system prompt.

SYSTEMIC PATTERN CHECK:
For each finding, grep the codebase to check if the same pattern exists in other files.
If the pattern is used in 2+ other places - it's a systemic issue, not a PR bug.
Still report it, but:
- Prefix with [SYSTEMIC]
- Note how many other files follow the same pattern
- Frame as "project-wide tech debt" not "bug in this PR"
- Reduce confidence by 15 points (it's not wrong here specifically)

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
| `lets:compliance` | Needs rules to check against | "Only flag violations EXPLICITLY mentioned in CLAUDE.md. Quote the rule being violated." |
| `lets:git-historian` | Needs to access project history | "Use git blame and git log to check historical context of modified files." |
| `lets:docs` | Needs to know what docs exist | "Check LETS plugin doc sync: (1) CLAUDE.md structure and architecture sections match actual files, (2) hooks/rules-context.md Skill Quick Reference table lists all commands, (3) commands/install.md skill tables are complete, (4) command frontmatter descriptions match behavior. When commands/*.md, agents/*.md, or hooks/ files changed - these docs MUST be verified." |
| `lets:pragmatist` | Specific review lens | "Assess if the solution is proportional to the problem. Flag overengineering." |

## Step 6: Filter & Aggregate Results

Wait for all agents, then:

1. **Filter:** Keep only issues with confidence >= 80
2. **Dedupe:** Remove duplicate issues found by multiple agents
3. **Separate:** Split into regular findings and `[SYSTEMIC]` findings
4. **Prioritize:** Sort by confidence (highest first)
5. **Count:** Tally issues by category

## Step 6.5: Verify Systemic Findings

For each `[SYSTEMIC]` finding from agents, quick-verify with grep:

```bash
# Example: agent flagged delete() without auth check
grep -r "delete(" --include="*.php" -l | head -10
```

- If confirmed systemic (2+ other files) - keep as systemic, note count
- If agent was wrong (only this file does it) - reclassify as regular finding, restore confidence

Systemic findings go into a separate section in the final report (see Step 9).

## Step 7: Determine Verdict

**Note:** Systemic findings do NOT count toward the verdict - they're informational.

| Condition | Verdict |
|-----------|---------|
| 0 critical security, <= 2 total issues | APPROVED |
| 1-3 issues, no critical | APPROVED WITH SUGGESTIONS |
| > 3 issues OR any critical security | CHANGES REQUESTED |

## Step 8: Save Review (BEFORE output)

**CRITICAL: Save first, then show results.**

```bash
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/reviews"
```

Save to:
- PR mode: `.lets/reviews/{date}-PR-{number}.md`
- Local mode: `.lets/reviews/{date}-local-review.md`

Content: Full review report with all issues, verdict, and summary.

## Step 8.5: JSON Output

If `--json` flag was provided, save structured JSON and skip Steps 9-10.

```bash
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/reviews"
```

Write to `.lets/reviews/{date}-{mode}.json`:

```json
{
  "date": "2026-02-26",
  "mode": "PR-42",
  "verdict": "CHANGES REQUESTED",
  "findings_count": 5,
  "findings": [
    {
      "id": 1,
      "title": "SQL injection in search query",
      "severity": "critical",
      "confidence": 95,
      "agent": "security",
      "file": "src/search.py",
      "line": 42,
      "description": "User input concatenated directly into SQL query",
      "suggestion": "Use parameterized queries"
    }
  ],
  "systemic": [
    {
      "title": "Missing input validation",
      "count": 5,
      "description": "Found in 5 files across the project"
    }
  ],
  "summary": {
    "compliance": "pass",
    "backend": "2 issues",
    "security": "1 issue",
    "architecture": "pass"
  }
}
```

After saving, inform user: "Review saved to: {path}"
Then STOP - skip Step 9 (Output) and Step 10 (Link to task).
The calling command handles output and task linking.

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

{if systemic findings exist}
### Systemic Issues

These patterns exist across the project, not just in this PR:

1. **{issue title}** - found in {N} files
   {brief description and scope}

{end if}

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
# Extract task ID from branch name (feature/<id>-<slug> or worktree-<id>-<slug>)
# Strategy: look for beads ID pattern anywhere in branch name

# Fallback: bd list --status=in_progress
```

If multiple in-progress tasks found via fallback, skip beads comment.
If active task found:

```bash
bd comments add <task-id> "Code review ({PR #X | local}): {verdict}. {N} issues found."
```

---

## Plan Review

**This section runs when `--plan` is detected. Skips all code review steps above.**

### P1: Load Plan

```bash
# ROOT = project-root from LETS Config

# If path provided: use it
# If no path: derive from branch name
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}
cat "$ROOT/.lets/plans/${SLUG}.md" 2>/dev/null

# Fallback: glob match by task-id
ls "$ROOT/.lets/plans/"*{task-id}* 2>/dev/null
```

If no plan files found, inform user and exit:
> "No plans found in `.lets/plans/`. Run `/lets:brainstorm` first."

Read the plan file and show title + task ID to user.

### P2: Gather Context

```bash
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -200
```

Read the codebase files referenced in the plan's "Files" sections (Create/Modify targets) to verify paths exist and understand current state.

### P3: Launch Plan Review Agents (Parallel)

**CRITICAL: Launch ALL agents in a SINGLE message.**

Always launch exactly 2 agents:

#### Architect

```
Task(
  subagent_type="lets:architect",
  prompt="ultrathink

PLAN REVIEW MODE. Review this implementation plan for quality and completeness.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Are all tasks 15-60 minutes of work? (not too big, not trivial)
- Are code snippets complete? (no // TODO, no // implement here, no placeholders)
- Are file paths exact and verified? (do referenced existing files actually exist?)
- Does every task have a verification step with expected output?
- Does every logical unit have a commit point?
- Are interfaces clearly defined? (function signatures, types, data flow)
- Is the task ordering logical? (dependencies respected, builds incrementally)
- Are edge cases addressed?
- Is the codebase map accurate? (do referenced patterns actually exist?)

OUTPUT FORMAT:
## Plan Review: Architecture

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Missing
{things the plan should cover but doesn't}

### Strengths
{1-2 things done well - keep feedback balanced}"
)
```

#### Pragmatist

```
Task(
  subagent_type="lets:pragmatist",
  prompt="ultrathink

PLAN REVIEW MODE. Review this implementation plan for feasibility and proportionality.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Is the overall approach proportional to the problem? (not overengineered?)
- Are there simpler alternatives for any task?
- Is the effort estimate realistic?
- Are there unnecessary abstractions or premature optimizations?
- Does the plan follow existing codebase patterns or reinvent the wheel?
- Is the scope well-defined? (clear what's in and out)
- Are there tasks that could be cut without losing core value?
- Will this plan survive contact with reality? (hidden dependencies, risky assumptions)

OUTPUT FORMAT:
## Plan Review: Pragmatism

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Scope Check
- Core value delivered: {yes/no, what}
- Unnecessary extras: {if any}
- Missing essentials: {if any}

### Bottom Line
{1-2 sentences: ship it or revise it, and why}"
)
```

### P4: Aggregate & Output

After both agents respond:

```
## Plan Review: **{plan title}** (`{task-id}`)

**Verdict:** {APPROVED | NEEDS REVISION}

### Architecture
{architect findings summary}

### Pragmatism
{pragmatist findings summary}

{if NEEDS REVISION}
### Action Items
1. {specific fix from architect}
2. {specific fix from pragmatist}
...
{end if}

Saved to: .lets/reviews/{date}-plan-review.md
```

### P5: Save & Link

Save to `.lets/reviews/{date}-plan-review.md`

If active task found:
```bash
bd comments add <task-id> "Plan review: {verdict}. {N} issues found."
```

### Plan Review Output

**If approved:**
```
┌─ LETS ──────────────────────────────────┐
│  Execute?     /lets:execute             │
│  New session? /lets:start               │
└─────────────────────────────────────────┘
```

**If needs revision:** No box. List action items to fix in the plan first.

---

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

## Rules

- Respond in user's language

## Output

**If no issues or approved:**
```
┌─ LETS ─────────────────┐
│  Commit? /lets:commit  │
└────────────────────────┘
```

**If changes requested:** No box. List issues to fix first.
