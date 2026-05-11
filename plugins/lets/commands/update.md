---
description: Sync this project with the current LETS release - checks .env, rules, the lets binary, and the plugin
---

# Update LETS

Sync the four drift-able LETS artifacts with the current release. Bridges to `lets update --json` (Go binary): auto-syncs `.lets/.env` (header refresh when `LETS_ENV_VERSION` is stale) and `.claude/rules/lets-rules.md` (re-copy when outdated/missing), and reports actionable version status for the `lets` binary and the Claude Code plugin (which it cannot self-update).

> **MANDATORY:** Execute every Step's bash block **literally as written**. Do not substitute output from earlier `ls`/`cat` in this conversation - `.env` and other dotfiles are invisible to plain `ls`. The `test -f` checks below ARE the contract.

Difference from `/lets:init`: `/lets:init` is first-time setup (it also asks config questions and sets up the statusline + beads). `/lets:update` only syncs what a new release changes - it never prompts and never touches `settings.json` or beads.

## Step 1: Pre-checks

```bash
command -v lets >/dev/null 2>&1 || { echo "NO_LETS_BINARY"; exit 0; }
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "NOT_GIT_REPO"; exit 0; }
test -f "$LETS_PROJECT_ROOT/.lets/.env" && echo "ENV_EXISTS" || echo "ENV_ABSENT"
```

Branch on output:
- `NO_LETS_BINARY` → tell user: "lets binary not found. Run `make install` from the lets-workflow repo or check `$PATH`." NO LETS box. STOP.
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

1. Print the artifact table - one line per `artifacts[]` entry:
   `<name>  v<current_version>  <status>  (latest v<latest_version>)  - <detail>`
   Omit the `(latest …)` and `- detail` parts when those fields are empty; print `?` for an empty `current_version`. (Versions already-tagged like `dev` print as-is, not `vdev`.)
2. If `consistent` is `false` → show: "⚠️ Local install is inconsistent (binary / plugin / rules versions differ) - likely a partial upgrade. Update everything to the same release."
3. If any `artifacts[]` entry has `status == "updated"` and `name == "rules"` → show the **restart hint** right before the LETS box:
   > ⚠️ Restart Claude Code to apply the updated workflow rules - run `/exit`, then reopen Claude Code in this directory.
4. List every non-empty `action` from `artifacts[]` (the things only the user can do - upgrade the binary, update the plugin):
   `- <name>: <action>`
5. Summary line: `<summary.up_to_date> up-to-date · <summary.updated> updated · <summary.action_needed> need action · <summary.unknown> unknown`.
6. If `ok == false` → show `error`; NO LETS box.

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
