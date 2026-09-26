---
description: Worktree lifecycle management - create, list, remove, info on interactive worktrees
argument-hint: "[create <name|task-id> [--branch <ref>] [--attach|--new-branch] [--flow plan|plan-workflow] [--auto] [--orca|--cmux|--tmux]|create --team [<callsign>] --area <a> [--orca|--cmux|--tmux]|list|info|remove <name>]"
---

# Worktree Management

Thin dispatcher for interactive parallel worktrees. All filesystem/git work lives in the Go subcommand `lets worktree` (`cli/internal/worktreecmd/`); this skill captures user intent via `AskUserQuestion`, shells out with `--json`, and renders the result.

**Interactive worktrees only.** Agent worktrees (`isolation: worktree`) use native Claude Code behavior — not this command.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Determine Subcommand

**If argument provided** (e.g., `/lets:worktree create auth-feature`), parse it:
- `create --team [<callsign>] --area <a>` -> go to **Create a team** (a standing team's worktree and its lead). Strip `--team`, the optional `<callsign>`, `--area <a>` and the launcher overrides (`--orca` / `--no-orca` / `--cmux` / `--no-cmux` / `--tmux` / `--no-tmux`). **Refused** with one line, then stop: together with `--flow` or `--auto` (a team's lead starts interactively), and from inside a worktree.
- `create <name>` -> go to Create. **First strip any `--orca` / `--no-orca` / `--cmux` / `--no-cmux` / `--tmux` / `--no-tmux` / `--auto` / `--flow <value>` / `--branch <ref>` / `--title-file <path>` token** out of the argument and carry them as overrides (`--orca`/`--no-orca` = the Orca launcher, Step C0; `--cmux`/`--no-cmux`/`--tmux`/`--no-tmux` = launcher; `--auto` = autonomous permission mode; `--flow plan|plan-workflow` = which command the launch lands in — all for Steps C0 / C3.5; `--branch <ref>` decouples the attached/created branch from the dir name — Step C2; `--title-file <path>` = a file holding the task title, passed by `take-task` so the branch name is derived in Go — Step C1). Bind the remainder as `<name>` (so `create auth --flow plan-workflow --auto` => name `auth`, not the flags; `create pwa-46696 --branch feature/pwa-46696` => name `pwa-46696`, branch `feature/pwa-46696`). A `<name>` that is a bare task id (it passes the detect-task id gate and `take-task` sent it) means **From task** in Step C1 with that id.
- `list` -> go to List
- `remove <name>` -> go to Remove
- `info` -> go to Info

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What do you want to do with worktrees?",
    header: "Action",
    options: [
      { label: "Create", description: "Create a new worktree for parallel work" },
      { label: "List", description: "Show all active worktrees" },
      { label: "Remove", description: "Remove a worktree and clean up" },
      { label: "Info", description: "Show current worktree status" }
    ],
    multiSelect: false
  }]
)
```

---

## Create

Create an interactive worktree. The Go subcommand owns the guard, name validation, `.gitignore` ensure, `git worktree add`, symlinks (`.lets/` and the tracker adapter's declared store links), verify, and rollback. The skill drives the user choices.

Optional launcher override on the argument: `--orca` / `--no-orca` / `--cmux` / `--no-cmux` / `--tmux` / `--no-tmux` force the launcher for this run (otherwise `$LETS_LAUNCHER` decides — Orca in Step C0, the others in Step C3.5).

Optional `--auto`: launch the session in `claude --permission-mode auto` (autonomous — auto-approves low-risk work, still gates push / PR / close / external per LETS AUTO MODE rules). Maps ONLY to `--permission-mode auto`, **never** `bypassPermissions`. Applies to the launcher paths in Step C3.5 / C4 (see Step C3.5).

Optional `--flow plan|plan-workflow`: which `/lets:*` command the spawned session lands in. `--flow` ONLY swaps the launch `--command` string — all other steering stays in the tracker task (the launch stays uniform/reproducible). Default (no `--flow`) → `/lets:start <id>` (today's behavior). `--flow plan` → `/lets:plan <id>` (interactive planning in the worktree; the human drives). `--flow plan-workflow` → `/lets:plan-workflow <id>` (autonomous planning). Because only the command string changes, `--flow` is **launcher-agnostic** — the cmux (C3.5) and terminal (C4) paths both inherit it (so does the future tmux launcher). Composes orthogonally with `--auto`. Requires a known task id; on a **taskless** worktree, ignore `--flow` with a one-line note (and `taskless + --flow + --auto` collapses to the existing taskless `--auto` path, `claude --permission-mode auto`). **plan-workflow is PREVIEW** (needs Claude Code ≥ 2.1.154 / paid / Dynamic Workflows) — the launch string can't probe that, so the launched `/lets:plan-workflow` is responsible: if the Workflow tool is unavailable it prints the standard PREVIEW-unavailable message and the operator re-runs `--flow plan`.

**Orchestrator binding.** When this session is a registered orchestrator, every worker it spawns is bound to it. Before building a launch prompt (C0's `--prompt`, C3.5's cmux / tmux `--command`, C4's printed terminal command), run `lets peers orchestrator --session "$CLAUDE_CODE_SESSION_ID" --json`; on `source=self` append ` --orc="<target.name>"` to the slash command of the first line - `/lets:start <id> --orc="<name>"`, and the `--flow` forms `/lets:plan <id> --orc="<name>"` / `/lets:plan-workflow <id> --orc="<name>"` (the spawned command strips it before its task-id test). The name comes from the registry through Go, so it already passed the name grammar; inside the single-quoted launch string it stays double-quoted, e.g. `claude --permission-mode auto '/lets:plan-workflow {task-id} --orc="MAIN PWA"'`. Any other `source`, a taskless worktree, or no binary -> the prompt is unchanged.

### Step C1: Get Name

If name not provided via argument, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Name for the worktree? (lowercase, no spaces - used for directory and branch)",
    header: "NameMode",
    options: [
      { label: "From task", description: "Auto-generate from the current or a selected tracker task" },
      { label: "Custom", description: "Enter a custom name" }
    ],
    multiSelect: false
  }]
)
```

