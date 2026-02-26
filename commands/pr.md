---
description: GitHub PR review lifecycle - analyze, discuss, post inline comments, follow-up, respond, approve
argument-hint: "[PR-url-or-number|--respond|--follow-up|--approve|--merge|--status|--cancel]"
---

# PR Review Lifecycle

Full GitHub PR review lifecycle: analyze code, discuss findings with user, post inline comments, follow-up on fixes, approve or request changes.

**Requires:** `gh` CLI installed and authenticated.

## Usage

```bash
/lets:pr <PR-url-or-number>     # Start new review
/lets:pr                        # Resume from saved state
/lets:pr --follow-up            # Check if fixes addressed comments
/lets:pr --respond [PR]         # Author: triage review comments, fix, reply
/lets:pr --approve              # Approve PR
/lets:pr --merge                # Merge PR
/lets:pr --status               # Show current review state
/lets:pr --cancel               # Clean up state, abandon review
```

## Step 1: Detect Mode

### Verify gh CLI

```bash
gh auth status 2>&1 || echo "gh not authenticated"
```

If gh not available or not authenticated, stop: "gh CLI required. Run `gh auth login`."

### Parse argument

Interpret user intent:
- PR URL or number -> extract PR number
- `--follow-up` -> Phase 3
- `--respond` -> Phase R (author respond)
- `--approve` -> Phase 4
- `--merge` -> merge only
- `--status` -> show state and exit
- `--cancel` -> clean up state and exit
- No argument, no flag -> check for existing state files

### Check for existing state

```bash
ROOT=$(git rev-parse --show-toplevel)
PR_DIR="$ROOT/.lets/execution/pr-{number}"
STATE_FILE="$PR_DIR/review.json"
```

### --status flag (exit early)

If --status: read state file, show progress summary (phase, findings count, posted status), exit.
If no state file: "No active PR review. Run `/lets:pr <PR>` to start one."

### --cancel flag (cleanup and exit)

If --cancel:
1. Delete PR folder: `rm -rf "$PR_DIR"` (removes state + all temp files)
2. If `stashed: true` in state, warn: "You have a stash from before the PR review. Run `git stash pop` to restore."
3. Switch back to previous_branch if stored
4. Inform user, exit.

### Route to phase

**State guard:** If `--follow-up`, `--approve`, `--merge`, or `--respond` is specified but no state file exists:
1. If a PR number is also provided (e.g., `/lets:pr --approve 2`), create a minimal state from `gh pr view`:
   ```bash
   REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
   gh pr view <PR> --json title,headRefOid,headRefName,baseRefName
   ```
   Write minimal state (pr_number, repo, title, branch, head_sha, findings: [], findings_posted: false) and continue.

   For `--respond` specifically, create a minimal response state (not review state):
   - If PR number provided: fetch PR info, create `$PR_DIR/response.json`, continue to Phase R
   - If no PR number: check for existing `pr-[0-9]*/response.json` files, or stop
2. If no PR number - stop: "No active PR review found. Run `/lets:pr <PR>` to start one."

| State | Action |
|-------|--------|
| No state file, PR number given | Phase 1 (new review) |
| State exists, findings_posted: false | Phase 2 (discuss & post) |
| State exists, findings_posted: true, new commits since head_sha | Phase 3 (follow-up) |
| State exists, findings_posted: true, no new commits | Show status, ask what to do |
| --follow-up flag (state required) | Phase 3 |
| --approve flag (state required) | Phase 4 |
| --respond flag (PR number or existing state) | Phase R |
| --merge flag (state required) | Merge only (Step 5.4) |

### No argument, no flag

Look for existing state files:

```bash
ls -d "$ROOT/.lets/execution/pr-"[0-9]*/ 2>/dev/null
```

For each found folder, check which state files exist:
- `review.json` only -> resume review (Phase 2/3/4)
- `response.json` only -> resume respond (Phase R)
- Both exist -> AskUserQuestion: "PR #{number} has both review and response state. Resume as reviewer or author?"
- Multiple PR folders -> AskUserQuestion which PR to resume
- None -> ask user for PR number

## Step 2: Phase 1 - Analyze

### 2.1 Get PR info

