# PR Review Lifecycle

Full cycle for reviewing pull requests - from first look to final approve.

```
Initial Review -> Post Comments -> Follow-up Review -> Approve / Re-request
      |                |                |                     |
  Find issues    Publish to BB    Check fixes         Close or repeat
```

Each PR may go through the cycle multiple times until all issues are resolved.

---

## Phase 1: Initial Review

### 1.1 Preparation

Gather context before reading any code:

```bash
# PR info + changed files
bb-api pr show <id>

# Dependency chain (which PR depends on which)
# Example: PR #20 -> PR #19 -> dev
bb-api pr show 20   # check "Branch: feature/X -> feature/Y"
```

Key questions:
- What's the PR about? (feature, bugfix, refactor)
- What's the dependency chain? Review base PRs first.
- How big is it? (number of files, lines changed)

### 1.2 Get the code locally

Always review locally - not from diffs. You need full file context.

**Default: simple checkout.** Untracked files are not affected by checkout.

```bash
cd code/pwa-admin-panel    # or wherever the repo is
git fetch origin
git checkout <branch-name>
```

**Worktree only when needed** - uncommitted changes in tracked files, or need to work in two branches simultaneously:

```bash
git worktree add /tmp/review-pr19 origin/feature/pwa-45418
# Review in /tmp/review-pr19
# Clean up after: git worktree remove /tmp/review-pr19
```

### 1.3 Understand the diff

Before running agents, understand the scope:

```bash
# What files changed vs target branch
git diff --name-status dev...HEAD

# Stats (lines added/removed per file)
git diff --stat dev...HEAD
```

Group changes mentally:
- **Models / migrations** - data structure changes
- **Controllers / requests** - business logic, validation
- **Views / frontend** - UI changes
- **Tests** - coverage
- **Config / infra** - deployment, Docker, CI

### 1.4 Run review agents

Use `/lets:review --local` for comprehensive multi-agent analysis. Agents are selected dynamically based on what files changed.

Typical agent set for a Laravel PR:
- compliance (strict_types, return types, no dd/dump)
- backend (business logic, error handling, API design)
- security (IDOR, XSS, SQL injection, mass assignment)
- architect (patterns, SOLID, coupling)
- database (migrations, indexes, queries)
- frontend (views, accessibility, Livewire)
- qa (test coverage, assertions, edge cases)

### 1.5 Consolidate findings

Agents produce noise. Filter ruthlessly:

**Keep:**
- Security issues (IDOR, XSS, injection) - always
- Logic bugs - always
- Data integrity issues (missing constraints, race conditions) - always
- Missing validation at system boundaries - always

**Skip:**
- Code style nits covered by linters (Pint handles this)
- Theoretical risks with low probability
- "Nice to have" refactoring (mention in summary, not inline)
- DRY violations unless egregious
- Accessibility in admin panels (mention in summary)

### 1.6 Create review plan

Structure the findings into a posting plan:

```markdown
## PR #19 - Review Plan

### Inline comments (4)
1. [critical] IDOR in delete() - PrelandingsTable.php:50
2. [critical] Mass assignment user_id - Prelanding.php:39
3. [important] Slug unique constraint - migration:22
4. [important] LIKE wildcards - PrelandingsTable.php:86

### Summary
Verdict: CHANGES REQUESTED
Key theme: missing authorization (systemic)
```

Rules:
- Max 5-7 inline comments per PR (more = noise)
- Critical and important only for inline
- Suggestions go in summary
- Group related issues (e.g. "IDOR in 3 places" = 1 systemic comment + 1-2 inline examples)

---

## Phase 1.5: Discuss Findings with PM

**This phase happens between consolidation and posting.** Do not skip it.

The PM (or reviewer) goes through each finding one by one:

1. **Deep dive** - look at the actual code together, explain the issue, verify it's real
2. **Classify** - PM decides: inline or summary (see decision rule below)
3. **Refine wording** - PM adjusts text, severity, tone for the team
4. **Drop or merge** - some findings turn out to be non-issues, some get combined

This is iterative and can take a full session for large PRs.

### Decision rule: inline vs summary

**Working code with a real bug → inline comment.**
The issue exists in code that's part of this PR, and can be pointed to on a specific line.

**Theoretical concern / code doesn't exist yet → summary observation.**
The issue is about what *might* happen in the future (e.g. "when CRUD is built, $fillable will be a problem"). No point cluttering the PR with inline comments about code that isn't written yet.

Examples:
- `trustHosts` will crash in production → **inline** (code exists, bug is real)
- `is_primary` in `$fillable` is a mass assignment risk → **summary** (no CRUD controller exists yet)
- Docs reference non-existent config key → **inline** (the doc file is in the PR, line is real)
- `scopePrimary()` is never used → **summary** (architectural observation, not a bug)

### Store drafts in beads

As comments are refined, save them to the beads task via `bd update <id> --notes="..."`. This way the draft text survives context window compaction and session restarts.

---

## Phase 2: Post Comments

