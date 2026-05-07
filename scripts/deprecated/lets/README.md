# Deprecated bash scripts (LETS plugin)

Original bash implementations of LETS plugin hooks and scripts, preserved
for reference after the Go CLI port (lets-7vtaw epic).

## What's here

| File | What it was | Replaced by |
|------|-------------|-------------|
| `session-start.sh` | SessionStart + PreCompact hook (rules-context.md injection + `## LETS Config` block, with yaml→env auto-migration) | `lets hook session-start` + `lets hook precompact` (Go subcommands; logic in `cli/internal/hook/sessionstart/`) |
| `init.sh` | `/lets:init` per-project setup (creates `.lets/.env`, copies statusline) | TODO: `lets init` (Phase 4 of lets-7vtaw) |
| `statusline.sh` | Per-project statusline at `.lets/statusline.sh` | TODO: `lets statusline` (Phase 4 of lets-7vtaw) |
| `README-original.md` | Original `scripts/lets/README.md` from main branch (pre-reorg) | n/a (kept for context on what the bash scripts did) |

## Why kept

These are reference for:
- Comparing Go output vs bash semantics during regression debugging
- Understanding edge cases the bash logic handled (CRLF, inline comments, etc.)
- Recovery if Go port is reverted for any reason

## Status

- `session-start.sh`: deleted from active codebase in commit a59d78b (Phase 3.5)
- `init.sh`, `statusline.sh`: still active in `plugins/lets/scripts/lets/` until Phase 4 ports them; they will be moved here on retirement, not duplicated

## Removal

These files can be deleted permanently after:
1. Go ports of all 3 stabilize across at least one tagged release
2. No production users on bash (lets-7vtaw end-state when 1.0.0 ships)
