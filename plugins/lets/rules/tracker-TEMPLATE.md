---
name: tracker-TEMPLATE
version: 0.0.0
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - they are managed by `lets init` / `lets update`. Edit the canonical source in plugins/lets/rules/ instead. This TEMPLATE is a skeleton an adapter author copies to tracker-<name>.md; it is never selected by LETS_TRACKER and never installed. -->

# Tracker adapter: TEMPLATE

Binds the neutral task-tracker verbs for ONE `tracker × transport`. Commands call a NEUTRAL VERB through a ` ```lets-tracker ` block (`verb key=value`, one per line) - never inline `bd`; the orchestrator finds the verb in the table below and runs ITS binding. How a block resolves (adapter lookup, read shape, bodies, degradation, preflight) lives in the `lets:protocol-tracker` skill - this file carries only this tracker's bindings.

- **Budget: adapter <= 8,192 B, board profile <= 4,096 B.** The adapter is in the model's context on every turn today (auto-loaded from `.claude/rules/`), and in the session from first use once it is loaded on demand - every byte is paid each turn. Keep bindings, cut prose.
- **Bindings are trusted, executed code.** Each cell runs as written. Keep it to the tracker's own transport (a CLI command, an `mcp__*` tool, a REST call) - never a destructive or exfiltrating command. The contract test pins this table's SHAPE, not a binding's SAFETY.
- **Orchestrator-only.** Subagents never get this file or call tracker verbs.
- **No "not loaded" fallback here** - a file that is not loaded cannot instruct anything; that rule lives in `lets:protocol-tracker`.

## Neutral statuses

A DECLARATION - prune it to this board. Naming an optional status AUTHORIZES a command to move a task there (`/lets:done` advances to `in_review` after a PR only if it is named here).

Required: `open`, `in_progress`, `closed`.
Optional - keep ONLY what this board has: {`in_review`, `blocked`}.

`show` / `list-by-status` MUST return status as one of these neutral names (the adapter translates native -> neutral) in the neutral shape `{id, title, status}` (+ `description` on `show`, + `url` only if the tracker has per-task links).

## Capabilities + bindings

<!-- PINNED CONTRACT: the header row below is fixed and machine-parsed by the adapter contract test and the beads golden test. Do not reorder or rename columns; put any extra detail inside the binding cell, never in a new column. -->

- **Declare fields in the NEUTRAL vocabulary:** `create` and `set-field` open with `accepts:`, `show` with `returns:`, listing only `id`, `title`, `status`, `url`, `description`, `type`, `priority`, `labels`, `design`, `assignee`. A native rename is written inline: `priority→severity` (callers match the neutral side). End the declaration with a period - it terminates the list, or the binding's own backticks read as field names. An unsupported verb still writes the marker: `accepts: nothing - absent`.
- **Declare every offerable field, and only the ones callers consume.** An undeclared field is collected and dropped, or rendered as if the tracker had answered (phantom success). Declaring nothing extra is fine; declaring nothing at all is not. Every extra field on a read verb multiplies its payload.

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes    | accepts: {NEUTRAL fields this create stores}. {how to create; returns `{id, url}`} |
| show           | CORE | yes    | returns: {NEUTRAL fields this show returns - `id`, `title`, `status`, `description` required when supported; `url` only with per-task links}. {how to fetch by id; status NEUTRAL} |
| comment-add    | CORE | yes    | {how to add a comment body} |
| set-status     | CORE | yes    | {how to set a NEUTRAL status} |
| close          | CORE | yes    | {how to close (optional reason)}. Returns the status the task is left in (a post-condition, never a guess): `closed` = closed; another neutral status = a process-gated terminal, the adapter made the legal advance and the caller reports a handoff; none = nothing happened (no-op). A FAILED binding is not a status - it HARD-FAILs |
| comment-list   | OPT  | yes/no | {how to list comments, or `absent`} |
| list-by-status | OPT  | yes/no | {how to list by neutral status (returned NEUTRAL), or `absent`} |
| search         | OPT  | yes/no | {how to search by text, or `absent`} |
| ready/stats    | OPT  | yes/no | {ready / stats / blocked views, or `absent`} |
| label          | OPT  | yes/no | {label ops, or `absent`} |
| assignee       | OPT  | yes/no | {assignee ops, or `absent`} |
| set-field      | OPT  | yes/no | accepts: {NEUTRAL fields this adapter can overwrite}. {how, or `absent`} |

## Degradation

- OPTIONAL verb absent (`supported = no`) -> the command continues and tells the user; never crash.
- CORE verb failing at runtime (e.g. MCP tool not connected) -> HARD-FAIL loud ("close FAILED - task NOT closed"); never report success - critical under AUTO MODE.

## Worktree

How a worktree shares this tracker's store and how tasks and branches are named. `lets worktree create` / `adopt` link the store; Go reads these lines (`lets worktree info` / `branch-name`) - never match them in markdown. One line per key, value in backticks, ending with a period.

- **`links:`** - repo-relative files a worktree shares with the main checkout, each with a mode (0600, 0640 or 0644; a credential is 0600; LETS only tightens an existing file), e.g. `links:` `` `.mytracker/token.json` (0600) ``. Never `.git*` or `.lets*`. Sharing is the trust decision this line makes. No local store -> `links: nothing.`
- **`id:`** - one RE2 fragment matching a task id: no anchors, no named groups; every match must pass the detect-task id gate (class `[A-Za-z0-9._-]`, no leading `-`). `id: nothing.` = names never carry an id.
- **`branch:`** / **`worktree-branch:`** - the branches LETS creates (take-task / `lets orca open`, and `/lets:worktree create <id>`): exactly one `{id}`, at most one `{slug}`, literal text only `[A-Za-z0-9/._-]`. Default `feature/{id}-{slug}` / `worktree-{id}-{slug}`.
- **`accept:`** (optional) - comma-separated extra shapes LETS never creates but `adopt` recognizes (e.g. `{id}-{slug}`).

A `branch:` / `worktree-branch:` shape is that task's branch by construction; an `accept:` shape is an unconfirmed candidate (`origin: branch`) that detect-task / take-task confirm first. No `id:` = no id is read off a name (`convention_undeclared`); `branch:` / `worktree-branch:` still render. A board file's own `## Worktree` may override `id:` / `branch:` / `worktree-branch:` / `accept:` per key (never `links:`), read from the main checkout only - e.g. tasks `PWA-45122` on branches `feature/PWA-45122-short-title`: `` id: `PWA-[0-9]+`. `` and `` branch: `feature/{id}-{slug}`. ``.

