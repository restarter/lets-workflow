---
name: tracker-planfix-mcp.board
version: 0.0.0
---

<!-- USER-OWNED TEMPLATE. `lets init` scaffolds this once and NEVER overwrites it. Fill it with YOUR Planfix board's values and edit freely.
     NEVER put a token/password/secret here: this file is auto-loaded into model context every session AND is git-shareable. The Planfix token lives ONLY in .mcp.json (gitignored) / the MCP server's env - never here, never in .lets/.env.
     PRIVACY: this file can be committed to your repo - do NOT paste real team-member names / user ids into a SHARED repo unless your team is fine with that. -->

# Planfix board profile (planfix-mcp)

Project-specific Planfix semantics the `tracker-planfix-mcp.md` adapter reads when resolving `create` / `show` / `set-status` / `close` / `list-by-status`. The GENERIC API mechanics (filter syntax, `failures[]`, custom-field how-to) live in the adapter file; THIS file holds only your board's own values. Replace every `<...>` below with your board's real ids (read them live from Planfix - there is no REST status-enumeration endpoint, so the status map is filled by hand).

## Connection
- mcp-server-name: `<server>`           # tools are mcp__<server>__*  (must match the server name in .mcp.json)
- The MCP server holds account + token in its own env (.mcp.json) and builds REST URLs itself.

## Create defaults
- Working project id: `<projectId>`. Task template id: `<templateId>`.
- Create binding: `planfix_request POST task/ { name, description, project:{id:<projectId>}, template:{id:<templateId>} }` -> `{result:"success", id}`. The task lands in the template's initial status.
- `template:{id}` is MANDATORY (a templateless task is off-process and cannot transition between board statuses). Do NOT use the dedicated `planfix_create_task` tool (broken for normal tasks - see adapter).

## Status map (neutral -> Planfix id)

| neutral       | Planfix name | id              |
|---------------|--------------|-----------------|
| open          | `<...>`      | `<id>`          |
| in_progress   | `<...>`      | `<id>`          |
| in_review     | `<...>`      | `<id>` (opt)    |
| closed        | `<...>`      | `<id>`          |
| blocked       | `<...>`      | `<id>` (opt)    |

List ALL your account's status ids here too (handy for filters), e.g. `Draft 0 · Archived 3 · New <id> · InProgress <id> · Done <id> · Rejected <id>`. Read them from Planfix status settings.

## Transitions (process-gated)
Planfix GATES transitions: a non-allowed target returns `{result:"success", failures:[{field:"status",error:"field change failed"}]}` and does NOT change the status. List your board's allowed path so the adapter can walk it hop-by-hop:
```
<openStatus> -> <backlog> -> <grooming> -> <sprint> -> <inProgressStatus> -> <doneStatus> -> <archive>
park: <inProgressStatus> <-> <waitingStatus>
reject: any -> <rejectedStatus>
```

## LETS flow on this board
- `/lets:start` / take-task: set in_progress (your `in_progress` status). If `set-status` returns a non-empty `failures[]`, walk the allowed path (above) instead of jumping.
- `/lets:done`: set closed (your `closed` status). Archive (if separate) is a manual step OUTSIDE LETS.

## Board structure (NOT lifecycle - do not map to neutral statuses)
Some Planfix statuses/fields are organizational, not lifecycle - never treat them as a LETS neutral status:
- Epics / parent tasks: enumerate via `task/list {filters:[{type:5,value:<projectId>},{type:10,value:[<epicStatusId>]}]}` - NOT `get_child_tasks` (unpaginated, explodes on big epics).
- Grouping/info columns (by a field): informational only.
- Importance/Tier or other custom-field columns: a List custom field, NOT a status. Discover it via `GET customfield/task {fields:"id,name,type"}`; filter by label `{type:106, field:<fieldId>, operator:"equal", value:"<label>"}` (see the adapter's custom-fields section).

## Common queries (fill with your project id)
- Active board (pickable + in-flight): `task/list {filters:[{type:5,value:<projectId>},{type:10,value:[<pickable+active ids>]}]}`.
- In-progress only: `...{type:10,value:[<inProgressId>]}`.
- One person's tasks: add `{type:2,operator:"equal",value:"user:<N>"}`. Unassigned: `{type:33,value:1}`.
- An epic's tasks: `task/list {filters:[{type:307,value:<epicId>}]}` (whole subtree) or `{type:73,value:<epicId>}` (direct children).

## Notifications / team (fill in)
Map your team's Planfix user ids so the adapter can mention/notify them (the adapter's "Comment notifications" section explains the unreliable-`recipients` -> user-mention-link mechanic). Replace with YOUR team:

| role        | user id    | mention slug (from an existing comment's HTML) |
|-------------|------------|------------------------------------------------|
| `<role>`    | `user:<N>` | `<slug>`                                        |

Policy - who to notify when: e.g. leads on architecture/blocker/spec-review posts; assignee-only for status/log entries.
<!-- PRIVACY: this section holds real names + ids. Fine in a PRIVATE team repo; do NOT put it in a public/shared repo unless the team agrees. -->

## Templates (if this account uses task templates)
Record the template id per project so `create` can pass `template:{id}` (see the adapter's "Create from a template"):

| project | project id     | template       | template id     |
|---------|----------------|----------------|-----------------|
| `<name>`| `<projectId>`  | `<templateName>` | `<templateId>` |

Also note your board's **Object** id (`<objectId>`) if dev tasks must attach one.

## Routing (if you use another tracker alongside Planfix)
Define which work goes to Planfix vs the other store (e.g. dev/implementation -> Planfix; PM/specs/decisions -> beads) and how you cross-link them (e.g. a `| Planfix [<id>]` suffix in the other tracker's titles). Keep cross-references findable by people who only have Planfix access (no internal repo paths / other-tracker ids in a Planfix comment).

## Principles
- Identity: a Planfix numeric task id is branch-safe -> `feature/<id>-<slug>` and detect-task.
- Read checklists separately (`planfix_request POST task/<id>/checklist/list`) - they define real scope.
