---
name: tracker-beads
version: 0.6.4
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->

# Tracker adapter: beads (beads x cli)

The reference adapter. Binds the neutral verbs to the `bd` CLI - the historical, fully-supported tracker. Runtime is byte-for-byte the same `bd` invocations LETS always made.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- `bd` already emits the neutral status names (`open` / `in_progress` / `closed`) - the status map is identity, no translation needed.
- Command/skill bodies carry ` ```lets-tracker ` blocks (see lets-rules "Tracker Adapters"); this table is how they resolve for beads - golden-pinned (`TestTrackerBeads_BindsBdCommands`) byte-for-byte against the historical `bd` invocations. A `comment-add` body arrives as `body-file=<path>`; the beads binding is `bd comments add <id> "$(cat <path>)"`.

## Neutral statuses

`open`, `in_progress`, `closed` (+ `blocked`, supported by beads). These ARE beads' native status names.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | `bd create --title=... --type=... --priority=... --labels=... --description="..."` (via the create-task skill); a multi-line `description` arrives as `description-file=<path>` → `--description="$(cat <description-file>)"`. Returns the new id/url |
| show           | CORE | yes | `bd show <id>` (text) or `bd show <id> --json` when a field is parsed; `--json` exposes `status` as a neutral name |
| comment-add    | CORE | yes | `bd comments add <id> "$(cat <body-file>)"` for `body-file=<path>`, or `bd comments add <id> "<body>"` for inline `body=`; empty body → HARD-FAIL, do not submit |
| set-status     | CORE | yes | `bd update <id> --status=<open\|in_progress\|closed>` |
| close          | CORE | yes | `bd close <id> [--reason="..."]` |
| comment-list   | OPT  | yes | `bd comments <id>` |
| list-by-status | OPT  | yes | `bd list --status=<status> [--json] [--format=ids]`; `--json` exposes `status`/`priority` for parsing |
| search         | OPT  | yes | `bd search <query>` |
| ready/stats    | OPT  | yes | `bd ready [--limit N]` / `bd stats` / `bd blocked` (dep-graph tree) / priority histogram (see Notes) |
| label          | OPT  | yes | bare `label` (all project labels, e.g. the `epic:*` set) → `bd label list-all`; `label task=<id>` → `bd label list <id>` (one issue's labels); `label add task=<id> value=<l>` → `bd label add <id> <l>`. The bare verb is `list-all`, NOT `bd label list` (which requires an id and errors without one) |
| assignee       | OPT  | yes | `bd update <id> --assignee=<name>` |
| set-field      | OPT  | yes | `bd update <id> --description="..."` (overwrite) |

## Degradation

beads supports every verb, so nothing degrades. A CORE verb that somehow fails (e.g. `bd` not on PATH) must HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); never report a phantom success.

## Notes

- `stats` dashboards (used by `/lets:status`): the `bd blocked` dep-graph tree and the priority histogram `bd list --status=open --json | jq -r '.[].priority' | sort | uniq -c | sort -rn` are the beads binding of the `stats`/`ready` views. An adapter that marks these `absent` → the calling command degrades those sections and tells the user (it does NOT render a broken/empty dashboard).
- `memory` (`bd remember`) and `dependencies`-write (`bd dep add`) exist in beads but no LETS command calls them, so they are not part of the neutral contract.
- State-changing ops (`bd close`, `bd update --status`, `bd dolt push`) stay gated under AUTO MODE per the workflow rules.
