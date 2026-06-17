---
name: tracker-beads
version: 0.6.4
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->

# Tracker adapter: beads (beads x cli)

The reference adapter. Binds the neutral verbs to the `bd` CLI - the historical, fully-supported tracker. Runtime is byte-for-byte the same `bd` invocations LETS always made.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- `bd` already emits the neutral status names (`open` / `in_progress` / `closed`) - the status map is identity, no translation needed.

## Neutral statuses

`open`, `in_progress`, `closed` (+ `blocked`, supported by beads). These ARE beads' native status names.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | `bd create --title=... --type=... --priority=... --labels=... --description="..."` (via the create-task skill); returns the new id/url |
| show           | CORE | yes | `bd show <id>` (text) or `bd show <id> --json` when a field is parsed; `--json` exposes `status` as a neutral name |
| comment-add    | CORE | yes | `bd comments add <id> "<body>"` |
| set-status     | CORE | yes | `bd update <id> --status=<open\|in_progress\|closed>` |
| close          | CORE | yes | `bd close <id> [--reason="..."]` |
| comment-list   | OPT  | yes | `bd comments <id>` |
| list-by-status | OPT  | yes | `bd list --status=<status> [--json] [--format=ids]`; `--json` exposes `status`/`priority` for parsing |
| search         | OPT  | yes | `bd search <query>` |
| ready/stats    | OPT  | yes | `bd ready [--limit N]` / `bd stats` / `bd blocked` |
| label          | OPT  | yes | `bd label list-all` / `bd label list` / `bd label add <id> <label>` |
| assignee       | OPT  | yes | `bd update <id> --assignee=<name>` |
| set-field      | OPT  | yes | `bd update <id> --description="..."` (overwrite) |

## Degradation

beads supports every verb, so nothing degrades. A CORE verb that somehow fails (e.g. `bd` not on PATH) must HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); never report a phantom success.

## Notes

- `memory` (`bd remember`) and `dependencies`-write (`bd dep add`) exist in beads but no LETS command calls them, so they are not part of the neutral contract.
- State-changing ops (`bd close`, `bd update --status`, `bd dolt push`) stay gated under AUTO MODE per the workflow rules.
