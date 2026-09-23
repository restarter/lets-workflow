---
name: loop-gh-pr
description: Compose a /loop that watches all open GitHub PRs for activity (new PRs, new commits, new comments) and auto-prepares a READ-ONLY /lets:review draft for each code change - never posting. Use when the user says "watch PRs", "monitor github PRs", "loop-gh-pr", "стеж за PR", "поллі гітхаб PR", or wants a hands-near PR-review watcher. Composes a paste-ready /loop block - does NOT invoke /loop (UI command). Local test skill.
---

# loop-gh-pr

Compose a `/loop` that periodically lists open GitHub PRs via `gh`, detects what is new since the last tick (new PR / new commits / new comments), auto-runs a READ-ONLY `/lets:review <N> --json` draft for each code change, fires a `lets cmux notify`, and reschedules - WITHOUT ever posting to a PR.

The skill **never invokes `/loop` itself** - Claude Code's `/loop` is a UI command. It composes the prompt and presents it for paste; once active, the autonomous run uses `ScheduleWakeup` per tick.

**Autonomy boundary (the whole point):** `/lets:review --json` is read-only - it computes findings and posts NOTHING. So the loop safely auto-runs review. Posting (inline / general / approve / merge) is ALWAYS the human's call, out-of-band. The loop stops at a ready draft in `.lets/reviews/` + a notification and keeps watching. Never saw -> reviewed -> posted.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified.

## Step 1: Pre-flight (always run)

Confirm github flow + `gh` auth + that listing PRs works, before composing a long-lived loop.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)

# 1. github flow only (v1). LETS_PR_FLOW is substituted by the orchestrator from LETS Config.
#    If LETS_PR_FLOW != github -> surface "loop-gh-pr is github-only in v1 (LETS_PR_FLOW={value});
#    bitbucket waits on lets-6pxvm" and exit.

# 2. gh present + authed
gh auth status 2>&1 | head -5 || { echo "gh not authed - run: gh auth login"; exit 1; }

# 3. smoke: list open PRs (the watch set). If `comments` is rejected as a list field,
#    drop it here and have the loop fall back to per-PR `gh pr view <N> --json comments`.
gh pr list --state open --json number,title,headRefOid,isDraft,url,updatedAt,comments --limit 30 2>&1 | head -40
```

Surface a short summary of the current open non-draft PRs (number, title, head SHA short, comment count) so the user sees exactly what the loop will watch. If `gh auth status` fails -> surface and exit. If `gh pr list` errors (e.g. not a GitHub remote) -> surface verbatim and exit. Never compose a loop the autonomous run cannot execute.

## Step 2: State file

`.lets/pr-watch-state.json` - one file, keyed by PR number. `.lets/` is gitignored and symlinked across worktrees (per-repo state - fine; we key by PR number).

Shape:

```json
{
  "captured_at": "<iso8601>",
  "prs": {
    "42": {
      "title": "...",
      "head_sha": "<headRefOid>",
      "comment_count": 3,
      "updated_at": "<updatedAt from gh>",
      "last_reviewed_sha": "<headRefOid the latest draft covers, or null if not yet/failed>",
      "last_seen": "<iso8601>"
    }
  }
}
```

- `head_sha` vs stored -> new-commits signal.
- `comment_count` vs stored -> new-comments signal; `updated_at` is a cheap secondary signal.
- `last_reviewed_sha == head_sha` -> already reviewed at this SHA; skip re-review (dedup).
- `last_reviewed_sha == null` (review never ran, or review FAILED) -> a failed review keeps it `null` so the next tick re-attempts; never advance it on failure.
- A PR absent from the current `gh pr list` but present in state -> merged/closed; drop it and report once.
- The baseline is captured ONCE on first tick; never silently re-baselined. On tick-1, PRs beyond `INIT_REVIEW_CAP` are baselined with `last_reviewed_sha = head_sha` (seen, not reviewed) and surfaced for manual review.

## Step 3: Compose /loop

Template (dynamic mode - the model self-paces via ScheduleWakeup). The body is always English. Present this for paste:

```text
/loop Watch all open GitHub PRs for activity and prepare READ-ONLY review drafts. NEVER post to any PR.

State file: .lets/pr-watch-state.json (keyed by PR number). Initial-review cap: 5.

Each tick:
1. Resolve root: LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
2. Fetch the watch set (READ-ONLY):
   gh pr list --state open --json number,title,headRefOid,isDraft,url,updatedAt,comments --limit 30
   If `comments` is rejected as a list field, re-run without it and read each count via
   `gh pr view <number> --json comments -q '.comments | length'`.
   Drop isDraft=true entries.

Review primitive (used below): run `/lets:review <number> --json`. It saves
.lets/reviews/<date>-PR-<number>.json + .md and posts NOTHING, and must run with NO interactive
prompt. If it returns no JSON artifact (failed/empty), treat the review as FAILED: do NOT advance
last_reviewed_sha for that PR (leave it null), surface "review failed for #<number> - will retry",
and let the next tick re-attempt. Never mark a PR reviewed when it was not.

FIRST tick (.lets/pr-watch-state.json does not exist):
- Create the file. Take the open non-draft PRs in listed order.
- For the FIRST 5: run the review primitive, read verdict + findings_count from the saved JSON,
  set last_reviewed_sha=head_sha only on success (null on failure).
