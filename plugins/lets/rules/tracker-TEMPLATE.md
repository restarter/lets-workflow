---
name: tracker-TEMPLATE
version: 0.0.0
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - they are managed by `lets init` / `lets update`. Edit the canonical source in plugins/lets/rules/ instead. This TEMPLATE is a skeleton an adapter author copies to tracker-<name>.md; it is never selected by LETS_TRACKER and never installed. -->

# Tracker adapter: TEMPLATE

This file teaches the orchestrator how to perform the neutral task-tracker verbs for ONE adapter (one `tracker × transport` instance). Each LETS command calls a NEUTRAL VERB; resolve it to the concrete call via the Bindings table below.

- **Command bodies invoke verbs via ` ```lets-tracker ` blocks** (`verb key=value`, one per line) — never inline `bd`. The orchestrator reads the block, finds the verb in the Bindings table below, and runs ITS binding (see lets-rules "Tracker Adapters").
- **Bindings are trusted/executed code.** Each binding cell is run as written when a command invokes its verb. Anyone installing this adapter is running these bindings — keep them to the tracker's own transport (a `bd`/CLI command, an `mcp__*` tool, a REST call), never a destructive or exfiltrating command. The contract test pins this table's SHAPE, not the SAFETY of a binding.
- **Verb resolution is ORCHESTRATOR-ONLY.** Subagents do not receive this file (it is not injected into their context) and never call tracker verbs directly.
- **The "no adapter file loaded" fallback does NOT live here.** A file that is not loaded cannot instruct anything - that rule lives in the command bodies and is conditioned on the `LETS_TRACKER` value (see CONTRIBUTING "Add a tracker adapter").

## Neutral statuses

**This section is a DECLARATION, not a vocabulary listing - prune it to this board.** Naming an optional status here is what AUTHORIZES a command to move a task there: `/lets:done` advances to `in_review` after opening a PR only if this section names `in_review`. Leave the line as copied and a caller will push tasks into a column your board does not have.

Required (every adapter): `open`, `in_progress`, `closed`.
Optional - list ONLY what this board really has, delete the rest: {`in_review`, `blocked`}.

`show` and `list-by-status` MUST return the task's status as one of these neutral names (the adapter translates native -> neutral), and the neutral task shape `{id, title, status}` (+ `description` on `show`, + `url` only from a tracker that has per-task links). This keeps consuming commands (e.g. `[ "$STATUS" = "in_progress" ]`) adapter-agnostic.

## Capabilities + bindings

<!-- PINNED CONTRACT: the header row below is fixed and machine-parsed by the adapter contract test and the beads golden test. Do not reorder or rename columns; put any extra detail inside the binding cell, never in a new column. -->

- **Declare fields with the NEUTRAL vocabulary.** `create` opens with `accepts:`, `show` with `returns:`, `set-field` with `accepts:`. List only neutral argument names - `id`, `title`, `status`, `url`, `description`, `type`, `priority`, `labels`, `design`, `assignee`. A tracker that stores one under a different native name writes the rename inline with an arrow: `priority→severity`. Never invent a name: a caller matches on the NEUTRAL side, so `accepts: severity` alone is invisible to it. **Close the declaration with a period** before the binding prose resumes - that period is the terminator, and without it the transport call's own backticks are read as field names. An adapter that marks the verb unsupported still writes the marker with nothing in it: `accepts: nothing - absent`.
- **An offerable field MUST be declared.** An undeclared field is one a command collects from the user and then drops, or renders as though the tracker had answered - the same phantom-success class as reporting a close that never happened. Declaring nothing extra is fine; declaring nothing at all is not.
- **Keep declared fields minimal.** Every field a read verb carries multiplies its payload. On a REST-backed adapter one extra custom field has repeatedly pushed a `list` response past what fits in a single reply - declare the fields callers actually consume, not everything the tracker can return.

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes    | accepts: {the NEUTRAL field names THIS create really stores - a caller must not collect anything else}. {how to create a task; returns `{id, url}`} |
| show           | CORE | yes    | returns: {the NEUTRAL field names THIS show really returns - `id`, `title`, `status`, `description` are required whenever `supported = yes` because commands read them unconditionally; `url` only if this tracker actually has a per-task link (beads has none); add any extra it carries}. {how to fetch them by id; status as a NEUTRAL name} |
| comment-add    | CORE | yes    | {how to add a comment body to a task} |
| set-status     | CORE | yes    | {how to set a NEUTRAL status on a task} |
| close          | CORE | yes    | {how to close/complete a task (optional reason)}. Returns the task's status AFTER the call, as a NEUTRAL name - a POST-CONDITION the adapter must actually have left the task in, never a diagnosis of what it would have done. `closed` = closed. Another neutral status = this board's terminal is process-gated, so the adapter performed the legal advance instead and the caller reports a handoff, not a close. No status at all = nothing happened (a no-op adapter). A binding that FAILED is not a status - it HARD-FAILs per Degradation |
| comment-list   | OPT  | yes/no | {how to list a task's comments, or `absent`} |
| list-by-status | OPT  | yes/no | {how to list tasks by neutral status (status returned NEUTRAL), or `absent`} |
| search         | OPT  | yes/no | {how to search tasks by text, or `absent`} |
| ready/stats    | OPT  | yes/no | {ready-graph / stats / blocked views, or `absent`} |
| label          | OPT  | yes/no | {label ops, or `absent`} |
| assignee       | OPT  | yes/no | {assignee ops, or `absent`} |
| set-field      | OPT  | yes/no | accepts: {the NEUTRAL field names this adapter can overwrite}. {how to overwrite them, or `absent`} |

## Degradation

- **OPTIONAL verb absent** (`supported = no`) -> the calling command continues and tells the user the capability is unavailable for this tracker. Never crash.
- **CORE verb unresolved at runtime** (binding can't be performed - e.g. an MCP tool is not connected) -> HARD-FAIL loud (e.g. "close FAILED - task NOT closed"); do NOT report success. This matters under AUTO MODE: a `/lets:done` must never claim it closed a task it did not.

## Worktree

Declares what a worktree needs from this tracker and how the tracker names tasks and branches. `lets worktree create` / `adopt` link the store, and every LETS path that creates or recognizes a task branch reads the same declaration (through `lets worktree info` / `lets worktree branch-name`, never by matching patterns in markdown). One declaration per line, each line starting with its key, the value in backticks, closed with a period.

- **`links:`** - repo-relative files a worktree must share with the main checkout, each with a mode: e.g. a line `links:` followed by `` `.mytracker/token.json` (0600) `` and a period. Modes are 0600, 0640 or 0644; a credential declares 0600, and LETS only ever tightens an existing file. Never `.git*` or `.lets*`. Every link shares the main checkout's file with every worktree - that is the trust decision this line makes. Write `links: nothing.` when the tracker keeps no local store.
- **`id:`** - one RE2 fragment matching a task id, with no anchors (`^`, `$`) and no named groups, or `nothing.` when names never carry an id. Every id it matches must also pass the detect-task id gate (class `[A-Za-z0-9._-]`, no leading `-`).
- **`branch:`** / **`worktree-branch:`** - the branch templates LETS creates: the task branch (take-task, `lets orca open`) and the one `/lets:worktree create <id>` makes. A template holds exactly one `{id}` and at most one `{slug}`; its literal text uses only `[A-Za-z0-9/._-]`. Omitted, they default to `feature/{id}-{slug}` and `worktree-{id}-{slug}`.
- **`accept:`** - a comma-separated list of extra shapes LETS never creates but `lets worktree adopt` recognizes (e.g. a worktree someone named `{id}-{slug}`). Optional.

**Trust rule.** A branch in a `branch:` / `worktree-branch:` shape is that task's branch by construction. An `accept:` shape is only an unconfirmed candidate: adopt records it as `origin: branch`, and detect-task / take-task confirm it before anything acts on it.

**Undeclared vs nothing.** No `id:` line at all means the adapter predates this section: every consumer keeps its previous behavior (`convention_undeclared`). `id: nothing.` is a declaration - names never yield an id.

**Board override.** A project's user-owned `tracker-<name>.board.md` may carry its own `## Worktree` with any of `id:` / `branch:` / `worktree-branch:` / `accept:`; each key present there replaces this file's value (`links:` is ignored there - it stays this adapter's trust decision). It is read from the main checkout only. Example - a board whose tasks are `PWA-45122` and whose team names branches `feature/PWA-45122-short-title` declares `` id: `PWA-[0-9]+`. `` and `` branch: `feature/{id}-{slug}`. ``.