**From task:** Show the tracker's `ready` view (top 5) and let user pick a task or use the current in-progress task (skipped when `take-task` already passed the id). The id goes through the detect-task id gate. Go derives the names, markdown never slugifies by eye: write the task title with the Write tool to `.lets/cache/title-<session6>.txt` (6 = first chars of `$CLAUDE_CODE_SESSION_ID`; reuse a passed `--title-file`), then

```bash
lets worktree branch-name --task '<task-id>' --title-file '<title-file>' --worktree --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
```

Take `branch` (the adapter's `worktree-branch:` shape, default `worktree-<task-id>-<slug>`) as `$BRANCH_REF` and the `dir` field as the dir `<name>` (e.g. `lets-hpi.3-worktree-start`) - never hand-built from the id and `slug`: Go lowers and hash-suffixes an id that is not a valid worktree name (an uppercase id). With the default templates these are exactly today's names. On `ok=false` surface `error.message` and stop.

**Custom:** Use provided text. Slugify: lowercase, spaces to hyphens, remove special chars, max 50 chars (the Go validator allows up to 64; the skill pre-truncates to 50 to leave headroom for `worktree-` prefixes and tmux pane labels). `lets worktree create` will reject invalid names with exit 2.

**Slash branch (git-flow / Bitbucket refs).** The dir NAME must not contain `/` (it's a directory + the validator forbids it). If the user names a branch ref that contains `/` (e.g. `feature/pwa-46696`, `bugfix/x`) — or passed `--branch <ref>` on the argument — **decouple the two**: derive a slash-free dir name (replace `/` with `-`, e.g. `feature/pwa-46696` -> `feature-pwa-46696`, or just the trailing segment `pwa-46696`) and pass the original ref via `--branch` in Step C2. The Go subcommand attaches to (or creates) that ref verbatim while the worktree dir keeps the sanitized name (lets-x5ucf).

### Step C0: Orca launcher (after C1, before C2)

Resolve the launcher: an explicit `--orca` / `--no-orca` override, else `$LETS_LAUNCHER`. Anything but orca -> go to C2 unchanged.

**orca** (`$LETS_LAUNCHER=orca` or `--orca`): Orca creates the worktree itself and opens Claude in its own pane, so this session never moves into it - do NOT ask Step C3; "Switch to worktree" would behave exactly like "Stay on current branch".

- **Name.** Orca turns `/` into `-` in `--name` and has no branch option (spike 1.0), so pass the `/`-free `<task-id>-<slug>` (same `lets worktree branch-name` call as C1, without `--worktree`; use its `slug`). Say in one line: "Orca names the branch after the worktree: `<task-id>-<slug>`; `lets worktree adopt` recognizes that shape". Taskless: the custom `<name>`.
- **Prompt.** `/lets:start <task-id>`; `--flow plan` -> `/lets:plan <task-id>`, `--flow plan-workflow` -> `/lets:plan-workflow <task-id>`. Taskless: no `--prompt`.
- **`--auto`.** Orca launches Claude with the user's configured Orca agent command and accepts no agent arguments (spike 1.0 item 5), so `--permission-mode auto` cannot reach it: print one line `orca_auto_unsupported - Orca cannot pass --permission-mode auto; opening without Orca` and go to C2 (the cmux / terminal paths honor `--auto`).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets orca open --repo "$LETS_PROJECT_ROOT" --name '<task-id>-<slug>' --prompt '/lets:start <task-id>' --json
```

Parse the `launch` block:
- `launched=true` -> Step C4 "orca" block. Done - no C2, C3 or C3.5.
- `reason=already_open` -> an Orca worktree with this name is already open at `{launch.path}`: tell the user to switch to its card in Orca (or re-run with `--force` on `lets orca open`) and stop.
- any other `reason` (`orca_not_found`, `orca_app_not_running`, `orca_capability_missing`, `orca_error`, ...) -> one line naming it, then continue at C2 with the non-orca chain: C3.5 uses **cmux** when `uname -s` is `Darwin` and `command -v cmux` succeeds, else **terminal**.

`--no-orca` (the `fallback_command` Orca returns) skips this step and uses the same cmux-else-terminal chain.

> **Keep in sync:** the launched/fallback contract mirrors `orcacmd.Open` (`cli/internal/orcacmd/open.go`). The Go layer never hard-fails - always render whatever `launch` reports.

### Step C2: Create

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cd "$LETS_PROJECT_ROOT"
lets worktree create "$NAME" --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
# From task (C1) or a slash branch ref decoupled from the dir name (Step C1 "Slash branch"):
#   lets worktree create "$DIR_NAME" --branch "$BRANCH_REF" --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
```

The Go subcommand auto-detects attach vs new-branch: if `refs/heads/<NAME>` exists, attaches to it; otherwise creates `worktree-<NAME>` from `LETS_MERGE_BRANCH`. Pass `--attach` or `--new-branch` to force a mode. **Pass `--branch <ref>`** to attach/create a branch whose ref differs from the dir `<NAME>` — required for git-flow refs containing `/` (e.g. `feature/x`) and for the C1 task branch; the ref is used verbatim (no `worktree-` prefix) while the dir keeps `<NAME>`. Pass `--switch-main-if-needed` to auto-switch main when attaching its current branch (refuses on dirty/mid-rebase tree). Pass `--no-symlink-lets` or `--no-store-links` to skip `.lets` or the declared store links. `--plugin-root` lets Go read the tracker adapter from the plugin when the installed copy predates its `## Worktree` section.

Parse the JSON. On `ok=false`, surface `error.message` and `error.remediation` to the user; if `rollback.residual` is non-empty, list the paths so the user can clean up.

### Step C3: Ask Where to Continue

```
AskUserQuestion(
  questions=[{
    question: "Worktree created. Where do you want to continue?",
    header: "Continue",
    options: [
      { label: "Stay on current branch", description: "Keep working here. Open worktree in a new terminal." },
      { label: "Switch to worktree", description: "Continue in this session inside the worktree" }
    ],
    multiSelect: false
  }]
)
```

**Stay on current branch:** open the worktree elsewhere via the launcher (Step C3.5).
**Switch to worktree:** Stay in worktree dir, then suggest `/lets:start`.

### Step C3.5: Launcher (Stay-on-current-branch only)

Decide how to open the worktree. Resolve in this order:

1. Explicit override on the command argument: `--cmux`/`--tmux` force that launcher, `--no-cmux`/`--no-tmux` force terminal.
2. Else `$LETS_LAUNCHER` from injected LETS Config (`terminal` default | `cmux` | `tmux` | `orca`).
3. `orca` reaches this step only after Step C0 fell back (or `--no-orca`): use `cmux` when `uname -s` is `Darwin` and `command -v cmux` succeeds, else `terminal`.
4. An unrecognized `$LETS_LAUNCHER` value → use `terminal` and print one line naming the bad value (`lets init --launcher` rejects these, but `.lets/.env` is hand-editable).

**terminal** (default / `--no-cmux` / `--no-tmux`): print the new-terminal command (Step C4 "terminal" block) — unchanged behavior.

**cmux** (`$LETS_LAUNCHER=cmux` or `--cmux`): derive the workspace **slug** from the task title **per the `/rename` slug rule in `/lets:start` Step 7** — that spec is the single source of truth; don't re-paraphrase it here (e.g. **Integrate cmux as parallel-worktree launcher** → `cmux-launcher`). Also stamp the workspace **description** with `{task-id} · {task-title}` (the FULL task title — the description/tooltip has no width budget like the `--name` slug does), so each running session self-identifies which task it belongs to. Then:

```bash
lets cmux open "{worktree.path}" --name "{slug}" --description "{task-id} · {task-title}" --command "claude '/lets:start {task-id}'" --json
```

**`--auto`:** when the `--auto` override was passed, the launched `claude` gains `--permission-mode auto` — the `--command` becomes `claude --permission-mode auto '/lets:start {task-id}'`. Maps ONLY to `--permission-mode auto`, never `bypassPermissions`.

**`--flow`:** when a `--flow` override was passed, the `/lets:start {task-id}` inside the `--command` is replaced by the flow's command — `--flow plan` → `'/lets:plan {task-id}'`, `--flow plan-workflow` → `'/lets:plan-workflow {task-id}'` — composing with `--auto` (e.g. `--command "claude --permission-mode auto '/lets:plan-workflow {task-id}'"`). Only the command string changes; the `lets cmux open` flags are otherwise identical. Requires a task id (a taskless worktree has no `{task-id}` to plan — ignore `--flow` and keep `--command "claude"`).

**Taskless worktree** (custom name, no tracker task): drop the `/lets:start {task-id}` argument — use `--command "claude"` and derive `{slug}` from the worktree name; **also drop `--description`** (no task id to stamp — the `--name` slug is identity enough). Only emit `/lets:start {task-id}` and `--description` when a task id is actually known. (`--auto` still applies: `--command "claude --permission-mode auto"`.)

Parse the `launch` block:
- `launched=true` → "Opened cmux workspace **{workspace_name}**" (Step C4 "cmux" block).
- `launched=false`, `reason=already_open` → a cmux workspace (**{existing_ref} {existing_title}**) already targets this worktree. Don't spawn a duplicate (one live session per worktree) — tell the user to switch to it, or re-run with `--force` to override.
- `launched=false`, other `reason` (cmux not found / not macOS / cmux error) → render `fallback_command` with a one-line note naming `reason` — same as the terminal block but prefixed with the reason.

> **Keep in sync:** the slug rule is sourced from `/lets:start` Step 7 by pointer (not copied — one authoritative definition); the description-stamp + launched/fallback contract mirrors `cmuxcmd.Open` (`cli/internal/cmuxcmd/open.go`). The Go layer never hard-fails — always render whatever `launch` reports.

**tmux** (`$LETS_LAUNCHER=tmux` or `--tmux`): derive the **slug** from the task title **per the `/rename` slug rule in `/lets:start` Step 7** (single source of truth). Then:

```bash
lets tmux open "{worktree.path}" --name "{slug}" --description "{task-id} · {task-title}" --command "claude '/lets:start {task-id}'" --json
```

`--auto` and `--flow` compose exactly as in the cmux branch — they only change the `--command` string.

**`--description` is stored, not displayed** (unlike cmux, where it shows in `workspace list --json`). tmux has no tooltip and its window name IS the status line, so the description is stamped into the `@lets_task` window option; the window name stays the short slug. Don't promise the user a visible label for it.

**Taskless worktree:** drop `/lets:start {task-id}` → `--command "claude"`, slug from the worktree name, and drop `--description`.

Parse the `launch` block:
- `launched=true`, `in_existing_session=true` (called from inside tmux) → "Opened tmux window **{target}**" — no attach needed.
- `launched=true`, `in_existing_session=false` → "Created detached tmux session **{workspace_name}** ({target})" + `attach_command`.
- `launched=false`, `reason=already_open` → a tmux pane (**{existing_target} {existing_title}**) already lives at this worktree. Don't spawn a duplicate; tell the user to switch to it, or re-run with `--force`.
- `launched=false`, other `reason` (`tmux_not_found` / `tmux_error`) → render `fallback_command` prefixed with a one-line note naming `reason`.

> **Keep in sync:** the launched/fallback contract mirrors `tmuxcmd.Open` (`cli/internal/tmuxcmd/open.go`); the slug rule is sourced from `/lets:start` Step 7 by pointer.

**`--auto` scope (where it does NOT apply):** `--auto` only affects the **stay-on-current-branch launcher paths** (this Step C3.5 cmux call + the Step C4 terminal block) — the paths that emit a `claude` launch command. If the user picked **"Switch to worktree"** (Step C3 option 2), there is no launch command (the session continues in-place and only suggests `/lets:start`); when `--auto` was passed with that choice, surface one line — "`--auto` applies only when opening the worktree in a separate session — re-launch with `claude --permission-mode auto` if you want autonomous mode here." Do NOT silently drop it.

### Step C4: Output

Use the JSON envelope's `worktree` block (`path`, `branch`, `branch_mode`, `lets_symlinked`, `store_linked`, `store_links[]` - each `{path, linked}`). Render `store_links[]` paths after `store=`; an adapter that declares no links (e.g. `none`) shows `store=-`.

**If staying on current branch (terminal launcher):**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} store={worktree.store_linked} ({store_links[].path})

Open a new terminal for the worktree:

```bash
cd {worktree.path} && claude
```

┌─ LETS ──────────────────────────┐
│  Continue?  /lets:start         │
│  List?      /lets:worktree list │
└─────────────────────────────────┘
```

(When `--auto` was passed, the printed command is `cd {worktree.path} && claude --permission-mode auto` instead — unchanged when `--auto` is absent. When `--flow` was passed, the launched command carries the flow's slash command — e.g. `cd {worktree.path} && claude --permission-mode auto '/lets:plan-workflow {task-id}'` — same string the cmux path uses, since `--flow` only swaps the command.)

**If staying on current branch (cmux launcher, `launched=true`):**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} store={worktree.store_linked} ({store_links[].path})

Opened cmux workspace {launch.workspace_name} → it's running `claude '/lets:start {task-id}'`.

┌─ LETS ──────────────────────────┐
│  List?  /lets:worktree list     │
└─────────────────────────────────┘
```

On `launched=false` (cmux absent / not macOS / cmux error), fall back to the terminal block above, prefixed with a one-line `{launch.reason}` note and the `{launch.fallback_command}`.

**If staying on current branch (tmux launcher, `launched=true`):**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} store={worktree.store_linked} ({store_links[].path})

