---
name: tracker-planfix-mcp
version: 0.6.4
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->
<!-- Bindings + filter mechanics below were VERIFIED 2026-06-18 against a LIVE @popstas/planfix-mcp-server run AND the official OpenAPI (https://help.planfix.com/restapidocs/swagger.json) - end-to-end create -> in_progress -> comment -> close (lets-5d48z). Project-specific values (project/template/status ids) live in tracker-planfix-mcp.board.md; bindings here are generic. -->

# Tracker adapter: planfix-mcp (planfix x mcp)

Binds the neutral verbs to the Planfix MCP server (popstas/planfix-mcp-server). Pure MCP: the server's generic `planfix_request` REST passthrough does the real work; the dedicated tools are CRM/lead-shaped and mostly NOT usable for plain task lifecycle. NO new I/O code and NO secret in the plugin - the Planfix token is owned by the MCP server's own config.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- `<server>` below = the name you gave the server in `.mcp.json`; tools are `mcp__<server>__<tool>` (e.g. `planfix`).
- Before `set-status`/`close`/`show`/`list-by-status`, read the sibling **`tracker-planfix-mcp.board.md`** to resolve a neutral status to its Planfix status id and to honor the board's allowed transitions. Translate the returned Planfix status id BACK to a neutral name so callers stay adapter-agnostic.
- Identity: a Planfix numeric task id is branch-safe - use it directly for `feature/<id>-<slug>` and detect-task.
- **SOURCE OF TRUTH = the OpenAPI at `https://help.planfix.com/restapidocs/swagger.json` (it IS fetchable).** When unsure about an endpoint body / param / value format, READ IT THERE. Do NOT guess - guessing filter `value` formats here silently returns wrong or empty results that look like "no data" or "broken server" (this cost three wrong conclusions during testing). Human-readable filter reference (the full ~60 filter types): `https://planfix.com/help/REST_API:_Complex_task_filters`. REST index: `https://planfix.com/help/REST_API`.

## Neutral statuses

`open`, `in_progress`, `closed` (+ optional `in_review` / `blocked` if the board defines them). The board profile maps each to a Planfix status id.

## Capabilities + bindings

(Binding cells are summaries; the detailed per-verb sections below are authoritative.)

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | `planfix_request POST task/ { name, description, project:{id}, template:{id} }` -> `{result:"success", id}`. NOT the dedicated `planfix_create_task` (broken for normal tasks). `template:{id}` MANDATORY. `name`<-neutral `title`; `description`<-`description=` inline or `description-file=<path>` (read the file first), rendered as **HTML not markdown** - convert (see "create" + the HTML note). The neutral `type`/`priority`/`labels` have NO binding here -> **DROPPED** (Planfix uses the object's mandatory fields + the board's Tier custom field; map priority->Tier in the board profile if wanted). |
| show           | CORE | yes | `planfix_request GET task/<id> { fields:"id,name,status,project,description,assignees,assigner" }`; map status id -> neutral via board |
| comment-add    | CORE | yes | `planfix_create_comment { taskId:<id>, description:<body>, silent:true }` (param is `description`, not `comment`). `body=` inline or `body-file=<path>` (read the file first); render markdown best-effort per the HTML note below |
| set-status     | CORE | yes¹ | `planfix_request POST task/<id> { status:{id:<target>} }` - PROCESS-GATED + must inspect `failures[]` |
| close          | CORE | yes¹ | set-status to the board `closed` id (same gated rules) |
| comment-list   | OPT  | yes | `planfix_request POST task/<id>/comments/list { ... }` |
| list-by-status | OPT  | yes | `planfix_request POST task/list { filters:[{type:5,operator:"equal",value:<projectId>},{type:10,operator:"equal",value:[<id>,...]}], fields:"id,name,status", pageSize:100 }` |
| search         | OPT  | yes | `planfix_search_task { taskTitle }` |
| ready/stats    | OPT  | partial | no beads-style ready-graph, but a `ready` view is buildable on `list-by-status` (active statuses of the working project) |
| label          | OPT  | no  | absent (tags exist on tasks but no list-by-label binding verified) |
| assignee       | OPT  | read-only | returned by `show` as `assignees:{users:[{id:"user:N",name}],groups:[]}`; no assign binding verified |
| set-field      | OPT  | yes | `planfix_request POST task/<id> { <field>:... }` |

¹ Conditional - see "set-status / close" below.

## task/list - filter reference (VERIFIED live + OpenAPI)

- **Filter object:** `{ type, operator, value, field?, subfilter? }`. `operator` = `equal` | `notequal` (date filters also have `gt`/`lt`/`gtAndEqual`/`ltAndEqual`). Multiple filter objects in the `filters` array combine with **AND**.
- **Filter `type` ids:** status=**10**, project=**5**, template=**51**, assignee=**2**, name=**8**, task-number=**57**, priority=**9** (value `"Urgent"`/`"NotUrgent"`), creation-date=**12**.
- **Multi-value OR within ONE filter = pass `value` as a JSON ARRAY of ints.** Example: `{type:10, operator:"equal", value:[114,113,105]}` returns tasks whose status is ANY of those. This is the documented "series of identifiers (OR)" format.
- **String separators do NOT work.** `value:"114;113"` or `"114,113"` are silently mis-parsed (status filter -> status 0 / Черновик; task-number filter -> empty). A SINGLE value may be a string (`"114"`) or int; a SERIES must be a real array. (This was misread as "filters broken" twice in testing - it was the wrong value type.)
- **A wrong `type` id returns `{result:"success", tasks:[]}`** - silent empty, NOT an error. Easy to misread as "no tasks" / "filters broken". Verify ids against the OpenAPI.
- **No sorting.** Per the OpenAPI, the task/list body is `offset, pageSize, filterId, runAsUserId, filters, fields, sourceId` - there is NO `sort`/`order`/`sorting` param; results come id-ascending. Filter by status rather than relying on recency.
- **list-by-status binding** (project-scoped, multi-status OR): `planfix_request POST task/list { filters:[{type:5,operator:"equal",value:<projectId>},{type:10,operator:"equal",value:[<statusId>,...]}], fields:"id,name,status", pageSize:100 }`; map status ids back to neutral names. This same query powers a `ready` fallback: list the working project's tasks in the active/pickable statuses grouped by status (Planfix has no native ready-graph).

## task/list - filter cookbook (how to filter by X)

Shape: `planfix_request POST task/list { filters:[...], fields:"id,name,status", pageSize:N }`. Add `{type:5,operator:"equal",value:<projectId>}` to scope to the board project. All examples below were VERIFIED live unless noted.

**Combining filters:**
- Multiple filter objects in `filters[]` = **AND**.
- **OR within one field = a JSON ARRAY `value`** (e.g. several statuses). The wiki says "semicolon-separated", but that does NOT work through this MCP server - use an array.
- `subfilter` is only for data-tag fields (type 93): `{type:93, operator:"equal", value:"X", subfilter:{dataTagId:<id>, filter:{type:<sub>, field:<id>}}}`.

**By status** (type 10): `{type:10, operator:"equal", value:[114,113,105]}` -> tasks in ANY of those statuses. [VERIFIED]

**By assignee / executor** (исполнитель, type 2): `{type:2, operator:"equal", value:"user:<N>"}` -> tasks where user `<N>` is an assignee. [VERIFIED] Variants: `97` just-assignee, `71` assignee-employee, `69` assignee-contact, `33` without-assignees, `1` assigner, `39` participant. Ids are prefixed `user:N` / `group:N` / `contact:N`.

**By super-task / parent** (надзадача):
- `{type:73, operator:"equal", value:<parentId>}` -> DIRECT children only. [VERIFIED]
- `{type:307, operator:"equal", value:<parentId>}` -> the WHOLE subtree under that parent. [VERIFIED] **Use 307 to list an epic's tasks** - it is the safe, paginated alternative to `get_child_tasks` (which dumps the whole tree unpaginated and blows the token limit).

**By a custom field (e.g. an importance / priority List field):** a custom field filters by the `type` that matches the field KIND, PLUS the `field` key = the field's numeric id:
- single-select "List" -> `{type:106, field:<fieldId>, operator:"equal", value:"<exact option label>"}` (the value is the option LABEL string, e.g. `"High"` - NOT a short code)
- "Set of values" (multi) -> `{type:111, field:<fieldId>, operator:"equal", value:<...>}` (docs say semicolon-separated values; NOT verified through this server - the `;`-string form failed for status/task-number, so test first)
- "Directory entry" -> `{type:107, field:<TierFieldId>, value:<entryId>}`
- short-text `101` · number `102` · date `103` · checkbox `105` · employee `109` · task `115` · project `117`
- has / hasn't ANY value in a field -> `152` / `153` with `value:<fieldId>`.
- single-select List example: `{type:106, field:<fieldId>, operator:"equal", value:"<exact option label>"}` (value = the option LABEL string, not a code). Full custom-field workflow in the next section.

**Data tags (теги данных)** are a SEPARATE structured-data mechanism (not custom fields). Discovery + filtering:
- **Discover per-task** (no global "list all data tags" endpoint exists): `GET task/<id> {fields:"id,name,dataTags"}` - also works in `task/list` (returns `dataTags` per row) - yields `dataTags:[{dataTag:{id,name}, key}]` (record the ids your board uses in the board profile).
- **Filter TASKS by a data-tag field value:** `{type:93, operator:"equal", value:<v>, subfilter:{dataTagId:<id>, filter:{type:<31xx>, field:<fieldId>}}}`.
- **List ENTRIES of a known tag:** `POST datatag/<id>/entry/list` with inner filter types 3101-3123 and `fields` naming the field ids (see https://planfix.com/help/REST_API:_Complex_data_tag_filters).

**Other handy filters:** template `51` · task-number `57` · name-contains `8` · priority `9` (`"Urgent"`/`"NotUrgent"`) · process `24` · object `325` · dates `12`(created)/`13`(planned-start)/`14`(planned-due)/`20`(completed)/`38`(last change)/`79`(last change-or-comment) · recurring `16` · overdue `17` · data-tag `11`(has)/`18`(hasn't). Full ~60-type table: the filters page linked at the top.

**Troubleshooting - READ THIS if a filter returns empty or wrong rows:** it is almost always (a) a WRONG `type` id, or (b) a WRONG `value` TYPE (single = int/string; OR = JSON array, NOT a `;`-string). A wrong `type` returns `{result:"success", tasks:[]}` silently. Re-check against `https://planfix.com/help/REST_API:_Complex_task_filters` and the OpenAPI `swagger.json` - do NOT guess the format.

## Custom fields - how to work with them (VERIFIED)

Custom fields are NOT returned by default and there is NO `customFieldData` shortcut token - know the field id first. Workflow:

**1. Discover the field ids.** `GET customfield/task {fields:"id,name,type"}` -> `{customfields:[{id,name,type}]}` - YOUR account's field map; record the ones you filter on in `tracker-planfix-mcp.board.md`. Sibling objects: `customfield/{contact,project,employee}`. There is NO `GET customfield/<id>` detail endpoint (404) and `customfield/list` also 404s - a field's full definition (incl. `enumValues`) rides inside a task's `customFieldData` (step 2). The `type` is the field KIND (a DIFFERENT numbering from filter types; full list: `https://planfix.com/help/REST_API:_Types_of_custom_fields` - e.g. `8` = List, `20` = Set-of-values).

**2. Read a value on a task.** `GET task/<id> {fields:"id,name,<fieldId>"}` -> `customFieldData:[{ field:{id,name,type,enumValues,...}, value, stringValue }]`. For a List/Set field, `value`/`stringValue` is the chosen option LABEL (e.g. `"High"`). Has-any-value filter: `{type:152, value:<fieldId>}` (152 = has, 153 = hasn't).

**3. Filter by a custom field** = the FILTER type matching the field KIND + the `field` key: List(8)->**106** · Set(20)->**111** · Number(1)->**102** · short-text(0)->**101** · date(3)->**103** · checkbox(7)->**105** · directory(9)->**107** · employee(11)->**109** · task(16)->**115** · project(22)->**117**. Example: `{type:106, field:<fieldId>, operator:"equal", value:"<label>"}`.

## create

- **Use the passthrough:** `planfix_request POST task/ { name, description, project:{id:<board project>}, template:{id:<board template>} }` -> `{result:"success", id}`. The task lands in the template's initial status (the board defines which neutral status that is).
- **Do NOT use the dedicated `planfix_create_task` tool** for a normal board task: it is CRM/lead-shaped - it pushes `name`/`project` into the description TEXT and sends `template.id=null` + a malformed `customFieldData:[{field:{id:null}}]`, so Planfix rejects it ("Error creating task").
- `template:{id}` is MANDATORY: a templateless task is created off-process and CANNOT transition between board statuses afterward. (An `object:{id}` that doubles as a template - id == template id - satisfies this: the created task carries `template:{that id}` and IS on-process. Verified live 2026-06-28: a `{project, object:44611}` create lands with `template:44611` and transitions normally, and the object auto-defaults its mandatory custom fields, e.g. Важность -> "Tier Z".)
- **`description` is HTML, not markdown** (same as comments - see "HTML markup"). The neutral `description-file=`/`description=` body is composed as markdown by the calling skill (`## heading`, `- bullet`), which Planfix shows **literally** - CONVERT it to compact HTML before sending.
- **Neutral `type`/`priority`/`labels` are DROPPED** - the beads-shaped `create` verb carries them, but Planfix create takes only `name`+`description`(+object/template). Don't error; the object's mandatory fields + the board's Tier custom field stand in. Map `priority`->Tier explicitly in the board profile only if a project needs it.

## set-status / close - PROCESS-GATED transitions + failures[] (CRITICAL)

1. **Binding:** `planfix_request POST task/<id> { status:{id:<target>} }`.
2. **`result:"success"` is NOT proof of change.** The response is `{"result":"success","failures":[{"field":"status","error":"field change failed"}]}` when the change is rejected - and `failures[]` is DOCUMENTED in the OpenAPI (HTTP 200 with field/error pairs). You MUST inspect `failures[]`; if it is non-empty for `status`, the status did NOT change.
3. **No direct jumps - transitions are gated by the board process.** To reach a target neutral status you may need to WALK the board's allowed path hop-by-hop (the open->in_progress jump may have to route through one or more intermediate statuses), confirming empty `failures[]` after each hop. Read the board profile's "Transitions" graph.
4. **Hard-fail loud** if a required hop fails and cannot be routed: surface "set-status / close FAILED - task NOT changed in Planfix, verify manually" and do NOT report success. Critical under AUTO MODE: `/lets:done` must never claim a close it did not perform.

## comment-add

`planfix_create_comment { taskId:<id>, description:<body>, silent:true }`. The param is **`description`** (not `comment`). `silent:true` avoids notifying real users on a shared account.

## show / custom fields / assignees

- `planfix_request GET task/<id> { fields:"id,name,status,project,description,assignees,assigner" }`.
- **Custom fields are NOT returned by a generic `customFieldData` key** - name the field id in `fields` (e.g. `fields:"id,name,<fieldId>"`). Full how-to is in the "Custom fields - how to work with them" section above; record YOUR board's field ids in `tracker-planfix-mcp.board.md`.
- Assignee shape: `assignees:{ users:[{id:"user:<N>", name:"..."}], groups:[] }`, `assigner:{ id:"user:<N>", name:"..." }`. User ids are prefixed `user:N`.

## get_child_tasks

`planfix_get_child_tasks { parentTaskId }` returns the ENTIRE subtree UNPAGINATED - for a real epic it returned 600k+ chars and blew the token limit (and returned empty for a leaf/template). Avoid on large epics; enumerate children via `task/list` instead.

## No status-enumeration endpoint

`GET task/status/list` -> 404, and the OpenAPI exposes no status-list / status-set / process-list endpoint. The board status map MUST be filled BY HAND from Planfix status settings.

## HTML markup (descriptions + comments)

Task **descriptions** and **comments** render **HTML, not Markdown** - `**bold**` / `` `code` `` show literally. Working tags: `<b>`/`<strong>`, `<i>`/`<em>`, `<u>`, `<code>`, `<pre>` (code block), `<ul>`/`<ol>`+`<li>`, `<a href>`, `<p>`, `<br>`.
- **Compact HTML - NO newline between block tags.** Planfix turns ANY newline between block-level tags (even a single `\n`, even indented) into a literal `<br>`. Write block HTML fully adjacent: `<p>First.</p><p><b>Heading</b></p><ul><li>a</li><li>b</li></ul>` (zero whitespace between tags). Newlines INSIDE flowing text are stripped (fine). For a deliberate gap use `<p>&nbsp;</p>`, never a blank line.
- Planfix auto-links URL-like strings inside `<pre>` - break up full URLs to keep a code block literal.
- **Tighten long structured posts** (`<p>` has ~1em top+bottom margin, so a `<p>` heading directly above a `<ul>` double-spaces): fold a sub-heading INTO the first `<li>` (not `<p>heading</p><ul>...`), use `<br>` within one `<p>` for tightly-related lines, and `<p>&nbsp;</p>` for a deliberate gap. Short post (1-3 paragraphs) -> plain `<p>` is fine; long post (3+ sections with lists) -> apply these.

## Comment notifications

- `planfix_create_comment { taskId, description, recipients?, silent? }`. **Omitting `recipients` notifies ONLY the assignee** (not author/participants). `silent:true` suppresses the notification (use on a shared/bot account to avoid pinging real users).
- **`recipients` via this MCP server is unreliable** (empirically a silent no-op - read the comment back and it carries none). For a RELIABLE notification, embed an inline user-mention link in the description HTML: `<a class="user-mention user-mention-login-<slug>" href="/user/<id>"><Display Name></a>` - Planfix extracts the mention from `href="/user/<id>"` + the class, rendering the tag AND sending the notification. Discover a user's `<slug>` by reading an existing comment's HTML (`task/<id>/comments/list`). (The user-id/slug map + who-to-notify policy are project-specific -> board profile.)
- Comment update/delete: `planfix_request POST task/<id>/comments/<commentId> { description }` · `planfix_request DELETE comment/<id>`.

## Create from a template (recommended for multi-field boards)

Many boards require an **Object** (a Planfix "digital twin" - a fixed set of fields/statuses/scenarios) and/or mandatory custom fields on every task. Rather than hand-assembling `object`/`project`/`customFieldData`, use the project's **task template** - it auto-fills project, object, mandatory fields and the initial status:
- `planfix_request POST task/ { template:{id:<templateId>}, name, description }` (HTML). Do NOT also pass `object`/`project` - the template owns them. Returns `{result:"success", id}`.
- List templates: `planfix_request GET task/templates?fields=id,name,project`. Record the template id(s) your board uses in the board profile.
- Hand-assembled alternative (board uses an Object but no template): pass `object:{id:<objectId>}` + `project:{id:<projectId>}` + the mandatory `customFieldData`; verify it attached by reading back `task/<id>?fields=...,object` (absent `object` = wrong id).

## Reading: pagination + checklists

- Single task: `planfix_request GET task/<id>?fields=id,name,description,status,parent` (add `assignees,participants,<fieldId>` as needed).
- **Checklists != subtasks.** Checklists are checkbox items (`{name,isDone}`) read SEPARATELY: `planfix_request POST task/<id>/checklist/list { fields:"id,name,isDone" }`. Subtasks are real tasks with a `parent`. Always read checklists when reviewing a task - they define the real scope.
- Bulk list paginates: `task/list` with `pageSize` (MAX 100) + `offset`, repeat until a page returns fewer than `pageSize` rows.
- Hierarchy: tasks nest via `parent` (Epic -> Task -> Subtask, unlimited).

## MCP server health (Node-ABI gotcha)

The popstas server bundles `better-sqlite3` (a native addon compiled against a specific Node ABI). **If the host Node major version changes (e.g. a `brew upgrade`), every tool call returns `NODE_MODULE_VERSION mismatch` even though the server shows "Connected"** (it starts, then fails on the first sqlite touch).
- **"Connected" != working** - confirm with a real read (`planfix_request GET task/<id>`).
- **Recovery:** `rm -rf ~/.npm/_npx/<hash>` (the npx cache; the `<hash>` is printed in the error path), then reconnect MCP (`/mcp`) or restart the session - the reconnect recompiles the addon against the current ABI. Pin Node to a versioned formula (e.g. `node@24`) to avoid surprise major bumps.
- **Don't conflate two failure shapes:** `NODE_MODULE_VERSION mismatch` = broken channel (recover above); `{"success":false,"error":"Access denied for task by id - <N>"}` = a HEALTHY channel returning a Planfix authorization response (the token is granted per-project; a task outside the grant is denied). An Access-denied is NOT a reason to clear caches / reconnect.

## Degradation

- OPTIONAL verb marked `no`/`partial` (ready/stats, label, assignee) -> the command continues and tells the user the capability is limited for Planfix.
- A CORE verb whose MCP tool is NOT connected at runtime (server absent - e.g. a headless/cron run without the interactively-authed MCP server) -> HARD-FAIL loud: "set-status / close FAILED - task NOT changed in Planfix, verify manually"; do NOT report success.
- A status change that returns a non-empty `failures[]` -> treat as FAILURE (above), never a phantom success.

## .mcp.json wiring

The MCP server must be connected in Claude Code (`.mcp.json` or MCP settings) - outside plugin control; the plugin only documents it.

```json
{
  "mcpServers": {
    "planfix": {
      "command": "npx",
      "args": ["-y", "@popstas/planfix-mcp-server"],
      "env": {
        "PLANFIX_ACCOUNT": "yourco",
        "PLANFIX_TOKEN": "${PLANFIX_TOKEN}"
      }
    }
  }
}
```

**NEVER commit a literal token.** `.mcp.json` is gitignored by `lets init`; use `${PLANFIX_TOKEN}` env-var expansion, never a literal. The token lives ONLY here / in the server's env - never in `.lets/.env` or a `.board.md`.

**Do NOT set `PLANFIX_BASE_URL` for a normal `.com` account** - the server already defaults to `https://<PLANFIX_ACCOUNT>.planfix.com/rest/`. Override it ONLY for a regional host (e.g. `.ru`), and then it MUST end in a **trailing slash** (`/rest/`): the server concatenates `` `${PLANFIX_BASE_URL}${path}` `` literally (popstas `src/helpers.ts`), so a missing slash produces `…/resttask/list` -> a 404 HTML page, and EVERY call then fails with `Unexpected token '<', "<!DOCTYPE"... is not valid JSON`. That is a "Connected"-but-not-working server returning HTML instead of JSON - not a token error (verified live 2026-06-28, lets-5d48z). Regional example: `"PLANFIX_BASE_URL": "https://<account>.planfix.ru/rest/"`.

## Limitations (this server build)

- `task/list` honors `filters` ONLY with the correct Planfix type ids AND correct value types (single = string/int, series = JSON array); a wrong type id or a string "series" silently returns empty/wrong, not an error.
- No `sort` on task/list (id-ascending only).
- No status-enumeration endpoint; status map by hand.
- `planfix_create_task` (dedicated) unusable for plain tasks; use the `task/` passthrough.
- `get_child_tasks` is unpaginated and can blow the token limit on large epics.
- An interactively-authenticated MCP server may be absent in headless/cron runs, so the autonomous pipeline on planfix-mcp is not guaranteed - CORE verbs hard-fail loudly rather than silently no-op.
