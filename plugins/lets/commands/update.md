---
description: Sync this project with the current LETS release - checks .env, rules, the lets binary, the plugin, and the user-level global rules when installed
---

# Update LETS

Sync the drift-able LETS artifacts (four core + the optional user-scope global rules) with the current release. Bridges to `lets update --json` (Go binary): auto-syncs `.lets/.env` (header refresh when `LETS_ENV_VERSION` is stale), `.claude/rules/lets-rules.md` (re-copy when outdated/missing), and `~/.claude/rules/lets-rules.md` (user-scope global rules - the `user-rules` row appears only when that file exists), and reports actionable version status for the `lets` binary and the Claude Code plugin (which it cannot self-update).

> **MANDATORY:** Execute every Step's bash block **literally as written**. Do not substitute output from earlier `ls`/`cat` in this conversation - `.env` and other dotfiles are invisible to plain `ls`. The `test -f` checks below ARE the contract.

Difference from `/lets:init`: `/lets:init` is first-time setup (it also asks config questions and sets up the statusline + beads). `/lets:update` only syncs what a new release changes - it never prompts and never touches `settings.json` or beads.

## Step 1: Pre-checks

```bash
command -v lets >/dev/null 2>&1 || { echo "NO_LETS_BINARY"; exit 0; }
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "NOT_GIT_REPO"; exit 0; }
test -f "$LETS_PROJECT_ROOT/.lets/.env" && echo "ENV_EXISTS" || echo "ENV_ABSENT"
```

Branch on output:
- `NO_LETS_BINARY` → tell user: "`lets` binary not found on `$PATH`. Install it — `! curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash` (the leading `!` runs it in this session; or the same command without `!` in a terminal). See the README → Quick Start." NO LETS box. STOP.
- `NOT_GIT_REPO` → tell user: "Not a git repository. `/lets:update` runs inside a LETS project." NO LETS box. STOP.
- `ENV_ABSENT` → not initialized. Tell user: "This project hasn't been set up - run `/lets:init` first." Then still run Step 2 (it reports `.env: not-initialized` plus the binary/plugin status, which is useful). In Step 4 show the LETS box pointing at `/lets:init` instead of `/lets:start`.
- `ENV_EXISTS` → normal path. Continue to Step 2.

(No bash worktree pre-check here - `lets update` itself runs the robust `DetectInsideWorktree()` check and exits with `ok == false` + a clear `error`; surfacing that via Step 3 is enough, and it avoids the fragile `*"/worktrees/"*` substring heuristic. `init.md` does the same - no bash worktree check.)

## Step 2: Exec

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets update --json --plugin-root="${CLAUDE_PLUGIN_ROOT}"
```

Capture stdout (single JSON object). On network trouble the binary degrades gracefully: `binary`/`plugin` come back `unknown` with an explanatory `detail`; it never fails the run for that. If you're in a git worktree, `lets update` exits with `ok == false` and an `error` explaining `.claude/` isn't shared into worktrees - render that per Step 3 (no LETS box).

## Step 3: Render

Parse JSON.

1. **Artifact table** - one line per `artifacts[]` entry:
   `<name>  v<current_version>  <status>  (latest v<latest_version>)  - <detail>`
   Omit `(latest …)` / `- <detail>` when those fields are empty; print `?` for an empty `current_version` (`dev` prints as-is, not `vdev`).
   Status vocabulary: `.env`/`rules` report `in-sync` - they track a *local* source (the `lets` binary for `.env`, the plugin for `rules`), not the latest release. `binary`/`plugin` report `up-to-date`/`outdated` against the *latest release*. So `.env` and `rules` can sit at different versions and both be `in-sync` - expected, not a contradiction; their `detail` names what they track and flags "itself behind latest v…" when that source is itself stale. `user-rules` (only present with a user-scope install) joins the `in-sync` frame: it tracks the *installed plugin*, same as `rules`. Its `ahead` status means the global file is newer than the plugin (customized or newer release) - deliberately NOT overwritten; relay the `detail` and do not treat it as an error.
2. **Summary line:** `<summary.up_to_date> in sync · <summary.updated> updated · <summary.action_needed> need action · <summary.unknown> unknown` (`summary.up_to_date` is the combined in-sync bucket: `in-sync` + `up-to-date`).
3. **What you need to do** - build ONE numbered list of everything left for the user. Skip the whole section if there's nothing (no `action` strings, no rules update). Order: binary → plugin → restart. Everything here runs from inside Claude Code.
   - For each `artifacts[]` entry with a non-empty `action` (i.e. `binary` / `plugin`), add a step.
     - **`binary`:** present the `curl …` command from the action **prefixed with `! `** so the user can run it right in the Claude Code prompt — `! curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash` — the leading `!` makes Claude Code run it as a shell command in this session, no terminal needed. Add: "(or run the same command **without** the `!` in a terminal.)"
     - **`plugin`:** relay the `action` string verbatim — it's all slash commands in Claude Code (`/plugin marketplace update lets-workflow`, then `/reload-plugins`); no terminal, no `--scope` to figure out.
   - If any `artifacts[]` entry has `status == "updated"` and `name == "rules"` or `name == "user-rules"`, add a final step: "Restart Claude Code so the updated workflow rules take effect — `/exit`, then reopen it in this folder." (The rules file is already re-copied; the restart is only so the *current* session reloads it. One `/exit` + reopen covers project + global rules + the plugin step above.)
4. If `consistent` is `false` → after the list (or, if there's no list, on its own), one line: "⚠️ Versions don't match (binary / plugin / rules) - a partial upgrade. Do the steps above to get everything onto the same release."
5. If `ok == false` → show `error` only; NO LETS box, no table, no other sections.

## Step 4: Output

If `ok == true` and Step 1 said `ENV_EXISTS`:

```
┌─ LETS ─────────────────┐
│  Start?  /lets:start   │
└────────────────────────┘
```

If Step 1 said `ENV_ABSENT`:

```
┌─ LETS ─────────────────┐
│  Init?  /lets:init     │
└────────────────────────┘
```

If `ok == false`: NO LETS box. Plain-text status only.

## Rules

- Respond in user's language (`$LETS_LANGUAGE`)
- Idempotent: re-running is safe; `.env`/rules only change when actually stale
- The binary backs up `.env` to `.lets/.env.bak` automatically when it regenerates the header
- `lets update` cannot replace the running binary or reinstall the plugin - it only tells you how
- On a dev build (`lets version` shows `dev`), `.env`'s `LETS_ENV_VERSION` flips to `dev` and the binary/plugin checks are best-effort - same as `/lets:init` on a dev build
