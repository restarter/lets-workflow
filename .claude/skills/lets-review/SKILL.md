---
name: lets-review
description: Full code review with dynamic agent selection (up to 10 specialized agents). Analyzes changes first, selects relevant experts.
---

# Full Code Review

Comprehensive code review with dynamic agent selection based on change types. Up to 10 specialized agents, confidence scoring. Works with:
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
- PR URL/number → GitHub PR mode
- `--local` / `--staged` / `--last-commit` → Local mode

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

## Step 2: Gather Context

```bash
# Get CLAUDE.md files
cat CLAUDE.md 2>/dev/null | head -200
# Check for CLAUDE.md in modified directories
gh pr diff <PR> --name-only | xargs -I{} dirname {} | sort -u | xargs -I{} cat {}/CLAUDE.md 2>/dev/null

# Get PR diff
gh pr diff <PR>

# Get PR summary
gh pr view <PR> --json title,body,commits
```

## Step 3: Analyze Changes & Select Agents

**BEFORE launching agents, analyze the diff to determine which experts are needed.**

### 3.1 Detect Change Types

Scan the diff for file patterns:

| Pattern | Change Type | Relevant Agents |
|---------|-------------|-----------------|
| `Dockerfile`, `docker-compose`, `Makefile` | Infrastructure | Infrastructure, Security, Docs |
| `*.php` (Laravel/Yii) | Backend PHP | All backend agents |
| `*.ts`, `*.tsx`, `*.vue` | Frontend | Frontend, Security (XSS), Docs |
| `*.kt`, `*.swift` | Mobile | Mobile, Security, Architecture |
| `config/*`, `.env*`, `*.yml` | Configuration | Security, Infrastructure, Docs |
| `migrations/*`, `*.sql` | Database | Database, Security (injection), Architecture |
| `tests/*`, `*.test.*` | Tests | Test Quality, Architecture |
| `docs/*`, `*.md`, `CLAUDE.md` | Documentation | Docs only |
| `package.json`, `composer.json` | Dependencies | Security (CVEs), Infrastructure |

### 3.2 Agent Catalog

| Agent | When to Include | Skip If |
|-------|-----------------|---------|
| **CLAUDE.md Compliance** | ALWAYS | Never skip |
| **Bug Scanner** | Any code changes | Docs-only changes |
| **Security Review** | Code, config, deps | Docs-only, test-only |
| **Architecture & Quality** | Code changes > 50 lines | Small fixes, docs |
| **Git History Context** | Changes to stable code | New files only |
| **Documentation Sync** | Any non-trivial change | Tiny fixes |
| **Infrastructure Review** | Docker, CI, Makefile | App code only |
| **Database Review** | Migrations, queries | No DB changes |
| **Frontend Review** | JS/TS/CSS changes | Backend only |
| **Test Quality** | Test file changes | No test changes |

### 3.3 Select Agents

Based on analysis, output:

```
## Agent Selection

Changes detected:
- [x] PHP backend code (15 files)
- [x] Database migrations (2 files)
- [ ] Frontend code
- [x] Docker config (1 file)
- [ ] Documentation

Selected agents (6 of 10):
1. CLAUDE.md Compliance (always)
2. Bug Scanner (PHP code)
3. Security Review (PHP + DB)
4. Architecture & Quality (>50 lines changed)
5. Database Review (migrations detected)
6. Infrastructure Review (Docker changes)

Skipped:
- Frontend Review (no frontend changes)
- Test Quality (no test files)
- Git History (mostly new files)
- Documentation Sync (will check inline)
```

**RULE: Default is INCLUDE. Only skip if clearly irrelevant.**

## Step 4: Launch Selected Agents (Parallel)

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

Each agent receives:
- PR diff content
- CLAUDE.md rules
- Their specific focus
- Confidence scoring rubric

---

## Agent Definitions

### Agent: CLAUDE.md Compliance

**When:** ALWAYS included

```
You are reviewing PR changes for CLAUDE.md compliance.

CLAUDE.MD RULES:
{claude_md_content}

PR DIFF:
{diff_content}

TASK:
Check if changes comply with project rules in CLAUDE.md.
Only flag violations that are EXPLICITLY mentioned in CLAUDE.md.

For each issue found, rate confidence 0-100:
- 0: False positive, doesn't violate CLAUDE.md
- 25: Might violate, but not explicitly called out
- 50: Likely violation, but minor
- 75: Clear violation of explicit rule
- 100: Definite violation with direct quote from CLAUDE.md

OUTPUT FORMAT:
## CLAUDE.md Compliance

### Issues Found
1. [Confidence: X] {description}
   - Rule violated: "{quote from CLAUDE.md}"
   - File: {path}:{line}
   - Fix: {suggestion}

### Compliant
- {what's done correctly}
```

### Agent: Bug Scanner

**When:** Any code changes (not docs-only)

