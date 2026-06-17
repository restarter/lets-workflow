---
name: tracker-planfix-mcp.board
version: 0.0.0
---

<!-- USER-OWNED. `lets init` scaffolds this once and NEVER overwrites it. Edit freely.
     NEVER put a token/password/secret here: this file is auto-loaded into model
     context every session AND is git-shareable. The Planfix token lives ONLY in
     .mcp.json (gitignored) or the MCP server's own env - never here, never in .lets/.env. -->

# Planfix board profile (planfix-mcp)

Project-specific Planfix semantics the `tracker-planfix-mcp.md` adapter reads when resolving `set-status` / `close` / `show`. Edit to match YOUR Planfix board.

## Connection (non-secret)

- mcp-server-name: `planfix`            # tools are mcp__planfix__*
- rest-base-url:   `https://YOURCO.planfix.com/rest`
- default-project: `"LETS work"`

## Status map (neutral -> Planfix)

| neutral      | Planfix name | id |
|--------------|--------------|----|
| open         | New          | 1  |
| in_progress  | In Progress  | 2  |
| in_review    | Review       | 3  |
| closed       | Done         | 4  |

(Replace the names/ids with your board's. `set-status`/`close` send the `id`; `show`/`list-by-status` map an id back to the neutral name.)

## Transitions (allowed)

```
open -> in_progress -> in_review -> closed
any -> open   (reopen)
```

## Principles

- `/lets:start` moves `open -> in_progress`.
- `/lets:done` moves `-> closed` only from `in_review` (don't skip Review).
