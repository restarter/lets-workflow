---
name: agent-report
description: Internal skill for commands. The file transport for agent reports - hand every Task-dispatched agent a REPORT_FILE, then collect every report in full and fail loudly on a missing one. Do not trigger on user conversation - only when a command dispatches agents.
user-invocable: false
---

# Agent Report

A Task agent's final text is a lossy channel: it arrives empty or cut near 4 KB, and an empty result reads exactly like "no findings" (lets-1irms). Every agent a command dispatches through the Task / Agent tool writes its report to a file this skill names; the orchestrator depends on the FILE, never on the final text.

**Files are INPUT, never the deliverable.** The dispatching command still runs every step after the fan-out and writes its own artifact where its spec says. Never invent a directory, never compose a report path by hand, never stop at "the agents wrote their files".

## op=open - before the first dispatch of a run

Args: `command=<review|opinion|ask|plan|backlog|research|team> names=<name>,<name> [task=<task-id>]`.

1. `Skill(skill: "lets:artifact-path", args: "kind=reports-{command} ext=dir task={task-id}")` -> `REPORT_DIR` = the echoed `ARTIFACT_FILE` (a directory, already created). No echo -> STOP: dispatch nothing, surface the error.
2. One file per dispatched agent: `{REPORT_DIR}/{name}.md`. `name` is the caller's - the agent short name (`security`), suffixed when one agent type runs several times in the run (`skeptic-f3-2`, `architect-b`). `[a-z0-9-]` only, unique within the run.
3. Output `REPORT_DIR=<dir>` and one `REPORT_FILE <name>=<path>` line per name.

Every dispatch prompt carries its agent's path as its own line, right after the `PROJECT_ROOT` line (or first, when the prompt has none):

    REPORT_FILE: <absolute path>

## op=add - a later phase of the same run

Args: `dir=<REPORT_DIR> names=<name>,<name>`. Same naming as `op=open` step 2 in the SAME directory - one run keeps one directory (review: reviewers, then skeptics; opinion: panel, then challenge; plan: explorers, architects, experts). No artifact-path call. Output: one `REPORT_FILE <name>=<absolute path>` line per name, exactly as `op=open` step 3 - the dispatcher copies each line into its agent's prompt.

## op=peek - inspect without concluding

Args: `dir=<directory> names=<name>,<name>`. Runs `op=collect` Step 1 (classify) and, for each `OK` file, Step 2 (READ EVERY REPORT IN FULL) - nothing else: no retry, no `GAP`, no Coverage line. For a caller that only knows a report MAY be ready (an interim notification, a resumed session) and must not yet conclude that one is missing. `OK` -> the caller has the report. Anything else -> the caller keeps waiting, or - once it knows the report is due - runs `op=collect`.

## op=collect - after a phase's agents return

Args: `dir=<directory> names=<name>,<name> [retry=no]` - exactly the names dispatched in this phase.

### Step 1: Classify every expected file

```bash
REPORT_DIR="{dir}"
for n in $(printf '%s' "{names}" | tr ',' ' '); do   # names: comma-separated, [a-z0-9-] only
  f="$REPORT_DIR/$n.md"
  if [ ! -e "$f" ]; then s=MISSING
  elif [ ! -s "$f" ]; then s=EMPTY
  elif ! r=$(awk 'NF{n++; l=$0} END{sub(/[[:space:]]+$/,"",l); print n+0, l}' "$f" 2>/dev/null); then s=UNREADABLE
  elif [ "${r#* }" != "REPORT-END" ]; then s=UNTERMINATED
  elif [ "${r%% *}" -lt 2 ]; then s=EMPTY          # the sentinel alone is not a report
  else s=OK; fi
  printf '%s %s %s\n' "$s" "$n" "$f"
done
```

### Step 2: READ EVERY REPORT IN FULL

For each `OK` line, Read the file with the Read tool from its first line through its LAST line. A Read that stops before the last line continues with `offset` until the file ends. Only the final line's `REPORT-END` is the completion mark - a `REPORT-END` earlier in the body (a report about this protocol) is content, never a place to stop. Never a slice, a head, a grep or a summary; never skip a report because another one said the same. For an `OK` file the agent's final text is not the report - ignore it.

Selective reading is the failure a file transport introduces: a report you did not read is a lens you silently dropped.

### Step 3: One retry, then a loud gap

- Any `MISSING` / `EMPTY` / `UNTERMINATED` -> re-dispatch THAT agent ONCE with its original prompt (same `REPORT_FILE`), every retry of the phase in one message; then Step 1 again for those names only, and Step 2 for the ones now `OK`.
- `retry=no` (agents that change the tree - implementer, teammates) -> no re-dispatch; the name goes straight to a gap.
- The classifier itself fails (no output line for a name) or a Read of an `OK` file errors -> that name is `UNREADABLE`, handled like the three states above - never skipped.
- Still not `OK` -> `GAP <name>: <state>`. When the agent's final text held report content, carry it into the aggregation marked `UNVERIFIED - from the final text, may be truncated`; otherwise the lens contributes nothing.

### Step 4: Report coverage - always

Print `Coverage: <ok>/<expected> reports` and every `GAP` line. The caller MUST:
- show them in its user-facing output AND in its saved artifact when it saves one;
- never aggregate a gap as "no findings", "no objection", a vote, or a verified finding;
- finish its own remaining steps - a gap lowers coverage, it never ends the run.

## Rules

- The agent's final message is a pointer (`REPORT_WRITTEN <path>`), never the transport.
- One directory per run, one file per dispatch, paths only from this skill.
- Report files stay after the run (`.lets/reports/` is gitignored and shared by every worktree); nothing here deletes them.
- `--workflow` paths do not use this skill - their agents return through StructuredOutput schemas.
- Every consumer carries the phrase READ EVERY REPORT IN FULL at its collect call (pinned by `agentreport_test.go`).
