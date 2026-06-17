---
name: tracker-planfix-mcp
version: 0.6.4
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->

# Tracker adapter: planfix-mcp (planfix x mcp)

Binds the neutral verbs to the Planfix MCP server (popstas/planfix-mcp-server). Pure MCP: dedicated tools where they exist, the server's generic `planfix_request` REST passthrough for the rest. NO new I/O code and NO secret in the plugin - the Planfix token is owned by the MCP server's own config.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- `<server>` below = the name you gave the server in `.mcp.json`; tools are `mcp__<server>__<tool>`. Confirm the exact identifiers against the connected server at runtime.
- Before `set-status`/`close`/`show`/`list-by-status`, read the sibling **`tracker-planfix-mcp.board.md`** to resolve a neutral status to its Planfix status id and to honor the board's allowed transitions. Translate the returned Planfix status id BACK to a neutral name so callers stay adapter-agnostic.
- Identity: a Planfix numeric task id is branch-safe - use it directly for `feature/<id>-<slug>` and detect-task.

## Neutral statuses

`open`, `in_progress`, `closed` (+ optional `in_review` / `blocked` if the board defines them). The board profile maps each to a Planfix status id.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | `mcp__<server>__planfix_create_task { name, description, project? }` -> {id, url} |
| show           | CORE | yes | `mcp__<server>__planfix_request { method:"GET", path:"task/<id>", body:{fields:"id,name,status,description"} }`; map the Planfix status id -> neutral name via the board |
| comment-add    | CORE | yes | `mcp__<server>__planfix_create_comment { taskId:<id>, comment:<body> }` |
| set-status     | CORE | yes | `mcp__<server>__planfix_request { method:"POST", path:"task/<id>", body:{status:{id:<board status id for the neutral status>}} }` |
| close          | CORE | yes | set-status to the board's `closed` status id (same `planfix_request` shape) |
| comment-list   | OPT  | yes | `mcp__<server>__planfix_request { method:"POST", path:"task/<id>/comments/list", body:{...} }` |
| list-by-status | OPT  | yes | `mcp__<server>__planfix_request { method:"POST", path:"task/list", body:{filters:[{type:status,...}]} }`; return statuses as neutral names |
| search         | OPT  | yes | `mcp__<server>__planfix_search_task { title }` |
| ready/stats    | OPT  | no  | absent - Planfix has no beads-style ready-graph; the dashboard rows degrade |
| label          | OPT  | no  | absent |
| assignee       | OPT  | no  | absent |
| set-field      | OPT  | yes | `mcp__<server>__planfix_request { method:"POST", path:"task/<id>", body:{<field>:...} }` |

(Child tasks / checklist: `mcp__<server>__planfix_get_child_tasks { parentTaskId:<id> }` when needed.)

## Degradation

- OPTIONAL verb marked `no` (ready/stats, label, assignee) -> the command continues and tells the user the capability is unavailable for Planfix.
- A CORE verb whose MCP tool is NOT connected at runtime (server absent - e.g. a headless/cron run without the interactively-authed MCP server) -> HARD-FAIL loud: surface "set-status / close FAILED - task NOT changed in Planfix, verify manually" and do NOT report success. Critical under AUTO MODE: `/lets:done` must never claim it closed a Planfix task it did not.

## .mcp.json wiring

The MCP server must be connected in Claude Code (`.mcp.json` or MCP settings) - outside plugin control; the plugin only documents it. Example entry:

```json
{
  "mcpServers": {
    "planfix": {
      "command": "npx",
      "args": ["-y", "@popstas/planfix-mcp-server"],
      "env": {
        "PLANFIX_ACCOUNT": "yourco",
        "PLANFIX_TOKEN": "${PLANFIX_TOKEN}",
        "PLANFIX_BASE_URL": "https://yourco.planfix.com/rest"
      }
    }
  }
}
```

**NEVER commit a literal token.** `.mcp.json` is gitignored by `lets init`; use `${PLANFIX_TOKEN}` env-var expansion (resolved from your shell), never a literal in the file. The token lives ONLY here / in the server's env - never in `.lets/.env` or a `.board.md`.

**Limitation:** an interactively-authenticated MCP server may be absent in headless/cron runs, so the autonomous pipeline on planfix-mcp is not guaranteed - CORE verbs hard-fail loudly (above) rather than silently no-op.
