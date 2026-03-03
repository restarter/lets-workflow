# PR Review Posting Workflow

How to conduct a code review with multiple agents, post comments to Bitbucket, and track everything in beads.

## Overview

```
Review (agents) -> Consolidated Plan -> Post inline + summary -> Track in beads -> Slack notification
```

The workflow splits into two phases:
1. **Review phase** - run agents, analyze findings, create consolidated plan (separate session)
2. **Posting phase** - verify line numbers, post comments one by one, track in beads

This document covers the **posting phase**.

## Prerequisites

- Consolidated review plan exists (from review phase)
- `bb-api` script configured with Bitbucket auth
- Beads task created and in_progress
- PR IDs known, dependency chain understood

## Posting Order

Post in dependency chain order (base branch first):

```
PR #19 (base) -> PR #20 (depends on #19) -> PR #18 (independent)
```

Within each PR:
1. Inline comments (critical first, then important)
2. Summary comment (after all inline are posted)

## Line Number Verification

**Critical step.** Agent-found line numbers are often DIFF output line numbers, not FILE line numbers. `bb-api pr inline` requires NEW FILE line numbers.

### How to find correct line numbers

1. Get the diff:
```bash
bb-api pr diff <PR_ID> | grep -n "^diff --git" | head -20
```

2. Find the file section and its hunk header:
```
@@ -0,0 +1,91 @@    # New file: lines 1-91
@@ -10,5 +10,8 @@    # Modified file: new version starts at line 10
```

3. For new files: count from `+<?php` (line 1) to target line
4. For modified files: use the `+N` from `@@ -old,len +new,len @@` as starting line, count only `+` and context lines (not `-` lines)

### Common pitfall

The ready-to-post commands from the review phase may have wrong line numbers. Always verify before posting.

## Posting Comments

### Inline comment

```bash
bb-api pr inline <PR_ID> "<file_path>" <line_number> "<markdown text>"
```

- Line number = line in the NEW version of the file
- Post on method declaration line (gives context) rather than specific problematic line
- Escape backticks, dollar signs in bash: `\`code\`` and `\$variable`

### Summary comment

```bash
bb-api pr comment <PR_ID> "<markdown text>"
```

Posted to the PR activity feed (not attached to any file).

### Comment format

**Inline comments:**
```markdown
**[severity]** Brief description in the author's language

Explanation of the issue.

\```php
// Fix suggestion with code
\```

Cross-references to related PRs if applicable.
```

**Severity levels:**
- `[critical]` - must fix before merge (security, data loss, broken logic)
- `[important]` - should fix (bad patterns, missing validation)
- `[suggestion]` - nice to have (code style, optimization)

**Summary comment:**
```markdown
## Code Review - PR #XX

**Verdict: CHANGES REQUESTED**

Ревью проведено с помощью N специализированных агентов (list agents).

### Scope
What was expected vs what was delivered. Example:
"В задаче предусмотрен CRUD доменов, но в PR реализован только бекенд.
Страница остается статическим mockup без контроллера."

### Critical (N)
1. **Issue** - brief description. See inline on `file.php`.

### Important (N)
1. **Issue** - brief description. See inline on `file.php`.

### Менее критичные замечания

Ниже - замечания, которые не блокируют мерж. Не стал постить inline,
чтобы не засорять PR. Прошу ознакомиться и учесть в дальнейшей работе.

- **Issue** (`file.php:NN`) - description with file path and line reference.
- **Issue** (`file.php:NN` vs `other.php:NN`) - description.

---
Generated with Claude Code
```

Key differences from the old template:
- **Scope section** - sets expectations about what's in the PR vs what was planned
- **"Менее критичные замечания"** - replaces "Observations". Explains WHY these are not inline.
- **File:line references** - every observation points to specific code, not just abstract advice
- **Language** - summary text in team's language (Russian), severity tags in English

## Language

- Comment text: in the language the dev team uses (Russian in our case)
- Code samples: always English
- Severity tags: always English (`[critical]`, `[important]`, etc.)
- Summary structure: can be in dev team's language

## Tracking in Beads

All posted comments are logged in the beads task via `bd comments add <task-id> "..."`.

### Notes format

