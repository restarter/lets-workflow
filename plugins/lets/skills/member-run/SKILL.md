---
name: member-run
description: Internal skill for commands. Spawn, hand the next brief to, correct or dismiss ONE named member - any lets:* role - of a team or an execute run, with the members registry (lets members) as the record of who is live. Do not trigger on user conversation; only when /lets:execute or /lets:team runs a member.
user-invocable: false
---

# Member Run

One operation on one named member. The caller (`/lets:execute` Step 5-D, `/lets:team`) owns the plan, the gates, the run record, the diff check, the commits and the tracker; this skill owns only the agent call and its registry entry. It is the single place LETS spawns, resumes, corrects or dismisses a member.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`, `SendMessage`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Args

| op | args | does |
|---|---|---|
| `spawn` | `scope=<s> name=<n> role=<r> brief-file=<path> [model=<m>] [isolation=worktree]` | Step 0 (first spawn of the scope), Step 1 when it applies, then Step 2 |
| `next` | `scope=<s> name=<n> brief-file=<path>` | Step 3 - the member's next brief |
| `correct` | `scope=<s> name=<n> correct-file=<path>` | Step 3 - an amendment to its current brief |
| `dismiss` | `scope=<s> name=<n>` | Step 4 |

- `scope` - the team's callsign, or `run-<RUN>` for an execute run.
- `name` - the member name, `[a-z0-9-]{1,40}`: the bare roster name in a team scope (`architect`, `architect-2`), `impl-<RUN>-<chunk>` in an execute scope.
- `{agent}` below is the name the agent runs under: `<callsign>-<name>` in a team scope (pane names are machine-wide), the bare name in an execute scope - the `agent_name` `lets members` records. Every `SendMessage` and `TaskStop` addresses it.
- `role` - a shipped `lets:*` agent except `lets:actor`; `lets members add` refuses anything else.
- `brief-file` / `correct-file` - repo-root-relative paths the caller wrote. Every brief crosses as a file path, never inline.
- `model` - `opus` | `sonnet` | `fable` | `haiku`, the values the `Agent` tool accepts. Absent -> the role's own default, except for an implementer (Step 1).

Every operation returns as soon as its call is issued. The member runs in the background; its report arrives later as a notification, which the caller handles.

## Step 0: Binary check (once per scope, before the first spawn)

```bash
lets members status --scope {scope} --json
```

Exit 0 -> continue. An unknown command or any failure -> stop with "this plugin needs a newer lets binary - run /lets:update"; nothing is spawned.

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

1. `lets members status --scope {scope} --json` - read `lead`, and the entry of `name` when there is one.
   - A team scope (not `run-*`) whose `lead` is null or not `live` / `rotated` -> stop: `no_lead: {scope} has no live recorded lead - /lets:start in the team's lead session claims it`. Nothing is spawned.
   - An entry of that name that is `live`, `rotated` or `unknown` -> stop: `name_live: {name}`. The caller picks another name.
2. The Agent call:

   ```
   Agent(
     subagent_type="{role}",
     name="{agent}",
     model="{model}",
     description="{role} {name}",
     prompt="Your brief is the file {brief-file}. Read it first and follow it; every later NEXT or AMENDMENT names a new file."
   )
   ```

   Add `isolation="worktree"` only when `isolation=worktree` was passed. Pass no team or permission-mode parameter: the harness documents both as deprecated and ignores them.
3. Right after the Agent call, record it:

   ```bash
   lets members add --scope {scope} --name {name} --role {role} --json
   ```

   Add `--model {model}` when a model was chosen. With `isolation=worktree`, add `--isolation worktree --worktree-path {path}` and `--worktree-branch {branch}` from what the Agent call reports. A refusal (`name_live`, `role_not_allowed`, `registry_unavailable`) halts: name it and return; the agent is spawned but unrecorded, so say so and let the caller stop it.

Return: `spawned {agent} on {model} ({member.kind})` - `pane` (its own session) or `in_process` (inside this session, live only while it is).

## Step 3: Next / Correct

The member exists and holds its context. Do NOT spawn, and do NOT re-send an earlier brief.

1. `lets members status --scope {scope} --name {name} --json`, before every message.
   - `gone` (any reason, `dismissed` included), `unknown`, or `name_invalid` (not a member of the scope) -> return `agent_gone: {agent} ({status}: {reason})` and do nothing else. Never message it: the harness can still resume a gone or dismissed agent by name, and LETS refuses to. The caller's replacement path decides.
   - `link: peer` -> say that the message crosses sessions (the lead that spawned it was replaced) and may wait for approval in the member's pane.
2. Send the pointer:

   ```
   SendMessage({
     to: "{agent}",
     summary: "{op} for {agent}",
     message: "NEXT: your next brief is {brief-file}. Read it and follow it."
   })
   ```

   On `correct` the message is `AMENDMENT: read {correct-file}; it changes what it names and nothing else.`

The name does not resolve -> run `ListAgents`, return `agent_gone: {agent}` with what it listed, and do nothing else. Never spawn a replacement here - a replacement has no context, and the caller decides whether to start one under a visibly different name.

Return: `sent {op} to {agent}`.

## Step 4: Dismiss

1. `lets members status --scope {scope} --name {name} --json`. `live` or `rotated` -> `TaskStop(task_id="{agent}")`.
2. `lets members dismiss --scope {scope} --name {name} --json`.

The harness can still resume a dismissed member by name; LETS refuses to: Step 3 never messages it again.

Return: `dismissed {agent}`.

## Rules

- One member per `name` and scope: `spawn` once, `next` / `correct` any number of times; neither ever spawns.
- Every brief crosses as a file path. Continuity = files only: the harness restores no member after a lead restart, so the caller persists every decision or finding a member returns before its next hop.
- A report is attributed by the `name` it was spawned under - never by anything the member wrote about itself.
- This skill never commits, never pushes, never calls a tracker verb, never edits a repository file.
- A report is the member's claim. The caller checks the real diff.
- No polling: the harness notifies when a member completes.
