---
description: Finish a task - document, create PR or merge, close
---

# Task Done

Complete the current task. Document work, create PR or merge locally, close in the tracker.

**This is NOT session end.** Use `/lets:end` to end a session. `/lets:done` finishes a TASK.

> **Convention used in this file (per CLAUDE.md "Naming Convention: LETS_*"):**
> - `{LETS_FOO}` placeholder inside ` ```bash ` snippets AND AskUserQuestion strings - the orchestrator substitutes the literal value before running / before the tool call. Required because Bash tool calls are fresh shells (`$LETS_FOO` unset) and AskUserQuestion renders strings literally.
> - `$LETS_FOO` in prose and section headings only - read-only reference to the LETS Config inject. Do NOT use `$LETS_FOO` in bash blocks or AskUserQuestion strings - it silently produces wrong commands or a literal `$LETS_FOO` in the rendered question.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Active Task Detection

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If no task found: ask user which task to close.

### Epic Guard

Check the detected task's type via the tracker's `show` verb (beads exposes `type`; an adapter that doesn't expose a type can't epic-guard - skip it).
If type is **epic** - do NOT close it automatically:
- Inform user: "This is an epic. Epics stay open for future tasks."
- Offer: close a specific child task instead, or confirm epic closure if user insists.

## Trunk-mode Routing

Several steps below have a conditional branch for **trunk-mode** — when HEAD is `$LETS_MERGE_BRANCH` (user opted in via the `take-task` picker option "Stay on current branch"). The check is HEAD-based at runtime via `git branch --show-current` compared to `$LETS_MERGE_BRANCH` from LETS Config; no persistent flag.

In trunk-mode the following gates fire:
- Step 4 commit range: `start:..HEAD` (the task boundary from `.task-<slug>`, not `$LETS_MERGE_BRANCH..HEAD`, which is empty when HEAD IS the merge-branch)
- Already-Merged Guard: **skip** (PR on same-source-target is not a valid PR, nothing to detect)
- Step 6 confirm: trunk-mode wording (push + close, no PR)
- Step 7 completion comment: commit range uses `start:..HEAD` (same reason as Step 4)
- Step 8 finish: upstream-aware push + close (tracker `close` verb; no PR, no merge, no `git branch -d`)
- Step 9 output: trunk-mode "Next" options (no "Merge & close", no "Switch to merge-branch")

Trunk-mode requires `detect-task` (Step 1) to have returned an active task. If no task — abort with the standard "no task" path.

## Step 2: Check Uncommitted Changes

```bash
git status --short
```

If uncommitted changes exist, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "You have uncommitted changes. What to do?",
    header: "Uncommitted",
    options: [
      { label: "Commit first", description: "Run /lets:commit before finishing task" },
      { label: "Skip", description: "Continue without committing (changes stay unstaged)" },
      { label: "Cancel", description: "Stop - go back to working on the task" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Commit first** -> invoke `Skill(skill: "lets:commit")`, then continue
- **Skip** -> warn and continue
- **Cancel** -> stop, return to work

## Already-Merged Guard

**Skip this entire guard if HEAD == `$LETS_MERGE_BRANCH` (trunk-mode):** PR on same-source-target is not a valid PR. Nothing to detect, nothing to short-circuit — proceed directly to Step 3.

If `$LETS_PR_FLOW == github`, the branch may already be merged - the PR was created and merged in a parallel session, so Step 8's `git push` + `gh pr create` would crash with `GraphQL: No commits between ...`.

```bash
git fetch origin --quiet 2>/dev/null
gh pr list --head "$(git branch --show-current)" --state merged --json number,url --limit 1 2>/dev/null
```

If this returns a merged PR, the work already shipped: skip Steps 3-8 and finish via Step 9's "After PR" Merge & close handling - with one change, since the PR is already merged, do NOT run `gh pr merge`; do Step 7's completion comment, then close (tracker `close` verb) + (if not in a worktree) `git checkout {LETS_MERGE_BRANCH} && git pull`. Report the PR number/URL.

Otherwise (no PR, or `gh` unavailable) continue to Step 3 - normal flow.

## Step 3: Verify Task Scope

**Before closing - verify ALL requirements from the task description are met.**

```lets-tracker
show task=<task-id>   # returns {id,title,status,url,description}; read description (+ type on beads) to verify scope
```

Compare the task description against actual changes:

1. Read the full task description and any design/notes fields
2. List each requirement or deliverable mentioned
3. For each one - check if it's actually implemented (read files, grep, verify)
4. Present a checklist to the user:

```
## Scope Verification

