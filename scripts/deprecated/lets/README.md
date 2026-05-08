# Deprecated bash scripts (LETS plugin)

Original bash implementations of LETS plugin hooks and scripts, preserved
for reference after the Go CLI port (lets-7vtaw epic).

## What's here

| File | What it was | Replaced by |
|------|-------------|-------------|
| `session-start.sh` | SessionStart + PreCompact hook (rules-context.md injection + `## LETS Config` block, with yaml→env auto-migration) | `lets hook session-start` + `lets hook precompact` (Go subcommands; logic in `cli/internal/hook/sessionstart/`) |
| `init.sh` | `/lets:init` per-project setup (creates `.lets/.env`, copies statusline) | `lets init` (Go subcommand; logic in `cli/internal/initcmd/`). Active bash version still in `plugins/lets/scripts/lets/init.sh` until lets-8ilsl rewrites the slash command |
| `statusline.sh` | Per-project statusline at `.lets/statusline.sh` | `lets statusline` (Go subcommand; logic in `cli/internal/statusline/`). Active bash version in `plugins/lets/scripts/lets/statusline.sh` is now a thin shim that delegates to the Go binary |
| `README-original.md` | Original `scripts/lets/README.md` from main branch (pre-reorg) | n/a (kept for context on what the bash scripts did) |

## Why kept

These are reference for:
- Comparing Go output vs bash semantics during regression debugging
- Understanding edge cases the bash logic handled (CRLF, inline comments, etc.)
- Recovery if Go port is reverted for any reason

## Status

- `session-start.sh`: archived in commit a59d78b (Phase 3.5). Replaced by `lets hook session-start`.
- `statusline.sh`: archived in commit 127b6df (Phase 4a). Plugin tree retains a thin shim that delegates to `lets statusline`.
- `init.sh`: archived in commit 6bb43fc (Phase 4b). Plugin tree retains the bash version pending lets-8ilsl (slash-command rewrite); it will be deleted once `/lets:init` invokes `lets init` directly.

## Removal

These files can be deleted permanently after:
1. Go ports of all 3 stabilize across at least one tagged release
2. No production users on bash (lets-7vtaw end-state when 1.0.0 ships)
