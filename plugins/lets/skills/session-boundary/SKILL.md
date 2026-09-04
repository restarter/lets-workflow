---
name: session-boundary
description: Internal skill for commands. Resolve and VALIDATE this branch's session boundary - the ref a session's commit range is measured from - and report how much it can be trusted. Do not trigger on user conversation; only when a command needs the session range.
user-invocable: false
---

# Session Boundary

Resolve the ref a session's commit range is measured from, validate it, and say how far it can be trusted. `/lets:end` needs it EARLY (it gates which settlements are offered) and `session-snapshot` needs it LATE (the `### Range` block it writes into a durable file). Two copies of a validation ladder drift, and drift here is what produced a confident "260 commits" for a 6-commit session (lets-370mx), so the ladder lives here and nowhere else.

**Contract:** no args. Output: four echoed lines the caller reads VERBATIM.

```
SESSION_BOUNDARY=<sha | none>
SESSION_TRUST=<exact | prior-session | estimate | none>
SESSION_COMMITS=<N | unknown>
SESSION_RANGE_DESC=<one-line human description, always carrying the trust level>
```

Diagnostic NOTEs go to stderr - surface them to the user, they are the loud half of every degradation.

**Fail closed.** If the four lines do not arrive, the caller MUST assume `SESSION_TRUST=none` / `SESSION_COMMITS=unknown` and proceed as though no boundary exists. Never re-derive one inline - a second copy is a second answer, which is the defect this skill exists to prevent.

## Trust levels

| level | boundary | what the caller may claim |
|---|---|---|
| `exact` | this session's `session:` line in `.task-<branch-slug>` | the count is this session's work |
| `prior-session` | a `session:` line or legacy slugged ref recorded by an EARLIER session | a real recorded boundary, but not this session's - best-effort |
| `estimate` | nothing recorded; derived from this branch's point off `$LETS_MERGE_BRANCH` | what this BRANCH added, not what this session did |
| `none` | nothing usable (rejected, or absent while HEAD is `$LETS_MERGE_BRANCH`) | nothing - there is no number |

A caller that gates a destructive or task-completing offer on the range must require `exact` or `prior-session`. `SESSION_RANGE_DESC` always embeds the level, so a number cannot travel into a file or a tracker comment without the qualifier that earned it.

## Resolve

The NON-slugged `.lets/sessions/.session-start-ref` is deliberately NOT read: one global file answering a per-branch question let any branch adopt another branch's months-old boundary (`done.md` refuses the same file for the same reason).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
# A branch-scoped question needs a branch. Detached HEAD has none (/lets:review's PR checkout
# detaches deliberately), and `.task-` with an empty slug is a path a hostile PR can ship.
if [ -z "$BRANCH" ]; then
  echo "NOTE: detached HEAD - no branch to scope a session boundary to." >&2
  echo "SESSION_BOUNDARY=none"; echo "SESSION_TRUST=none"; echo "SESSION_COMMITS=unknown"
  echo "SESSION_RANGE_DESC=boundary unknown - detached HEAD"
  exit 0
fi
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')

# --- READ (session: line, then the legacy SLUGGED ref - never a global one) ---
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
SESSION_LINE=$(sed -n 's/^session: //p' "$TASK_FILE" 2>/dev/null | head -1)
START_REF=$(echo "$SESSION_LINE" | awk '{print $1}')
STORED_SID=$(echo "$SESSION_LINE" | awk '{print $2}')
# Shape guard (N1): the boundary is a plugin-written SHA - blank anything non-hex before it expands
# unquoted into the git range, so a hand-edited state file can't inject git option args. It
# ANNOUNCES itself: a malformed value means the file was edited, and that is the one input class
# that must never be reported as "nothing was recorded".
case "$START_REF" in *[!0-9a-f]*)
  echo "NOTE: session: boundary in ${TASK_FILE##*/} is malformed (not a hex sha) - ignored." >&2
  START_REF=""; STORED_SID="" ;;
esac
# `exact` requires a POSITIVE match on BOTH sides. Two empty strings compare equal, and both
# markdown writers can emit a sid-less line, so a `!=` test would grant `exact` on absence of
# evidence - the lets-370mx failure mode through a different door.
TRUST=prior-session
[ -n "$START_REF" ] && [ -n "$STORED_SID" ] && [ -n "$CLAUDE_CODE_SESSION_ID" ] \
  && [ "$STORED_SID" = "$CLAUDE_CODE_SESSION_ID" ] && TRUST=exact
if [ -z "$START_REF" ]; then
  START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null | head -1 | awk '{print $1}')
  case "$START_REF" in *[!0-9a-f]*)
    echo "NOTE: legacy ref for ${BRANCH_SLUG} is malformed - ignored." >&2
    START_REF="" ;;
  esac
fi