Opened tmux session {launch.workspace_name} ({launch.target}) → running `claude '/lets:start {task-id}'`.
Attach from a terminal:  {launch.attach_command}
(Launched from inside tmux? It's a new window {launch.target} in your current session — no attach needed.)

┌─ LETS ──────────────────────────┐
│  List?  /lets:worktree list     │
└─────────────────────────────────┘
```

On `launched=false` (tmux absent / tmux error), fall back to the terminal block above, prefixed with a one-line `{launch.reason}` note and `{launch.fallback_command}`.

Recommended scripted idiom (e.g. tmux composition):

```bash
WT=$(lets worktree create my-feature --print-cd) || exit 1
cd "$WT" && claude
```

**Orca launcher (Step C0, `launched=true`):**

```
Orca worktree created: {launch.path}
Branch: {launch.branch}

Orca opened Claude there with `{prompt}`. `orca.yaml` runs `lets worktree adopt` to link `.lets` and the tracker store; if that hook did not run, `/lets:start` in the new pane self-heals.

┌─ LETS ──────────────────────────┐
│  List?  /lets:worktree list     │
└─────────────────────────────────┘
```

**If switching to worktree:**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} store={worktree.store_linked} ({store_links[].path})

┌─ LETS ──────────────────────────┐
│  Start?  /lets:start            │
│  Info?   /lets:worktree info    │
└─────────────────────────────────┘
```

---

## Create a team

A standing team's worktree `team_<c>` on branch `team_<c>` (`<c>` = the callsign), its team file `.lets/teams/<c>.md`, and ONE session: the lead, named `<c>-lead`, which claims the lead through `/lets:start`. Go creates the worktree for EVERY launcher, Orca included; a launcher only opens the lead. Members are spawned later by the lead (`/lets:team spawn`).

### Step T1: Callsign

A `<callsign>` on the argument is used as given. Otherwise:

```bash
lets worktree team-init --suggest-callsign --json
```

Offer its `callsign`; the owner accepts it or types another (a callsign is `[a-z0-9-]`, 1-40 characters, not `run-*`). `--area <a>` is required: missing -> ask in words for one line on what the team owns.

Before anything is created, check the callsign - a given one and a suggested one alike; it writes nothing:

```bash
lets worktree team-init --check --callsign '<c>' --json
```

`team_exists` (27) -> the callsign has a team file: its worktree is on disk -> "Reopen a team" below; otherwise (a disbanded team keeps its file as history) pick another callsign. `callsign_live` (34) or a usage error (2) -> pick another callsign. Only an `ok=true` goes on to T2.

### Step T2: Create the worktree (every launcher)

Fetch as `lets worktree switch` does: `git fetch --no-tags origin {LETS_MERGE_BRANCH}` bounded to 20 s; it fails and `origin/{LETS_MERGE_BRANCH}` exists -> go on with a one-line staleness warning; no `origin/{LETS_MERGE_BRANCH}` -> stop (`no_remote_base`) - the local merge-branch is never a base. Then:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cd "$LETS_PROJECT_ROOT"
lets worktree create "team_<c>" --branch "team_<c>" --new-branch --base "origin/{LETS_MERGE_BRANCH}" --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
```

`ok=false` -> surface `error.message` and stop. Assert, then go on: `worktree.path` ends in `/team_<c>`, and `git -C "<path>" branch --show-current` prints `team_<c>`; anything else -> stop and show both.

### Step T3: Team file

```bash
lets worktree team-init --callsign '<c>' --area '<a>' --worktree "<path>" --plugin-root "${CLAUDE_PLUGIN_ROOT}" --json
```

`team_exists` (27) or `callsign_live` (34) here means another writer took the callsign after the T1 check (a race): say so and stop, and name the cleanup of the worktree T2 created - `/lets:worktree remove team_<c>`. Read `agent_command` and `orca_agent` from the written file's frontmatter (both default `claude`).

### Step T4: Setup hook

The hook point is `.lets/hooks/team-setup` in the main checkout. Absent, or not executable -> one line naming the hook point, and go on to T5.

Present -> show its path and the whole hook when it is 40 lines or fewer; a longer one shows its first 20 lines and "{N} more lines", and the gate adds "Show the rest" - approving lines nobody saw defeats the gate:

```
AskUserQuestion(
  questions=[{
    question: "Run the team setup hook {hook path} in {path}?",
    header: "Setup hook",
    options: [
      { label: "Run it (Recommended)", description: "Runs in the team worktree; its output fills the team file's Workspace" },
      { label: "Show the rest", description: "Print the whole hook, then ask this again" },  /* only when the hook was cut */
      { label: "Skip", description: "Launch the lead without it; Workspace stays unfilled" }
    ],
    multiSelect: false
  }]
)
```

The hook runs only after that yes: cwd = the team worktree, env `TEAM_CALLSIGN=<c>`, `TEAM_WORKTREE=<path>`, `TEAM_FILE=<team file>`. Record what it prints - the docker prefix, the compose project, the port block - in the team file's `## 2. Workspace` section. **A hook that fails stops here, before the lead is launched:** the worktree and the team file stay, its output is shown, and the "Reopen a team" gate below offers to run it again (on the user's yes) before any launch.

### Step T5: Launch the lead

The launcher resolves as in Step C3.5 (an override, else `$LETS_LAUNCHER`). The lead's command is `{agent_command} --name '<c>-lead' '/lets:start'`; the orca path uses `{orca_agent}` in its place. `agent_command` / `orca_agent` come from the shared, writable team file, so they never go inside a double-quoted argument of this session's own Bash call (a `$(...)`, a backtick or a `"` there would expand here): they cross through a quoted heredoc.

- **cmux / tmux** (`<launcher>`):

  ```bash
  CMD=$(cat <<'EOF'
  {agent_command} --name '<c>-lead' '/lets:start'
  EOF
  )
  lets <launcher> open "<path>" --name "<c>-lead" --command "$CMD" --json
  ```

- **terminal:** print, for the human to run in a new terminal: `cd "<path>" && {agent_command} --name '<c>-lead' '/lets:start'`
- **orca:** ONLY the lead's terminal goes through Orca - the worktree is the one T2 created, never an Orca worktree create:

  ```bash
  CMD=$(cat <<'EOF'
  {orca_agent} --name '<c>-lead' '/lets:start'
  EOF
  )
  lets orca terminal --worktree "<path>" --title '<c>-lead' --command "$CMD" --json
  ```

  `terminal.launched=true` -> record `terminal.handle` in the team file's Workspace section (`Orca lead terminal: <handle>`). `launched=false` (Orca absent, not running or refusing) -> print its `fallback_command` for a terminal, prefixed with one line naming `reason` - never an Orca worktree create.

cmux / tmux `launched=false` -> print their `fallback_command` the same way. A second lead from an ambiguous success is harmless: its `/lets:start` gets `lead_held` and stops.

### Reopen a team

`/lets:worktree create --team <c>` for a team whose file exists (T3 `team_exists`, or `.lets/teams/<c>.md` present with its worktree on disk) reopens it instead: read `agent_command` / `orca_agent` from the file, show the T5 command for the resolved launcher verbatim, and run it - through the same quoted heredoc - only after the user's yes:

```
AskUserQuestion(
  questions=[{
    question: "Reopen team <c>: launch its lead with the command above?",
    header: "Reopen",
    options: [
      { label: "Launch the lead (Recommended)", description: "Runs exactly the command shown, in the team worktree" },
      { label: "Run the setup hook first", description: "Run .lets/hooks/team-setup again, then show this gate again" },
      { label: "Cancel", description: "Launch nothing" }
    ],
    multiSelect: false
  }]
)
```

A last T4 run that failed makes "Run the setup hook first" the recommended option; that pick goes through T4's display and gate, so the hook still runs only on its own yes.

> **Keep in sync:** the lead's `--name '<c>-lead'` is the session name `lets members` and `/lets:start` read as the team's lead; `lets orca terminal` mirrors `orcacmd.OpenTerminal` (`cli/internal/orcacmd/terminal.go`) and never creates a worktree.

---

## List

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets worktree list --json --quiet
```

Parse the JSON envelope's `worktrees[]` and `main` blocks. Each row exposes: `name`, `path`, `branch`, `kind` (`interactive` | `agent` | `other`), `lets_symlinked`, `store_linked`, `store_links[]`, `changes_clean`, `changes_modified`, `changes_untracked`, optional `task`, `locked` / `prunable` / `detached`.

### Output

```
## Worktrees

| Path | Branch | Kind | LETS | Store | Changes |
|------|--------|------|------|-------|---------|
| {worktrees[i].path} | {worktrees[i].branch} | {worktrees[i].kind} | {symlinked / -} | {linked / - (none declared)} | clean / N modified / M untracked |

{count} worktrees (main: {main.branch})

┌─ LETS ──────────────────────────┐
│  Create?  /lets:worktree create │
│  Remove?  /lets:worktree remove │
└─────────────────────────────────┘
```

`.claude/worktrees/` rows are agent worktrees (native Claude Code) — surface with `kind=agent`. `.worktrees/` rows are interactive (this command) — `kind=interactive`. A worktree Orca or a teammate created elsewhere is `kind=other`; `lets worktree adopt` inside it links it without moving it.

---

## Remove

Two-call flow because branch cleanup is a separate user decision after worktree removal:

### Step R1: Identify

If name not provided, list candidates with `lets worktree list --json --quiet` (filter `kind=interactive`) and ask user to pick. If a single interactive worktree exists, confirm it.

### Step R2: Remove Worktree

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cd "$LETS_PROJECT_ROOT"
lets worktree remove "$NAME" --json
```

Parse JSON. On `error.kind=dirty_worktree` (exit 14) ask:

```
AskUserQuestion(
  questions=[{
    question: "Worktree has uncommitted changes. Remove anyway?",
    header: "Cleanup",
    options: [
      { label: "Force remove", description: "Delete worktree and discard changes" },
      { label: "Cancel", description: "Keep the worktree - commit or stash first" }
    ],
    multiSelect: false
  }]
)
```

On `error.kind=unpushed_commits` (exit 21) ask separately — these are commits, not just dirty files:

```
AskUserQuestion(
  questions=[{
    question: "{error.message}. Remove anyway?",
    header: "Unpushed",
    options: [
      { label: "Force remove", description: "Discard the unpushed commits along with the worktree" },
      { label: "Cancel", description: "Push the branch first, then retry" }
    ],
    multiSelect: false
  }]
)
```

On either **Force remove**, retry with `--force`. On `error.kind=worktree_not_found`, exit cleanly. On `error.kind=worktree_external`, the worktree lives outside `.worktrees/` (an Orca workspace): tell the user to archive it in Orca (which runs `lets worktree release`) or run `lets worktree release` inside it, and stop. When the success envelope carries `removed.already_gone=true`, say one line - "worktree already gone - finishing the branch step" - and continue. Capture `removed.branch` from the success envelope for R3.

### Step R3: Branch Cleanup (optional)

Ask:

```
AskUserQuestion(
  questions=[{
    question: "Delete branch {removed.branch} too?",
    header: "Branch",
    options: [
      { label: "Delete", description: "Branch deletion (merged into origin/{LETS_MERGE_BRANCH} is detected; squash merges need force)" },
      { label: "Keep", description: "Keep the branch for reference" }
    ],
    multiSelect: false
  }]
)
```

On **Delete**, follow up with `--branch-only` (the worktree is already gone — second `remove` would fail with `worktree_not_found`):

```bash
lets worktree remove "$NAME" --branch-only --branch "$BRANCH" --delete-branch --json
```

If response is `error.kind=branch_unmerged` (exit 15), ask user to confirm force delete; retry with `--force-branch`.

### Step R5: Sweep merged branches (optional)

Offer to clean up other task branches that are already merged. Run `lets worktree sweep --json` (a dry run: it lists `merged` and `unmerged`, deletes nothing). When `merged` is non-empty, list them and ask:

```
AskUserQuestion(
  questions=[{
    question: "{N} task branches are already merged into origin/{LETS_MERGE_BRANCH}. Delete them?",
    header: "Sweep",
    options: [
      { label: "Delete merged", description: "Delete the listed merged branches; unmerged ones stay" },
      { label: "Keep", description: "Leave every branch as it is" }
    ],
    multiSelect: false
  }]
)
```

**Delete merged** -> `lets worktree sweep --apply --json`, then report `deleted`. `unmerged` branches are shown as "unmerged (maybe squashed)" and never deleted here - a squash merge is not detectable, so they need `--force-branch` one by one. With `merged` empty, say nothing.

### Step R4: Output

```
Worktree removed: {removed.path}
Branch: {removed.branch} ({deleted / kept})

┌─ LETS ──────────────────────────┐
│  List?  /lets:worktree list     │
└─────────────────────────────────┘
```

---

## Info

```bash
lets worktree info --json --quiet
```

Parse JSON. `in_worktree=true` means cwd is inside a worktree; `main_root` is the main repo path; `worktree` block has the worktree's data.

### Output

**If in a worktree** (`in_worktree=true`):

```
## Worktree Info

Location: {worktree.path}
Main repo: {main_root}
Branch: {worktree.branch}
LETS: {symlinked / local}
Store: {linked / local / - (none declared)}
Changes: {clean / N modified, M untracked}

┌─ LETS ──────────────────────────┐
│  List?    /lets:worktree list   │
│  Remove?  /lets:worktree remove │
└─────────────────────────────────┘
```

**If in main repo** (`in_worktree=false`):

```
## Worktree Info

You are in the main repository (not a worktree).
Path: {main_root}

┌─ LETS ──────────────────────────┐
│  Create?  /lets:worktree create │
│  List?    /lets:worktree list   │
└─────────────────────────────────┘
```

On `error.kind=not_in_repo` (exit 10), surface "not inside a git repository" plainly — no LETS box.

---

## Migration recipe (from legacy bd-worktree state)

If a worktree was created with `bd worktree create` (pre-lets-rqep4), it has `.beads/redirect` instead of `.beads/.env` symlink. Two-step migration:

```bash
# 1. Remove the legacy worktree (force because random .beads/ files).
lets worktree remove <name> --force

# 2. Recreate with current mechanism.
lets worktree create <name>
```

The new worktree gets the LETS-managed `.lets/` symlink and (if the main has `.beads/.env`) a targeted `.beads/.env` symlink with chmod 0o600 / parent 0o700.

---

## Rules

- **Interactive worktrees only.** Agent worktrees (`isolation: worktree`) use native Claude Code behavior.
- **Location:** `.worktrees/` at project root is where lets creates worktrees (NOT `.claude/worktrees/` — that's for agents). A worktree Orca creates lives in Orca's workspace directory and is adopted there (`orca.yaml` setup hook, or `/lets:start` self-heal), never moved; `remove` refuses it (`worktree_external`) - archive it in Orca, which runs `lets worktree release`.
- **`.gitignore` invariants:** `lets worktree create` calls `initcmd.EnsureGitignore` (race-safe via flock + integrity check). Both `.worktrees/` and `.lets` (no slash — matches dir AND symlink) are appended if absent.
- **Worktree-effective ignores (`info/exclude`, lets-x5ucf):** the tracked `.gitignore` lives on a branch — a fresh worktree checks out its branch's committed copy, which can lack the LETS entry (or carry only a dir-only `/.lets/` that misses the `.lets` symlink), so `.lets` and the declared store links (beads: `.beads/.env`) would show as untracked inside the worktree. `create` and `adopt` therefore also write the narrow entries `.lets`, `.lets.pre-adopt*` and each linked store path to the shared `.git/info/exclude` (common git dir → effective in main + every worktree, untracked, never pushed). Only the actually-linked paths are added, and the patterns are narrow so other untracked store content (e.g. the rest of `.beads/`) still surfaces in `git status`.
- **Branch lifecycle:** worktrees attach to existing branches by default; new branches are prefixed `worktree-<name>`. Refuses to attach the branch currently checked out in main (override with `--switch-main-if-needed` + clean tree). **`--branch <ref>` decouples the branch from the dir name** — the dir keeps the slash-free `<name>` while the ref (which may contain `/`, e.g. `feature/x`) is attached/created verbatim with no `worktree-` prefix (lets-x5ucf).
- **Never force-remove without user approval.** `--force` and `--force-branch` always pass through an AskUserQuestion gate.
- **Each worktree = separate terminal = separate Claude Code session.**
- **Credential threat model:** the tracker adapter's declared store links (beads: `.beads/.env`) share the same credential across all worktrees. Don't store cross-context secrets there; each link target is `chmod 0o600` and missing parents are created `0o700` (hardened by Create and adopt).
- Respond in user's language.
