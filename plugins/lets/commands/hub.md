---
description: Cross-project hub on Orca - see every project's orchestrators, ask a stopped one read-only, wake one for gated work (Orca addon)
argument-hint: "[project or orchestrator] [question]"
---

# Hub

One session that looks across the projects Orca knows: which orchestrators exist, which are alive, a read-only answer from a stopped one, or waking one in a visible Orca terminal when the work needs a human at its gates. An Orca addon: it runs only with `LETS_LAUNCHER=orca`.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Preflight (before any `lets` call)

```bash
[ "{LETS_LAUNCHER}" = "orca" ] || echo HUB_NEEDS_ORCA
```

`HUB_NEEDS_ORCA` -> print one line `/lets:hub is an Orca addon - pick the Orca launcher in /lets:init (LETS_LAUNCHER=orca)` and stop. Otherwise:

```bash
lets orca status --json
```

Not running (`status.running=false` or a `reason`) -> `orca_unavailable` plus the reason; stop.

## Step 2: List

```bash
lets peers who --orca-repos --json
```

Go validates every repo Orca lists and tags each row with `repo_index`; later calls pass `--repo-index <n>`, never a path. A project's orchestrators are the names recorded in its merge-branch task-state, and only when none of them is live the `<repo>-orc[-<scope>]` convention. Render one row per orchestrator - live ones from `peers[]`, stopped ones from `last_orchestrators[]`:

| project (`repos[index].name`) | orchestrator | scope | alive | last seen |

Name every `degraded[]` entry in one line each. A project with several orchestrators lists each.

## Step 3: Route the request

Pick the target: the orchestrator the user named; else, when the project has several:

```
AskUserQuestion(
  questions=[{
    question: "Which orchestrator of {project}?",
    header: "Orchestrator",
    options: [ /* up to 3: label = name, description = "{scope} - {alive | stopped}" */ ],
    multiSelect: false
  }]
)
```

Classify the request, and when unsure treat it as gated. Every message to a live orchestrator goes by its NAME from this run's listing, with `via=hub`: the orc skill sends outside the hub's own repo only to a row this hub run printed. A row marked `duplicate_name` cannot be addressed by name - say so and stop for it.

| request | target alive | target stopped |
|---|---|---|
| read-only (status, backlog, "what is X doing") | `Skill(skill: "lets:orc", args: "target=\"<name>\" via=hub verb=ask footer=none text=<question>")` | Step 4 ask-ro |
| gated (tracker write, commit, PR, anything else) | `Skill(skill: "lets:orc", args: "target=\"<name>\" via=hub verb=tell footer=none text=<request>")` | wake, then the same tell |

Wake (stopped target, gated request), with the session and pid from its `last_orchestrators[]` row:

```bash
lets orca wake --repo-index <n> --session '<session>' --pid <pid> --title '<name>' --json
```

`woken=true` -> the tell above to that row's name (a resume keeps the session and its name), then tell the user the gates are pressed in `<name>`'s Orca terminal. `main_alive` -> it is running after all: use the alive column. `liveness_unknown` from wake or ask-ro -> say so and stop for that project; never retry around it.

## Step 4: ask-ro (stopped target, read-only request)

1. Write the question with the Write tool to `.lets/cache/ask-ro-<session6>.txt` (`<session6>` = the first 6 chars of the row's `session`).
2. `lets peers ask-ro --repo-index <n> --session '<session>' --pid <pid> --question-file .lets/cache/ask-ro-<session6>.txt --json` (Go reads the file once and removes it).
3. `answered=true` -> relay `answer` whole, marked as that orchestrator's words from a read-only fork. `headless_readonly_unenforceable` -> this `claude` cannot narrow a headless run; say so, offer wake instead. Any other `reason` -> name it.

## Rules

- Scheduled digests are not offered: an Orca automation cannot run in plan permission mode (spike 6.0), and a digest with default permissions in another project is refused rather than widened.
- The hub never resolves another project's tracker verbs, never widens headless permissions, and never starts a second process on a live orchestrator.
- Cross-project reads happen only on an explicit `/lets:hub` request; nothing is written into another project.
- Every message goes through the orc skill with `footer=none`.

## Footer

Close: one prose line - what was relayed, which orchestrator was woken and is waiting on its human gate, or nothing after the listing.
