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
2. One file per dispatched agent: `{REPORT_DIR}/{name}.md`. `name` is the caller's - the agent short name (`security`), suffixed when one agent type runs several times in the run (`skeptic-f3-2`, `architect-b`). `[a-z0-9.-]` only (a beads id may carry `.N`), unique within the run. A name never starts with `report`, `summary`, `findings` or `analysis` (any case): Claude Code's Write tool refuses a subagent's `.md` whose basename starts with one of them, and the report is lost (lets-jyjxc). A name never ends in `-r<digits>`: that suffix is Step 3's retry. `reportnames_test.go` pins both for every template in the plugin.
3. Output `REPORT_DIR=<dir>` and one `REPORT_FILE <name>=<path>` line per name.

Every dispatch prompt carries its agent's path as its own line, right after the `PROJECT_ROOT` line (or first, when the prompt has none):

    REPORT_FILE: <absolute path>

## op=add - a later phase of the same run

Args: `dir=<REPORT_DIR> names=<name>,<name>`. Same naming as `op=open` step 2 in the SAME directory - one run keeps one directory (review: reviewers, then skeptics; opinion: panel, then challenge; plan: explorers, architects, experts). No artifact-path call. Run as written:

```bash
REPORT_DIR="{dir}"
for n in $(printf '%s' "{names}" | tr ',' ' '); do   # names: comma-separated, [a-z0-9.-] only
  f="$REPORT_DIR/$n.md"
  if [ -e "$f" ]; then printf 'TAKEN %s %s\n' "$n" "$f"
  else printf 'REPORT_FILE %s=%s\n' "$n" "$f"; fi
done
```

A `REPORT_FILE <name>=<path>` line is that agent's path, exactly as `op=open` step 3 - the dispatcher copies each line into its agent's prompt. A `TAKEN <name> <path>` line hands out NO path: that file belongs to an earlier dispatch, and pointing a new agent at it would let `op=collect` read the earlier report as this one's. Dispatch nothing under that name - pick a fresh one (the caller's own suffix rule) and run `op=add` for it. For Step 3's `{name}-r2`, `TAKEN` is a second guard against a repeat retry (a `-r2` file already on disk, e.g. after a resume); the first guard is the run's own dispatch record (Step 3).

## op=peek - inspect without concluding

Args: `dir=<directory> names=<name>,<name>`. Runs `op=collect` Step 1 (classify) and, for each `REPORT` line in state `OK`, Step 2 (READ EVERY REPORT IN FULL) - nothing else: no retry, no `GAP`, no Coverage line. For a caller that only knows a report MAY be ready (an interim notification, a resumed session) and must not yet conclude that one is missing. `OK` -> the caller has the report. Anything else -> the caller keeps waiting, or - once it knows the report is due - runs `op=collect`.

## op=collect - after a phase's agents return

Args: `dir=<directory> names=<name>,<name> [retry=no]` - exactly the names dispatched in this phase.

### Step 1: Classify every expected file

This block is run as written. It holds no dollar sign followed by a digit and no dollar-ARGUMENTS placeholder: Claude Code substitutes those with this skill's arguments before the text reaches you, so an awk whole-line field reference would arrive as `op=collect` (pinned by `agentreport_test.go`).

It prints ONE line per expected name, `REPORT <name> <state> <path|none>`, judging the name's own file and its `-r2` retry file together: an `OK` file wins (the retry's, else the original - a late original counts); otherwise the richest file with content (an `UNTERMINATED` file - the larger non-blank line count, a tie to the retry); none with content -> `none`. Every consumer keys by `<name>`; none strips a retry suffix.

