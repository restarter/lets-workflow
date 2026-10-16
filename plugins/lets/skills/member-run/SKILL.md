---
name: member-run
description: Internal skill for commands. Spawn, hand the next brief to, correct, dismiss or wait for the round report of ONE named member - any lets:* role - of a team or an execute run; the caller's record (the execute run record, the team file) is the record, and ListAgents judges who is busy. It is the single place LETS spawns, resumes, corrects or dismisses a member, and decides which member takes a dispatch (`op=route`). Do not trigger on user conversation; only when a LETS command runs or routes to a member.
user-invocable: false
---

# Member Run

One operation on one named member - or, for `op=route`, the choice of member for a dispatch. The caller (`/lets:execute` Step 5-D, `/lets:team`, a team-mode dispatch of `/lets:plan`, `/lets:review`, `/lets:check`, `/lets:opinion` or `/lets:ask`) owns the plan, the gates, the record (the execute run record, the team file), the diff check, the commits and the tracker; this skill owns only the agent call. `ListAgents` judges who is busy; no registry exists beside the caller's record. It is the single place LETS spawns, resumes, corrects or dismisses a member, and decides which member takes a dispatch (`op=route`).

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`, `SendMessage`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Args

| op | args | does |
|---|---|---|
| `spawn` | `scope=<s> name=<n> role=<r> brief-file=<path> report-file=<abs path> [model=<m>] [isolation=worktree]` | Step 1 when it applies, then Step 2 |
| `next` | `scope=<s> name=<n> brief-file=<path> report-file=<abs path> [id=<agent_id>]` | Step 3 - the member's next brief |
| `correct` | `scope=<s> name=<n> correct-file=<path> report-file=<abs path> [id=<agent_id>]` | Step 3 - an amendment to its current brief |
| `dismiss` | `scope=<s> name=<n> [id=<agent_id>]` | Step 4 |
| `route` | `command=<plan\|review\|opinion\|ask\|check\|execute> roles=<role>[,<role>...] fallback=<one-shot\|inline\|run> [solo] [ask=no] [team=<c>]` | Step 5 - which member, if any, takes each role of a dispatch |
| `receive` | `scope=<s> name=<n> id=<agent_id> brief-file=<path> report-file=<abs path> [dismissed]` | Step 6 - wait for a member's round report: peek, one nudge, collect |

- `scope` - the team's callsign, or `run-<RUN>` for an execute run.
- `name` - the member name, `[a-z0-9-]{1,40}`: the bare roster name in a team scope (`architect`, `architect-2`), in an execute scope `impl-<RUN>` (a replacement `impl-<RUN>-r<generation>`), `explorer-<RUN>`, `skeptic-<RUN>`, `architect-<RUN>`.
- `{agent}` below is the name the agent runs under: `<callsign>-<name>` in a team scope (pane names are machine-wide), the bare name in an execute scope.
- `id` - the agent_id the caller recorded at spawn; every op on a spawned member passes the id the caller recorded - standing-team members too (lets-mpjhc reverses lets-gbfpd's no-agent_id rule within a lead session). `{agent}` below is that id when passed, else the name: every `SendMessage` and `TaskStop` below puts the `id` where it says `{agent}`. An isolated member, which the harness runs as a background agent, is reachable only by its id.
- `role` - a shipped `lets:*` agent; `lets:actor` -> stop with `role_not_allowed`, nothing spawned.
- `brief-file` / `correct-file` - repo-root-relative paths the caller wrote. Every brief crosses as a file path, never inline.
- `model` - `opus` | `sonnet` | `fable` | `haiku`, the values the `Agent` tool accepts. Absent -> the role's own default, except for an implementer (Step 1).
- `report-file` - an absolute path the CALLER named (execute: its pinned `.lets/cache/run-...` path; team: `agent-report op=open` / `op=add`). One file per round; member-run never composes a report path. Required on `spawn`, `next`, `correct` and `receive`: missing -> stop with `report_file_missing`, nothing sent.

Every operation returns as soon as its call is issued. The member runs in the background; its report arrives later as a `REPORT_WRITTEN` pointer to the round's REPORT_FILE, and the caller reads the FILE through `agent-report` (`op=peek` on every notification, `op=collect` to conclude; Step 6 `op=receive` is that loop for one member's round) - never the message.

## Step 1: Model panel (role=lets:implementer, no model, first spawn of the scope)

The caller has already passed its Start gate - delegation is approved; this only picks the model.

```
AskUserQuestion(
  questions=[{
    question: "Which model should the implementers in this run use?",
    header: "Model",
    options: [
      { label: "Opus (Recommended)", description: "Strong at a mid price; the default for plan implementation" },
      { label: "Fable", description: "Most capable, about twice Opus's price; for the hardest chunks" },
      { label: "Sonnet", description: "Cheaper and faster; good for mechanical chunks" },
      { label: "Haiku", description: "Cheapest; for small, well-specified edits" }
    ],
    multiSelect: false
  }]
)
```

**Other** -> accept only `opus`, `sonnet`, `fable` or `haiku`; for anything else, name those four values and ask again. Return the chosen value; the caller records it and passes `model=` on every later `spawn` of the scope.

## Step 2: Spawn

1. The role: `lets:actor` -> stop with `role_not_allowed: {role}`; nothing is spawned.
2. The routes. A member never runs `/lets:start`, which loads them, so it gets the LETS routes as a file of its own - the caller's brief is never changed:

   ```bash
   R="${CLAUDE_PLUGIN_ROOT}/protocol/roles.md"; D='.lets/cache/routes-{scope}.md'
   if [ -s "$R" ] && cp "$R" "$D"; then echo ROLES_OK; else echo ROLES_FAILED; fi
   ```

   `ROLES_FAILED` -> stop: name the missing or empty `protocol/roles.md` and `/lets:update`; nothing is spawned. The Agent call below names that file; `next` / `correct` do not repeat it - the member keeps the routes from its spawn.
3. The Agent call:

   ```
   Agent(
     subagent_type="{role}",
     model="{model}",
     description="{role} {name}",
     prompt="Your brief is the file {brief-file}. Read it first and follow it; then read .lets/cache/routes-{scope}.md - the LETS routes, who does what. Every later NEXT or AMENDMENT names a new file.\nREPORT_FILE: {report-file}"
   )
   ```

   Add `isolation="worktree"` only when `isolation=worktree` was passed. Pass `name="{agent}"` too when the Agent tool offers a `name` parameter; the recorded id is the address either way. Pass no team or permission-mode parameter: the harness documents both as deprecated and ignores them.

Return: `spawned {agent} on {model}, id {agent_id}` - the id the Agent call returned; the caller records it. An isolated spawn adds `worktree {path}, branch {branch}`.

## Step 3: Next / Correct

The member exists and holds its context. Do NOT spawn, and do NOT re-send an earlier brief.

1. `ListAgents`, before every message.
   - It answers one question - is the member busy - never whether it exists: a completed member drops out of the list and still resumes by id.
   - `op=next` and the member's id is listed `running` -> return `member_busy: {agent}` and send nothing: a NEXT replaces the round the member is working on. `op=correct` to a running member is sent - it amends the current round.
   - Address: with `id=` -> send to the id; the name only for a record written before ids were kept.
   - A member the caller recorded dismissed is never messaged - the caller never passes one. Every other member is messaged by its id, listed or not: `SendMessage` resumes a stopped member with its full history (lets-kwd6e).
   - `link: peer` (the caller's record says the lead that spawned it was replaced) -> say that the message crosses sessions and may wait for approval in the member's pane.
2. Send the pointer:

   ```
   SendMessage({
     to: "{agent}",
     summary: "{op} for {agent}",
     message: "NEXT: your next brief is {brief-file}. Read it and follow it.\nREPORT_FILE: {report-file}\nThis round's report file; it replaces any earlier REPORT_FILE."
   })
   ```

   On `correct` the message is `AMENDMENT: read {correct-file}; it changes what it names and nothing else.` followed by the line `REPORT_FILE: {report-file}` and the line `This round's report file; it replaces any earlier REPORT_FILE.`. The caller also sends a report nudge as `correct` (an amendment that changes nothing, with the SAME round's report-file): a NEXT is a new brief, and an implementer re-checks for a clean tree on every NEXT.