Task: **{title}** ({task-id})

- [x] {requirement 1} - done in {file}
- [x] {requirement 2} - done in {file}
- [ ] {requirement 3} - NOT FOUND

{if all done}
All requirements met. Proceeding.
{else}
Missing: {list}. Fix first or update task scope?
```

**If any requirement is missing**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Some requirements are missing. How to proceed?",
    header: "Scope",
    options: [
      { label: "Fix first", description: "Stop closing - go back and implement missing items" },
      { label: "Update scope", description: "Adjust task description to match what was actually done" },
      { label: "PR only, keep open", description: "Create PR but keep task open - remaining work tracked in task" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Fix first** -> stop, do NOT proceed to closing
- **Update scope** -> update the task description via the tracker's `set-field` verb, then proceed
- **PR only, keep open** -> proceed to Step 4. In Step 8, create PR but do NOT close the task. In Step 9, skip "Merge & close" option - user explicitly chose to keep the task open for remaining work.

**Only continue to Step 4 when all requirements are verified OR user chose "PR only, keep open".**

## Step 4: Collect Commits

Use `$LETS_MERGE_BRANCH` from LETS Config. Fallback: `git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo main`

**Guard:** if `$LETS_MERGE_BRANCH` is unset or empty, STOP with error: "LETS_MERGE_BRANCH is not configured. Edit `.lets/.env` or run `/lets:init`. Refusing to proceed - empty value would cause `git checkout` no-op and merge into wrong branch." Do NOT use the fallback for merge/checkout operations - the fallback is for context only (showing the diff against a reasonable base). Merge target must be explicit.

**Lower bound = the task's `start:` boundary** (from `.task-<slug>`), uniform across trunk and feature branches. On the merge-branch `{LETS_MERGE_BRANCH}..HEAD` is empty (HEAD IS the merge-branch), so the recorded `start:` is the only valid bound there; on a feature/worktree branch a missing `start:` falls back to the merge-base diff.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
HEAD_BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$HEAD_BRANCH" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
START=$(sed -n 's/^start: //p' "$TASK_FILE" 2>/dev/null | head -1)
# Ancestry guard: a recorded start that isn't an ancestor of HEAD is stale (rebase/reset/wrong line).
if [ -n "$START" ] && ! git merge-base --is-ancestor "$START" HEAD 2>/dev/null; then
  echo "WARN: recorded task start ($START) is not an ancestor of HEAD - ignoring it." >&2
  START=""
fi
# Back-compat is ASYMMETRIC: the legacy .session-start-ref is a SESSION boundary, NOT a task
# boundary, so it is NOT used as start: here (that conflation is the bug this path fixes).
if [ -z "$START" ]; then
  if [ "$HEAD_BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
    echo "ERROR: no task boundary recorded for this trunk task (started before upgrade, or rebased)." >&2
    echo "{LETS_MERGE_BRANCH}..HEAD is empty on the merge-branch, so the task range can't be inferred. Set it, then re-run /lets:done:" >&2
    echo "  printf 'task: %s\\nstart: %s\\nsession: %s %s\\n' '<task-id>' \"\$(git rev-parse HEAD~N)\" \"\$(git rev-parse HEAD)\" \"\$CLAUDE_CODE_SESSION_ID\" > \"$TASK_FILE\"" >&2
    exit 1
  fi
  RANGE="{LETS_MERGE_BRANCH}..HEAD"   # feature/worktree: merge-base diff (correct, non-empty here)
else
  RANGE="${START}..HEAD"
fi
git log ${RANGE} --oneline
git diff --stat ${RANGE}
```

