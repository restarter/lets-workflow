---
name: artifact-path
description: Internal skill for commands. Resolve a unique, task-scoped, collision-safe path for an artifact written under .lets/ (plans, reviews, session snapshots). Do not trigger on user conversation - only when a command is about to write an artifact.
user-invocable: false
---

# Artifact Path

`.lets/` is ONE directory shared by every worktree of the repo (whole-dir symlink). Two sessions writing the same artifact kind on the same day used to overwrite each other (lets-05c4s - a plan review was lost). This skill is the single place that names an artifact and guarantees the path is free.

**Contract:** args `kind=<kind> ext=<md|json> [task=<task-id>]`. Output: ONE echoed line `ARTIFACT_FILE=<absolute path>`. The caller writes to that path VERBATIM - never recomputes the stamp, never strips the `-vN` suffix, never writes anywhere else.

## Name shape

`{YYYY-MM-DD}-{HHMM}-{ID}-{KIND}[-vN].{EXT}` under the kind's directory:

| kind | directory |
|------|-----------|
| `plan` | `.lets/plans/` |
| `review-local`, `review-branch`, `review-pr-<n>`, `review-plan` | `.lets/reviews/` |
| `snapshot` | `.lets/sessions/` |

`ID` is the task id when one is active (MANDATORY whenever a task exists - same rule as `feature/<task-id>-...` branches). With no task: `{branch-slug}-{6hex}`, 6hex = first 6 chars of `$CLAUDE_CODE_SESSION_ID`, so two taskless sessions on `main` still get distinct names. `-vN` (v2, v3, ...) is appended whenever the path already exists - for task and taskless alike.

## Step 1: Resolve the task id

If the caller passed `task=`, use it. Otherwise invoke `Skill(skill: "lets:detect-task", args: "fallback=no")` and use its result (may be empty). `fallback=no` is mandatory: the searched fallback asks a question, and a taskless snapshot on `/lets:end` must stay silent.

## Step 2: Compute the path

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
KIND="{kind}"; EXT="{ext}"; TASK_ID="{task-id or empty}"
case "$KIND" in
  plan) DIR=plans ;;
  review-*) DIR=reviews ;;
  snapshot*) DIR=sessions ;;
  *) echo "artifact-path: unknown kind '$KIND'"; exit 1 ;;
esac
mkdir -p "$LETS_PROJECT_ROOT/.lets/$DIR"
STAMP=$(date +%Y-%m-%d-%H%M)
if [ -n "$TASK_ID" ]; then
  ID="$TASK_ID"
else
  HEX=$(printf '%s' "${CLAUDE_CODE_SESSION_ID:-$$}" | cut -c1-6)   # session-id prefix; PID outside Claude Code
  ID="$(git branch --show-current | tr '/' '-')-${HEX}"
fi
BASE="$LETS_PROJECT_ROOT/.lets/$DIR/${STAMP}-${ID}-${KIND}"
ARTIFACT_FILE="${BASE}.${EXT}"; N=2
while [ -e "$ARTIFACT_FILE" ]; do ARTIFACT_FILE="${BASE}-v${N}.${EXT}"; N=$((N+1)); done
echo "ARTIFACT_FILE=$ARTIFACT_FILE"
```

## Return

Return the echoed `ARTIFACT_FILE` to the caller. If the echo is missing or `exit 1` fired, the caller MUST NOT write anything - surface the error.

## Rules

- NEVER overwrite: the loop is the guard; the caller never "fixes" a path by hand.
- NEVER compute a second `date` in the caller - the stamp is captured here once.
- Task id in the name is mandatory when a task is active.
- No tracker calls here beyond `detect-task`.