- For any open non-draft PR BEYOND the first 5: baseline as already-seen (last_reviewed_sha=head_sha)
  WITHOUT reviewing, and add to a "baselined without initial review (large open set): #a #b ..." line.
- Write baseline per PR: {title, head_sha=headRefOid, comment_count=(comments|length),
  updated_at=updatedAt, last_reviewed_sha=(per above), last_seen=now}.
- Surface a combined summary (one line per reviewed PR: #N "title" -> verdict, K findings ->
  .lets/reviews/<file>), plus the baselined-without-review line if any.
- Build the notify body safely, then fire one notification:
    BODY=$(printf '%s' "<combined summary>")
    lets cmux notify --cwd "$LETS_PROJECT_ROOT" --title 'PR review drafts ready - needs you' --body "$BODY" --json 2>/dev/null || true
- Reschedule: ScheduleWakeup(delaySeconds=1500, prompt=<this entire /loop prompt verbatim>, reason="watching N open PRs - baseline").

SUBSEQUENT ticks (state file exists):
- Read state. For each current open non-draft PR, compare to state:
  - NOT in state -> NEW PR -> review primitive -> add to state (last_reviewed_sha=head_sha on success). Mark for notify.
  - head_sha changed -> NEW COMMITS -> review primitive -> on success update head_sha + last_reviewed_sha + comment_count + updated_at. Mark for notify.
  - comment_count increased (and head_sha unchanged) -> NEW COMMENTS -> do NOT review; record "#N: M new comments - reply via /lets:github-pr --respond <number>". Update comment_count + updated_at. Mark for notify.
  - nothing changed -> skip silently.
- Any PR in state but absent from the current list -> merged/closed -> drop from state, add a "#N <title> - closed/merged" line.
- If anything was marked for notify, build the body safely and fire ONE:
    BODY=$(printf '%s' "<summary>")
    lets cmux notify --cwd "$LETS_PROJECT_ROOT" --title 'PR activity - needs you' --body "$BODY" --json 2>/dev/null || true
- Surface the tick result: the change lines (incl. any "review failed for #N"), or "no change across <K> open PRs (T+<minutes>m)".
- Reschedule: ScheduleWakeup(delaySeconds=1500, prompt=<this entire /loop prompt verbatim>, reason="watching <K> open PRs - <changed|no change>").

HARD constraints (READ-ONLY - the autonomy boundary):
- NEVER run gh pr comment / gh pr review / gh pr merge / gh pr edit, never approve/decline, never push to any branch.
- The ONLY review path is /lets:review <number> --json (it posts nothing). Never /lets:review without --json (that path can post).
- Review must be non-interactive: if it ever blocks on a prompt, STOP and surface (do not blindly answer).
- A FAILED review never advances last_reviewed_sha (retry next tick); never mark a PR reviewed when it was not.
- Posting any draft is the human's decision, out-of-band. The loop only drafts + notifies + keeps watching.
- Drafts persist in .lets/reviews/ - do NOT re-review a PR whose last_reviewed_sha == head_sha (dedup).
- Build notify --body from a printf-assigned variable passed as --body "$BODY"; never interpolate raw PR text into single quotes.
- On gh auth error (gh auth status fails / 401/403): report "GitHub auth failed" and STOP (no ScheduleWakeup).
- On transient error (timeout/5xx/network): log and reschedule ONCE. Second consecutive same-type error -> STOP and surface.
- Never re-baseline silently - baseline is captured once on the first tick.
- delaySeconds: 1500 (25 min). Never exactly 300 (cache TTL trap).

Termination (STOP = no ScheduleWakeup; surface a one-paragraph summary):
- auth-failure: gh auth failed
- review-blocked: /lets:review prompted interactively (needs investigation)
- transient-2x-failure: same transient error twice in a row
- (otherwise continuous; the human cancels by re-issuing the same /loop prompt or interrupting the session)
```

## Step 4: Present paste block

```
## Composed /loop for **all open GitHub PRs**

Watching: {K} open non-draft PRs ({#a, #b, ...})
Tick cadence: ~1500s (25 min, dynamic mode, cache-cold)
State file: .lets/pr-watch-state.json
Drafts: .lets/reviews/<date>-PR-<N>.md (you post them manually when ready)

Copy and paste:

```text
{COMPOSED_LOOP}
```

**Autonomy boundary:** the loop auto-runs READ-ONLY reviews and notifies - it never posts. You post drafts yourself out-of-band (`/lets:github-pr <N>` or `gh pr comment`).
**Mode note:** dynamic mode - skip the ScheduleWakeup call to end the loop. Does NOT survive session close (interval mode is a later variant). `/clear` may nuke pending wakeups.
**Cancel:** re-issue the same /loop prompt, or interrupt the session.
```

## Rules

- NEVER call /loop yourself - initial /loop is a UI command, paste-only.
- NEVER compose a loop that mutates PR state (comment/review/approve/merge/push) - read-only polling + read-only /lets:review --json only.
- ALWAYS --json on the review path (it skips posting). A bare /lets:review can post - never compose that.
- Composed /loop body: always English (written-artifact rule). Surrounding paste-block prose follows the user's language.
- github-only in v1 (LETS_PR_FLOW=github). Bitbucket waits on lets-6pxvm.
- Local test skill; promotion to a plugin surface (/lets:pr-watch vs /lets:github-pr --watch) is decided after validation, tracked on lets-lhnob.
