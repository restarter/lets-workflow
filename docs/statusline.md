# Statusline

The LETS statusline is the bordered box Claude Code draws at the bottom of the
window. `lets init` wires it into `.claude/settings.json`:

```json
{
  "statusLine": { "type": "command", "command": "lets statusline" }
}
```

Claude Code pipes a JSON snapshot of the session to `lets statusline` on every
render; the command writes back the box. It needs no API key and makes no
network calls on the render path (see [Where the data comes from](#where-the-data-comes-from)).

## Anatomy

The default is a closed box wrapping up to four lines — identity, budget, task,
and a rotating tip:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ⚘ LETS Workflow dev · ☰ lets-workflow · ⎇ main · +120 -30 · ⇄ #94 approved    │
│ ✦ Opus 4.8 (1M context) high · window 42% (424k/1000k) · 5h 58% (2h 10m) …    │
├─────────────────────────────────────────────────────────────────────────────┤
│ ✓ lets-ds6bc Statusline 2.0 · 3 comments (2h) ← /lets:note                    │
│ * Quick sanity check before committing? Run /lets:check (~30s, no agents).    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Line 1 — identity.** Brand + `lets` version, a `☰` location pill, the branch,
the session diff (`+added -removed`), and the PR (`⇄ #94` + review state) when
one is open.

**Line 2 — budget.** Model (`✦`) + effort level, then context-window and quota
gauges: `window 42% (used/total)`, plus `5h` and `7d` rate-limit usage with the
time until each resets. No progress bars — just numbers.

**Line 3 — task.** The active beads task (`✓ <id> <title>`) when the branch maps
to a real, bd-confirmed task, with the comment count and a `← /lets:note` hint.
A branch with no task drops *only* this line — the divider stays so the frame is
consistent. Hide it explicitly with `--no-task`, which also skips the background
`bd` lookup that refreshes it.

**Line 4 — tip.** A rotating workflow hint. Hide it with `--no-tip`.

### Glyphs

All glyphs are plain text (no [Nerd Font](https://www.nerdfonts.com/) needed), so
they take the palette color and stay one cell wide — no emoji, nothing to render
as a "tofu" box on a bare terminal.

| Glyph | Means |
|---|---|
| `⚘` | LETS brand |
| `☰` | location (folder / worktree) |
| `⎇` | git branch |
| `⇄` | pull request |
| `✦` | model |
| `✓` | active task |
| `←` | `/lets:note` hint |
| `*` | tip line |

## Width tiers

The box reads the terminal width from `COLUMNS` (Claude Code sets it) and picks
one of two tiers:

| Tier | Width | What changes vs Full |
|---|---|---|
| **Full** | ≥ 72 cols | everything above |
| **Compact** | < 72 cols | short `LETS` brand; drops the location pill, PR, and the `(… context)` / token detail; `window` → `w`; task line is `id + title` only |

In **Compact** the model name and effort level stay — only the `(… context)`
suffix is shed. Within either tier, a long task title or tip clips first (with
`…`), keeping the task id and the note/hint in the frame.

The box fills the window below 90 cols and hugs its content above that, capped so
it never stretches across a very wide screen.

## Worktrees

Inside a [worktree](parallel-work.md) the location pill reads just **`worktree`**
— the worktree directory name already equals the branch name shown next to `⎇`,
so repeating it would be noise. Outside a worktree the pill shows the project
folder name (taken from the git top-level, so it doesn't change as you `cd` into
subdirectories like `cli/`).

## Flags

Set these in the `command` string in `.claude/settings.json` — e.g.
`"command": "lets statusline --light --no-tip"`.

| Flag | Env equivalent | Effect |
|---|---|---|
| `--light` | — | Light-terminal palette (default is dark) |
| `--no-tip` | `LETS_STATUSLINE_TIP=off` | Hide the bottom tip line |
| `--no-dir` | `LETS_STATUSLINE_DIR=off` | Hide the location pill |
| `--no-task` | `LETS_STATUSLINE_TASK=off` | Hide the task line (also skips its background `bd` refresh) |
| `--compact` | — | Render the legacy 2-line bar instead of the box (fallback for terminals where the box misbehaves) |

The env equivalents accept `off`, `0`, `false`, or `no`. (Note: env vars only
work if your terminal exports them to the statusline subprocess; the flags are
the reliable channel.)

```json
{
  "statusLine": {
    "type": "command",
    "command": "lets statusline --light --no-dir"
  }
}
```

## Where the data comes from

Everything on the bar comes from the JSON Claude Code pipes in on each render —
**including the 5h / 7d rate limits**, which modern Claude Code sends directly
(no API key, no keychain, no network call on the render path). Two pieces are
refreshed off the hot path by short-lived background subprocesses and cached
under `.lets/cache/`:

- **Task line** — `lets statusline --fetch-task-only` runs `bd show` (90s TTL)
  so the render never blocks on beads or the network.
- **Quota fallback** — on older Claude Code versions that don't send rate limits
  in the payload, `--fetch-usage-only` fills them in from the usage API; on
  current versions this path is unused.

## Caveats

- **Border alignment** depends on your terminal counting glyph widths the way the
  renderer does. All glyphs are plain 1-cell text (no emoji), so the box leaves a
  small right margin to absorb ambiguous-width edge cases; if a border still
  drifts, it's an ambiguous-width glyph rendering double in your font — hide the
  affected element (`--no-dir` / `--no-tip` / `--no-task`) or file an issue.

## See also

- [configuration.md](configuration.md) — `.lets/.env` and `lets init`
- [installation.md](installation.md) — installing the `lets` binary