```
You are scanning PR changes for bugs.

PR DIFF:
{diff_content}

TASK:
Shallow scan for obvious bugs. Focus on:
- Logic errors
- Null/undefined handling
- Off-by-one errors
- Race conditions
- Resource leaks
- Type mismatches

Ignore: style issues, missing tests, linter catches.

For each issue, rate confidence 0-100:
- 0: Not a bug, false positive
- 25: Might be a bug, unclear
- 50: Probably a bug, but edge case
- 75: Very likely a bug that will occur in practice
- 100: Definite bug, will break functionality

OUTPUT FORMAT:
## Bug Scan

### Issues Found
1. [Confidence: X] {description}
   - Type: {logic/null/race/etc}
   - File: {path}:{line}
   - Impact: {what breaks}
   - Fix: {suggestion}

### No Issues
- Scanned X files, no obvious bugs found
```

### Agent: Git History Context

**When:** Changes to existing stable code (not new files only)

```
You are analyzing PR changes in context of git history.

PR DIFF:
{diff_content}

TASK:
Use git blame and history to understand context:

```bash
# For each modified file
git blame <file> | grep -A2 -B2 <modified_lines>
git log --oneline -5 -- <file>
```

Look for:
- Reverted changes being re-introduced
- Breaking changes to stable code
- Conflicts with recent commits
- Patterns from past bug fixes being ignored

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Git History Analysis

### Issues Found
1. [Confidence: X] {description}
   - Historical context: {what git history shows}
   - File: {path}:{line}
   - Concern: {why this is problematic}

### Context Notes
- {relevant historical observations}
```

### Agent: Security Review

**When:** Code, config, or dependency changes (not docs-only, not test-only)

```
You are a Security Engineer reviewing PR for vulnerabilities.

PR DIFF:
{diff_content}

TASK:
Check for security issues:
- OWASP Top 10 (injection, XSS, CSRF, etc.)
- Input validation
- Authentication/authorization flaws
- Secrets/credentials exposure
- Insecure deserialization
- SQL injection
- Path traversal

For each issue, rate confidence 0-100:
- 0: Not a security issue
- 25: Theoretical concern, unlikely exploitable
- 50: Possible vulnerability, needs context
- 75: Likely exploitable vulnerability
- 100: Critical vulnerability, definitely exploitable

OUTPUT FORMAT:
## Security Review

### Issues Found
1. [Confidence: X] {vulnerability type}
   - File: {path}:{line}
   - Risk: {exploitation scenario}
   - OWASP: {category if applicable}
   - Fix: {remediation}

### Secure Patterns
- {good security practices observed}
```

### Agent: Architecture & Code Quality

**When:** Code changes > 50 lines (skip for small fixes, docs)

```
You are reviewing PR for architecture and code quality.

CLAUDE.MD RULES:
{claude_md_content}

PR DIFF:
{diff_content}

TASK:
Check for:
- SOLID principle violations
- Design pattern misuse
- High coupling, low cohesion
- Code duplication (DRY)
- Naming conventions
- Function complexity
- Missing error handling

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Architecture & Quality

### Issues Found
1. [Confidence: X] {description}
   - Category: {SOLID/DRY/complexity/etc}
   - File: {path}:{line}
   - Impact: {maintainability concern}
   - Fix: {suggestion}

### Good Practices
- {positive observations}
```

### Agent: Documentation Sync

**When:** Any non-trivial change (skip for tiny fixes)

```
You are a Documentation Specialist ensuring all docs stay in sync with code changes.

CHANGED FILES:
{list of modified files}

PR DIFF:
{diff_content}

TASK:
Check documentation completeness and accuracy:

1. **CLAUDE.md sync:**
   - If code architecture changed, is CLAUDE.md updated?
   - Are new features/tools documented in Quick Reference?
   - Are removed features cleaned up from docs?

2. **docs/ folder sync:**
   - If Docker/infra changed, is docs/local-setup.md updated?
   - If new commands added, are they documented?
   - Are removed features cleaned from all doc files?

3. **Beads task tracking:**
   - Is there an active task for this work?
   - Should task description be updated with new info?
   - Are there TODO comments that should become beads tasks?

4. **README/config docs:**
   - If .env variables changed, is .env.example updated?
   - If Makefile changed, are command docs updated?
   - Are there orphaned references to removed features?

For each issue, rate confidence 0-100:
- 0: Documentation is complete and accurate
- 25: Minor doc improvement possible but not required
- 50: Documentation exists but is incomplete
- 75: Documentation is outdated or misleading
- 100: Critical documentation missing or wrong

OUTPUT FORMAT:
## Documentation Sync

### Issues Found
1. [Confidence: X] {description}
   - Category: {CLAUDE.md/docs/beads/config}
   - File: {path}
   - What's missing: {specific content needed}
   - Fix: {suggestion}

### Documentation Status
- CLAUDE.md: {in sync / needs update}
- docs/: {in sync / needs update}
- Beads: {tracked / not tracked}
- Config docs: {in sync / needs update}
```

### Agent: Infrastructure Review

**When:** Dockerfile, docker-compose, Makefile, CI/CD, nginx, shell scripts

