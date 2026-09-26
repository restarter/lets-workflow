---
name: implementer-run
description: Internal skill for commands. Spawn or correct ONE named implementer subagent - model panel on the first spawn of a run, the Agent call, the SendMessage correction, attribution by name. Do not trigger on user conversation; only when /lets:execute delegates a plan chunk.
user-invocable: false
---

# Implementer Run

One operation on one named implementer. The caller (`/lets:execute` Step 5-D) owns the plan, the gates, the run record, the diff check, the commits and the tracker; this skill owns only the agent call. It is the single place LETS spawns or corrects an implementer - `/lets:team`'s rebuild (lets-7dwc1) is required to call it rather than duplicate it.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`, `SendMessage`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Args

| op | args | does |
|---|---|---|
| `spawn` | `name=<n> chunk-file=<path> [model=<m>]` | Step 1 when `model` is absent, then Step 2 |
| `correct` | `name=<n> correct-file=<path>` | Step 3 |

- `name` - `impl-<run>-<chunk>` from the caller's run record, `[a-z0-9-]{1,40}`. One agent per name: `spawn` a name exactly once, `correct` it any number of times.
- `chunk-file` / `correct-file` - repo-root-relative paths the caller wrote. Multi-line text crosses as a file, never inline.
- `model` - `opus` | `sonnet` | `fable` | `haiku`, the values the `Agent` tool accepts.

Both operations return as soon as the call is issued. The agent runs in the background; its report arrives later as a `REPORT_WRITTEN` pointer to the file its brief named; the caller reads the file.

## Step 1: Model panel (first spawn of a run only)

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

**Other** -> accept only `opus`, `sonnet`, `fable` or `haiku`; for anything else, name those four values and ask again. Return the chosen value; the caller records it and passes `model=` on every later `spawn` of the run.

## Step 2: Spawn

Read `chunk-file`, then:

```
Agent(
  subagent_type="lets:implementer",
  name="{name}",
  model="{model}",
  description="Implement {chunk id}",
  prompt="{chunk-file contents, verbatim}"
)
```

Never pass `team_name`, `mode` or `isolation`: `team_name` and `mode` are documented "Deprecated; ignored" on the current harness - `mode` being ignored is what stranded `/lets:team` (lets-7dwc1).

Return: `spawned {name} on {model}`.

## Step 3: Correct

The agent exists and holds its context. Do NOT spawn, and do NOT re-send the chunk:

```
SendMessage({
  to: "{name}",
  summary: "correction for {name}",
  message: "{correct-file contents, verbatim}"
})
```

Return: `corrected {name}`.

The name does not resolve -> run `ListAgents`, return `agent_gone: {name}` with what it listed, and do nothing else. Never spawn a replacement here - a replacement has no context, and the caller decides (its recovery step) whether to start one under a visibly different name.

## Rules

- One agent per `name`: `spawn` once, `correct` any number of times; `correct` never spawns.
- A report is attributed by the `name` it was spawned under - never by anything the agent wrote about itself.
- This skill never commits, never pushes, never calls a tracker verb, never edits a repository file.
- A report - the file, never the message - is the agent's claim. The caller checks the real diff.
- No polling: the harness notifies when an agent completes.
