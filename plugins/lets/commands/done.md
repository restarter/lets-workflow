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
- Existing-PR Guard: **skip** (PR on same-source-target is not a valid PR, nothing to detect)
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

## Existing-PR Guard

**Skip this entire guard if HEAD == `$LETS_MERGE_BRANCH` (trunk-mode):** PR on same-source-target is not a valid PR. Nothing to detect, nothing to short-circuit — proceed directly to Step 3. Skip it too when `$LETS_PR_FLOW` is neither `github` nor `bitbucket` (a local merge opens no PR).

A branch can already have a PR: still open (a re-run after "Stay on branch"), merged (a parallel session or the browser shipped it, or the branch is reused for new work), or declined. Ask the forge for EVERY PR of this branch before anything is pushed, and decide by COMMITS - never by the branch name, never against `{LETS_MERGE_BRANCH}` (a squash merge leaves the branch's commits outside it).

A PR **holds HEAD** when its head is HEAD (an abbreviated hash: a prefix of HEAD) or HEAD is an ancestor of it: `git merge-base --is-ancestor HEAD <pr-head>`. Only a merged PR is asked, and the answer is yes, no, or cannot tell (its head does not resolve locally - on github fetch `refs/pull/<number>/head` first). An open PR needs no such test: Step 8's plain push decides.

**Lookup** - read-only, after `git fetch origin --quiet`. Only PRs whose head lives in the repository `git push origin` updates count - parse its owner/name from `git remote get-url --push origin` (the push URL - the fetch URL can name another repository); origin with more than one push URL cannot be vouched for (`git remote get-url --push --all origin` lists them). A PR from any other repository that shares the branch name is not this branch's PR and is ignored:

- `github` - every PR whose head is this branch, in all states, keeping only those whose head repository is the one `git push origin` updates (`headRepository` `nameWithOwner` equal to the owner/name parsed from `git remote get-url --push origin` - gh may be set to query another repository by default, so never trust a same-repository flag), with number, URL, state, base and head sha. All of them: pass a `--limit` well above the count you expect; a result count equal to the limit may be incomplete. Light example: `gh pr list --head "$(git branch --show-current)" --state all --limit 200 --json number,url,state,baseRefName,headRefOid,headRepository`.
- `bitbucket` - `bbb pr list` has no branch filter, so read-only `bbb raw` GETs on the repository-relative `pullrequests` endpoint, filtered by source branch name and asking for every state (Bitbucket lists only OPEN otherwise), following every page (the response's `next` link) until there is none, keeping only PRs whose source repository full name is the workspace/slug parsed from `git remote get-url --push origin` (the repository `git push origin` updates). Read each PR's id, state, URL, destination branch and source commit hash (it may be abbreviated - resolve it with `git rev-parse --verify`). Current syntax: `bbb help raw`.

| Forge CLI | Then |
|---|---|
| `gh` / `bbb` not installed, or `gh` not logged in | No lookup - continue. Nothing can open a PR without it; Step 8's own check offers local merge or cancel. |
| A lookup that ran and failed, came back incomplete or unreadable, or cannot tell a PR's source repository - or origin's push URL does not parse into owner/name, or origin has more than one push URL | **STOP before any push:** "Could not get every PR of this branch from {GitHub / Bitbucket} - nothing pushed. Fix the lookup and re-run `/lets:done`." A failed lookup never reads as "no PR" - that is how the duplicate PR happened. |
| Answered, complete | The decision table below. |

Decision over the PRs the lookup kept - first matching row wins, so an open PR always wins. Carry the outcome, the PR number and its URL to Steps 5, 6, 8 and 9:

| PRs of this branch | Outcome | Then |
|---|---|---|
| Exactly one open PR, into `{LETS_MERGE_BRANCH}` | `open` #N | Continue to Step 3. Step 8 pushes to PR #N and never opens a second one; #N is the PR Step 9 acts on. |
| More than one open PR, or the only open one targets another base | stop | Name them: "This branch already has an open PR `/lets:done` will not reuse (another base, or more than one) - close or retarget it and re-run." Never open a second PR, never force-push. |
| No open PR; a merged PR into `{LETS_MERGE_BRANCH}` holds HEAD | `shipped` #N | Every commit already shipped. Continue to Step 3; Step 5 and Step 8's push and create are skipped, and Step 8 closes the task after Finish. |
| Anything else: no PR, a declined / closed-unmerged one, a merged one into another base (a stacked PR - not shipped to `{LETS_MERGE_BRANCH}`), a merged one that does not hold HEAD (new work on a reused branch), or a merged one whose head cannot be resolved (cannot tell - never close what is not proven shipped) | `fresh` | Continue to Step 3. Step 8 opens a new PR; Step 6 names any earlier PR and why. |

## Step 3: Verify Task Scope

**Before closing - verify ALL requirements from the task description are met.**

**Tracker protocol:** `Skill(skill: "lets:protocol-tracker")` before this and any later `lets-tracker` block here, unless its text is in your current context.

```lets-tracker
show task=<task-id>   # returns {id,title,status,url,description}; read description - plus any field the adapter's `show` declares (beads: `type`) - to verify scope
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

**If any requirement is missing**, use **AskUserQuestion**. Add the "Ask orchestrator" option per the Orchestrator offer protocol (Act shape; load `Skill(skill: "lets:protocol-orchestrator-offer")` here unless its text is in your current context; not loaded -> no offer).

```
AskUserQuestion(
  questions=[{
    question: "Some requirements are missing. How to proceed?",
    header: "Scope",
    options: [
      { label: "Fix first", description: "Stop closing - go back and implement missing items" },
      { label: "Update scope", description: "Adjust task description to match what was actually done" },
      { label: "PR only, keep open", description: "Create PR but keep task open - remaining work tracked in task" },
      { label: "Ask orchestrator", description: "Stay at this gate; /lets:orc ask with the missing requirements" }  /* only per Orchestrator offer */
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Fix first** -> stop, do NOT proceed to closing
- **Update scope** -> update the task description via the tracker's `set-field` verb, then proceed
- **PR only, keep open** -> proceed to Step 4. In Step 8, create the PR (or push to the open one the Existing-PR Guard found; on its `shipped` outcome nothing is pushed) but do NOT close the task. In Step 9, skip "Merge & close" option - user explicitly chose to keep the task open for remaining work.
- **Ask orchestrator** -> `Skill(skill: "lets:orc", args: "verb=ask footer=none text=Closing {task-id} with requirements missing: {list}. Fix first, update the scope, or PR and keep open?")`. After the reply is relayed, show this gate again without that option - the user picks.

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

**Park commits.** A `wip(<id>): park` (or `wip: park`) subject in `git log --format='%h %s' ${RANGE}` is a commit `lets worktree switch --park` made and never unparked. It is a WARNING, never a refusal - name each one (`<sha> <subject>`). A pushed park commit can only be removed by a force push, so pushing or merging it is the owner's explicit call, asked once, before Step 6:

```
AskUserQuestion(
  questions=[{
    question: "The range holds {N} park commit(s): {sha subject, ...}. Push / merge them as they are?",
    header: "Park commit",
    options: [
      { label: "Keep working", description: "Stop; fold or drop the park commits first" },
      { label: "Push them anyway", description: "Finish with the park commits in history for good" }
    ],
    multiSelect: false
  }]
)
```

**Keep working** -> stop, return to work. **Push them anyway** -> continue; the Step 6 gate still asks as usual. No park commit -> nothing here.

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

**If the Existing-PR Guard's outcome is `shipped`:** skip this step - nothing can be added to a merged PR.

**If `[Unreleased]` already has an entry naming this task id** (a re-run - e.g. more commits for an open PR): show that entry and ask only whether to update it (**Edit first**) or leave it (**Skip**) - drop **Add entry**; never write a second entry for the same task.

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

> **Note:** Three cases below - `github` (push + PR via `gh`), `bitbucket` (push + PR via `bbb`, mirrors github), and `local` (local merge). `local` is also the **fallback** for any unrecognized `LETS_PR_FLOW` value, so an unknown value never routes nowhere.

**Existing-PR Guard outcome** (github / bitbucket). `open` -> the Finish description reads `Push any new commits to PR #{number} - no new PR`. `shipped` -> `Close the task - PR #{number} is already merged, nothing to push`. `fresh` after an earlier PR -> append ` - PR #{old} was declined, this opens a new one` / ` - PR #{old} merged earlier, this is new work` / ` - PR #{old} merged into {base}, not {LETS_MERGE_BRANCH}` / ` - PR #{old} could not be checked against HEAD`. Header and labels stay `Finish` / `Keep working`.

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

### If $LETS_PR_FLOW == bitbucket:

```
AskUserQuestion(
  questions=[{
    question: "Ready to finish {task title}?",
    header: "Finish",
    options: [
      { label: "Finish", description: "Push branch and create a Bitbucket PR to {LETS_MERGE_BRANCH}" },
      { label: "Keep working", description: "Not done yet - go back to the task" }
    ],
    multiSelect: false
  }]
)
```

### If $LETS_PR_FLOW == local (or any unrecognized value):

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

The bash block computes the body to a temp file; the `comment-add` verb then submits it via `body-file=` (`lets:protocol-tracker` "Bodies" - no multi-line value crosses into the block; the orchestrator fills the `{...}` narrative fields before running):

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

### Team worktree: promote to Standing knowledge

Only when `lets worktree info --json` reports a `team`; otherwise skip this subsection. Before the close in Step 8, list the candidate facts - from the team file's `## 8. Decisions` and this task's notes and comments - that a future task of this team should know, then one separate question - at most 3 candidates (more -> the 3 strongest, the rest named in the question text) and a last "None" option:

```
AskUserQuestion(
  questions=[{
    question: "Promote any of these to the team's Standing knowledge?",
    header: "Knowledge",
    options: [
      { label: "{fact 1, one line}", description: "{where it came from}" },
      { label: "{fact 2, one line}", description: "{where it came from}" },
      { label: "None", description: "Promote nothing" }
    ],
    multiSelect: true
  }]
)
```

The lead appends each picked fact to the team file's `## 9. Standing knowledge` (`.lets/teams/<team>.md`; the lead is its only writer). "None" (or nothing picked) -> nothing written. It never blocks the close.

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

Read the status `close` returned. `closed` -> report the task closed. Another status -> the board does not permit a close from here and advanced the task instead: report "**{title}** (`{id}`) advanced to {status}, not closed - a human carries it the rest of the way", and do NOT describe the task as done. No status at all -> no tracker is configured; say that, and claim neither. A close that FAILED is not a status - HARD-FAIL per the Degradation rule.

Then drop the closed task's boundary (the close is a state change - HARD-FAIL loud if the binding can't run; do NOT proceed to cleanup if the close failed):

```bash
# Cleanup (B4): task closed, but the trunk branch lives on (it hosts more tasks). Drop the closed
# task's task:/start:/origin:, KEEP session: and every other line so /lets:end still has a valid
# session boundary. Do NOT rm the whole file - the next claim overwrites task:/start:, and a stray
# rm would strand /lets:end. `lets worktree task-state` owns the file (locked, validated); without
# the binary, remove only this step's own keys.
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH_SLUG=$(echo "$(git branch --show-current)" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
if [ -f "$TASK_FILE" ]; then
  SESSION_ARGS=""
  grep -q '^session: ' "$TASK_FILE" || SESSION_ARGS="--session-sha $(git rev-parse HEAD) --session-id $CLAUDE_CODE_SESSION_ID"
  if command -v lets >/dev/null 2>&1; then
    lets worktree task-state set --clear-task $SESSION_ARGS --json
  else
    tmp=$(mktemp "${TASK_FILE}.XXXX")
    { grep -v -e '^task: ' -e '^start: ' -e '^origin: ' "$TASK_FILE"; grep -q '^session: ' "$TASK_FILE" || printf 'session: %s %s\n' "$(git rev-parse HEAD)" "$CLAUDE_CODE_SESSION_ID"; } > "$tmp"; mv -f "$tmp" "$TASK_FILE"
  fi
fi
```

After this, skip the `### If $LETS_PR_FLOW == github / == bitbucket / == local` blocks below — they don't apply in trunk-mode.

### If the Existing-PR Guard's outcome is `shipped`:

PR #{number} is merged into `{LETS_MERGE_BRANCH}` and holds every commit - nothing is pushed, no PR is created. Close the task - skip only the close when Step 3 chose "PR only, keep open" (the task keeps its remaining scope):

```lets-tracker
close task=<task-id> reason="Shipped in merged PR #{number}"
```

Read the status `close` returned exactly as the local-merge block below does (`closed` / advanced / no tracker / HARD-FAIL). If not in a worktree: `git checkout {LETS_MERGE_BRANCH} && git pull`. Then skip the `### If $LETS_PR_FLOW == github / == bitbucket / == local` blocks below.

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
- **Local merge** -> jump to the "If $LETS_PR_FLOW == local / fallback (local merge)" section below
- **Cancel** -> stop, return to work

**If gh is available, proceed with PR.**

**Re-check before any push.** Repeat the Existing-PR Guard's lookup (read-only) - the gates since it ran can take minutes, and a PR may have been opened, merged or replaced meanwhile. The same outcome and the same PR number as Step 6 confirmed (for `fresh`: still no PR this branch could reuse) -> go on; a new local commit, such as Step 5's CHANGELOG entry, does not change the outcome by itself. Anything else - another outcome, another PR number, or no answer -> STOP before any push, nothing pushed: "PR state changed since you confirmed (now: {outcome} #N) - re-run `/lets:done`." Never loop back into an earlier step.

**Outcome `open` (PR #N):** never `gh pr create`. Push with a plain `git push origin <branch>`, never `--force`: "Everything up-to-date" means nothing to push; a non-fast-forward rejection means STOP here with git's message. PR #N is the PR from here on - the read-back below runs on it.

**Outcome `fresh`:** push the branch and create the PR:

```bash
# Push branch
git push -u origin <branch>

# Create PR
gh pr create --base "{LETS_MERGE_BRANCH}" --title "<type>: <task title>" --body "$(cat <<'EOF'
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

**Read back what was actually created (MANDATORY - never report a PR you have not looked at).** A URL from `gh pr create` proves the PR exists, not that it can be merged or even built: a PR born CONFLICTING gets no merge ref, `pull_request` workflows run against that ref, so **not one check ever starts** and `gh pr checks` answers "no checks reported" - indistinguishable from a repo with no CI. That state survived a full `/lets:done` and two more pushes before anyone noticed (lets-ufiam, PR #153).

```bash
PR=<number from the create output, or the open PR's number from the guard>
# Mergeability is computed asynchronously - UNKNOWN right after create is normal, not clean.
# Retry a couple of times; a persistent UNKNOWN is "could not determine", NEVER "fine".
for i in 1 2 3; do
  OUT=$(gh pr view "$PR" --json mergeable,mergeStateStatus --jq '.mergeable + " " + .mergeStateStatus' 2>/dev/null)
  case "$OUT" in UNKNOWN*) sleep 2 ;; *) break ;; esac
done
echo "MERGEABLE=${OUT%% *}  STATE=${OUT##* }"
```

Report what came back. The clean case costs ONE line; every other case is stated out loud:

| `mergeable` | `mergeStateStatus` | say |
|---|---|---|
| `MERGEABLE` | `CLEAN` / `HAS_HOOKS` | one line - PR #N is mergeable |
| `MERGEABLE` | `UNSTABLE` | mergeable, but checks are still running or already failing - name which |
| `MERGEABLE` | `BLOCKED` / `BEHIND` / `DIRTY` / `DRAFT` | name the state and what it blocks - never swallow it |
| `CONFLICTING` | any | conflicts with `{LETS_MERGE_BRANCH}`, and **CI WILL NOT RUN AT ALL until they are resolved** - fix with `git merge {LETS_MERGE_BRANCH}` (or a rebase) and push. The second sentence is the load-bearing one: "there are conflicts" alone does not tell the user their checks are silently absent |
| `UNKNOWN`, or the read failed / returned nothing | any | mergeability could not be determined - report it as **unverified**, never imply the PR is fine |

This is an additive READ: no new gate, no branch switch, nothing is skipped on a bad result. The user is told; Step 9 proceeds as normal.

Orca card, best effort (single-quoted values):

```bash
[ "{LETS_LAUNCHER}" = "orca" ] && lets orca card --phase pr --comment 'PR #{number} {PR URL}' --json 2>/dev/null || true
```

Record the PR on the task:
```lets-tracker
comment-add task=<task-id> body="PR #XX: <PR URL>"
```

The task is now waiting on review rather than being worked on. Advance it ONLY if the active adapter's `## Neutral statuses` section names `in_review`:

```lets-tracker
set-status task=<task-id> status=in_review
```

If it does not name `in_review`, skip this call silently - the task stays where it is and nothing was lost. Do not infer support from the tracker's name; read the adapter's own status list. On a board that does carry it, this is the whole handoff: the task lands in the review column and a human takes it from there.

Task stays **open** until PR is merged.

**Do NOT switch branches yet** - user decides in Step 9.

### If $LETS_PR_FLOW == bitbucket (PR flow):

Mirror the github flow via `bbb`. First check bbb is available (its `pr list` takes only `--state`/`--author` - there is no `--limit` and no branch filter, so don't pass one). If it isn't, offer the same fallback github does: fall back to a local merge (warn it bypasses review) or cancel and fix bbb first.

**Re-check before any push.** Repeat the Existing-PR Guard's lookup exactly as the github branch does: the same outcome and the same PR number as Step 6 confirmed -> go on; another outcome, another PR number, or no answer -> STOP before any push, nothing pushed, and ask for a re-run of `/lets:done`.

**Outcome `open` (PR #N):** never create a PR. Push with a plain `git push origin <branch>`, never `--force`: "Everything up-to-date" means nothing to push; a non-fast-forward rejection means STOP here with git's message. PR #N and its URL are the PR from here on.

**Outcome `fresh`:** push the branch, then create the PR with bbb targeting `{LETS_MERGE_BRANCH}` - a title and a body built from the task, the same content the github branch builds - and take its id and URL from what bbb returns.

Record the PR on the task:

```lets-tracker
comment-add task=<task-id> body="Bitbucket PR #XX: <PR URL>"
```

The task is now waiting on review rather than being worked on. Advance it ONLY if the active adapter's `## Neutral statuses` section names `in_review`:

```lets-tracker
set-status task=<task-id> status=in_review
```

If it does not name `in_review`, skip this call silently - the task stays where it is and nothing was lost. Do not infer support from the tracker's name; read the adapter's own status list.

The task stays **open** until the PR is merged - no local merge, no branch delete, no close. **Do NOT switch branches yet** - user decides in Step 9.

### If $LETS_PR_FLOW == local / fallback (local merge) AND NOT in worktree:

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

Read the status `close` returned. `closed` -> report the task closed. Another status -> the board does not permit a close from here and advanced the task instead: report "**{title}** (`{id}`) advanced to {status}, not closed - a human carries it the rest of the way", and do NOT describe the task as done. No status at all -> no tracker is configured; say that, and claim neither. A close that FAILED is not a status - HARD-FAIL per the Degradation rule.

### If $LETS_PR_FLOW == local / fallback (local merge) AND in worktree:

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

Read the status `close` returned. `closed` -> report the task closed. Another status -> the board does not permit a close from here and advanced the task instead: report "**{title}** (`{id}`) advanced to {status}, not closed - a human carries it the rest of the way", and do NOT describe the task as done. No status at all -> no tracker is configured; say that, and claim neither. A close that FAILED is not a status - HARD-FAIL per the Degradation rule.

Do NOT delete the branch or remove the worktree here - `/lets:worktree remove` handles cleanup.

## Step 9: Output

**Session record (every variant, before its gate).** The work is finished now, and nothing guarantees a later `/lets:end`: the session may stop at "Stay here" or "Merge & close", and a merge with `--delete-branch` from another checkout removes a linked worktree outside Orca's archive hook (gh >= 2.99). Write the record now, whatever is picked below:

`Skill(skill: "lets:session-snapshot", args: "kind=session pointer=off task-id={task-id}")`

`pointer=off` - this command writes the task-side record itself. Print one line `Session record: {snapshot path}` (the skill's Return), then the variant's output. "End session" still runs `/lets:end`, whose `kind=end` snapshot supersedes this one.

**Orca worktrees.** When `{LETS_LAUNCHER}` is `orca`, every "`/lets:worktree remove {name}`" reminder below reads "archive the worktree in Orca - its hook records the release": `remove` refuses a worktree outside `.worktrees/` (`worktree_external`). This command's own `gh pr merge --delete-branch` is safe - gh leaves the current and the main worktree in place; only a merge run from another checkout removes a worker's linked worktree (Read `${CLAUDE_PLUGIN_ROOT}/protocol/worktrees.md` "A worktree can vanish outside Orca" before any merge of a worker's PR run from another checkout).

**Orca card on a confirmed close.** Wherever a handler below (or Step 8 on the Existing-PR Guard's `shipped` outcome) ran `close` and it returned `closed` - not an advance, not a failure:

```bash
[ "{LETS_LAUNCHER}" = "orca" ] && lets orca card --phase closed --comment 'task {task-id} closed' --json 2>/dev/null || true
```

**Which PR.** In every "After PR" variant below, `#{number}` / `{PR URL}` is the PR Step 8 created - or, on the Existing-PR Guard's `open` outcome, the open PR it pushed to (the number the Step 8 re-check confirmed); never a second one. Its `PR:` line then reads `#{number} - {PR URL} (existing PR)`, and "Merge & close" merges that PR. On the `shipped` outcome there is no "After PR" variant: use the matching `### After local merge` one, and replace its `Merged to ...` and `Branch ... deleted` lines with `PR: #{number} - {PR URL} (already merged)` - this path merged and deleted nothing. When Step 3 chose "PR only, keep open", no close ran: its Task line reads `- kept open (remaining scope)` instead of a close status.

### After trunk-mode finish (HEAD == `$LETS_MERGE_BRANCH`):

```
Task: **{title}** ({task-id}) - {CLOSED, or "advanced to {status}" when close returned another status, or "unchanged - no tracker" when it returned none}
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

### Ping offer (every "After PR" variant below)

Before the variant's "Next step" question, resolve this chat's orchestrator (skip entirely under AUTO MODE):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
command -v lets >/dev/null 2>&1 && lets peers orchestrator --session "$CLAUDE_CODE_SESSION_ID" --cwd "$LETS_PROJECT_ROOT" --json 2>/dev/null
```

When `source` is `bound` or `single` and `target.alive` is `alive`, ask the variant's question and this one in the SAME call (for `ambiguous`, `none`, `self`, a dead target or no binary: no Ping question - the orc skill's own ping asks which):

```
AskUserQuestion(
  questions=[
    { /* the variant's own "Next step" question, unchanged */ },
    {
      question: "Ping the orchestrator ({target.name}) about PR #{number}?",
      header: "Ping",
      options: [
        { label: "Ping orchestrator", description: "Run /lets:orc ping PR #{number} {PR URL}" },
        { label: "Skip", description: "Nothing is sent" }
      ],
      multiSelect: false
    }
  ]
)
```

- **Ping orchestrator** -> `Skill(skill: "lets:orc", args: "verb=ping footer=none text=PR #{number} {PR URL}")`, then handle the "Next step" answer as usual.

### After PR ($LETS_PR_FLOW == github), NOT in worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Mergeable: {the read-back line from Step 8 - CONFLICTING also says CI will not run at all}
Status: {in_review if the advance ran, else open} (close after PR merge)
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
  2. close the task (tracker `close` verb) - read what it returned: `closed` means closed; another status means the board advanced the task instead, so report the handoff and do NOT call it done; no status means no tracker; a failed close HARD-FAILs
  3. `git checkout {LETS_MERGE_BRANCH} && git pull`
  4. If merge fails (conflicts, checks not passed) -> inform user, fall back to "Stay on branch"
- **Stay on branch** -> stay on current branch, no checkout. User continues working freely.
- **Next task** -> `git checkout {LETS_MERGE_BRANCH}`, then show the tracker's `ready` view (top 5). When user picks: invoke `Skill(skill: "lets:take-task", args: "<task-id>")` for status update + branch setup.
- **End session** -> `git checkout {LETS_MERGE_BRANCH}`, then invoke `Skill(skill: "lets:end")`

### After PR ($LETS_PR_FLOW == github), IN worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Mergeable: {the read-back line from Step 8 - CONFLICTING also says CI will not run at all}
Status: {in_review if the advance ran, else open} (close after PR merge)
Worktree: {worktree path}
```

**If user chose "PR only, keep open" in Step 3**, skip "Merge & close" - task has remaining work. Use same AskUserQuestion below but WITHOUT the "Merge & close" option.

**Normal flow** - use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "PR #{number} ready. What's next?",
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
  2. close the task (tracker `close` verb) - read what it returned: `closed` means closed; another status means the board advanced the task instead, so report the handoff and do NOT call it done; no status means no tracker; a failed close HARD-FAILs
  3. If merge fails (conflicts, checks not passed) -> inform user, fall back to "Stay here"
  4. After merge, remind: "Worktree can be removed: `/lets:worktree remove {name}` from the main repo terminal."
- **Stay here** -> stay in worktree. User continues working.
- **End session** -> invoke `Skill(skill: "lets:end")`. After the end-session flow completes, remind:
  "After PR merges, clean up: `/lets:worktree remove {name}` from the main repo terminal."

### After PR ($LETS_PR_FLOW == bitbucket), NOT in worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: {in_review if the advance ran, else open} (the reviewer merges on Bitbucket)
```

No "Merge & close" - a Bitbucket PR is merged on the platform by the reviewer, not by `/lets:done`; that is the whole point (don't bypass team review).

```
AskUserQuestion(
  questions=[{
    question: "Task done. What's next?",
    header: "Next step",
    options: [
      { label: "Stay on branch", description: "Stay on feature branch - for PR fixes or follow-up work" },
      { label: "Next task", description: "Switch to {LETS_MERGE_BRANCH}, then claim another task via take-task" },
      { label: "End session", description: "Switch to {LETS_MERGE_BRANCH}, run /lets:end" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Stay on branch** -> stay on current branch, no checkout.
- **Next task** -> `git checkout {LETS_MERGE_BRANCH}`, then show the tracker's `ready` view (top 5); on pick, invoke `Skill(skill: "lets:take-task", args: "<task-id>")`.
- **End session** -> `git checkout {LETS_MERGE_BRANCH}`, then invoke `Skill(skill: "lets:end")`.

### After PR ($LETS_PR_FLOW == bitbucket), IN worktree:

```
Task: **{title}** ({task-id})
PR: #{number} - {PR URL}
Status: {in_review if the advance ran, else open} (the reviewer merges on Bitbucket)
Worktree: {worktree path}
```

No "Merge & close" (reviewer merges on Bitbucket), no "Next task" (can't switch branches in a worktree).

```
AskUserQuestion(
  questions=[{
    question: "PR #{number} ready. What's next?",
    header: "Next step",
    options: [
      { label: "Stay here", description: "Stay in this worktree for PR fixes or follow-up" },
      { label: "End session", description: "Run /lets:end - save context and wrap up" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Stay here** -> stay in worktree. User continues working.
- **End session** -> invoke `Skill(skill: "lets:end")`. After the end-session flow completes, remind:
  "After the PR merges, clean up: `/lets:worktree remove {name}` from the main repo terminal."

### After local merge ($LETS_PR_FLOW == local / fallback), NOT in worktree:

```
Task: **{title}** ({task-id}) - {CLOSED, or "advanced to {status}" when close returned another status, or "unchanged - no tracker" when it returned none}
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

### After local merge ($LETS_PR_FLOW == local / fallback), IN worktree:

```
Task: **{title}** ({task-id}) - {CLOSED, or "advanced to {status}" when close returned another status, or "unchanged - no tracker" when it returned none}
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
- **NEVER open a second PR for a branch that already has an open one** - the Existing-PR Guard's `open` outcome pushes to it; a lookup that ran and failed (or came back incomplete) STOPS before any push and never reads as "no PR"
- **NEVER report a created PR without reading it back** - a CONFLICTING PR runs no CI at all and looks identical to a passing one
- **NEVER merge without user approval**
- Document BEFORE finishing (Step 7 before Step 8)
- If PR flow: task stays open, user closes after merge - or the Existing-PR Guard's `shipped` outcome closes it after Finish
- If local merge: task closes immediately
- If HEAD == `$LETS_MERGE_BRANCH` (trunk-mode): skip PR creation (same-source-target is not a valid PR), push (upstream-aware) + close (tracker `close` verb) instead — regardless of `$LETS_PR_FLOW`
- Respond in user's language