```bash
gh pr view <PR> --json state,isDraft,title,body,additions,deletions,changedFiles,headRefOid,headRefName,baseRefName
```

Skip if: closed or draft. Warn if trivial (< 10 lines changed).

PR size check - if additions + deletions > 5000 or changedFiles > 50:

```
AskUserQuestion(
  questions=[{
    question: "Large PR ({additions}+ {deletions}- across {files} files). How to proceed?",
    header: "LETS",
    options: [
      { label: "Full review", description: "Review everything" },
      { label: "Focus", description: "Pick specific files or areas" },
      { label: "Cancel", description: "Too large, skip" }
    ],
    multiSelect: false
  }]
)
```

### 2.2 Detect active task (before branch switch)

Resolve task-id now - after checkout the branch name changes to the PR branch.

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Fallback: bd list --status=in_progress
# If ambiguous or not found: skip beads logging later
```

Store detected task-id for use in `bd comments add` calls throughout the lifecycle.

### 2.3 Save current branch and checkout PR

IMPORTANT: Before checking out the PR branch, save current state.

```bash
# Save current branch name for later restore
PREVIOUS_BRANCH=$(git branch --show-current)

# Check for uncommitted changes
git status --short
```

If uncommitted changes exist:

```
AskUserQuestion(
  questions=[{
    question: "Uncommitted changes on current branch. What to do?",
    header: "LETS",
    options: [
      { label: "Stash", description: "git stash, switch to PR, remind to pop later" },
      { label: "Commit first", description: "Run /lets:commit, then switch" },
      { label: "Cancel", description: "Abort PR review" }
    ],
    multiSelect: false
  }]
)
```

If stash: run `git stash`, proceed. If commit: run /lets:commit, proceed. If cancel: exit.

```bash
# Checkout PR branch
gh pr checkout <PR>
```

If checkout fails:
- If stashed: run `git stash pop` to restore
- Show error and exit: "Failed to checkout PR branch. Check for conflicts."
- Do NOT create state file on checkout failure.

### 2.3 Create state file

```bash
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$PR_DIR"
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
```

Write initial state to `.lets/execution/pr-{number}/review.json` with Phase 1 fields.
**IMPORTANT:** Use `REPO` from `gh repo view` above - do NOT guess from directory name.

```json
{
  "pr_number": 42,
  "pr_url": "https://github.com/${REPO}/pull/42",
  "repo": "${REPO}",
  "title": "Add feature X",
  "branch": "feature/xyz",
  "base_branch": "main",
  "head_sha": "abc1234def5678",
  "previous_branch": "feature/my-current-work",
  "stashed": false,
  "task_id": "lets-plugin-claude-0nf.26",
  "review_started": "2026-02-26",
  "findings": [],
  "findings_posted": false
}
```

**Add fields as phases need them:**
- Phase 1 adds: `review_json`, `verdict`, `findings` (from review output)
- Phase 2 adds: `review_id`, `summary_comment_id` (after posting)
- Phase 3 adds: `followup_count` (after follow-up)

**Finding sub-fields** (added per finding during Phase 2-3):
- `disposition`: null | "inline" | "summary" | "dropped" | "edited"
- `disposition_note`: user's edit text if disposition is "edited"
- `posted_comment_id`: GitHub comment ID after posting
- `followup_status`: null | "fixed" | "not_fixed" | "unclear"

**State fields:**
- `stashed`: true if user chose "Stash" before checkout - cleanup must warn or pop
- `task_id`: detected before branch switch, used for `bd comments add` calls

### 2.4 Run review analysis

Invoke `/lets:review <PR> --json`.

- `--json`: saves structured findings to `.lets/reviews/{date}-PR-{number}.json` and skips GitHub posting (pr controls all posting)

After review completes, read the JSON file.
Copy findings array into state file.
Copy verdict into state file.
Save review_json path in state file.

### 2.5 Show findings summary

```
## PR #{number}: {title}

**Verdict:** {verdict}
**Findings:** {N} issues (confidence >= 80)

| # | Severity | Title | File | Line |
|---|----------|-------|------|------|
| 1 | critical | {title} | {file} | {line} |
| 2 | important | {title} | {file} | {line} |
...

