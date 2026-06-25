# Task Trackers

LETS is **tracker-agnostic**. Instead of hardcoding beads (`bd`), a project binds whatever task store it wants through a single **adapter file**. One config key, `LETS_TRACKER`, names the adapter; `lets init` installs exactly one `.claude/rules/tracker-<name>.md` (auto-loaded, drift-tracked like `lets-rules.md`) that binds the neutral verbs to concrete calls.

## The model

- **Neutral verbs** — every command speaks a small shared vocabulary: `create`, `show`, `comment-add`, `set-status`, `close` (CORE) plus `comment-list`, `list-by-status`, `search`, `ready`/`stats`, `label`/`assignee`/`set-field` (OPTIONAL).
- **One adapter file per tracker** — `tracker-<name>.md` is self-contained: a capability table mapping each verb to a concrete binding (a `bd` command, an `mcp__*` tool, a REST call) + a degradation section. Transport is encoded **per verb** inside the file — there is no second "driver" config axis.
- **Resolution** (the rule lives in `lets-rules.md` → "Tracker Adapters"): the `bd …` commands you see in LETS commands are the **beads-default binding**. With `LETS_TRACKER=beads` they run as-is (runtime is unchanged from pre-platform LETS). With a non-beads adapter, each verb resolves through the loaded `tracker-<name>.md` instead.

## Shipped adapters

| `LETS_TRACKER` | Transport | Notes |
|----------------|-----------|-------|
| `beads` (default) | `bd` CLI | Reference adapter. Full capabilities. Runtime byte-for-byte unchanged. |
| `planfix-mcp` | Planfix MCP server | Pure MCP — dedicated `planfix_*` tools + the server's generic `planfix_request` REST passthrough. No new I/O code, token owned by the server. |
| `none` | — | Null adapter. Every verb is a no-op; commands degrade to a no-tracker stance. The "no-beads" mode. |

## Choosing a tracker

`lets init` (via `/lets:init`) prompts for the tracker on a fresh project and installs the matching adapter file. To change it later, edit `LETS_TRACKER` in `.lets/.env` and run `/lets:update` (which re-syncs the adapter file). A non-beads tracker skips `bd init`.

```env
# .lets/.env
LETS_TRACKER=planfix-mcp
```

`lets init` then installs `.claude/rules/tracker-planfix-mcp.md`, scaffolds `.claude/rules/tracker-planfix-mcp.board.md` (once — see below), and removes any previously-installed shipped adapter file. An invalid name (or one with no shipped `tracker-<name>.md`) is skipped with a warning, never a crash.

**Switching trackers leaves the previous adapter's `tracker-<name>.board.md` in place** — the board file is user-owned and is never auto-removed (only the managed `tracker-<name>.md` is cleaned up). Delete the stale board file by hand after a switch, otherwise its now-inactive board instructions keep loading into model context every session.

## Planfix (`planfix-mcp`) setup

The Planfix adapter talks to the [Planfix MCP server](https://github.com/popstas/planfix-mcp-server), which must be connected in Claude Code. The plugin documents and scaffolds the wiring; it never holds the token.

### 1. Wire the MCP server (`.mcp.json`)

`lets init` gitignores `.mcp.json` (it can carry a secret). Add the server:

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

**Never commit a literal token.** Use `${PLANFIX_TOKEN}` env-var expansion (resolved from your shell). The token lives only here / in the server's env — never in `.lets/.env` (world-readable, injected into model context) and never in a `.board.md`.

### 2. Describe your board (`tracker-planfix-mcp.board.md`)

This **user-owned** file (scaffolded once, never overwritten by updates) maps neutral statuses to your Planfix status ids and declares the allowed transitions the adapter honors on `set-status`/`close`:

```markdown
## Status map (neutral -> Planfix)
| neutral      | Planfix name | id |
| open         | New          | 1  |
| in_progress  | In Progress  | 2  |
| in_review    | Review       | 3  |
| closed       | Done         | 4  |
```

It is auto-loaded into model context and is git-shareable — so **never put a secret in it**.

### Caveats

- The MCP server must be connected in your Claude Code session. An interactively-authenticated server may be absent in **headless / cron runs**, so the autonomous pipeline on `planfix-mcp` is not guaranteed — CORE verbs (`set-status`/`close`) hard-fail loudly rather than silently no-op, so `/lets:done` never claims a close it didn't perform.
- End-to-end Planfix verification is **manual** — it needs a live, authenticated server and cannot run in CI.

## Adding a new tracker

Authoring an adapter is writing one markdown file. The full recipe (the verb contract, the pinned capability-table format, the board/secret conventions, and the contract test that covers it) is in **CONTRIBUTING.md → "Add a tracker adapter"**.