```bash
REPORT_DIR="{dir}"
classify() {   # sets s (state) and c (non-blank lines) for the file in f
  c=0
  if [ ! -e "$f" ]; then s=MISSING
  elif [ ! -s "$f" ]; then s=EMPTY
  elif [ ! -r "$f" ]; then s=UNREADABLE
  else
    c=$(grep -c '[^[:space:]]' "$f" 2>/dev/null); c=${c:-x}              # non-blank lines
    l=$(grep '[^[:space:]]' "$f" 2>/dev/null | tail -n 1 | sed 's/[[:space:]]*$//')   # last non-blank line
    if [ "$c" = x ]; then s=UNREADABLE; c=0
    elif [ "$c" -eq 0 ]; then s=EMPTY               # whitespace only
    elif [ "$l" != "REPORT-END" ]; then s=UNTERMINATED
    elif [ "$c" -lt 2 ]; then s=EMPTY               # the sentinel alone is not a report
    else s=OK; fi
  fi
}
for n in $(printf '%s' "{names}" | tr ',' ' '); do   # names: comma-separated, [a-z0-9.-] only
  f="$REPORT_DIR/$n.md"; classify; s1=$s; c1=$c; p1=$f
  f="$REPORT_DIR/$n-r2.md"; s2=""; c2=0; p2=""
  if [ -e "$f" ]; then classify; s2=$s; c2=$c; p2=$f; fi
  if [ "$s2" = OK ]; then printf 'REPORT %s OK %s\n' "$n" "$p2"
  elif [ "$s1" = OK ]; then printf 'REPORT %s OK %s\n' "$n" "$p1"
  elif [ "$s2" = UNTERMINATED ] && { [ "$s1" != UNTERMINATED ] || [ "$c2" -ge "$c1" ]; }; then printf 'REPORT %s UNTERMINATED %s\n' "$n" "$p2"
  elif [ "$s1" = UNTERMINATED ]; then printf 'REPORT %s UNTERMINATED %s\n' "$n" "$p1"
  else printf 'REPORT %s %s none\n' "$n" "${s2:-$s1}"; fi
done
```

### Step 2: READ EVERY REPORT IN FULL

For each `REPORT <name> OK <path>` line, Read `<path>` with the Read tool from its first line through its LAST line. A Read that stops before the last line - or is refused for size - continues with `offset` / `limit` until the file ends; size is never a reason to call a file UNREADABLE. Only the final line's `REPORT-END` is the completion mark - a `REPORT-END` earlier in the body (a report about this protocol) is content, never a place to stop. Never a slice, a head, a grep or a summary; never skip a report because another one said the same. For an `OK` file the agent's final text is not the report - ignore it.

Selective reading is the failure a file transport introduces: a report you did not read is a lens you silently dropped.

### Step 3: One retry, then a loud gap

- Any `REPORT` line not in state `OK` (no `REPORT` line for a name, or a Read of an `OK` path that errored, counts as `UNREADABLE` - never skipped) -> re-dispatch THAT agent ONCE per run with a FRESH report file: `op=add dir={dir} names={name}-r2`. A name this run already re-dispatched (your own record of this run - the retry message you sent) is never re-dispatched again, even when its `-r2` file is missing: it goes straight to resolution and, still not `OK`, to its gap. `TAKEN` on `{name}-r2` is the second guard - a `-r2` file already on disk (a collect run again after a resume): no second retry either. Otherwise the re-dispatch is the original prompt with its `REPORT_FILE:` line replaced by the `-r2` path and, right after it, the one line `Your previous attempt left its report file {state}. Write the WHOLE report to REPORT_FILE in ONE Write call, last line REPORT-END.` - never the failed content or its path (the failed attempt is the only evidence of what went wrong, and the Write tool refuses to overwrite a file the new agent has not read). Every retry of the phase goes in one message; then Step 1 again for the retried names (it judges each name's original AND `-r2` file - a late original counts, never a false gap) and Step 2 for its `OK` lines.
- Coverage counts names, not files: a name is `OK` when its `REPORT` line is.
- `retry=no` (agents that change the tree - implementer, teammates) -> no re-dispatch; the name goes straight to a gap.
- A Read error on the post-retry pass -> `GAP <name>: UNREADABLE`.
- Still not `OK` -> `GAP <name>: <state>`. A `REPORT` line with a path (an `UNTERMINATED` file - content without `REPORT-END`) -> Read that file IN FULL and carry it into the aggregation marked `UNVERIFIED - report file without REPORT-END, may be incomplete`. A path of `none` -> when the agent's final text held report content, carry it marked `UNVERIFIED - from the final text, may be truncated` (the retry's final text, else the first attempt's); otherwise the lens contributes nothing. A file with content beats the channel this skill exists to replace.

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
