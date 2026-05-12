# Configuration

## `.lets/.env` — project settings

`/lets:init` creates `.lets/.env` with sensible defaults. Edit it to change behavior:

```env
# Default response language — write the English name, like every value here
# (English, Ukrainian, Russian, Japanese, ...)
LETS_LANGUAGE=English

# Target branch for merges and PR base
LETS_MERGE_BRANCH=main

# PR flow: github | bitbucket | local
LETS_PR_FLOW=github

# Task tracker (currently: beads)
LETS_TRACKER=beads
```

| Key | Purpose |
|-----|---------|
| `LETS_LANGUAGE` | The language Claude responds in when it isn't clear from your message. An English language name (`English`, `Ukrainian`, `Japanese`, …) regardless of the script it's written in. |
| `LETS_MERGE_BRANCH` | The branch tasks merge into and PRs target. Used wherever LETS would otherwise assume `main`. |
| `LETS_PR_FLOW` | `github` — `/lets:done` pushes and opens a PR via `gh`. `bitbucket` — planned. `local` — no PR; `/lets:done` merges locally. |
| `LETS_TRACKER` | The task tracker. Currently `beads`. (The schema is reserved for future trackers; today everything calls `bd` regardless.) |

The first line of the file is `LETS_ENV_VERSION` — that's metadata (which `lets` version last wrote the file), not something you set. Keys you add yourself are preserved across `/lets:init` and `/lets:update`, kept under a `# User-added keys` separator.

> **Not for secrets.** The SessionStart hook injects the `LETS_*` values from this file into Claude's context every session, and the file is world-readable on disk. GitHub auth goes through `gh auth login`; other secrets belong in your OS keychain or a tool-specific credential file (for example `.beads/.env` for beads).

> **Migrating from `config.yaml`:** if a project still has `.lets/config.yaml` from an older version, `/lets:init` migrates it to `.lets/.env`. The yaml file is left in place for reference but no longer read.

## File layout

Everything LETS generates lives under `.lets/` (gitignored):

```
.lets/.env              Project settings (the keys above)
.lets/.env.example      Reference defaults (regenerated each `lets init`)
.lets/sessions/         Session summaries and start references
.lets/reviews/          Saved review reports
.lets/plans/            Implementation plans from /lets:plan
.lets/execution/        PR review state and team records
.lets/cache/            Usage stats and cached data
```

Interactive worktrees live in `.worktrees/` at the project root (also gitignored).

The workflow rules are the exception — they live outside `.lets/` because they're part of Claude Code's project-instructions channel:

```
.claude/rules/lets-rules.md   Workflow rules — copied from the plugin by /lets:init, re-synced by /lets:update
```

Don't edit `.claude/rules/lets-rules.md` by hand — it's a managed copy, and `/lets:init` / `/lets:update` will rewrite it on the next sync. (If you're customizing the plugin itself, edit `plugins/lets/rules/lets-rules.md` instead — see [`CONTRIBUTING.md`](../CONTRIBUTING.md).)

## Setup order: `lets init`, then `bd init`

Run `/lets:init` first — it'll run `bd init` for you if both `lets` and `bd` are on `$PATH`; you can also run `bd init` manually afterwards. The two tools touch different files:

- **`lets init`** writes only LETS-owned `.gitignore` entries (`.lets/`, `.worktrees/`), creates the `.lets/` layout and `.lets/.env`, copies `.claude/rules/lets-rules.md`, and sets the statusline in `.claude/settings.json`.
- **`bd init`** adds its own `.gitignore` entries (`.beads/...`, `issues.jsonl`, `.beads-credential-key`), installs a project-scoped SessionStart + PreCompact hook block into `.claude/settings.json`, and creates `CLAUDE.md` / `AGENTS.md` if they don't exist yet.

> **Known quirk:** bd's project-scoped PreCompact hook runs in addition to the plugin's, so on a compaction the rules get injected twice. Harmless — being tracked for a fix.

For a shared task database across a team, `bd init --server --database=<name>` connects to a Dolt remote — see `scripts/remote/dolt/README.md` in this repo and the beads docs.

## Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| [Claude Code](https://claude.com/claude-code) | Yes | The AI coding agent LETS plugs into |
| [git](https://git-scm.com/) | Yes | Version control, branching, worktrees |
| [beads](https://github.com/steveyegge/beads) | Yes | Task tracking (Claude Code plugin / `bd` CLI) |
| [gh](https://cli.github.com/) | Optional | GitHub PR workflow (`LETS_PR_FLOW=github`) |
| [jq](https://jqlang.github.io/jq/) | Optional | Statusline JSON parsing |

Installing the `lets` binary itself: see **[installation.md](installation.md)**.

## See also

- **[installation.md](installation.md)** — installing the binary and the plugin
- **[tasks.md](tasks.md)** — beads and the task workflow
- **[commands.md](commands.md)** — `/lets:init`, `/lets:update`
