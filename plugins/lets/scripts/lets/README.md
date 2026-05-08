# LETS Scripts (legacy)

These bash files are **legacy shims** kept active until lets-8ilsl ships. New work happens in the Go CLI:

- `init.sh` is invoked by the `/lets:init` slash command today. After lets-8ilsl rewrites `commands/init.md` to call `lets init` directly, this file moves to `scripts/deprecated/lets/`.
- `statusline.sh` is a thin shim that delegates to `lets statusline` (Go binary). Per-project `.lets/statusline.sh` exists only for backward compat with users whose `.claude/settings.json` still references the bash path.

For current architecture, install steps, and contributor workflow, see:

- [`cli/README.md`](../../../cli/README.md) — Go CLI build, install, conventions
- [`CLAUDE.md`](../../../CLAUDE.md) — project layout, architecture decisions, recipes
- [`scripts/deprecated/lets/README.md`](../../../scripts/deprecated/lets/README.md) — what the bash hooks did before being ported

Cleanup of these files is tracked in lets-8ilsl + Phase 6 of lets-7vtaw.