The address does not resolve -> run `ListAgents`, return `agent_gone: {agent}` with what it listed, and do nothing else. Never spawn a replacement here - a replacement has no context, and the caller decides whether to start one under a visibly different name.

Return: `sent {op} to {agent}`.

## Step 4: Dismiss

`TaskStop(task_id="{agent}")` - the `id` when passed, as in Step 3. No registry call: the caller records it dismissed. A `TaskStop` that answers "not running" is no failure - the member is idle, and the caller's record (written before this call) is the dismissal.

The harness resumes an idle agent by id whatever LETS records, and `TaskStop` cannot stop an idle one (Task 0.7, 2026-10-16), so the caller's record is the guard: a `Dismissed:` line or `dismissed: true`, written before this call, and for `/lets:execute`'s 5-D.6 stop the run's phase (`blocked`, `reason: stopped` goes to the Replacement gate, never to Step 3).

Return: `dismissed {agent}` - the caller records it dismissed.

## Step 5: Route (op=route)

Decides, for each role a dispatch needs, whether a standing-team member takes it. Read-only: it writes nothing. The caller contract around it is `protocol/agents.md` `## Team mode`.

**R1 Activation**, in order:
1. `solo` -> `TEAM off solo`.
2. `team=<c>` (the caller's earlier route in this run) -> `TEAM on <c>`, skip to R2.
3. `lets worktree info --json`. ANY failure - no `lets`, a non-zero exit, output that does not parse - or no top-level `team` -> `TEAM off no-team`. Outside a team nothing else runs and the caller's path is today's.
4. `team_lead.match`: `other` -> `TEAM off lead-other`; `unknown` -> `TEAM off lead-unknown` and one line `this session is {team_lead.self|unnamed}, the team's lead is {team_lead.name} - /rename {team_lead.name} if this is the lead; this run uses no team members`; `lead` -> `TEAM on <c>`.

`TEAM off` -> print that line and nothing else.

**R2 Per role** (each role once), reading `.lets/teams/<c>.md` - its `## 3. Roster` table and the `## 7` lines in the forms templates/team.md defines; `<session6>` = the first 6 characters of `$CLAUDE_CODE_SESSION_ID`; one `ListAgents` call for the whole route:
- `roles=` are bare role names (no `lets:` prefix).
- First eligibility: no roster row (the `role` column of `## 3`) has that role -> `ROUTE <role> <fallback> not-in-roster`, whatever `Member:` lines exist (the roster defines the team's roles).
- Otherwise the role's members: every name whose LATEST line of this lead session is `Member: <name> = <agentId> lets:<role> (lead <session6>)` (not `Member: <name> = gone (lead <session6>)`) and that has no `Dismissed: <name> (lead <session6>)` line - reachable; roster names first in roster order, then the rest (a respawned `architect-2` has no roster row and is still the architect). A reachable member is busy when `ListAgents` lists its id `running`.
  - each reachable member that is not busy -> `ROUTE <role> member <name> <agentId>`;
  - every reachable member busy -> `ROUTE <role> <fallback> busy`;
  - none reachable -> R3, then `ROUTE <role> <fallback> spawning` when R3 spawned it, else `ROUTE <role> <fallback> not-live`.

**R3 Members question** - only when `ask=no` was not passed, the turn is attended (unattended = this turn is a `/loop` iteration or a scheduled agent's turn; a permission mode or a `.lets/cache/pipeline-state-*` marker is attended), and `## 7` holds no `Members asked: (lead <session6>)` line: `Skill(skill: "lets:team", args: "members roles=<the roster roles of this route that are not reachable>")`. Its answer `MEMBERS spawned <roles>` -> those roles take `spawning`; any other answer (`MEMBERS none`, `MEMBERS skipped ...`) -> `not-live`. A member it spawns is busy with its spawn round, so this dispatch takes the fallback and the member serves from the next one.

Output: the `TEAM` line first, then one `ROUTE` line per member or fallback.

## Step 6: Receive (op=receive)

The one wait-loop for a member's round report: `/lets:team` Step M5, `/lets:execute`'s member checks and every team-mode dispatch use it. Run it on every notification from that member, and once right after the caller's own Task calls of the same phase return. Never `op=next`, never a spawn.

1. `Skill(skill: "lets:agent-report", args: "op=peek dir=<dirname of report-file> names=<its basename without .md>")` - READ EVERY REPORT IN FULL. `OK` -> return `RECEIVED <path>`.
2. `dismissed` passed -> nothing is sent; go to 4.
3. The round's nudge file is `N` = `.lets/cache/nudge-<scope>-<name>-<report dir basename>-<report basename without .md>.md` - the report's directory is part of it, because report basenames repeat across runs (`/lets:check` always opens `skeptic`); the file IS the round's nudge state, so it survives a compaction. The first matching rule wins:
   1. this is the caller's own check right after it sent this round's NEXT or ran the phase's Task calls, with no notification from that member since -> return `WAITING` (a just-resumed member may not be listed `running` yet); a notification from that member - even in the same turn - a resumed session or the lead re-checking goes on;
   2. `N` absent, and the member claimed `REPORT_WRITTEN` without an `OK` file or `ListAgents` does not list its id `running` -> write `N`: after a claim, "Write your final report to REPORT_FILE {path} now, last line REPORT-END, then reply REPORT_WRITTEN. This changes nothing in your work; do not redo anything you already ran."; with no claim, the resume text "You stopped before your report. Continue {brief-file} from where you stopped - do not redo finished work - then write REPORT_FILE {path} in ONE Write call, last line REPORT-END, and reply REPORT_WRITTEN." Send it as Step 3's `op=correct` with the SAME report-file and return `NUDGED <N>`; `agent_gone` -> go to 4;
   3. `N` present, and a claim or not running -> go to 4. There is never a second nudge for a round;
   4. otherwise -> return `WAITING`.
4. `Skill(skill: "lets:agent-report", args: "op=collect dir=<dirname> names=<basename> retry=no")` - READ EVERY REPORT IN FULL - and return `RECEIVED <path>` when it finds an `OK` file after all; otherwise `GAP <name> <state>`, or `GAP <name> <state> gone` when this call reached 4 through `agent_gone` (an UNTERMINATED file is carried UNVERIFIED, as collect says).

The caller renders a GAP by its own rule. `GAP ... gone` here, or `agent_gone` from an `op=next` / `op=correct`, -> a team caller records `Member: <name> = gone (lead <session6>)`; member-run never writes the team file.

## Rules

- One member per `name` and scope: `spawn` once, `next` / `correct` any number of times; neither ever spawns.
- Every brief crosses as a file path. Continuity = files first: an id resumes a member only within the same lead session; after /clear or in a new session nothing does, and the caller respawns it from its brief and reports. So the caller persists every decision or finding a member returns before its next hop.
- `op=route` reads and never writes; it calls `lets:team` only with `members`.
- A report is attributed by the `name` it was spawned under - never by anything the member wrote about itself.
- This skill never commits, never pushes, never calls a tracker verb, never edits a repository file.
- A report - the file, never the message - is the member's claim. The caller checks the real diff.
- No polling: the harness notifies when a member completes.