```markdown
## Posted Comments Log

### PR #19
1. ✅ **Issue name** (line N, filename) - [comment-ID](link) - severity
2. ✅ **Issue name** (line N, filename) - [comment-ID](link) - severity
...
N. ✅ **Summary** - [comment-ID](link)

### PR #20
...
```

### Why track in beads

- Single source of truth for what was posted
- Compare with original plan (what we planned vs what we actually posted)
- Links to each comment for quick access
- History survives session compaction

### Update after each comment

For large reviews (5+ comments across multiple PRs): update notes after EVERY posted comment. If something goes wrong mid-posting, beads has the accurate state.

For small reviews (2-3 comments on a single PR): acceptable to batch-log after all comments are posted.

## Slack Notification

After completing all comments for a PR, prepare a Slack message:

```
**PR #XX (Feature Name) - Code Review done**

Conducted review of PR #XX (description). Posted N inline comments + 1 summary:

- N critical: brief list of critical issues
- N important: brief list of important issues

Verdict: **Changes Requested**. Key takeaway in one sentence.

PR: https://bitbucket.org/workspace/repo/pull-requests/XX
```

Rules:
- Language: match the team's communication language
- Keep concise - Slack is not the place for details
- Link to PR so people can read full comments there
- Mention the verdict clearly

## bb-api Gotchas

### Bitbucket markdown formatting

Bitbucket markdown requires an **empty line before lists**. Without it, list items render as inline text.

Bad (renders as one line):
```markdown
3 places without nullsafe:
- `File.php:10` - description
- `File.php:20` - description
```

Good (renders as proper list):
```markdown
3 places without nullsafe:

- `File.php:10` - description
- `File.php:20` - description
```

This applies to both `-` lists and `1.` numbered lists.

### Replying to existing comment threads

`bb-api` doesn't have a reply command. Use `raw-post` with `parent.id`:

```bash
bb-api raw-post "/pullrequests/<PR_ID>/comments" '{
  "parent": {"id": <ORIGINAL_COMMENT_ID>},
  "content": {"raw": "Reply markdown text"}
}'
```

The reply attaches to the same file/line as the parent comment. If the thread was resolved by the author, the reply appears inside the collapsed thread but **does not auto-reopen it**. Reopen manually in Bitbucket UI.

### Path newline bug (fixed)

`echo "$path" | jq -Rs .` adds a trailing `\n` to the path, causing Bitbucket to post file-level comments instead of line-level. Fix: use `printf '%s' "$path"` instead of `echo "$path"`.

Applies to both `cmd_pr_inline` and `cmd_pr_inline_old` functions.

### Deleting comments

```bash
bb-api pr delete-comment <PR_ID> <comment_id>
```

Or delete through Bitbucket UI. Comments are published immediately - there's no draft mode.

### Verifying posted comment

Check via API that the comment is properly attached:
```bash
bb-api raw "/pullrequests/<PR_ID>/comments/<comment_id>" | jq '{id, inline, type}'
```

Look for:
- `inline.to` has correct line number
- `inline.path` has no trailing `\n`
- `inline.outdated` is false

## Workflow Checklist

```
For each PR (in dependency order):
  [ ] Get diff, find correct file line numbers
  [ ] Show each inline comment text to user for approval
  [ ] Post inline comments one by one
  [ ] Log each comment to beads immediately after posting
  [ ] User verifies comment appears correctly in Bitbucket
  [ ] Post summary comment
  [ ] Log summary to beads
  [ ] Prepare Slack notification text
  [ ] Move to next PR

After all PRs:
  [ ] DO NOT close beads task - it stays in_progress until fixes are verified (Phase 3-4 in lifecycle)
  [ ] bd sync --flush-only
```

## Example Session Flow

```
User: /lets:start
  -> Pick task pwa-service-5vr (Code Review posting)

Read consolidated plan
Read ready-to-post commands

For PR #19:
  Get diff -> verify line numbers
  Comment #1: show text -> user OK -> post -> log to beads -> user verifies
  Comment #2: show text -> user OK -> post -> log to beads
  ...
  Summary: show text -> user OK -> post -> log to beads
  Slack message for PR #19

For PR #20:
  Same flow...

For PR #18:
  Same flow...

Close task, sync beads
```
