---
description: Full code review with dynamic agent selection (up to 12 specialized agents). Analyzes changes first, selects relevant experts. Also reviews implementation plans.
argument-hint: "[PR-url-or-number|--local|--staged|--last-commit|--plan|--file <path>] [--json]"
---

# Full Code Review

Comprehensive code review with dynamic agent selection based on change types. Up to 12 specialized agents, tiered severity scoring. Works with:
- GitHub PRs (posts comment to PR)
- Local changes (saves to file)
- Implementation plans (reviews `.lets/plans/` files)

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```bash
/lets:review                     # Interactive - asks what to review
/lets:review <PR-url-or-number>  # GitHub PR
/lets:review --local             # Uncommitted changes
/lets:review --staged            # Staged changes only
/lets:review --last-commit       # Last commit
/lets:review --plan              # Review latest plan in .lets/plans/
/lets:review --plan <path>       # Review specific plan file
/lets:review --file <path>       # Review an existing file (full content, not diff)
```

## Step 1: Determine Review Mode

**If argument provided:**
- PR URL/number -> GitHub PR mode
- `--local` / `--staged` / `--last-commit` -> Local mode
- `--plan` / `--plan <path>` -> **Plan review mode** (skip to Plan Review section below)
- `--file <path>` -> **File review mode** - reviews entire file content, not a diff

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What are we reviewing?",
    header: "Target",
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

### For File Review (--file):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/{path}"
```

Read the entire file. Use file content as "diff" for agents. No git diff needed - this reviews existing code, not changes.

If file not found, inform user and exit.

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
| `lets:docs` | ALWAYS | Never skip |
| `lets:devops` | Docker, CI, Makefile, scripts | App code only |
| `lets:database` | Migrations, queries, ORM | No DB changes |
| `lets:frontend` | JS/TS/CSS changes | Backend only |
| `lets:qa` | Test file changes | No test changes |
| `lets:pragmatist` | Large changes (> 200 lines) | Small changes |
| `lets:actor` | Explicit user request only | Always (never auto-selected) |

**Actor note:** Actor is never auto-selected. When user explicitly requests it, use the **actor-fetch-personality** skill (read `${CLAUDE_PLUGIN_ROOT}/skills/actor-fetch-personality/SKILL.md`) to fetch personality. Pass `PERSONALITY:` block in the actor's Task prompt only.

### 4.2.1 File Mode Adjustments

When reviewing a single file (`--file` mode), adjust agent selection:

- **Skip git-historian** - no diff context, git blame adds noise for full-file review
- **Skip systemic pattern check instruction** - not comparing against a diff baseline, remove SYSTEMIC PATTERN CHECK from agent prompts
- **Adjust pragmatist threshold** - include for files >100 lines (not ">200 lines changed")
- **Display header** - show `Reviewing: {filename} ({N} lines)` instead of `Changes detected:`

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

Selected agents (7 of 12):
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

> Subagents have a separate rate limit - no cost to your conversation.
```

**For `--file` mode**, adjust the display:

```
## Agent Selection

Reviewing: `{file path}` ({N} lines, {extension} file)

Selected agents ({M}):
1. compliance (always)
2. backend (PHP file)
3. security (source code)
4. architect (>50 lines)

Skipped for file mode:
- git-historian (no diff context)

> Subagents have a separate rate limit - no cost to your conversation.
```

**RULE: Default is INCLUDE. Only skip if clearly irrelevant.**

## Step 5: Launch Selected Agents (Parallel)

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent, use the Task tool with:

- **subagent_type**: The agent identifier from the catalog (e.g., `lets:architect`)
- **prompt**: Provide review context (see below)
- Agents use their own model from frontmatter (opus for architect/security/backend, session model for others)

### Task Prompt Template

Each agent receives this context in their task prompt. Agents define their own expertise, tiered scoring, output format, and mode-specific behavior in `agents/*.md` - do NOT duplicate those in the prompt.

```
ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: review

Review the following code from your expert perspective.

SYSTEMIC PATTERN CHECK:
For each finding, grep the codebase to check if the same pattern exists in other files.
If the pattern is used in 2+ other places - it's a systemic issue, not a PR bug.
Still report it, but:
- Prefix with [SYSTEMIC]
- Note how many other files follow the same pattern
- Frame as "project-wide tech debt" not "bug in this PR"
- Downgrade tier by one level (e.g. [SUGGESTION] becomes [NIT])

CLAUDE.MD RULES:
{claude_md_content}

CHANGED FILES:
{list of modified files with stats, or single file path for --file mode}

CODE:
{diff_content, or full file content for --file mode}

```

## Step 6: Filter & Aggregate Results

Wait for all agents, then:

1. **Filter:** Keep [BLOCKER] and [SUGGESTION]. Include [NIT] only for small diffs (<50 lines)
2. **Dedupe:** Remove duplicate issues found by multiple agents. When deduplicating, keep the highest tier (BLOCKER > SUGGESTION > NIT)
3. **Separate:** Split into regular findings and `[SYSTEMIC]` findings
4. **Prioritize:** Sort by tier ([BLOCKER] first, then [SUGGESTION], then [NIT])
5. **Count:** Tally issues by category

## Step 6.5: Verify Systemic Findings

For each `[SYSTEMIC]` finding from agents, quick-verify with grep:

```bash
# Example: agent flagged delete() without auth check
grep -r "delete(" --include="*.php" -l | head -10
```

- If confirmed systemic (2+ other files) - keep as systemic, note count
- If agent was wrong (only this file does it) - reclassify as regular finding, restore original tier

Systemic findings go into a separate section in the final report (see Step 9).

## Step 7: Determine Verdict

**Note:** Systemic findings do NOT count toward the verdict - they're informational.

