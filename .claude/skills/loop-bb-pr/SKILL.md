---
name: loop-bb-pr
description: Compose a /loop that monitors a Bitbucket PR for activity (new comments, review state changes, merge, decline) via the bb-api wrapper. Use when the user says "watch bitbucket PR", "monitor BB PR", "loop-bb-pr", "слідкуй за PR", "поллі bb pr", or pastes a bitbucket.org/<workspace>/<repo>/pull-requests/<id> URL and wants periodic check-ins. Composes a paste-ready /loop block — does NOT invoke /loop (UI command). Local test skill.
---

# loop-bb-pr

Compose a `/loop` that periodically polls a Bitbucket PR via the `bb-api` wrapper and surfaces changes (new comments, review state, merge, decline).

The skill **never invokes `/loop` itself** at initial start — Claude Code's `/loop` is a UI command. It composes the prompt and presents for paste; once active, the autonomous run uses `ScheduleWakeup` directly per tick (that part is model-callable).

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified.

## Step 1: Resolve PR URL

Parse `args` parameter for URL. If not provided, AskUserQuestion:

```
AskUserQuestion(
  questions=[{
    question: "Bitbucket PR URL to watch?",
    header: "PR URL",
    options: [
      { label: "I'll paste it", description: "Send the bitbucket.org/.../pull-requests/N URL in your next message" },
      { label: "Cancel", description: "Abort composition" }
    ],
    multiSelect: false
  }]
)
```

If user picks "I'll paste it", wait for URL in their next message.

**Normalize then validate** — real BB URLs often have query strings (`?at=branch&type=...`) or trailing slashes. Strip before regex match:

```bash
URL="<from input>"
URL="${URL%%\?*}"   # strip query string (everything from ? onward)
URL="${URL%#*}"     # strip fragment (everything from # onward)
URL="${URL%/}"      # strip trailing slash
```

Then **strict validation** — normalized URL must match exactly:

```
^https://bitbucket\.org/[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+/pull-requests/[0-9]+$
```

If validation fails: surface error, exit. Do NOT proceed with malformed URL — the composed prompt may persist (if user switches to interval mode in v1.1) and we don't want arbitrary input in `.claude/scheduled_tasks.json`.

Extract via regex or sed (from normalized URL):
- `WORKSPACE` — first segment after `bitbucket.org/`
- `REPO` — second segment
- `PR_ID` — final numeric segment

## Step 2: Verify bb-api availability

```bash
which bb-api 2>/dev/null
```

If `bb-api` not found in PATH:

```
┌─ LETS ─────────────────────────────────┐
│  bb-api not available.                 │
│  Install bb-api wrapper first.         │
│  (curl fallback planned for v1.1.)     │
└────────────────────────────────────────┘
```

Exit. v0 hard-requires bb-api.

## Step 3: Verify access (always run)

Default-on pre-flight: confirm bb-api auth + URL points at a real PR before composing a 1200s loop. Cheaper to fail here than to ship a /loop the autonomous run immediately rejects.

```bash
bb-api get "/repositories/$WORKSPACE/$REPO/pullrequests/$PR_ID" 2>&1 | head -20
```

Always surface the result to the user as a short summary (title, state, comment count) — confirms what they're about to watch.

If error:
- **401/403 (auth)** → surface "Bitbucket auth failed — check bb-api credentials" and exit
- **404** → surface "PR not found at <URL> — check URL" and exit
- **other errors** → surface verbatim error and exit

Never proceed to compose a /loop if Step 3 fails.

## Step 4: Compose /loop

Substitute slots:
- `<URL>` — validated URL from Step 1
- `<WS>` `<REPO>` `<ID>` — parsed components
- `<STATE_FILE>` — `.lets/loop-state/bb-pr-<ID>.json` (relative to project root)

Template (dynamic mode — model self-paces via ScheduleWakeup):

