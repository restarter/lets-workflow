---
name: tracker-beads
version: 0.8.0
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
| create         | CORE | yes | accepts: `title`, `type`, `priority`, `labels`, `description`. `bd create --title=... --type=... --priority=... --labels=... --description="..."` (via the create-task skill); a multi-line `description` arrives as `description-file=<path>` → `--description="$(cat <description-file>)"`. Returns the new id/url |
| show           | CORE | yes | returns: `id`, `title`, `status`, `description`, `type`, `design`. NO `url` - beads has no per-task link (`[VERIFIED 2026-08-04]`: no `--url` flag on create/update, none in `bd show` output). `type` is `[UNVERIFIED]` against the JSON key, which may be `issue_type`; if so this becomes `type→issue_type`. `bd show <id>` (text) or `bd show <id> --json` when a field is parsed; `--json` exposes `status` as a neutral name. Parse the FIRST `"status"` in the JSON - the task's own top-level field. A greedy match takes the LAST, which is a nested dependency's status, and mis-classifies the task |
| comment-add    | CORE | yes | `bd comments add <id> "$(cat <body-file>)"` for `body-file=<path>`, or `bd comments add <id> "<body>"` for inline `body=`; empty body → HARD-FAIL, do not submit |
| set-status     | CORE | yes | `bd update <id> --status=<open\|in_progress\|closed\|blocked>`; a `status` outside this set (e.g. `in_review`) is NOT supported - skip the call entirely and report that nothing changed. Do NOT pass it through: `bd` rejects an unknown status |
| close          | CORE | yes | `bd close <id> [--reason="..."]` `[VERIFIED 2026-08-02]`; returns `closed` - beads has no gated terminal, so close is always a legal transition |
| comment-list   | OPT  | yes | `bd comments <id>` |
| list-by-status | OPT  | yes | `bd list --status=<status> [--json] [--limit N]`; `--json` exposes `status`/`priority` for parsing. For id-only output, parse `--json` — bd has no `--format=ids` (an unknown `--format` value yields empty output, not an error) |
| search         | OPT  | yes | `search query=<q>` → `bd search <query>` |
| ready/stats    | OPT  | yes | `bd ready [--limit N]` (`limit=0` → `--limit 0` = ALL ready tasks; used by `/lets:backlog`) / `bd stats` (native totals) / `bd blocked` (dep-graph tree) |
| label          | OPT  | yes | bare `label` (all project labels, e.g. the `epic:*` set) → `bd label list-all`; `label task=<id>` → `bd label list <id>` (one issue's labels); `label add task=<id> value=<l>` → `bd label add <id> <l>`. The bare verb is `list-all`, NOT `bd label list` (which requires an id and errors without one) |
| assignee       | OPT  | yes | `bd update <id> --assignee=<name>` |
| set-field      | OPT  | yes | accepts: `description`, `priority`. `set-field task=<id> description-file=<path>` → `bd update <id> --description="$(cat <path>)"` (overwrite); `set-field task=<id> priority=<0-4>` → `bd update <id> --priority=<0-4>` (backlog Reprioritize) |

## Degradation

beads supports every verb, so nothing degrades. A CORE verb that somehow fails (e.g. `bd` not on PATH) must HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); never report a phantom success.

## Notes

- `stats` (used by `/lets:backlog` + the orient snapshot's optional `## Project` counts): `bd stats` gives native status/priority totals (no jq); `bd blocked` gives the dep-graph tree (native, no jq). The old per-`epic:*` label-progress bars and the jq-based priority histogram were removed with the 5-view `/lets:status` dashboards (orient unification, lets-qsgmd) — no command renders them now, so the fragile jq-over-bd-JSON control-char path is gone from the adapter.
- `memory` (`bd remember`) and `dependencies`-write (`bd dep add`) exist in beads but no LETS command calls them, so they are not part of the neutral contract.
- State-changing ops (`bd close`, `bd update --status`, `bd dolt push`) stay gated under AUTO MODE per the workflow rules.