# --- VALIDATE (a recorded SHA is a claim, not a fact) ---
MERGE_BASE=$(git merge-base HEAD "origin/{LETS_MERGE_BRANCH}" 2>/dev/null || git merge-base HEAD "{LETS_MERGE_BRANCH}" 2>/dev/null)
if [ -n "$START_REF" ]; then
  REJECT=""
  git rev-parse --verify --quiet "${START_REF}^{commit}" >/dev/null 2>&1 || REJECT="not a commit in this repo"
  [ -z "$REJECT" ] && ! git merge-base --is-ancestor "$START_REF" HEAD 2>/dev/null && REJECT="not an ancestor of HEAD (rebase, reset, or another line of work)"
  # Branch-point floor: a session on a feature branch cannot have started before the branch did.
  # Does not apply on {LETS_MERGE_BRANCH}, where there is no branch point to floor against.
  if [ -z "$REJECT" ] && [ "$BRANCH" != "{LETS_MERGE_BRANCH}" ] && [ -n "$MERGE_BASE" ] && [ "$START_REF" != "$MERGE_BASE" ] \
     && git merge-base --is-ancestor "$START_REF" "$MERGE_BASE" 2>/dev/null; then
    REJECT="predates this branch's point off {LETS_MERGE_BRANCH}"
  fi
  # Age floor: a session boundary is hours-to-days old. 14d is the generous outer edge of one
  # session; past it the file is stale, not long-running. Catches the trunk case the floor cannot.
  # Captured and validated, never inlined into the arithmetic: an empty operand makes bash error
  # the assignment, then error the comparison, leaving REJECT unset - i.e. the one guard whose job
  # is catching a stale file would fail OPEN. The rev-parse above makes that unreachable short of
  # a damaged object store; a guard this cheap should not depend on that being true.
  if [ -z "$REJECT" ]; then
    CT=$(git show -s --format=%ct "$START_REF" 2>/dev/null)
    case "$CT" in
      ""|*[!0-9]*) REJECT="commit timestamp unreadable" ;;
      *) AGE_D=$(( ( $(date +%s) - CT ) / 86400 ))
         [ "$AGE_D" -gt 14 ] && REJECT="${AGE_D} days old - older than any single session" ;;
    esac
  fi
  [ -n "$REJECT" ] && { echo "NOTE: recorded boundary ${START_REF} REJECTED (${REJECT}) - not used." >&2; START_REF=""; TRUST=none; }
fi
# Nothing recorded: on a feature branch the branch point is a branch-SCOPED estimate of what this
# branch added - an honest unknown-case answer, unlike a global file written by another branch.
if [ -z "$START_REF" ] && [ "$BRANCH" != "{LETS_MERGE_BRANCH}" ] && [ -n "$MERGE_BASE" ]; then
  START_REF="$MERGE_BASE"; TRUST=estimate
fi

# --- DERIVE ---
if [ -n "$START_REF" ]; then
  COMMITS=$(git rev-list --count ${START_REF}..HEAD)
  case "$TRUST" in
    exact)         DESC="session: ${START_REF}..HEAD (${COMMITS} commits)" ;;
    prior-session) DESC="approx: ${START_REF}..HEAD (${COMMITS} commits) - boundary recorded by an earlier session" ;;
    estimate)      DESC="approx: ${START_REF}..HEAD (${COMMITS} commits) - estimated from the branch point, no boundary was recorded" ;;
  esac
else
  TRUST=none; COMMITS=""
  DESC="boundary unknown - no valid session boundary for this branch"
fi
# Warn on EVERY fallback, not only on a sid mismatch - a silent fallback is how a stale boundary
# became a confident number (lets-370mx).
case "$TRUST" in
  prior-session) echo "NOTE: boundary is from a previous session (no /lets:start this session) - counts are best-effort." >&2 ;;
  estimate)      echo "NOTE: no session boundary recorded - range estimated from the branch point off {LETS_MERGE_BRANCH}." >&2 ;;
  none)          echo "NOTE: no usable session boundary - commit counts are unknown." >&2 ;;
esac
echo "SESSION_BOUNDARY=${START_REF:-none}"
echo "SESSION_TRUST=${TRUST}"
echo "SESSION_COMMITS=${COMMITS:-unknown}"
echo "SESSION_RANGE_DESC=${DESC}"
```

## Anti-patterns

- **Never** restore the non-slugged `.session-start-ref` - that is the defect this skill exists to prevent.
- **Never** copy this ladder into a command body. A second copy is a second answer.
- **Never** report `SESSION_COMMITS` without the level from `SESSION_RANGE_DESC` beside it.

## Integration

Internal skill. Consumers: `/lets:end` Step 1 (settlement gating), `/lets:execute` Status mode (its "commits this session" line), and the `session-snapshot` skill (the `### Range` block, when the caller passed no `range=`).