```
/loop check Bitbucket PR <URL> for activity since baseline.

State file: .lets/loop-state/bb-pr-<ID>.json (workspace=<WS>, repo=<REPO>, id=<ID>)

Use bb-api to fetch (READ-ONLY):
- `bb-api get /repositories/<WS>/<REPO>/pullrequests/<ID>` → state, title, participants
- `bb-api get /repositories/<WS>/<REPO>/pullrequests/<ID>/comments` → count, latest activity timestamp

On FIRST tick (state file does not exist):
- Create .lets/loop-state/ directory if missing
- Write baseline to .lets/loop-state/bb-pr-<ID>.json as JSON: {state, comment_count, latest_activity, approver_count, title, captured_at}
- Report "baseline established for PR #<ID> (<title>): state=<X>, comments=<N>, approvers=<M>"
- Reschedule via ScheduleWakeup(delaySeconds=1200, prompt=<this entire /loop prompt verbatim>, reason="watching BB PR <ID> — baseline")

On SUBSEQUENT ticks (state file exists):
- Read baseline from .lets/loop-state/bb-pr-<ID>.json
- Fetch current state via bb-api (both endpoints above)
- Compare:
  - If state changed to DECLINED or MERGED → report final state and STOP (do NOT call ScheduleWakeup)
  - If comment_count increased → report "N new comments since last check, latest from <user> at <time>" and STOP
  - If approver_count increased → report "approver added: <username>" and STOP
  - If nothing changed → report "no change (T+<minutes>m since baseline)" and reschedule via ScheduleWakeup(1200, verbatim, "watching BB PR <ID> — no change")

Hard constraints (READ-ONLY enforcement):
- Loop must NOT call bb-api with POST/PUT/DELETE
- Loop must NOT comment, approve, decline, merge, or change PR state
- Loop must NOT push to the PR's branch
- On bb-api auth error (401/403): report "Bitbucket auth failed" and STOP (do NOT retry blind — user must investigate)
- On 404 (PR not found / deleted): report "PR not accessible" and STOP
- On other transient errors (timeout, 5xx): log error and reschedule ONCE. If second consecutive error of same type, STOP and surface.
- Never re-baseline silently — the baseline is captured ONCE on first tick

Termination (each STOP surfaces a single-paragraph summary in assistant text):
- state-change-final: PR closed/merged/declined
- comment-detected: new comments since baseline
- approver-change: approver count changed
- auth-failure: bb-api 401/403
- not-found: PR 404
- transient-2x-failure: same transient error twice in a row
```

## Step 5: Present paste block

```
## Composed /loop for **Bitbucket PR #{PR_ID}**

URL: {URL}
Workspace/Repo: {WS}/{REPO}
Tick cadence: ~1200s (20 min, dynamic mode, cache-cold)
State file: {STATE_FILE}

Copy and paste:

```text
{COMPOSED_LOOP}
```

**Mode note:** Dynamic mode — the model running the loop calls `ScheduleWakeup` after each tick. Skip the call → loop ends silently. Avoid `delaySeconds=300` exactly (cache TTL trap; we use 1200).

**Cancel:** re-issue the same /loop prompt to cancel this watcher, OR user-interrupt the session to cancel all pending loops.

**Gotcha:** `/clear` may nuke pending wakeups silently. Dynamic mode does NOT persist across session close (unlike interval mode which writes to `.claude/scheduled_tasks.json`). For session-survival, would need v1.1 interval mode.
```

## Rules

- NEVER call /loop yourself — initial /loop is UI command, paste-only
- NEVER recommend a /loop that mutates state (commit, push, bd close, BB comment/approve/merge) — read-only polling only
- Strict URL validation BEFORE composition (normalize then regex `^https://bitbucket\.org/[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+/pull-requests/[0-9]+$`)
- Strict reject on bb-api unavailability — never compose a /loop the autonomous run can't execute
- Composed /loop body: always English (written artifact rule). Surrounding paste-block prose and headers follow user's language
- This is a local test skill; promotion to plugin (as a /lets:loop subcommand or separate) is tracked separately