Review JSON: {review_json path}
```

Transition to Phase 2 immediately (no need for separate invocation).

## Step 3: Phase 2 - Discuss & Post

### 3.1 Discuss each finding

For each finding in state, show code context and ask user.

We have the PR branch checked out, so we can read actual files.

For each finding:

```
## Finding {N}/{total}: {title} [{severity}]

File: `{file}:{line}`
Agent: {agent}
```

```{language}
{Read 5-10 lines of actual code around the finding using Read tool}
```

```
{description}

**Suggestion:** {suggestion}
```

Then AskUserQuestion:

```
AskUserQuestion(
  questions=[{
    question: "Finding {N}: {title} [{severity}]",
    header: "PR Review",
    options: [
      { label: "Inline", description: "Post as inline comment on {file}:{line}" },
      { label: "Summary", description: "Include in summary comment only" },
      { label: "Drop", description: "Not worth mentioning" },
      { label: "Edit", description: "Adjust wording before posting" }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- Inline -> set finding disposition to "inline"
- Summary -> set to "summary"
- Drop -> set to "dropped"
- Edit -> user provides revised text, set to "edited", save disposition_note

After all findings discussed, save updated state.

### 3.2 Show posting plan

```
## Posting Plan

### Inline Comments ({N})
1. [{severity}] {title} - {file}:{line}
2. ...

### Summary Observations ({N})
1. {title} - {description}
2. ...

### Dropped ({N})
1. {title}
```

```
AskUserQuestion(
  questions=[{
    question: "Ready to post to PR #{number}?",
    header: "LETS",
    options: [
      { label: "Post all", description: "Post {N} inline + summary to GitHub" },
      { label: "Review again", description: "Go back through findings" },
      { label: "Save for later", description: "Keep state, post in next session" }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- Post all -> proceed to 3.3
- Review again -> loop back to 3.1
- Save for later -> save state, show LETS box with `/lets:pr` to resume

### 3.3 Verify line numbers against diff

CRITICAL: GitHub inline review comments require the line to exist in the PR diff.
The `line` field in the API = line number in the NEW version of the file (RIGHT side).

For each finding marked as "inline":

1. Get the full PR diff and filter by file:

```bash
# gh pr diff does NOT support -- file_path syntax
# Filter the diff output to extract hunks for the target file
gh pr diff <PR> | awk '/^diff --git.*{file_path}$/,/^diff --git/'
```

2. Parse diff hunks to determine which new-side line numbers are present:
   - Each hunk starts with: `@@ -old_start,old_count +new_start,new_count @@`
   - Lines starting with `+` or ` ` (space/context) are on the new side
   - New-side line numbers run from new_start, incrementing for each `+` or ` ` line
   - Lines starting with `-` are old-side only (don't increment new counter)

3. Check if finding's line number falls within any hunk's new-side range:
   - A line is in the diff if it falls within [new_start, new_start + new_count) of any hunk

4. If line IS in diff -> keep as "inline", use that line number
5. If line is NOT in diff (unchanged code) -> GitHub API will reject it (422 error).
   Demote to "summary" disposition, warn user:
   "Finding '{title}' at {file}:{line} is outside the diff. Moving to summary comment."

Save verified dispositions to state.

### 3.4 Post inline comments (batch)

All inline comments go in a single review submission via gh api.

Step 1: Build the complete review JSON payload.

```bash
ROOT=$(git rev-parse --show-toplevel)
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
HEAD_SHA=$(gh pr view <PR> --json headRefOid -q .headRefOid)
```

Write the FULL payload to `$PR_DIR/payload.json`:

```json
{
  "commit_id": "{HEAD_SHA}",
  "event": "COMMENT",
  "body": "## Code Review\n\n**Verdict:** {verdict}\nFound {N} issues across {M} files.\n{1-2 sentence summary}",
  "comments": [
    {
      "path": "src/search.py",
      "line": 42,
      "side": "RIGHT",
      "body": "**[critical]** SQL injection in search query\n\nUser input concatenated directly into SQL query.\n\n**Suggestion:** Use parameterized queries.\n\n```python\ncursor.execute(\"SELECT * FROM items WHERE name = %s\", (user_input,))\n```"
    },
    {
      "path": "src/auth.py",
      "line": 15,
      "side": "RIGHT",
      "body": "**[important]** Missing rate limiting on login endpoint\n\n..."
    }
  ]
}
```

Field requirements:
- `commit_id`: HEAD SHA of the PR (required - without it GitHub uses latest which may differ)
- `event`: "COMMENT" to post immediately. NOT "PENDING" (creates draft) or "APPROVE"/"REQUEST_CHANGES" (submits verdict)
- `body`: review summary in markdown
- `comments[].path`: file path relative to repo root
- `comments[].line`: NEW-file line number (verified in Step 3.3)
- `comments[].side`: always "RIGHT" (commenting on new code)
- `comments[].body`: markdown comment body

Step 2: Post the review.

```bash
gh api repos/${REPO}/pulls/{PR}/reviews \
  --method POST \
  --input "$PR_DIR/payload.json"
```

Step 3: Parse response.
The response JSON includes:
- `.id` -> save as review_id in state
- `.comments[].id` -> map to findings by path+line, save as posted_comment_id

If gh api returns error (400/422):
1. Show the error message
2. Most common cause: line number not in diff (should be caught in 3.3, but edge cases exist)
3. Offer fallback: post all findings as a single `gh pr comment` instead of inline
4. Fallback command:

```bash
gh pr comment <PR> --body-file "$PR_DIR/fallback.md"
```

(Write all findings as a formatted list in the fallback file)

### 3.5 Post summary comment (if needed)

If there are findings with disposition "summary" or "edited" that weren't included in inline:

```bash
ROOT=$(git rev-parse --show-toplevel)
```

Write to `$PR_DIR/summary.md`:

```
### Additional Observations

These items don't warrant inline comments but are worth noting:

1. **{title}** [{severity}]
   {file}:{line} - {description}

2. ...
```

```bash
gh pr comment <PR> --body-file "$PR_DIR/summary.md"
```

Save summary_comment_id from output (parse the URL or ID from gh output).

### 3.6 Update state and clean up

- Set findings_posted: true
- Save posted_comment_id for each inline finding
- Save review_id and summary_comment_id
- Temp files (payload.json, summary.md, fallback.md) stay in PR folder for reference

Log to beads:

```bash
bd comments add {task_id} "PR review posted on #<PR>: {N} inline comments, {M} summary items. Verdict: {verdict}"
# Skip if task_id is null
```

## Step 4: Phase 3 - Follow-up

Triggered when state has findings_posted: true and either --follow-up flag or auto-detected new commits.

### 4.1 Detect new commits

```bash
git fetch origin
REVIEW_SHA={head_sha from state}
CURRENT_SHA=$(gh pr view <PR> --json headRefOid -q .headRefOid)
```

If REVIEW_SHA == CURRENT_SHA:
  "No new commits since review. Waiting for fixes."
  Show LETS box: /lets:pr --approve (if ready) or /lets:pr --status
  Exit.

### 4.2 Checkout updated PR and get fix delta

```bash
gh pr checkout <PR>

git log --oneline ${REVIEW_SHA}..HEAD
git diff ${REVIEW_SHA}..HEAD --stat
git diff ${REVIEW_SHA}..HEAD
```

### 4.3 Classify each finding

For each finding with a posted_comment_id:

1. Check if the file was changed in the fix delta:

```bash
git diff --name-only ${REVIEW_SHA}..HEAD
```

2. If file changed, read current code around the finding's line
3. Classify based on whether the issue was addressed:

| Status | Meaning |
|--------|---------|
| fixed | Issue resolved (fully or via alternative approach) |
| not_fixed | Not addressed or insufficient |
| unclear | Needs discussion - add freetext note |

Save followup_status for each finding.
Update head_sha in state to CURRENT_SHA.
Increment followup_count.

### 4.4 Show follow-up report

```
## Follow-up Review - PR #{number}

New commits since review:
{commit list}

| # | Finding | Status | Notes |
|---|---------|--------|-------|
| 1 | {title} | Fixed | {brief explanation} |
| 2 | {title} | Not fixed | {why it matters} |
...

Summary: {X}/{Y} findings addressed
**Verdict:** {Ready for approval / Still needs changes}
```

### 4.5 Post follow-up replies

Show the follow-up table from 4.4, then ask once:

```
AskUserQuestion(
  questions=[{
    question: "Post auto-replies to inline comments?",
    header: "PR Review",
    options: [
      { label: "Post all", description: "Reply to fixed ('Verified fixed'), flag unresolved" },
      { label: "Review individually", description: "Decide per finding" },
      { label: "Skip replies", description: "No replies needed" }
    ],
    multiSelect: false
  }]
)
```

**Auto-reply text by status:**
- fixed: "Verified fixed. Thanks!"
- not_fixed: "This still needs attention: {brief reason}"
- unclear: "Needs discussion: {note}"

Post replies using GitHub's reply-to-comment API:

```bash
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)

gh api repos/${REPO}/pulls/{PR}/comments \
  --method POST \
  -F body="{reply text}" \
  -F in_reply_to={posted_comment_id}
```

Save state after each reply.

## Step 5: Phase 4 - Approve / Request Changes

### 5.1 Check readiness

Show approval checklist based on findings and follow-up status:

```
## Approval Checklist - PR #{number}

Critical findings:
- [x] {title} - Fixed
- [ ] {title} - Not fixed

Important findings:
- [x] {title} - Fixed
- [ ] {title} - Not fixed

Summary: {X}/{Y} critical fixed, {A}/{B} important fixed
```

### 5.2 User decides

```
AskUserQuestion(
  questions=[{
    question: "PR #{number} - your verdict?",
    header: "LETS",
    options: [
      { label: "Approve", description: "All critical issues resolved" },
      { label: "Request changes", description: "Still needs work" },
      { label: "Skip", description: "Don't submit a review verdict" }
    ],
    multiSelect: false
  }]
)
```

### 5.3 Submit verdict

```bash
ROOT=$(git rev-parse --show-toplevel)
```

Write verdict body to `$PR_DIR/verdict.md`:

For approve:
```
LGTM. All critical findings have been addressed.
{brief summary of what was fixed}
```

For request changes:
```
Still needs work on {N} findings:
{list of unresolved critical/important items}
```

Then:

```bash
# Approve
gh pr review <PR> --approve --body-file "$PR_DIR/verdict.md"

# OR Request changes
gh pr review <PR> --request-changes --body-file "$PR_DIR/verdict.md"
```

**Handle self-approve error:** If `gh pr review --approve` fails with "Can not approve your own pull request":
1. Inform user: "GitHub doesn't allow approving your own PR."
2. Offer fallback: post verdict as a regular comment instead (`gh pr comment <PR> --body-file ...`)
3. Do NOT retry --approve.

Save verdict to state.

### 5.4 Optional merge

After approval (or if --merge flag):

```
AskUserQuestion(
  questions=[{
    question: "PR approved. Merge now?",
    header: "LETS",
    options: [
      { label: "Squash merge", description: "Squash and merge, delete branch" },
      { label: "Merge commit", description: "Create merge commit, delete branch" },
      { label: "Later", description: "Leave for author to merge" }
    ],
    multiSelect: false
  }]
)
```

```bash
# Squash
gh pr merge <PR> --squash --delete-branch

# Merge commit
gh pr merge <PR> --merge --delete-branch
```

### 5.5 Clean up

After merge or after approve (if not merging):

1. Switch back to previous_branch:

```bash
git checkout {previous_branch from state}
```

2. If `stashed: true` in state, warn: "You have a stash from before the PR review. Run `git stash pop` to restore your changes."

3. Delete PR folder: `rm -rf "$PR_DIR"`

4. Log to beads (using task_id from state):

```bash
bd comments add {task_id} "PR #{number}: {approved/merged/changes requested}"
```

If task_id is null, skip beads logging.

If not merging and not cleaning up (review posted, waiting for fixes):
- Keep state file for follow-up
- Switch back to previous_branch

## Step 6: Phase R - Author Respond

Triggered by `--respond` flag. This is the author's flow - mirror of reviewer phases.

### 6.1 Verify author context

```bash
CURRENT_BRANCH=$(git branch --show-current)
PR_BRANCH=$(gh pr view <PR> --json headRefName -q .headRefName)
```

If not on PR branch:

```
AskUserQuestion(
  questions=[{
    question: "You're on '{CURRENT_BRANCH}', PR branch is '{PR_BRANCH}'. Switch?",
    header: "LETS",
    options: [
      { label: "Checkout", description: "Switch to {PR_BRANCH}" },
      { label: "Stay", description: "Continue on current branch" },
      { label: "Cancel", description: "Abort" }
    ],
    multiSelect: false
  }]
)
```

If Checkout: `gh pr checkout <PR>` (same stash handling as Step 2.3).

**Detect active task** (before branch switch, same logic as Step 2.2):
```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>
# Fallback: bd list --status=in_progress
```
Store detected task_id for beads logging in 6.6.

Check for existing `$PR_DIR/response.json`:
- If exists and `replies_posted: false` -> offer to resume (skip to 6.3, only triage comments where `decision == null`)
- If exists and `replies_posted: true` -> "Already responded to this PR. Run again to respond to new comments since last response."
- If not exists -> continue to 6.2

### 6.2 Fetch review comments from GitHub

**Step 1: Resolve REPO first.** All subsequent API calls depend on this value. Do NOT run API calls in parallel with this - resolve REPO, then use it.

```bash
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
HEAD_SHA=$(gh pr view <PR> --json headRefOid -q .headRefOid)
```

**Step 2: Fetch comments.** Three API calls (can run in parallel - they all use REPO). Use `--paginate` on all calls.

```bash
# 1. Review submissions (review body text + verdict)
gh api repos/${REPO}/pulls/{PR}/reviews --paginate

# 2. Inline review comments (attached to specific lines)
gh api repos/${REPO}/pulls/{PR}/comments --paginate

# 3. General PR comments (issue-style, flat thread)
gh api repos/${REPO}/issues/{PR}/comments --paginate
```

**Filter reviews:** Exclude `PENDING` state reviews (drafts not yet submitted). Keep: `APPROVED`, `CHANGES_REQUESTED`, `COMMENTED`, `DISMISSED`.

**No self-filter:** Fetch all comments for full context. Author skips their own during triage if needed.

**Thread context for inline comments:**
- If a comment has `in_reply_to_id`, fetch the thread for display during triage (show conversation context).
- Always reply directly to the comment's own `github_id` via `in_reply_to`. No thread chain reconstruction needed - GitHub handles threading.

**Separate review verdicts from actionable comments:**
- Review submissions (type `review_body`) are **context, not actionable items**. They contain the reviewer's overall verdict and summary. Show them as a header in the overview (Step 6.3), but do NOT include them in the triage loop.
- Only inline comments and general comments go into the `comments` array for triage.

**Build comment list for triage (inline + general only):**

For each inline or general comment, create an entry:

```json
{
  "id": "inline-{comment_id}" | "general-{comment_id}",
  "github_id": 12345678,
  "type": "inline" | "general",
  "reviewer": "username",
  "path": "src/file.py",
  "line": 42,
  "body": "Full comment text",
  "decision": null,
  "decision_note": null,
  "reply_text": null,
  "reply_posted": false,
  "reply_posted_id": null
}
```

Field details:
- `reply_posted`: per-comment flag for idempotent resume.

Write to `$PR_DIR/response.json`:

```json
{
  "pr_number": 42,
  "repo": "owner/repo",
  "title": "Add feature X",
  "branch": "feature/xyz",
  "head_sha": "abc1234",
  "previous_branch": "feature/my-current-work",
  "stashed": false,
  "task_id": "project-0nf.26",
  "respond_started": "2026-02-26",
  "reviews": [
    { "reviewer": "username", "state": "CHANGES_REQUESTED", "body": "Review summary..." }
  ],
  "comments": [ ... ],
  "fixes_committed": false,
  "fix_commit_sha": null,
  "replies_posted": false
}
```

Note: `reviews` array stores review verdicts for display context. `comments` array stores only triageable items (inline + general).

### 6.3 Show overview and triage

Show review context (from `reviews` array), then comment table (from `comments` array):

```
## PR #{number}: {title} - Responding to Review

**Review by @{reviewer}:** {state} (e.g., CHANGES_REQUESTED)
> {review body, first 2-3 lines}

---

Comments to triage: {N} inline, {M} general

| # | Type | Location | Reviewer | Preview |
|---|------|----------|----------|---------|
| 1 | inline | src/search.py:42 | @user | SQL injection risk... |
| 2 | inline | src/auth.py:15 | @user | Missing rate limiting... |
| 3 | general | - | @user | Additional observations... |
```

Then offer bulk vs individual triage:

```
AskUserQuestion(
  questions=[{
    question: "{N} review comments. How to triage?",
    header: "PR Response",
    options: [
      { label: "Agree to all", description: "Mark all as 'Agree' - will fix everything" },
      { label: "Triage individually", description: "Decide per comment (fix/agree/disagree/skip)" },
      { label: "Skip all", description: "No replies needed" }
    ],
    multiSelect: false
  }]
)
```

**Agree to all:** Set all comments to `decision: "agree"`, generate default reply "Agreed, will fix." for each. Skip to 6.4.

**Skip all:** Set all to `decision: "skip"`, `reply_posted: true`. Skip to output (done).

**Triage individually:** For each comment (inline first grouped by file, then general):

```
## Comment {N}/{total}: [{type}] by @{reviewer}

{If inline with thread:}
File: `{path}:{line}`

Thread:
> **@{author1}:** {message 1}
> **@{author2}:** {reply 1}
> **@{author3}:** {latest message}

{If inline without thread:}
File: `{path}:{line}`
```{language}
{Read 5-10 lines of actual code around the line using Read tool}
```
> {comment body}

{If general:}
> {comment body}
```

```
AskUserQuestion(
  questions=[{
    question: "Comment {N}: {first 60 chars of body}",
    header: "PR Response",
    options: [
      { label: "Fix", description: "AI will fix this (mechanical change)" },
      { label: "Agree", description: "Will reply 'agreed' (manual fix or no code change needed)" },
      { label: "Disagree", description: "Push back with your explanation" },
      { label: "Skip", description: "No reply needed" }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- **Fix**: Set `decision: "fix"`. AI will fix in step 6.4. Reply text: "Fixed." (updated with commit SHA after push)
- **Agree**: Set `decision: "agree"`. Ask: "Brief note? (optional, Enter to skip)". Reply: "Agreed." or "Agreed - {note}."
- **Disagree**: Set `decision: "disagree"`. Ask: "Why? (this becomes your reply)". Reply: user's text directly.
- **Skip**: Set `decision: "skip"`, `reply_posted: true`.

Save state after each decision (crash recovery).

### 6.4 Fix code

If any comments have `decision: "fix"`:

Show fix summary:

```
## Fix Plan

{N} items marked for AI fix:
1. {path}:{line} - {first line of comment}
2. ...

{M} items agreed (reply only, no code change):
1. {comment preview}
...
```

For each "fix" comment:
- Read the file and code context
- Understand the reviewer's suggestion
- Make the fix using Edit tool
- **Guard rail:** Only attempt mechanical fixes. If the comment is architectural, inform user: "This comment is too broad for auto-fix. Changing to 'Agree' - you may want to fix manually."
  - **Mechanical (AI fixes):** typo, null check, missing import, variable rename, add error handling, fix off-by-one, add missing validation
  - **Too complex (decline):** redesign module, change architecture, refactor approach, "consider using X instead", performance optimization

After all fixes:

```
AskUserQuestion(
  questions=[{
    question: "Fixes ready. How to proceed?",
    header: "LETS",
    options: [
      { label: "Commit & push", description: "git add, commit, push to PR branch" },
      { label: "Commit only", description: "Commit locally, push later" },
      { label: "Review first", description: "Show diff before committing" }
    ],
    multiSelect: false
  }]
)
```

If "Review first": show `git diff`, then ask again.

If "Commit & push" or "Commit only":
```bash
git add -A
git commit -m "fix: Address PR review comments"
```

If "Commit & push": `git push`

Save `fix_commit_sha` in response.json. Update "fix" reply texts: "Fixed in {short_sha}."
Set `fixes_committed: true`.

If no "fix" comments (all agree/disagree/skip): skip this step entirely.

### 6.5 Post replies

Show reply plan (skip entries with `decision: "skip"`):

```
## Reply Plan

| # | Type | Decision | Reply |
|---|------|----------|-------|
| 1 | inline | Fix | "Fixed in abc1234." |
| 2 | inline | Disagree | "This is intentional because..." |
| 3 | general | Agree | "Agreed, good catch." |
```

```
AskUserQuestion(
  questions=[{
    question: "Post {N} replies to PR #{number}?",
    header: "LETS",
    options: [
      { label: "Post all", description: "Post {N} replies to GitHub" },
      { label: "Review individually", description: "Edit each reply before posting" },
      { label: "Save for later", description: "Keep state, post next session" }
    ],
    multiSelect: false
  }]
)
```

If "Review individually": show each reply, allow editing via AskUserQuestion (Other option) before posting.

If "Save for later": save state, exit with resume LETS box.

If "Post all": post each reply by type.

**Inline comments** (threaded reply):
```bash
gh api repos/${REPO}/pulls/{PR}/comments \
  --method POST \
  -F body="{reply_text}" \
  -F in_reply_to={github_id}
```

**General comments** (new top-level comment with quote):
```bash
gh api repos/${REPO}/issues/{PR}/comments \
  --method POST \
  -F body="> {first 2 lines of original}\n\n{reply_text}"
```

**Per-reply error handling:** If a reply fails (404 - comment deleted, 422 - validation error):
1. Log: "Reply to comment {id} failed: {error}. Skipping."
2. Mark entry with error in state, continue with remaining replies.
3. Do NOT abort the batch.

Mark each as `reply_posted: true` and save `reply_posted_id` after successful post. Save state after each reply (idempotent resume).

### 6.6 Update state

After all replies posted:
- Set `replies_posted: true`
- Log to beads:

```bash
bd comments add {task_id} "Responded to PR #{number} review: {N} fixed, {M} agreed, {K} disagreed"
# Skip if task_id is null
```

Switch back to previous branch if we checked out the PR branch in 6.1.

## Step 7: Output

### After Phase 1 (findings shown):
Transition directly to Phase 2 (discuss findings).
No LETS box needed - flow continues.

### After Phase 2 (comments posted):

```
Posted {N} inline + {M} summary to PR #{number}

┌─ LETS ────────────────────────────────┐
│  Follow-up?  /lets:pr --follow-up     │
│  Approve?    /lets:pr --approve       │
└───────────────────────────────────────┘
```

### After Phase 3 (follow-up done):

```
┌─ LETS ─────────────────────────┐
│  Approve?  /lets:pr --approve  │
│  Merge?    /lets:pr --merge    │
└────────────────────────────────┘
```

### After Phase 4 (verdict submitted):

```
PR #{number} {approved/merged/changes requested}

┌─ LETS ─────────────────────────┐
│  Done?  /lets:done             │
│  End?   /lets:end              │
└────────────────────────────────┘
```

### After Phase R (replies posted):

```
Replied to {N} comments on PR #{number} ({X} fixed, {Y} agreed, {Z} disagreed)

┌─ LETS ─────────────────────────┐
│  Done?    /lets:done           │
│  Status?  /lets:pr --status    │
└────────────────────────────────┘
```

### After Phase R (saved for later):

```
Response saved to .lets/execution/pr-{number}/response.json

┌─ LETS ──────────────────────────────┐
│  Resume?  /lets:pr --respond {PR}   │
└─────────────────────────────────────┘
```

### After --status:

```
┌─ LETS ────────────────────────────────┐
│  Resume?     /lets:pr                 │
│  Follow-up?  /lets:pr --follow-up     │
└───────────────────────────────────────┘
```

### After --cancel:

```
Review state cleaned up.

┌─ LETS ─────────────────────────┐
│  New review?  /lets:pr <PR>    │
└────────────────────────────────┘
```

## Rules

- **NEVER post to GitHub without user approval** - every gh api/gh pr command that sends data needs explicit "Post all" or similar confirmation
- **NEVER approve or merge without user confirmation**
- **Save state before every external action** (gh api, gh pr review, gh pr merge)
- **Verify line numbers** against actual diff before posting inline comments (Step 3.3)
- **Use --body-file** for all multiline comment posting (never heredoc)
- **Single JSON payload** for batch inline comments via --input (never mix -f and --input)
- **Fallback on failure** - if gh api fails, offer to post as single comment instead
- **Per-PR folders** - all state and temp files live in `.lets/execution/pr-{number}/`
- **Restore previous branch** on cleanup/cancel/merge
- **Error recovery** - if gh pr checkout fails after stash, pop stash before exiting
- Respond in user's language