```
You are a DevOps Engineer reviewing infrastructure changes.

PR DIFF:
{diff_content}

TASK:
Review infrastructure for:
- Dockerfile best practices (layer caching, multi-stage, security)
- docker-compose correctness (volumes, networks, depends_on)
- Makefile target organization and documentation
- CI/CD pipeline correctness
- Shell script safety (shellcheck issues)
- Environment variable handling
- Port conflicts and networking
- Resource limits and health checks

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Infrastructure Review

### Issues Found
1. [Confidence: X] {description}
   - Category: {Docker/CI/Makefile/Scripts}
   - File: {path}:{line}
   - Impact: {what breaks}
   - Fix: {suggestion}

### Good Practices
- {positive observations}
```

### Agent: Database Review

**When:** Migrations, SQL files, ORM model changes, query builders

```
You are a Database Engineer reviewing schema and query changes.

PR DIFF:
{diff_content}

TASK:
Review database changes for:
- Migration safety (reversible, no data loss)
- Index usage and missing indexes
- N+1 query patterns
- Transaction handling
- Foreign key constraints
- Data type appropriateness
- Query performance (EXPLAIN concerns)
- Naming conventions

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Database Review

### Issues Found
1. [Confidence: X] {description}
   - Category: {Migration/Query/Schema/Index}
   - File: {path}:{line}
   - Impact: {performance/data integrity concern}
   - Fix: {suggestion}

### Schema Changes
- {summary of migrations}
```

### Agent: Frontend Review

**When:** JS/TS/TSX/Vue/CSS changes, package.json frontend deps

```
You are a Frontend Engineer reviewing UI and client-side code.

PR DIFF:
{diff_content}

TASK:
Review frontend code for:
- React/Vue component patterns
- State management correctness
- Accessibility (a11y) issues
- Performance (bundle size, re-renders)
- CSS/styling issues
- TypeScript type safety
- Browser compatibility
- XSS vulnerabilities in DOM manipulation

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Frontend Review

### Issues Found
1. [Confidence: X] {description}
   - Category: {Component/State/a11y/Performance/Security}
   - File: {path}:{line}
   - Impact: {UX/performance concern}
   - Fix: {suggestion}

### Good Patterns
- {positive observations}
```

### Agent: Test Quality Review

**When:** Test file changes (*.test.*, *.spec.*, tests/*)

```
You are a QA Engineer reviewing test code quality.

PR DIFF:
{diff_content}

TASK:
Review tests for:
- Test coverage gaps
- Assertion quality (not just "doesn't throw")
- Test isolation (no shared state leaks)
- Mocking correctness
- Edge case coverage
- Test naming clarity
- Flaky test patterns
- Setup/teardown correctness

For each issue, rate confidence 0-100.

OUTPUT FORMAT:
## Test Quality Review

### Issues Found
1. [Confidence: X] {description}
   - Category: {Coverage/Assertions/Isolation/Mocking}
   - File: {path}:{line}
   - Impact: {test reliability concern}
   - Fix: {suggestion}

### Test Coverage
- New tests added: {count}
- Coverage assessment: {adequate/needs more}
```

---

## Step 5: Filter & Aggregate Results

Wait for all agents, then:

1. **Filter:** Keep only issues with confidence >= 80
2. **Dedupe:** Remove duplicate issues found by multiple agents
3. **Prioritize:** Sort by confidence (highest first)
4. **Count:** Tally issues by category

## Step 6: Determine Verdict

| Condition | Verdict |
|-----------|---------|
| 0 critical security, <= 2 total issues | ✅ APPROVED |
| 1-3 issues, no critical | ⚠️ APPROVED WITH SUGGESTIONS |
| > 3 issues OR any critical security | ❌ CHANGES REQUESTED |

## Step 7: Save Review (BEFORE output)

**CRITICAL: Save first, then show results.**

```bash
mkdir -p .claude/sessions/reviews
```

Save to:
- PR mode: `.claude/sessions/reviews/{date}-PR-{number}.md`
- Local mode: `.claude/sessions/reviews/{date}-local-review.md`

Content: Full review report with all issues, verdict, and summary.

## Step 8: Output Results

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
- CLAUDE.md Compliance: {pass/N issues}
- Bug Scan: {pass/N issues}
- Git History: {pass/N issues}
- Security: {pass/N issues}
- Architecture: {pass/N issues}
- Documentation Sync: {pass/N issues}

Generated with Claude Code
EOF
)"
```

**Link format:** `https://github.com/owner/repo/blob/{full-sha}/path/file.ext#L10-L15`

### For Local Mode:

Display full report in console.

**Always end with:** `Saved to: .claude/sessions/reviews/{filename}`

## Step 9: Update Beads (if task linked)

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
Work → /lets-review --local → Fix issues → /lets-commit → Push → PR
```

### Option B: Review after PR
```
Work → /lets-commit → Push → PR → /lets-review <PR>
                                        ↓
                                   Comment on PR
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
