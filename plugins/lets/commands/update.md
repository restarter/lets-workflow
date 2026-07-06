---
description: Sync this project with the current LETS release - a self-driving one-step loop; self-heals .env + rules (rules deferred while the plugin is behind), installs the binary in-session on approval, and points at the single next action until everything is on the same version
---

# Update LETS

Sync the drift-able LETS artifacts (four core + two optional: the user-scope global rules and the active `tracker-<name>.md` adapter) with the current release. Bridges to `lets update --json` (Go binary): auto-syncs `.lets/.env` (header refresh when `LETS_ENV_VERSION` is stale), `.claude/rules/lets-rules.md` (re-copy when outdated/missing - but **deferred** when the plugin is behind, so rules never sync to a stale plugin), and `~/.claude/rules/lets-rules.md` (user-scope global rules - the `user-rules` row appears only when that file exists). It then computes a single ordered `next_action` (the self-driving loop): the binary step can be run in-session (approval-gated), the plugin step is a user-only Claude Code slash command, and a fully-synced machine prints `✓ Everything on vX.Y.Z`.

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

`lets update` is a **self-driving loop**: each run advances ONE step and tells you the single next thing to do. Do that one thing, re-run `/lets:update`, repeat until it prints `✓ Everything on vX.Y.Z`.

1. **Artifact table** - one line per `artifacts[]` entry:
   `<name>  v<current_version>  <status>  (latest v<latest_version>)  - <detail>`
   Omit `(latest …)` / `- <detail>` when those fields are empty; print `?` for an empty `current_version` (`dev` prints as-is, not `vdev`).
   **When `next_action.kind == "done"`, SKIP the table entirely** - print only the `✓ Everything on v<next_action.version>` line (no status matrix on a fully-synced machine).
   Status vocabulary: `.env`/`rules` report `in-sync` - they track a *local* source (the `lets` binary for `.env`, the plugin for `rules`), not the latest release. `binary`/`plugin` report `up-to-date`/`outdated` against the *latest release*. So `.env` and `rules` can sit at different versions and both be `in-sync` - expected, not a contradiction; their `detail` names what they track and flags "itself behind latest v…" when that source is itself stale. `rules`/`user-rules` may report `deferred` - the plugin is behind, so syncing the rules now would write a stale lower version; the row's `detail` explains it and `next_action` steers you to the plugin step. Not an error, not a contradiction. `user-rules` (only present with a user-scope install) joins the `in-sync` frame: it tracks the *installed plugin*, same as `rules`. Its `ahead` status means the global file is newer than the plugin (customized or newer release) - deliberately NOT overwritten; relay the `detail` and do not treat it as an error. `rules` may also report `delegated` (`LETS_RULES_SCOPE=user`): the project deliberately has no own copy and lives on the global rules - a healthy state, not an error. `tracker-rules` (present only when `LETS_TRACKER` names a shipped adapter) joins the same frame - it tracks the *installed plugin* like `rules`, so it reports `in-sync` / `deferred` / `updated`; relay its `detail` switch notes verbatim (a failed stale-adapter removal is a user action item). Relay any `detail` hints verbatim (duplication / missing-global) - they are the user's action items, but NEVER offer to delete files yourself.
2. **Next action** - render `result.next_action` (exactly ONE; do NOT also list per-artifact `action` strings - that reintroduces the multi-step UX this loop replaces). Branch on `next_action.kind`:
   - `done`: print `✓ Everything on v<next_action.version>` and STOP (table already skipped). If `version` is empty (couldn't verify the latest release): print `Nothing to do (couldn't verify the latest release - re-run with network).`
   - `init`: relay `next_action.message` (points at `/lets:init`).
   - `binary`: go to **Step 3.5** (self-driving install).
   - `plugin`: relay `next_action.message` verbatim - all Claude Code slash commands (`/plugin marketplace update lets-workflow`, then `/reload-plugins`); no terminal.
   - `reload`: relay `next_action.message` - one `/exit` + reopen covers project + global rules.
3. If `consistent` is `false` AND `next_action.kind` is `reload` or `done`: one line "⚠️ Versions don't match (binary / plugin / rules) - a partial upgrade; re-run `/lets:update` to converge." **Suppress this line when `next_action.kind` is `binary` or `plugin`** - the single next action already explains the partial state (a deferred-rules run trips `consistent == false` by design).
4. If `ok == false` → show `error` only; NO LETS box, no table, no other sections.

## Step 3.5: Self-driving binary update (when `next_action.kind == "binary"`)

The binary is outdated. Offer to update it in-session.

**MANDATORY GATE:** running `next_action.command` (`curl … | bash`) is a system-changing external action - it ALWAYS requires the `AskUserQuestion` approval below, even in AUTO MODE (per the AUTO MODE rule: external / system-changing actions are always gated). The Bash call MUST be reachable ONLY via the "Run install.sh" option - no other branch may execute it.

Before asking, show the verbatim command and its provenance so the user can verify the host before approving:
- Command: render `next_action.command` literally (do NOT paraphrase or interpolate).
- Source: official installer at `raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh` - served over HTTPS; it verifies the binary SHA256 before installing.

```
AskUserQuestion(
  questions=[{
    question: "lets binary v{current} < v{latest}. Run the install script now?",
    header: "Binary",
    options: [
      { label: "Run install.sh (Recommended)", description: "Runs the curl install.sh | bash command in this session" },
      { label: "Show command only", description: "Print it; I'll run it myself in a terminal" },
      { label: "Skip", description: "Leave the binary; re-run /lets:update later" }
    ],
    multiSelect: false
  }]
)
```

- **Run install.sh** → Bash-run `next_action.command` verbatim, then go to **Step 3.6**.
- **Show command only** → print `next_action.command` (and note the `! `-prefixed form runs it in the Claude Code prompt). STOP.
- **Skip** → acknowledge, STOP.

## Step 3.6: Re-run after the binary update (bounded)

The binary changed - re-run the bridge ONCE to advance one step:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets update --json --plugin-root="${CLAUDE_PLUGIN_ROOT}"
```

**At most ONE install attempt per `/lets:update` invocation.** If the re-run STILL reports `next_action.kind == "binary"` (binary unchanged - failed install, or PATH shadowing where the shell resolves an older `lets` than the one just installed), do NOT loop back into Step 3.5 / another `curl | bash`. Report: "install ran but the binary is still outdated - likely PATH shadowing (an older `lets` earlier on `$PATH`); check `which -a lets`" and STOP.

Otherwise re-render via Step 3 with the new output (the next action is now `plugin`, `reload`, or `done`). Only the binary step auto-executes; `plugin`/`reload` are user actions, so the loop naturally hands back.

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
- Idempotent self-driving loop: each run advances one step and prints one `next_action`; re-run until `✓ Everything on vX.Y.Z`. Safe to re-run; `.env`/rules only change when actually stale.
- The binary backs up `.env` to `.lets/.env.bak` automatically when it regenerates the header
- The Go binary cannot replace itself or reinstall the plugin. The `/lets:update` orchestrator CAN run the binary installer in-session (Step 3.5, approval-gated, one attempt per run); the plugin step stays a user-only Claude Code slash command.
- On a dev build (`lets version` shows `dev`), `/lets:update` reports `.env: dev` and **skips** the `.env` regen (only `/lets:init` restamps `LETS_ENV_VERSION`); the binary/plugin checks stay best-effort