| Condition | Verdict |
|-----------|---------|
| 0 [BLOCKER]s, 0-2 [SUGGESTION]s | APPROVED |
| 0 [BLOCKER]s, 3+ [SUGGESTION]s | APPROVED WITH SUGGESTIONS |
| 1+ [BLOCKER]s | CHANGES REQUESTED |

## Step 8: Save Review (BEFORE output)

**CRITICAL: Save first, then show results.**

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/reviews"
```

Save to:
- PR mode: `.lets/reviews/{date}-PR-{number}.md`
- Local mode: `.lets/reviews/{date}-local-review.md`

Content: Full review report with all issues, verdict, and summary.

## Step 8.5: JSON Output

If `--json` flag was provided, save structured JSON and skip Steps 9-10.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/reviews"
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
      "tier": "BLOCKER",
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

Found {N} issues:

1. **[{TIER}] {issue title}**

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

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If multiple tasks found via fallback, skip beads comment.
If active task found:

```bash
bd comments add <task-id> "Code review ({PR #X | local}): {verdict}. {N} issues found."
```

---

## Plan Review

**This section runs when `--plan` is detected. Skips all code review steps above.**

### P1: Load Plan

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)

# If path provided: use it
# If no path: derive from branch name
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}
cat "$LETS_PROJECT_ROOT/.lets/plans/${SLUG}.md" 2>/dev/null

# Fallback: glob match by task-id
ls "$LETS_PROJECT_ROOT/.lets/plans/"*{task-id}* 2>/dev/null
```

If no plan files found, inform user and exit:
> "No plans found in `.lets/plans/`. Run `/lets:plan` first."

Read the plan file and show title + task ID to user.

### P2: Gather Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/CLAUDE.md" 2>/dev/null | head -200
```

Read the codebase files referenced in the plan's "Files" sections (Create/Modify targets) to verify paths exist and understand current state.

### P3: Analyze Plan & Select Agents

Scan the plan content for domain signals:

| Signal in Plan | Detected When | Select Agent |
|----------------|--------------|-------------|
| Backend/API | Mentions controllers, endpoints, services, business logic, PHP/Python/Go/Java | backend |
| Security | Mentions auth, tokens, encryption, secrets, validation, permissions | security |
| Database | Mentions migrations, schema, queries, ORM, indexes, tables | database |
| Frontend | Mentions components, CSS, JS/TS, UI, templates, views, React/Vue | frontend |
| Infrastructure | Mentions Docker, CI/CD, deploy, nginx, scripts, Makefile | devops |
| Testing | Mentions test files, coverage, assertions, TDD, test strategy | qa |

**Always include:** architect (plan structure) + pragmatist (feasibility)
**Add per signal:** each detected domain signal adds its agent
**Never include for plan review:** compliance, git-historian, docs, explorer, implementer, actor (unless user requests)

Show selection before launching:

```
## Agent Selection

Plan signals detected:
- [x] Backend (API endpoints in tasks 2, 4)
- [x] Database (migration in task 3)
- [ ] Frontend
- [ ] Security
...

Selected agents ({N}):
1. architect (always for plan review)
2. pragmatist (always for plan review)
3. backend (API signals detected)
4. database (migration signals detected)

> Subagents have a separate rate limit - no cost to your conversation.
```

### P4: Launch Plan Review Agents (Parallel)

**CRITICAL: Launch ALL selected agents in a SINGLE message.**

#### Architect (always included)

```
Task(
  subagent_type="lets:architect",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

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

#### Pragmatist (always included)

```
Task(
  subagent_type="lets:pragmatist",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

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

#### Domain Experts (dynamically selected in P3)

For each additional agent selected in P3:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

PLAN REVIEW MODE. Review this implementation plan from your domain expertise.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Are the plan's assumptions correct in your domain?
- Are there risks or gaps the plan misses from your perspective?
- Are the proposed solutions following best practices in your area?
- Are there simpler alternatives for tasks in your domain?
- Grep the codebase to verify plan claims about existing code.

OUTPUT FORMAT:
## Plan Review: {Your Domain}

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Domain-Specific Risks
{risks from your area of expertise}

### Strengths
{1-2 things done well}"
)
```

### P5: Aggregate & Output

After all agents respond:

```
## Plan Review: **{plan title}** (`{task-id}`)

**Verdict:** {APPROVED if all approve | NEEDS REVISION if any agent flags revision}
**Agents:** {N} ({agent names})

### Architecture
{architect findings summary}

### Pragmatism
{pragmatist findings summary}

{for each additional domain agent that reported findings}
### {Agent Domain}
{agent findings summary}
{end for}

{if NEEDS REVISION}
### Action Items
1. {specific fix}
2. {specific fix}
...
{end if}

Saved to: .lets/reviews/{date}-plan-review.md
```

### P6: Save & Link

Save to `.lets/reviews/{date}-plan-review.md`

If active task found:
```bash
bd comments add <task-id> "Plan review: {verdict}. {N} issues found."
```

### Plan Review Output

**If approved:**
```
┌─ LETS ───────────────────────┐
│  Execute?      /lets:execute │
│  New session?  /lets:start   │
└──────────────────────────────┘
```

**If needs revision:** No box. List action items to fix in the plan first.

---

## Notes

- Use `gh` CLI for GitHub operations
- Always use full git SHA in links
- Filter by tier: keep [BLOCKER] and [SUGGESTION], skip [NIT] for large diffs
- For PR: post comment even if no issues (confirms review happened)
- For local: show full report in console + save to file
- Agents define their own expertise, tiered scoring, output format, and mode behavior in agents/*.md
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
┌─ LETS ───────────────────────┐
│  Commit?  /lets:commit       │
│  Check?   /lets:check        │
└──────────────────────────────┘
```

**If changes requested:** No box. List issues to fix first.
