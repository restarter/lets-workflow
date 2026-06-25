---
name: tracker-TEMPLATE
version: 0.0.0
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - they are managed by `lets init` / `lets update`. Edit the canonical source in plugins/lets/rules/ instead. This TEMPLATE is a skeleton an adapter author copies to tracker-<name>.md; it is never selected by LETS_TRACKER and never installed. -->

# Tracker adapter: TEMPLATE

This file teaches the orchestrator how to perform the neutral task-tracker verbs for ONE adapter (one `tracker × transport` instance). Each LETS command calls a NEUTRAL VERB; resolve it to the concrete call via the Bindings table below.

- **Verb resolution is ORCHESTRATOR-ONLY.** Subagents do not receive this file (it is not injected into their context) and never call tracker verbs directly.
- **The "no adapter file loaded" fallback does NOT live here.** A file that is not loaded cannot instruct anything - that rule lives in the command bodies and is conditioned on the `LETS_TRACKER` value (see CONTRIBUTING "Add a tracker adapter").

## Neutral statuses

Required: `open`, `in_progress`, `closed`. Optional: `in_review`, `blocked`.

`show` and `list-by-status` MUST return the task's status as one of these neutral names (the adapter translates native -> neutral), and the neutral task shape `{id, title, status, url}`. This keeps consuming commands (e.g. `[ "$STATUS" = "in_progress" ]`) adapter-agnostic.

## Capabilities + bindings

<!-- PINNED CONTRACT: the header row below is fixed and machine-parsed by the adapter contract test and the beads golden test. Do not reorder or rename columns; put any extra detail inside the binding cell, never in a new column. -->

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes    | {how to create a task; returns `{id, url}`} |
| show           | CORE | yes    | {how to fetch `{id, title, status, url, description}` by id; status as a NEUTRAL name} |
| comment-add    | CORE | yes    | {how to add a comment body to a task} |
| set-status     | CORE | yes    | {how to set a NEUTRAL status on a task} |
| close          | CORE | yes    | {how to close/complete a task (optional reason)} |
| comment-list   | OPT  | yes/no | {how to list a task's comments, or `absent`} |
| list-by-status | OPT  | yes/no | {how to list tasks by neutral status (status returned NEUTRAL), or `absent`} |
| search         | OPT  | yes/no | {how to search tasks by text, or `absent`} |
| ready/stats    | OPT  | yes/no | {ready-graph / stats / blocked views, or `absent`} |
| label          | OPT  | yes/no | {label ops, or `absent`} |
| assignee       | OPT  | yes/no | {assignee ops, or `absent`} |
| set-field      | OPT  | yes/no | {overwrite a field (e.g. description), or `absent`} |

## Degradation

- **OPTIONAL verb absent** (`supported = no`) -> the calling command continues and tells the user the capability is unavailable for this tracker. Never crash.
- **CORE verb unresolved at runtime** (binding can't be performed - e.g. an MCP tool is not connected) -> HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); do NOT report success. This matters under AUTO MODE: a `/lets:done` must never claim it closed a task it did not.

## Board profile (optional)

Project-specific semantics (native status ids, transitions, principles, default project, server name, REST nuances) live in a sibling `tracker-TEMPLATE.board.md` - user-owned, scaffolded once by `lets init`, NEVER overwritten, auto-loaded as a project instruction. If present, honor its status map + transitions. Switching `LETS_TRACKER` does NOT remove a board file (only the managed adapter `.md` is cleaned up) - delete a stale `*.board.md` by hand so it stops loading into context.

<!-- NEVER put a token/password/secret in a .board.md: it is auto-loaded into model context every session AND is git-shareable. Secrets belong only in the adapter's transport config (e.g. an MCP server's own env), never here and never in .lets/.env. -->

## Secrets (only if this adapter calls an external API directly)

The shipped adapters do not need this (beads uses `.beads/.env`; planfix-mcp's token is owned by the MCP server). A future direct-REST adapter that needs a token MUST read it from a gitignored, user-private file (e.g. `.lets/trackers/<name>/.env`) - NEVER from `.lets/.env` (mode 644, injected into model context); add the file + 0600/0700 perms helper when the first such adapter lands.
