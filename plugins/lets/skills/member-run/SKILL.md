---
name: member-run
description: Internal skill for commands. Spawn, hand the next brief to, correct or dismiss ONE named member - any lets:* role - of a team or an execute run; the caller's record (the execute run record, the team file) is the record, and ListAgents judges who is live. Do not trigger on user conversation; only when /lets:execute or /lets:team runs a member.
user-invocable: false
---

# Member Run

One operation on one named member. The caller (`/lets:execute` Step 5-D, `/lets:team`) owns the plan, the gates, the record (the execute run record, the team file), the diff check, the commits and the tracker; this skill owns only the agent call. `ListAgents` judges who is live; no registry exists beside the caller's record. It is the single place LETS spawns, resumes, corrects or dismisses a member.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`, `SendMessage`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Args

| op | args | does |
|---|---|---|
| `spawn` | `scope=<s> name=<n> role=<r> brief-file=<path> report-file=<abs path> [model=<m>] [isolation=worktree]` | Step 1 when it applies, then Step 2 |
| `next` | `scope=<s> name=<n> brief-file=<path> report-file=<abs path> [id=<agent_id>]` | Step 3 - the member's next brief |
| `correct` | `scope=<s> name=<n> correct-file=<path> report-file=<abs path> [id=<agent_id>]` | Step 3 - an amendment to its current brief |
| `dismiss` | `scope=<s> name=<n> [id=<agent_id>]` | Step 4 |

- `scope` - the team's callsign, or `run-<RUN>` for an execute run.
- `name` - the member name, `[a-z0-9-]{1,40}`: the bare roster name in a team scope (`architect`, `architect-2`), in an execute scope `impl-<RUN>` (a replacement `impl-<RUN>-r<generation>`), `explorer-<RUN>`, `skeptic-<RUN>`, `architect-<RUN>`.
- `{agent}` below is the name the agent runs under: `<callsign>-<name>` in a team scope (pane names are machine-wide), the bare name in an execute scope.
- `id` - the agent_id the caller recorded at spawn; `{agent}` below is that id when passed, else the name: every `SendMessage` and `TaskStop` below puts the `id` where it says `{agent}`. An isolated member, which the harness runs as a background agent, is reachable only by its id.
- `role` - a shipped `lets:*` agent; `lets:actor` and `lets:lead` -> stop with `role_not_allowed`, nothing spawned.
- `brief-file` / `correct-file` - repo-root-relative paths the caller wrote. Every brief crosses as a file path, never inline.
- `model` - `opus` | `sonnet` | `fable` | `haiku`, the values the `Agent` tool accepts. Absent -> the role's own default, except for an implementer (Step 1).
- `report-file` - an absolute path the CALLER named (execute: its pinned `.lets/cache/run-...` path; team: `agent-report op=open` / `op=add`). One file per round; member-run never composes a report path. Required on `spawn`, `next` and `correct`: missing -> stop with `report_file_missing`, nothing sent.

Every operation returns as soon as its call is issued. The member runs in the background; its report arrives later as a `REPORT_WRITTEN` pointer to the round's REPORT_FILE, and the caller reads the FILE through `agent-report` (`op=peek` on every notification, `op=collect` to conclude) - never the message.

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

1. The role: `lets:actor` or `lets:lead` (a session agent, never a member) -> stop with `role_not_allowed: {role}`; nothing is spawned.
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
     name="{agent}",
     model="{model}",
     description="{role} {name}",
     prompt="Your brief is the file {brief-file}. Read it first and follow it; then read .lets/cache/routes-{scope}.md - the LETS routes, who does what. Every later NEXT or AMENDMENT names a new file.\nREPORT_FILE: {report-file}"
   )
   ```

   Add `isolation="worktree"` only when `isolation=worktree` was passed. Pass no team or permission-mode parameter: the harness documents both as deprecated and ignores them.

Return: `spawned {agent} on {model}, id {agent_id}` - the id the Agent call returned; the caller records it. An isolated spawn adds `worktree {path}, branch {branch}`.

## Step 3: Next / Correct

The member exists and holds its context. Do NOT spawn, and do NOT re-send an earlier brief.

1. `ListAgents`, before every message.
   - The member's agent name listed (busy or idle) -> send below.
   - Not listed: with `id=` -> send to the id (after a restart of this process that resumes the same session, the id alone still resumes it); without `id=` -> return `agent_gone: {agent} (not listed)` and do nothing else. Never message it: the harness can still resume a `gone` or dismissed agent by name or id, and LETS refuses to - a caller never passes a member it recorded dismissed. The caller's replacement path decides.
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

`TaskStop(task_id="{agent}")` - the `id` when passed, as in Step 3. No registry call: the caller records it dismissed.

The harness can still resume a dismissed member by name or id; LETS refuses to: the caller never passes it to Step 3 again.

Return: `dismissed {agent}` - the caller records it dismissed.

## Rules

- One member per `name` and scope: `spawn` once, `next` / `correct` any number of times; neither ever spawns.
- Every brief crosses as a file path. Continuity = files first: an id resumes a member only within the same session; after /clear or in a new session nothing does, and the caller respawns it from its brief and reports. So the caller persists every decision or finding a member returns before its next hop.
- A report is attributed by the `name` it was spawned under - never by anything the member wrote about itself.
- This skill never commits, never pushes, never calls a tracker verb, never edits a repository file.
- A report - the file, never the message - is the member's claim. The caller checks the real diff.
- No polling: the harness notifies when a member completes.
