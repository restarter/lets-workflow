---
description: Sync this project with the current LETS release - a self-driving one-step loop; self-heals .env + project rules (rules deferred while the plugin is behind), reports the hook-maintained global rules, installs the binary and refreshes the plugin in-session on approval, and points at the single next action until everything is on the same version
---

# Update LETS

Sync the drift-able LETS artifacts (four core + two optional: the user-scope global rules and the active `tracker-<name>.md` adapter) with the current release. Bridges to `lets update --json` (Go binary): auto-syncs `.lets/.env` (header refresh when `LETS_ENV_VERSION` is stale), `.claude/rules/lets-rules.md` (re-copy when outdated/missing - but **deferred** when the plugin is behind, so rules never sync to a stale plugin), and reports `~/.claude/rules/lets-rules.md` (the global rules - maintained by the session hook as a cache of the running plugin, only reported here, never written; the `user-rules` row appears only when that file exists). It then computes a single ordered `next_action` (the self-driving loop): the binary and plugin steps can be run in-session (approval-gated), and a fully-synced machine prints `✓ Everything on vX.Y.Z`.

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

(No bash worktree pre-check here - `lets update` detects a worktree itself: the project rows (`.env`, `rules`, `tracker-rules`) come back `skipped` naming the main checkout, the binary / plugin / global-rules rows are checked as usual, and `next_action` names the main checkout.)