See [pr-review-posting-workflow.md](pr-review-posting-workflow.md) for detailed posting instructions.

Key points:
- Post in dependency order (base PR first)
- Verify line numbers against actual diff before posting
- Log every posted comment to beads immediately
- User approves each comment before posting
- Language: team's language for text, English for code and severity tags
- **Task stays in_progress after posting** - do NOT close until Phase 4 (approve/reject)

---

## Phase 3: Follow-up Review

Developer pushes fixes. Time to check if our comments were addressed.

### 3.1 Identify fix commits

```bash
cd code/pwa-admin-panel
git fetch origin

# Check what's new on the branch
git log --oneline origin/<branch>
```

Look for commits like "fixes by PR's comments", "address review feedback", etc. Note the fix commit hash and the previous commit hash.

### 3.2 Get the fix delta

**This is the key step.** Don't re-read the full PR diff. Look only at what changed since our review.

```bash
# Diff between last reviewed commit and fix commit
git diff <previous-commit>..<fix-commit> --name-status
git diff <previous-commit>..<fix-commit> --stat
git diff <previous-commit>..<fix-commit>
```

Example:
```bash
git diff 856a48b0..840d516f   # PR #19: original -> fix
```

### 3.3 Review the fix delta

For each file in the fix delta:
- Read the full diff of that file
- If it's a new file (new Policy class, new middleware) - read it entirely
- Cross-reference with our original comments

**Important:** Developer may fix issues differently than we suggested:
- We said `abort_if()` inline - they created a Policy class
- We said "escape with `e()`" - they moved to Blade views with auto-escaping
- We said "add unique constraint" - they added a different constraint scope

All of these are valid. Check the *intent* was addressed, not the exact code we suggested.

### 3.4 Check each comment

Go through each of our posted comments and classify:

| Status | Meaning | Action |
|--------|---------|--------|
| **Fixed** | Issue fully resolved | Resolve the comment thread |
| **Partially fixed** | Addressed but incomplete or has new issue | Reply with what's missing |
| **Not fixed** | Developer disagreed or missed it | Reply - escalate if critical, accept if minor |
| **Fixed differently** | Different approach than suggested, but correct | Resolve + optional "nice approach" reply |
| **New issue** | Fix introduced a new problem | New inline comment |
| **Not an issue** | Re-evaluation shows the finding was wrong or irrelevant | Drop silently, don't post |

#### Verify each "not fixed" carefully

Don't just check the original file - the developer may have:
- Created a new class/file that handles the issue
- Fixed it at a different layer (controller vs middleware vs policy)
- Made it irrelevant through other changes

Use `git show origin/<branch>:<path>` to inspect specific files on the remote branch without full checkout:

```bash
# Check current state of a specific file
git show origin/feature/pwa-45418:app/Models/Pwa/Prelanding.php | grep -A10 "fillable"

# Check how a function is actually called
git show origin/feature/pwa-45483:app/Services/Pwa/Prelanding/PrelandingService.php
```

#### Re-evaluate severity in context

Initial review assigns severity theoretically. During follow-up, check **actual exploitability**:

- **Mass assignment `user_id` in $fillable** - sounds critical, but if the service always sets `user_id = auth()->id()` explicitly and uses `$request->validated()` (not `$request->all()`), the vulnerability is not exploitable. Downgrade to important (best practice).
- **LIKE wildcards not escaped** - sounds important, but in admin panel search over user's own data, `%` matching everything is harmless. Drop entirely.
- **XSS in content field** - sounds critical, but if the field is only rendered in `{{ }}` (escaped Blade) and public rendering doesn't exist yet, it's not exploitable now. Note for future, don't block merge.

#### Check the target branch

Before marking something as "not fixed in this PR", check if it was **already broken on the target branch**:

```bash
# Is abort_if already commented on dev?
git show origin/dev:app/Http/Controllers/Pwa/PwaController.php | grep "abort_if"
```

If the issue pre-existed on dev, it's not this PR's regression. A different PR may already fix it.

### 3.5 Check for regressions

The fix itself can introduce problems. Look for:
- New files without `declare(strict_types=1)`
- Removed tests that were validating something
- Incomplete refactoring (e.g. moved auth to Policy but forgot one method)
- Copy-paste from suggestion without adapting to context

### 3.6 Write follow-up report

Before posting anything, create a summary for the user:

```markdown
## Follow-up Review - PR #19 (fix commit 840d516f)

### Our comments status:
1. IDOR delete - FIXED (added user_id check in where clause)
2. Mass assignment - NOT FIXED (user_id still in $fillable)
3. Slug unique - PARTIALLY FIXED (added per-user unique, but we suggested global)
4. LIKE wildcards - NOT FIXED

### New issues found:
- None

### Verdict: Still needs changes / Ready to approve
```

### 3.7 Post follow-up

After user approves the follow-up findings:

