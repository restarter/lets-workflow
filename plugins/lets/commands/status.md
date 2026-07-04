---
description: Read-only orientation snapshot - where you are, what's in flight, what's next (tracker-universal)
---

# Status

A fast, read-only orientation: where you are, what's in flight, what's next. No menu, no dialog, no mutation, no agents. Argless.

For working the backlog (pulse / ideas / cleanup) use `/lets:backlog`. To claim a task and start, `/lets:start`.

## Step 1: Render the snapshot

Invoke `Skill(skill: "lets:orient")` - it renders `## Where you are` / `## In flight` / `## Next up` (and `## Project` counts when the tracker provides them), degrading per tracker (`beads` | `planfix-mcp` | `none`).

If a legacy view argument was passed (`overview` / `ready` / `labels` / `blocked` / `full`), ignore it and note once: "Status is now a single orient snapshot; the old views were removed. Deep beads dashboards live in native `bd stats` / `bd blocked`."

## Step 2: Footer (Response Footer model)

State-driven Nav (per `.claude/rules/lets-rules.md` `### Response Footer`):
- Uncommitted changes on disk -> `/lets:commit`.
- Active task, clean tree -> `/lets:note` + `/lets:check`.
- No active task -> `/lets:start`.

## Rules

- Read-only - never mutate or claim from here.
- Respond in the user's language.