Show summary:
```
## Task Summary

Commits (N):
- abc1234 feat: Add session restore
- def5678 fix: Handle edge case

Files: X changed, Y insertions, Z deletions
```

## Step 5: Update CHANGELOG

Keep `CHANGELOG.md` in sync with merged work so release notes don't have to be back-filled later. This step runs before the branch is pushed/merged, so the CHANGELOG commit lands in the same PR as the task work.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
test -f "$LETS_PROJECT_ROOT/CHANGELOG.md" && echo "has-changelog" || echo "no-changelog"
```

**If `no-changelog`** (no `CHANGELOG.md` at the project root): tell the user "No `CHANGELOG.md` at the project root - skipping changelog update." and proceed to Step 6. Do nothing else.

**If `has-changelog` but the task is pure infra / tests / internal refactor** (no user-visible change to commands, skills, agents, rules, CLI, or README): say so briefly and proceed to Step 6.

**Otherwise** — draft a one-line entry from the task title + commit subjects, pick the right Keep-a-Changelog section (`Added` / `Changed` / `Fixed` / `Removed`), and prepare to insert it under `[Unreleased]` following the file's existing conventions (section headers, task-id link style). Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Add this entry to CHANGELOG.md [Unreleased]?",
    header: "CHANGELOG",
    options: [
      { label: "Add entry", description: "{drafted entry} - under {section}" },
      { label: "Edit first", description: "Show me the draft, I'll adjust wording before it's written" },
      { label: "Skip", description: "Don't touch CHANGELOG for this task" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Add entry** -> edit `CHANGELOG.md`, then commit it via `/lets:commit` (`docs(<task-id>): update CHANGELOG`). This commit joins the same branch/PR. Then proceed to Step 6.
- **Edit first** -> show the draft, apply the user's wording, then edit + commit as above. Proceed to Step 6.
- **Skip** -> proceed to Step 6, CHANGELOG untouched.

## Step 6: Confirm with User

Show what will happen based on `$LETS_PR_FLOW` from LETS Config:

> **Note:** Conditionals below are binary (`== github` vs `!= github`). When Bitbucket integration lands, every `!= github` branch needs a 3rd case (currently `bitbucket` value falls into local-merge path). Search for `LETS_PR_FLOW != github` to find all sites.

### If HEAD == `$LETS_MERGE_BRANCH` (trunk-mode):

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title} on {LETS_MERGE_BRANCH}?",
    header: "Finish",
    options: [
      { label: "Finish", description: "Push to {LETS_MERGE_BRANCH} and close task (no PR — same-source-target)" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Finish** -> proceed to Step 7
- **Keep working** -> stop, return to work

### If $LETS_PR_FLOW == github:

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "Finish",
    options: [
      { label: "Finish", description: "Push branch and create PR to {LETS_MERGE_BRANCH}" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

### If $LETS_PR_FLOW != github:

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "Finish",
    options: [
      { label: "Finish", description: "Merge to {LETS_MERGE_BRANCH} and delete branch" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

Next steps presented via AskUserQuestion (replaces LETS box).

**Handle response:**
- **Finish** -> proceed to Step 7
- **Keep working** -> stop, return to work

## Step 7: Document in the Tracker

Add completion comment to the task. **MANDATORY:** the `Claude session: $CLAUDE_CODE_SESSION_ID` line MUST appear in the comment between `## Completed` and `### Commits` — don't drop it. `$CLAUDE_CODE_SESSION_ID` is the Bash subprocess env var Claude Code injects (see CLAUDE.md → "Claude Code session identity"); bash expands it inside the heredoc at runtime (the body is written to a temp file, submitted via the `comment-add` verb's `body-file=`), so the tracker receives the literal session UUID. No pre-assignment / template substitution needed.

**Self-contained bash** — computes `RANGE` locally so the comment body is correct regardless of whether Step 4's `START` is still in scope (each Bash tool call is a fresh shell — no cross-Step env). Range from the task's `start:` boundary (uniform, with the same ancestry guard as Step 4); falls back to `$LETS_MERGE_BRANCH..HEAD` on a feature/worktree branch when no `start:` is recorded (on trunk Step 4 already aborted if it was empty). Git operations use bash `$(...)` substitution; only the narrative fields stay as orchestrator-filled `{...}` templates.