- **Fixed comments:** resolve the thread (or reply "Fixed, thanks")
- **Not fixed:** reply to the original comment explaining why it still matters
- **New issues:** new inline comment
- **Update beads:** add follow-up status to task notes

#### Replying to existing comments

Use `bb-api raw-post` with `parent.id` to reply in existing threads:

```bash
bb-api raw-post "/pullrequests/<PR_ID>/comments" '{
  "parent": {"id": <COMMENT_ID>},
  "content": {"raw": "Reply text in markdown"}
}'
```

#### Resolved threads

If the developer clicked "Resolve" on a thread, your reply will appear inside the resolved (collapsed) thread. **It does not auto-reopen.** The developer gets a notification about the reply, but the thread stays visually resolved.

To reopen: click "Reopen" in Bitbucket UI. No API endpoint for this.

---

## Phase 4: Approve or Re-request

### When to approve

All of these must be true:
- All **critical** issues are fixed
- All **important** issues are either fixed or have a valid reason to defer (with a tracked task)
- No new critical issues from the fix
- Tests pass (check pipeline)

### When to re-request changes

- Any critical issue remains unfixed
- Fix introduced new critical issues
- Developer disagreed on a security issue without valid justification

### When to accept "defer to later task"

Valid for non-security issues:
- "Will add in RBAC task" - OK if there's an actual RBAC task in the backlog
- "Will refactor in next sprint" - OK for code quality, NOT OK for security

Not valid:
- Deferring security fixes (IDOR, XSS, injection) without even a temporary mitigation
- Deferring data integrity issues (missing unique constraints) that can cause production bugs

### Approve flow

```bash
# Approve the PR
bb-api pr approve <id>

# Post approval comment (optional)
bb-api pr comment <id> "LGTM. All critical issues resolved."
```

### Close the cycle

**Only close the beads task after Phase 4** - not after posting comments (Phase 2).

```bash
# Update beads task
bd update <task-id> --notes="Follow-up complete. PR #19 approved. ..."

# If all PRs in the review are done
bd close <task-id> --reason="All PRs reviewed and approved"
bd sync --flush-only
```

Task lifecycle:
```
Phase 1 (review)   → task in_progress
Phase 2 (posting)  → task in_progress (comments posted, waiting for fixes)
Phase 3 (follow-up) → task in_progress (checking fixes)
Phase 4 (approve)  → task closed
```

---

## Tools Reference

### bb-api commands

```bash
bb-api pr list                              # Open PRs
bb-api pr show <id>                         # PR info + files
bb-api pr diff <id>                         # Full PR diff
bb-api pr comments <id>                     # All comments
bb-api pr comment <id> "text"               # General comment
bb-api pr inline <id> "path" <line> "text"  # Inline on new code
bb-api pr inline-old <id> "path" <line> "text"  # Inline on deleted code
bb-api pr approve <id>                      # Approve
bb-api pr delete-comment <pr_id> <comment_id>   # Delete comment
```

### Git commands for review

```bash
# Preparation
git fetch origin
git checkout <branch>                       # or worktree

# Initial review - scope
git diff --name-status <target>...HEAD      # Changed files
git diff --stat <target>...HEAD             # Change stats
git log --oneline <target>...HEAD           # Commits

# Follow-up - fix delta only
git log --oneline origin/<branch>           # Find fix commit
git diff <prev>..<fix> --name-status        # What files changed in fix
git diff <prev>..<fix> --stat               # Fix stats
git diff <prev>..<fix>                      # Full fix diff
```

### Beads tracking

```bash
# Create review task
bd create --title="Code Review: PR #X, #Y - Feature name" --type=task --priority=1

# Track posted comments in notes (after each comment)
bd update <id> --notes="## Posted Comments Log ..."

# Track follow-up
bd update <id> --notes="## Follow-up Review ..."

# Close when done
bd close <id> --reason="All PRs reviewed and approved"
```

---

## Skill Integration Notes

For building a `/lets:review-pr` skill, each phase maps to a skill step:

### Phase 1 input/output
- **Input:** PR ID(s), repo path
- **Process:** fetch, checkout, run agents, consolidate
- **Output:** review plan (findings + posting plan)
- **User approval:** required before posting

### Phase 2 input/output
- **Input:** approved review plan
- **Process:** verify lines, post comments, track in beads
- **Output:** posted comments log
- **User approval:** each comment before posting

### Phase 3 input/output
- **Input:** beads task with posted comments log, fix commit hash
- **Process:** diff fix commit, check each comment, classify
- **Output:** follow-up report (fixed/partial/not fixed per comment)
- **User approval:** required before posting follow-up replies

### Phase 4 input/output
- **Input:** follow-up report
- **Process:** approve or re-request
- **Output:** PR status change, beads task closed
- **User approval:** required for approve

### Automation boundaries

**Automate:**
- Fetching PR info and diffs
- Running review agents
- Classifying fix status per comment
- Generating reports

**Always require user approval:**
- Which findings to post (filter noise)
- Each comment text before posting
- Approve/reject decision
- Follow-up replies
