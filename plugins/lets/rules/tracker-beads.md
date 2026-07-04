---
name: tracker-beads
version: 0.6.4
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->

# Tracker adapter: beads (beads x cli)

The reference adapter. Binds the neutral verbs to the `bd` CLI - the historical, fully-supported tracker. Runtime is byte-for-byte the same `bd` invocations LETS always made.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- `bd` already emits the neutral status names (`open` / `in_progress` / `closed`) - the status map is identity, no translation needed.
- Command/skill bodies carry ` ```lets-tracker ` blocks (see lets-rules "Tracker Adapters"); this table is how they resolve for beads - golden-pinned (`TestTrackerBeads_BindsBdCommands`: per-cell fragment pins covering the behavior-critical flags) against the historical `bd` invocations. A `comment-add` body arrives as `body-file=<path>`; the beads binding is `bd comments add <id> "$(cat <path>)"`.

## Neutral statuses

`open`, `in_progress`, `closed` (+ `blocked`, supported by beads). These ARE beads' native status names. Note: `list-by-status status=blocked` filters the status FIELD (rarely set explicitly); for dependency-blocked tasks (open issues with unmet deps) use `stats view=blocked` (= `bd blocked`), NOT `status=blocked`.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | `bd create --title=... --type=... --priority=... --labels=... --description="..."` (via the create-task skill); a multi-line `description` arrives as `description-file=<path>` → `--description="$(cat <description-file>)"`. Returns the new id/url |
| show           | CORE | yes | `bd show <id>` (text) or `bd show <id> --json` when a field is parsed; `--json` exposes `status` as a neutral name |
| comment-add    | CORE | yes | `bd comments add <id> "$(cat <body-file>)"` for `body-file=<path>`, or `bd comments add <id> "<body>"` for inline `body=`; empty body → HARD-FAIL, do not submit |
| set-status     | CORE | yes | `bd update <id> --status=<open\|in_progress\|closed\|blocked>` |
| close          | CORE | yes | `bd close <id> [--reason="..."]` |
| comment-list   | OPT  | yes | `bd comments <id>` |
| list-by-status | OPT  | yes | `bd list --status=<status> [--json] [--limit N]`; `--json` exposes `status`/`priority` for parsing. For id-only output, parse `--json` — bd has no `--format=ids` (an unknown `--format` value yields empty output, not an error) |
| search         | OPT  | yes | `bd search <query>` |
| ready/stats    | OPT  | yes | `bd ready [--limit N]` (`limit=0` → `--limit 0` = ALL ready tasks; used by `/lets:backlog`) / `bd stats` / `bd blocked` (dep-graph tree) / per-label progress + priority histogram (see Notes) |
| label          | OPT  | yes | bare `label` (all project labels, e.g. the `epic:*` set) → `bd label list-all`; `label task=<id>` → `bd label list <id>` (one issue's labels); `label add task=<id> value=<l>` → `bd label add <id> <l>`. The bare verb is `list-all`, NOT `bd label list` (which requires an id and errors without one) |
| assignee       | OPT  | yes | `bd update <id> --assignee=<name>` |
| set-field      | OPT  | yes | `set-field task=<id> description-file=<path>` → `bd update <id> --description="$(cat <path>)"` (overwrite); `set-field task=<id> priority=<0-4>` → `bd update <id> --priority=<0-4>` (backlog Reprioritize) |

## Degradation

beads supports every verb, so nothing degrades. A CORE verb that somehow fails (e.g. `bd` not on PATH) must HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); never report a phantom success.

## Notes

- `stats` dashboards (used by `/lets:backlog` + the orient snapshot's optional `## Project` counts): the `bd blocked` dep-graph tree and the priority histogram `bd list --status=open --json | jq -r '.[].priority' | sort | uniq -c | sort -rn` are the beads binding of the `stats`/`ready` views. **Label-group progress** (the per-`epic:*` NN/MM bars): for each label from `bd label list-all`, run `bd list --label <label> --json --all` — `--all` is REQUIRED (closed tasks are part of MM; without it progress % overcounts — the 3ad2a05 fix). An adapter that marks these `absent` → the calling command degrades those sections and tells the user (it does NOT render a broken/empty dashboard).
- `memory` (`bd remember`) and `dependencies`-write (`bd dep add`) exist in beads but no LETS command calls them, so they are not part of the neutral contract.
- State-changing ops (`bd close`, `bd update --status`, `bd dolt push`) stay gated under AUTO MODE per the workflow rules.