## Step 2: Exec

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets update --json --plugin-root="${CLAUDE_PLUGIN_ROOT}"
```

Capture stdout (single JSON object). On network trouble the binary degrades gracefully: `binary`/`plugin` come back `unknown` with an explanatory `detail`; it never fails the run for that. In a git worktree the run succeeds: project rows are `skipped` (`.claude/` isn't shared into worktrees) and `next_action.kind == "main-checkout"` names the main checkout (`main_checkout`).

## Step 3: Render

Parse JSON.

`lets update` is a **self-driving loop**: each run advances ONE step and tells you the single next thing to do. Do that one thing, re-run `/lets:update`, repeat until it prints `✓ Everything on vX.Y.Z`. NEVER suggest a re-run for `reload` (a newer plugin already installed), `new-session` or `user-rules` - a re-run before that step reports the same.

1. **Artifact table** - one line per `artifacts[]` entry:
   `<name>  v<current_version>  <status>  (latest v<latest_version>)  - <detail>`
   Omit `(latest …)` / `- <detail>` when those fields are empty; print `?` for an empty `current_version` (`dev` prints as-is, not `vdev`).
   **When `next_action.kind == "done"`, SKIP the table entirely** - print only the `✓ Everything on v<next_action.version>` line (no status matrix on a fully-synced machine).
   Status vocabulary: `.env`/`rules` report `in-sync` - they track a *local* source (the `lets` binary for `.env`, the plugin for `rules`), not the latest release. `binary`/`plugin` report `up-to-date`/`outdated` against the *latest release*. So `.env` and `rules` can sit at different versions and both be `in-sync` - expected, not a contradiction; their `detail` names what they track and flags "itself behind latest v…" when that source is itself stale. `rules`/`tracker-rules` may report `deferred` - the plugin is behind, so syncing the rules now would write a stale lower version; the row's `detail` explains it and `next_action` steers you to the plugin step. Not an error, not a contradiction. `user-rules` (only present when `~/.claude/rules/lets-rules.md` exists) is informational - the session hook owns that file, update never writes it: `delegated` = verified cache of the installed plugin (healthy); `unknown` = not recorded yet / edited / stale / not verified - relay the `detail`. `skipped` (worktree run) = the project row was not checked here; its `detail` names the main checkout - not up to date, not an error. `rules` may also report `delegated` (`LETS_RULES_SCOPE=user`): the project deliberately has no own copy and lives on the global rules - a healthy state, not an error. `tracker-rules` (present when `LETS_TRACKER` names a shipped adapter, OR a user-authored/unshipped adapter with an installed `.claude/rules/tracker-<name>.md` copy) joins the same frame - for a shipped adapter it tracks the *installed plugin* like `rules` and reports `in-sync` / `deferred` / `updated`, and for a user-authored adapter with no shipped source it reports `delegated` ("left as-is", a healthy green state, not plugin-managed); relay its `detail` switch notes verbatim (a failed stale-adapter removal is a user action item). Relay any `detail` hints verbatim (duplication / missing-global) - they are the user's action items, but NEVER offer to delete files yourself.
2. **Next action** - render `result.next_action` (exactly ONE; do NOT also list per-artifact `action` strings - that reintroduces the multi-step UX this loop replaces). Branch on `next_action.kind`:
   - `done`: print `✓ Everything on v<next_action.version>` and STOP (table already skipped). If `version` is empty (couldn't verify the latest release): print `Nothing to do (couldn't verify the latest release - re-run with network).`
   - `init`: relay `next_action.message` (points at `/lets:init`).
   - `binary`: go to **Step 3.5** (self-driving install).
   - `plugin`: go to **Step 3.7** (gated plugin update).
   - `reload`: relay `next_action.message` verbatim - either the rules just synced, or a newer plugin is already installed and this session still runs the older one. Needs `/reload-plugins` or a new session; no command to run here.
   - `main-checkout`: relay `next_action.message` - it names the main checkout path; `/lets:update` there syncs the project files. No command to run here.
   - `new-session`: one line - the global rules (`~/.claude/rules/lets-rules.md`) refresh at the next Claude Code session start. No command; NEVER "re-run /lets:update".
   - `user-rules`: relay `next_action.message` (the diagnostic) as given. No command; NEVER "re-run /lets:update".
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

Otherwise re-render via Step 3 with the new output (the next action is now a later step). The binary and plugin steps run only through their gates; every other kind is a user action, so the loop naturally hands back.

## Step 3.7: Gated plugin update (when `next_action.kind == "plugin"`)

**MANDATORY GATE:** refreshing the plugin on disk is a system-changing action - it ALWAYS requires the `AskUserQuestion` approval below, even in AUTO MODE. The Bash call MUST be reachable ONLY via "Run it".

Before asking, show the exact command:

```
claude plugin marketplace update lets-workflow && claude plugin update lets@lets-workflow
```

Always print: "Refreshing the plugin on disk does not change this session - /reload-plugins (or a new session) is still needed; this saves one round trip, not the reload."

```
AskUserQuestion(
  questions=[{
    question: "Plugin behind. Refresh it on disk now?",
    header: "Plugin",
    options: [
      { label: "Run it (Recommended)", description: "Updates the plugin on disk; this session keeps its loaded version until /reload-plugins" },
      { label: "I'll do it", description: "Show the /plugin commands only" }
    ],
    multiSelect: false
  }]
)
```

- **Run it** → Bash-run the command above verbatim ONCE, then go to **Step 3.8**. If it fails, relay its output and the `I'll do it` text below; STOP.
- **I'll do it** → relay `next_action.message` (`/plugin marketplace update lets-workflow`, then `/reload-plugins` or a new session - all in Claude Code). STOP.

## Step 3.8: Re-run after the plugin update (bounded)

Re-run the bridge ONCE - it resolves the newly installed plugin, so the project rules sync to it in this same run:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets update --json --plugin-root="${CLAUDE_PLUGIN_ROOT}"
```

Render via Step 3. **At most ONE plugin update per `/lets:update` invocation** - never loop back into Step 3.7; a `plugin` result again means the refresh did not take: report it and STOP. The expected result is `reload` ("already installed; this session still runs …").

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
- The Go binary cannot replace itself or reinstall the plugin. The `/lets:update` orchestrator CAN run the binary installer (Step 3.5) and the on-disk plugin refresh (Step 3.7) in-session - each approval-gated, one attempt per run. Loading the new plugin into the session stays the user's `/reload-plugins` or a new session.
- `~/.claude/rules/lets-rules.md` is the session hook's cache - `/lets:update` only reports it; never write, copy or delete it.
- On a dev build (`lets version` shows `dev`), `/lets:update` reports `.env: dev` and **skips** the `.env` regen (only `/lets:init` restamps `LETS_ENV_VERSION`); the binary/plugin checks stay best-effort