## Claim hygiene

Mark every claim you have NOT exercised against the running tracker (from docs, inferred, guessed - a payload limit, a transition, a field name) inline, inside the cell it qualifies: `[ASSUMED]` (from docs), `[UNVERIFIED]` (a guess), `[VERIFIED <date>]` once confirmed. A binding you ran, or one checkable from `--help`, needs no marker. An unexercised claim stated flatly is acted on; markers on self-evident bindings are noise.

## Board profile (optional)

Project-specific semantics (native status ids, transitions, default project, REST nuances) live in a sibling `tracker-TEMPLATE.board.md` - user-owned, scaffolded once by `lets init`, never overwritten, auto-loaded; honor its status map and transitions. Switching `LETS_TRACKER` removes only the managed adapter - delete a stale `*.board.md` by hand.

<!-- NEVER put a token/password/secret in a .board.md: it is auto-loaded into model context every session AND is git-shareable. Secrets belong only in the adapter's transport config (e.g. an MCP server's own env), never here and never in .lets/.env. -->

## Secrets (only for an adapter that calls an external API directly)

Shipped adapters need none (an MCP adapter's token is the MCP server's). A direct-REST adapter MUST read its token from a gitignored, user-private file (e.g. `.lets/trackers/<name>/.env`) - NEVER from `.lets/.env` (mode 644, injected into context).
