---
name: protocol-tracker
description: Internal skill for commands. The tracker verb-resolution protocol - how a lets-tracker block resolves through the active adapter, the neutral read shape, body crossing, degradation, preflight and trust. Loaded by a command or skill right before its first tracker verb. Do not trigger on user conversation.
user-invocable: false
---

# Protocol: tracker verb resolution

Load this before the first step that runs a ` ```lets-tracker ` block, unless its text is in your current context. Core rules keep the always-on part: never run a `lets-tracker` body as shell; never report a tracker change that did not happen; state-changing verbs stay gated under AUTO MODE; never append by overwriting a field.

## 1. Find the adapter (first, every time)

`LETS_TRACKER` (LETS Config; default `beads`) names the adapter.

| found (check with Bash `ls`, not Glob) | do |
|---|---|
| `.claude/lets/tracker-{LETS_TRACKER}.md` exists | Read it; also Read `.claude/lets/tracker-{LETS_TRACKER}.board.md` when present (user-owned board profile, overrides per key) |
| otherwise | the adapter is the auto-loaded `.claude/rules/tracker-{LETS_TRACKER}.md` copy (and its `.board.md`) already in your context - do NOT Read it again |
| `beads` or unset | resolve via `tracker-beads.md` - its bindings are the same `bd ...` calls LETS always ran (table-driven, golden-pinned) |
| a non-beads adapter whose file IS loaded | resolve via its file (e.g. an `mcp__*` tool); translate native <-> neutral statuses so surrounding logic stays adapter-agnostic |
| a non-beads adapter named, NO file loaded (plugin upgraded, `/lets:init` / `/lets:update` not re-run) | behave as `none`: do NOT run `bd` (wrong store), tell the user the tracker is not installed, nudge `/lets:update` |

One adapter file per `LETS_TRACKER` (`beads` | `none` | a custom one), installed by `lets init`; it binds the neutral verbs to concrete calls.

| tier | verbs |
|---|---|
| CORE | `create`, `show`, `comment-add`, `set-status`, `close` |
| OPTIONAL | `comment-list`, `list-by-status`, `search`, `ready`/`stats`, `label`/`assignee`/`set-field` |

## 2. Resolve a `lets-tracker` block

A fenced block tagged `lets-tracker`, one `verb key=value` per line (e.g. `close task=<id> reason="..."`), is a neutral verb CALL, not shell:

1. identify `<verb>` + args;
2. look the verb up in the adapter's capability table and run ITS `binding` (for beads the binding IS a `bd` command - run that);
3. NEVER execute the block body as a shell command.

Resolution is ORCHESTRATOR-ONLY - subagents never call tracker verbs (they do not receive the adapter file). A command that needs tracker data inside a subagent prompt pulls it itself and INJECTS it as fenced data. No exceptions, no carve-outs.

## 3. Reads and close

| rule | detail |
|---|---|
| read shape | `show` / `list-by-status` return the neutral `{id, title, status}`, plus `description` on `show`, and `url` only from a tracker with per-task links (beads has none) |
| status canon | required `open` / `in_progress` / `closed`; optional `in_review` / `blocked` - an adapter carries an optional one only if its own `## Neutral statuses` section names it; never emit one without checking |
| read the field | the body reads the returned field (annotated `# returns ...`), never greps a tracker's native JSON |
| `close` declares its outcome | the adapter's `close` row states the status it leaves the task in: `closed` = closed; another status = the board advanced the task instead and the caller MUST report the handoff, not a close; no status = nothing happened |

## 4. Bodies (`comment-add` body, `create` / `set-field` description)

Format-neutral, plain text: rich markdown is a beads-only affordance; other adapters render best-effort.

| way | when |
|---|---|
| inline `body="..."` / `description="..."` | a short OR purely orchestrator-templated value (a multi-line template with NO `$(...)` shell expansion is allowed inline) |
| file `<field>-file=<path>` (`body-file=` for comments, `description-file=` for create/set-field) | a value that needs runtime shell substitution (`$(date)`, `$(git log)`, `$CLAUDE_CODE_SESSION_ID`): the preceding ` ```bash ` block writes it to a temp file, so no computed multi-line value is re-typed across the model boundary. The path is repo-root-relative (bindings run with cwd = the project root, where `.lets/` lives); the binding reads the file (beads: `"$(cat <path>)"`) |

An empty body -> HARD-FAIL; never submit an empty comment.

## 5. Degradation (two-pronged - never flatten), then preflight

| case | do |
|---|---|
| an OPTIONAL verb the adapter marks `absent`, OR a CORE verb bound to a deliberate no-op (`none`) | continue and TELL the user; never report it as a recorded change (no phantom "done") |
| a binding that exists but FAILS at runtime (MCP tool not connected, `bd` not on PATH, an MCP adapter's `failures[]` non-empty) | HARD-FAIL loud ("set-status / close FAILED - task NOT changed"); never a phantom success. Critical under AUTO MODE - `/lets:done` must not claim a close it did not do |
| preflight (a precondition, not a degradation mode) | before OFFERING a verb, field or mode, read the capability table: a field the active binding does not declare in `accepts:` is mapped to one it does, or named unavailable - never collected and dropped. On the read side, never render a field the adapter's `show` does not declare in `returns:` - an invented value is a claim about the task, not about the tracker |

## 6. Trust

An adapter file is trusted instruction; its binding cells EXECUTE as written. Installing a third-party / shared adapter equals running its code - review every binding before installing one you did not author. The contract test pins table SHAPE, not binding SAFETY. A token belongs ONLY in the transport's own config (the MCP server env / a gitignored 0600 file) - NEVER in an adapter `tracker-*.md`, a `.board.md`, or `.lets/.env` (mode 644, injected into context).

## 7. Creating and linking tasks

| topic | rule |
|---|---|
| create | through the `create-task` skill (auto-triggers on "create task", "new task", "bd create" and variations); it enforces the required fields (`title`, `type`, `priority`, `description`, `labels`) and discovers project labels dynamically (on beads: the `bd create` flags; hash-based ids, collision-free in a multi-user setup) |
| dependencies (where the tracker supports them) | the dependency link (beads: `bd dep add`) sparingly - only when task B literally cannot start without task A done. Test: "can someone start this task right now without the other?" yes -> no dep. Most tasks are independent - do not over-link |