The bash block computes the body to a temp file; the `comment-add` verb then submits it via `body-file=` (lets-rules "Tracker Adapters" - no multi-line value crosses into the block; the orchestrator fills the `{...}` narrative fields before running):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
HEAD_BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$HEAD_BRANCH" | tr '/' '-')
START=$(sed -n 's/^start: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}" 2>/dev/null | head -1)
if [ -n "$START" ] && ! git merge-base --is-ancestor "$START" HEAD 2>/dev/null; then START=""; fi
if [ -n "$START" ]; then
  RANGE="${START}..HEAD"
else
  RANGE="{LETS_MERGE_BRANCH}..HEAD"
fi
mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
cat > "$LETS_PROJECT_ROOT/.lets/cache/comment-<task-id>.md" <<EOF
## Completed $(date +%Y-%m-%d)

Claude session: $CLAUDE_CODE_SESSION_ID

### Commits
$(git log $RANGE --oneline)

### Summary
{1-2 sentence overview of what was done}

### Key decisions
- {any important choices made during this task}

### Files changed
$(git diff --stat $RANGE)
EOF
```

```lets-tracker
comment-add task=<task-id> body-file=.lets/cache/comment-<task-id>.md
```

## Step 8: Finish Task

### Worktree Detection

Before finishing, check if we're in a worktree:

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

Set `IN_WORKTREE=true` if `$GIT_DIR` contains `worktrees/` (is NOT `.git`).

If in a worktree, resolve the main repo path:

```bash
MAIN_ROOT=$(cd "$(git rev-parse --git-common-dir)/.." 2>/dev/null && pwd)
```

### If HEAD == `$LETS_MERGE_BRANCH` (trunk-mode):

Push any unpushed commits, then close the task. No PR, no merge, no branch deletion.

**Upstream-aware push** — first-push case (no upstream configured) must NOT silently no-op. The naive `git log @{u}..HEAD 2>/dev/null` returns 0 commits when upstream is unset, which would skip push entirely while the close still runs — leaving the task marked done with work only on the local clone. This block detects upstream first:

```bash
if git rev-parse --abbrev-ref @{u} >/dev/null 2>&1; then
  # Upstream exists — push only when ahead
  UNPUSHED=$(git log @{u}..HEAD --oneline | wc -l | tr -d ' ')
  if [ "$UNPUSHED" -gt 0 ]; then
    git push origin {LETS_MERGE_BRANCH}
  fi
else
  # First push from this clone — set upstream
  git push -u origin {LETS_MERGE_BRANCH}
fi
```

```lets-tracker
close task=<task-id> reason="Trunk-mode: committed on {LETS_MERGE_BRANCH}, no PR"
```

Then drop the closed task's boundary (the close is a state change - HARD-FAIL loud if the binding can't run; do NOT proceed to cleanup if the close failed):

```bash
# Cleanup (B4): task closed, but the trunk branch lives on (it hosts more tasks). Drop the closed
# task's task:/start:, KEEP session: so /lets:end still has a valid session boundary. Do NOT rm the
# whole file — the next claim overwrites task:/start:, and a stray rm would strand /lets:end.
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH_SLUG=$(echo "$(git branch --show-current)" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
if [ -f "$TASK_FILE" ]; then
  SID_LINE=$(sed -n 's/^session: //p' "$TASK_FILE" | head -1)
  [ -z "$SID_LINE" ] && SID_LINE="$(git rev-parse HEAD) $CLAUDE_CODE_SESSION_ID"
  tmp=$(mktemp "${TASK_FILE}.XXXX"); printf 'session: %s\n' "$SID_LINE" > "$tmp" && mv -f "$tmp" "$TASK_FILE"
fi
```

After this, skip the `### If $LETS_PR_FLOW == github (PR flow)` / `!= github` blocks below — they don't apply in trunk-mode.

### If $LETS_PR_FLOW == github (PR flow):

**Guard: verify gh CLI first**

```bash
gh auth status 2>&1
```

If gh is not installed or not authenticated, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "gh CLI is not available but LETS_PR_FLOW=github. What to do?",
    header: "gh CLI",
    options: [
      { label: "Local merge", description: "Fall back to local merge for this task" },
      { label: "Cancel", description: "Stop - fix gh auth first (gh auth login)" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Local merge** -> jump to "If $LETS_PR_FLOW != github" section below
- **Cancel** -> stop, return to work

**If gh is available, proceed with PR:**

```bash
# Push branch
git push -u origin <branch>

# Create PR
gh pr create --title "<type>: <task title>" --body "$(cat <<'EOF'
## Summary
{task description from the tracker}

## Changes
{git log {LETS_MERGE_BRANCH}..HEAD --oneline}

## Task
{task-id}: {title}

---
Generated with LETS plugin
EOF
)"
```

After PR created:
```lets-tracker
comment-add task=<task-id> body="PR #XX created: <PR URL>"
```

Task stays **open** until PR is merged.

**Do NOT switch branches yet** - user decides in Step 9.

### If $LETS_PR_FLOW != github (local merge) AND NOT in worktree:

Use `$LETS_MERGE_BRANCH` from LETS Config for target branch.

```bash
git checkout {LETS_MERGE_BRANCH}
git merge <branch>
git branch -d <branch>
```

After merge:
```lets-tracker
close task=<task-id> reason="Merged locally. Commits: {list}"
```

### If $LETS_PR_FLOW != github (local merge) AND in worktree:

Cannot `git checkout` or `git branch -d` from inside a worktree. Use `git -C` to operate on the main repo:

Use `$LETS_MERGE_BRANCH` from LETS Config for target branch.

```bash
MAIN_ROOT=$(cd "$(git rev-parse --git-common-dir)/.." 2>/dev/null && pwd)
BRANCH=$(git branch --show-current)

# Ensure main repo is on the merge branch before merging
MAIN_CURRENT=$(git -C "$MAIN_ROOT" branch --show-current)
if [ "$MAIN_CURRENT" != "{LETS_MERGE_BRANCH}" ]; then
  git -C "$MAIN_ROOT" checkout {LETS_MERGE_BRANCH}
fi

git -C "$MAIN_ROOT" merge "$BRANCH"
```

After merge:
```lets-tracker
close task=<task-id> reason="Merged locally from worktree. Commits: {list}"
```

Do NOT delete the branch or remove the worktree here - `/lets:worktree remove` handles cleanup.

## Step 9: Output

### After trunk-mode finish (HEAD == `$LETS_MERGE_BRANCH`):

```
Task: **{title}** ({task-id}) - CLOSED
Branch: {LETS_MERGE_BRANCH} (trunk-mode, no PR)
Pushed: {N} commits to origin/{LETS_MERGE_BRANCH}
```

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "Next step",
    options: [
      { label: "Next task", description: "Pick and claim another task via take-task skill" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Next task** -> show the tracker's `ready` view (top 5), ask user to pick. When picked: invoke `Skill(skill: "lets:take-task", args: "<task-id>")` for status update + branch setup. Do NOT inline take-task logic.
- **End session** -> invoke `Skill(skill: "lets:end")`

### After PR ($LETS_PR_FLOW == github), NOT in worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: open (close after PR merge)
```

**If user chose "PR only, keep open" in Step 3**, skip "Merge & close" - task has remaining work. Use same AskUserQuestion below but WITHOUT the "Merge & close" option.

**Normal flow** - use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "Next step",
    options: [
      { label: "Merge & close", description: "Merge PR #{number}, close task, switch to {LETS_MERGE_BRANCH}" },
      { label: "Stay on branch", description: "Stay on feature branch - for PR fixes or follow-up work" },
      { label: "Next task", description: "Switch to {LETS_MERGE_BRANCH}, then claim another task via take-task" },
      { label: "End session", description: "Switch to {LETS_MERGE_BRANCH}, run /lets:end" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Merge & close**:
  1. `gh pr merge {number} --squash --delete-branch`
  2. close the task (tracker `close` verb)
  3. `git checkout {LETS_MERGE_BRANCH} && git pull`
  4. If merge fails (conflicts, checks not passed) -> inform user, fall back to "Stay on branch"
- **Stay on branch** -> stay on current branch, no checkout. User continues working freely.
- **Next task** -> `git checkout {LETS_MERGE_BRANCH}`, then show the tracker's `ready` view (top 5). When user picks: invoke `Skill(skill: "lets:take-task", args: "<task-id>")` for status update + branch setup.
- **End session** -> `git checkout {LETS_MERGE_BRANCH}`, then invoke `Skill(skill: "lets:end")`

### After PR ($LETS_PR_FLOW == github), IN worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: open (close after PR merge)
Worktree: {worktree path}
```

**If user chose "PR only, keep open" in Step 3**, skip "Merge & close" - task has remaining work. Use same AskUserQuestion below but WITHOUT the "Merge & close" option.

**Normal flow** - use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "PR created. What's next?",
    header: "Next step",
    options: [
      { label: "Merge & close", description: "Merge PR #{number}, close task" },
      { label: "Stay here", description: "Stay in this worktree for PR fixes or follow-up" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

No "Next task" option - can't switch branches in a worktree. To start a new task, user opens a different terminal.

**Handle response:**
- **Merge & close**:
  1. `gh pr merge {number} --squash --delete-branch`
  2. close the task (tracker `close` verb)
  3. If merge fails (conflicts, checks not passed) -> inform user, fall back to "Stay here"
  4. After merge, remind: "Worktree can be removed: `/lets:worktree remove {name}` from the main repo terminal."
- **Stay here** -> stay in worktree. User continues working.
- **End session** -> invoke `Skill(skill: "lets:end")`. After the end-session flow completes, remind:
  "After PR merges, clean up: `/lets:worktree remove {name}` from the main repo terminal."

### After local merge ($LETS_PR_FLOW != github), NOT in worktree:

```
Task: **{title}** ({task-id}) - CLOSED
Merged to {LETS_MERGE_BRANCH}
Branch {feature-branch} deleted
```

Already on `$LETS_MERGE_BRANCH` after merge. Use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "Next step",
    options: [
      { label: "Next task", description: "Pick and claim another task via take-task skill" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Next task** -> show the tracker's `ready` view (top 5), ask user to pick. When picked: invoke `Skill(skill: "lets:take-task", args: "<task-id>")` for status update + branch setup.
- **End session** -> invoke `Skill(skill: "lets:end")`

### After local merge ($LETS_PR_FLOW != github), IN worktree:

```
Task: **{title}** ({task-id}) - CLOSED
Merged to {LETS_MERGE_BRANCH} (from worktree via git -C)
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Merged. Clean up worktree?",
    header: "Cleanup",
    options: [
      { label: "Remove worktree", description: "Switch to main repo and run /lets:worktree remove {name}" },
      { label: "Keep working", description: "Stay in the worktree and keep working - clean up later with /lets:worktree remove" },
      { label: "End session", description: "Run /lets:end to save context and wrap up - worktree stays for later removal" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Remove worktree** -> inform: "Switch to main repo terminal and run `/lets:worktree remove {name}`" (cannot remove worktree from inside it)
- **Keep working** -> no session change; continue working in the worktree. Remind: clean up later with `/lets:worktree remove {name}` from the main repo.
- **End session** -> invoke `Skill(skill: "lets:end")` to save session context (the worktree stays; remove it later from the main repo)

## Rules

- **NEVER push or create PR without user approval**
- **NEVER merge without user approval**
- Document BEFORE finishing (Step 7 before Step 8)
- If PR flow: task stays open, user closes after merge
- If local merge: task closes immediately
- If HEAD == `$LETS_MERGE_BRANCH` (trunk-mode): skip PR creation (same-source-target is not a valid PR), push (upstream-aware) + close (tracker `close` verb) instead — regardless of `$LETS_PR_FLOW`
- Respond in user's language
