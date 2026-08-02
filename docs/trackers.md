# Task Trackers

LETS is **tracker-agnostic**. Instead of hardcoding beads (`bd`), a project binds whatever task store it wants through a single **adapter file**. One config key, `LETS_TRACKER`, names the adapter; `lets init` installs exactly one `.claude/rules/tracker-<name>.md` (auto-loaded, drift-tracked like `lets-rules.md`) that binds the neutral verbs to concrete calls.

## The model

- **Neutral verbs** — every command speaks a small shared vocabulary: `create`, `show`, `comment-add`, `set-status`, `close` (CORE) plus `comment-list`, `list-by-status`, `search`, `ready`/`stats`, `label`/`assignee`/`set-field` (OPTIONAL).
- **One adapter file per tracker** — `tracker-<name>.md` is self-contained: a capability table mapping each verb to a concrete binding (a `bd` command, an `mcp__*` tool, a REST call) + a degradation section. Transport is encoded **per verb** inside the file — there is no second "driver" config axis.
- **Resolution** (the rule lives in `lets-rules.md` → "Tracker Adapters"): command/skill bodies invoke neutral verbs via ` ```lets-tracker ` fenced blocks — NOT inline `bd`. The orchestrator resolves each block through the loaded `tracker-<name>.md` and runs its binding. With `LETS_TRACKER=beads` that binding is the same `bd` command LETS always ran (golden-pinned, so beads runtime is unchanged); with a non-beads adapter it's that adapter's binding (e.g. an `mcp__*` tool). Computed comment bodies cross via a `body-file=` temp file; reads return the neutral `{id, title, status, url, description}` shape.
- **Adapters declare their fields, and `close` declares its outcome** — `create`/`set-field` open with `accepts:`, `show` with `returns:`, listing neutral field names (a native rename is written `priority`→`severity`). A command reads that declaration *before* it offers a field, so it can never collect a value from you that the tracker has nowhere to store. `close` returns the status the task ended in: `closed` when it really closed, another neutral status when this board's terminal is process-gated and the legal move was an advance — the caller then reports a handoff ("advanced to review, not closed") rather than claiming a close that never happened.
- **Subagents never touch the tracker** — they don't receive the adapter file, so a command that needs task data in a subagent prompt pulls it itself and injects it as fenced data. There is no allowlist and no exception; CI asserts the exemption map ships empty and scans every fence, plus the `*.workflow.js` workflow assets, for a stray `bd`.

## Shipped adapters

| `LETS_TRACKER` | Transport | Notes |
|----------------|-----------|-------|
| `beads` (default) | `bd` CLI | Reference adapter. Full capabilities. The `bd` bindings are golden-pinned, so beads runtime is unchanged. |
| `none` | — | Null adapter. Every verb is a no-op; commands degrade to a no-tracker stance. The "no-beads" mode. |

## Choosing a tracker

`lets init` (via `/lets:init`) prompts for the tracker on a fresh project and installs the matching adapter file. To change it later, edit `LETS_TRACKER` in `.lets/.env` and run `/lets:update` (which re-syncs the adapter file). A non-beads tracker skips `bd init`.

```env
# .lets/.env
LETS_TRACKER=beads
```

`lets init` then installs `.claude/rules/tracker-<name>.md` for the chosen adapter (and scaffolds its optional `tracker-<name>.board.md` once, if the adapter ships one), removing any previously-installed shipped adapter file. An invalid name (or one with no shipped `tracker-<name>.md`) is skipped with a warning, never a crash.

**Switching trackers leaves the previous adapter's `tracker-<name>.board.md` in place** — the board file is user-owned and is never auto-removed (only the managed `tracker-<name>.md` is cleaned up). Delete the stale board file by hand after a switch, otherwise its now-inactive board instructions keep loading into model context every session.

## planfix-mcp — prototyped, not shipped

`planfix-mcp` (Planfix via the [popstas/planfix-mcp-server](https://github.com/popstas/planfix-mcp-server), over the server's generic `planfix_request` REST passthrough) was prototyped and verified end-to-end against a live server, but it is **not a shipped adapter** — its board profile is deeply org-specific and would install a half-empty, per-user profile into every project. It lives in the maintainer's private reference materials, not the plugin. A popular-platform worked example (Jira or Trello) is planned to fill the "worked example beyond the TEMPLATE" role. To wire your own Planfix — or any — tracker, follow **Adding a new tracker** below.

## Trust & secrets

A `tracker-<name>.md` is **trusted instruction**: it auto-loads into model context every session, and after the neutral-verb rewrite its binding cells are *what executes* when a command invokes a verb. So installing a third-party / shared adapter is equivalent to running its code — a binding could be `bd close <id>; rm -rf …` or an exfiltrating `curl`. **Review every binding before installing an adapter you didn't author.** The contract test pins the table's *shape* (header + CORE rows + a `## Degradation` section + the `accepts:`/`returns:` declarations, checked against the neutral field vocabulary), NOT the *safety* of a binding.

Adapter files also carry a **verification marker on every factual claim** — `[VERIFIED <date>]`, `[ASSUMED]` or `[UNVERIFIED]`, inline in the cell it qualifies. The file is auto-loaded instruction, so a claim stated flatly is one the agent will act on; an unverified claim without a marker is worse than no claim at all.

A **token never belongs in an adapter or board file.** Both `tracker-*.md`/`*.board.md` (git-shareable, auto-loaded into context) and `.lets/.env` (mode 644, injected into context) are the wrong place. Secrets live only in the transport's own config — the MCP server's env, or a gitignored 0600 file for a future direct-REST adapter.

## Adding a new tracker

Authoring an adapter is writing one markdown file. The full recipe (the verb contract, the pinned capability-table format, the board/secret conventions, and the contract test that covers it) is in **CONTRIBUTING.md → "Add a tracker adapter"**.