## Claim hygiene

**Scope: claims you have NOT exercised against the running tracker.** A binding you have run, or one checkable from a local `--help`, needs no marker - the next reader verifies it in seconds. What needs one is everything taken from documentation, inferred, or guessed: a payload limit, a rendering quirk, a transition the board may or may not permit, a field name you have not seen in a response.

Mark those inline, in the cell or sentence they qualify: `[ASSUMED]` (from docs, not yet run), `[UNVERIFIED]` (a deliberate guess), `[VERIFIED <date>]` once confirmed. A worked row:

| close | CORE | yes | `PATCH /tasks/<id>`; returns `in_review`, not `closed` `[ASSUMED]` - the gated terminal is documented but untested |

The marker goes INSIDE the binding cell: the table header is pinned, so a new column is not an option.

An unexercised claim stated flatly is worse than none - this file is auto-loaded instruction, so the agent acts on it. The rule is scoped rather than universal on purpose: a convention nobody can follow completely is one nobody follows at all, and a file dense with `[VERIFIED]` on self-evident bindings teaches the next author that markers are noise.

## Board profile (optional)

Project-specific semantics (native status ids, transitions, principles, default project, server name, REST nuances) live in a sibling `tracker-TEMPLATE.board.md` - user-owned, scaffolded once by `lets init`, NEVER overwritten, auto-loaded as a project instruction. If present, honor its status map + transitions. Switching `LETS_TRACKER` does NOT remove a board file (only the managed adapter `.md` is cleaned up) - delete a stale `*.board.md` by hand so it stops loading into context.

<!-- NEVER put a token/password/secret in a .board.md: it is auto-loaded into model context every session AND is git-shareable. Secrets belong only in the adapter's transport config (e.g. an MCP server's own env), never here and never in .lets/.env. -->

## Secrets (only if this adapter calls an external API directly)

The shipped adapters do not need this (beads uses `.beads/.env`; an MCP-based adapter's token is owned by the MCP server). A future direct-REST adapter that needs a token MUST read it from a gitignored, user-private file (e.g. `.lets/trackers/<name>/.env`) - NEVER from `.lets/.env` (mode 644, injected into model context); add the file + 0600/0700 perms helper when the first such adapter lands.
