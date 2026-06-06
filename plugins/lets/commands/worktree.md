---
description: Worktree lifecycle management - create, list, remove, info on interactive worktrees
---

# Worktree Management

Thin dispatcher for interactive parallel worktrees. All filesystem/git work lives in the Go subcommand `lets worktree` (`cli/internal/worktreecmd/`); this skill captures user intent via `AskUserQuestion`, shells out with `--json`, and renders the result.

**Interactive worktrees only.** Agent worktrees (`isolation: worktree`) use native Claude Code behavior — not this command.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Determine Subcommand

**If argument provided** (e.g., `/lets:worktree create auth-feature`), parse it:
- `create <name>` -> go to Create. **First strip any `--cmux` / `--no-cmux` token** out of the argument and carry it as the launcher override for Step C3.5; bind the remainder as `<name>` (so `create auth --cmux` => name `auth`, not `auth --cmux`).
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

Create an interactive worktree. The Go subcommand owns the guard, name validation, `.gitignore` ensure, `git worktree add`, symlinks (`.lets/`, `.beads/.env`), verify, and rollback. The skill drives the user choices.

Optional launcher override on the argument: `--cmux` / `--no-cmux` force the launcher for this run (otherwise `$LETS_LAUNCHER` decides — see Step C3.5).

### Step C1: Get Name

If name not provided via argument, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Name for the worktree? (lowercase, no spaces - used for directory and branch)",
    header: "NameMode",
    options: [
      { label: "From task", description: "Auto-generate from current or selected beads task" },
      { label: "Custom", description: "Enter a custom name" }
    ],
    multiSelect: false
  }]
)
```

**From task:** Run `bd ready --limit 5` and let user pick a task or use the current in-progress task. Generate name as `<task-id>-<slugified-title>` (e.g., `lets-hpi.3-worktree-start`).

**Custom:** Use provided text. Slugify: lowercase, spaces to hyphens, remove special chars, max 50 chars (the Go validator allows up to 64; the skill pre-truncates to 50 to leave headroom for `worktree-` prefixes and tmux pane labels). `lets worktree create` will reject invalid names with exit 2.

### Step C2: Create

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cd "$LETS_PROJECT_ROOT"
lets worktree create "$NAME" --json
```

The Go subcommand auto-detects attach vs new-branch: if `refs/heads/<NAME>` exists, attaches to it; otherwise creates `worktree-<NAME>` from `LETS_MERGE_BRANCH`. Pass `--attach` or `--new-branch` to force a mode. Pass `--switch-main-if-needed` to auto-switch main when attaching its current branch (refuses on dirty/mid-rebase tree). Pass `--no-symlink-lets` or `--no-symlink-beads` to skip a specific symlink.

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

1. Explicit override on the command argument: `--cmux` forces cmux, `--no-cmux` forces terminal.
2. Else `$LETS_LAUNCHER` from injected LETS Config (`terminal` default | `cmux`).

**terminal** (default / `--no-cmux`): print the new-terminal command (Step C4 "terminal" block) — unchanged behavior.

**cmux** (`$LETS_LAUNCHER=cmux` or `--cmux`): derive the workspace **slug** from the task title **per the `/rename` slug rule in `/lets:start` Step 7** — that spec is the single source of truth; don't re-paraphrase it here (e.g. **Integrate cmux as parallel-worktree launcher** → `cmux-launcher`). Then:

```bash
lets cmux open "{worktree.path}" --name "{slug}" --command "claude '/lets:start {task-id}'" --json
```

**Taskless worktree** (custom name, no beads task): drop the `/lets:start {task-id}` argument — use `--command "claude"` and derive `{slug}` from the worktree name. Only emit `/lets:start {task-id}` when a task id is actually known.

Parse the `launch` block:
- `launched=true` → "Opened cmux workspace **{workspace_name}**" (Step C4 "cmux" block).
- `launched=false`, `reason=already_open` → a cmux workspace (**{existing_ref} {existing_title}**) already targets this worktree. Don't spawn a duplicate (one live session per worktree) — tell the user to switch to it, or re-run with `--force` to override.
- `launched=false`, other `reason` (cmux not found / not macOS / cmux error) → render `fallback_command` with a one-line note naming `reason` — same as the terminal block but prefixed with the reason.

> **Keep in sync:** the slug rule is sourced from `/lets:start` Step 7 by pointer (not copied — one authoritative definition); the launched/fallback contract mirrors `cmuxcmd.Open` (`cli/internal/cmuxcmd/open.go`). The Go layer never hard-fails — always render whatever `launch` reports.

### Step C4: Output

Use the JSON envelope's `worktree` block (`path`, `branch`, `branch_mode`, `lets_symlinked`, `beads_symlinked`).

**If staying on current branch (terminal launcher):**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} beads={worktree.beads_symlinked}

Open a new terminal for the worktree:

```bash
cd {worktree.path} && claude
```

┌─ LETS ──────────────────────────┐
│  Continue?  /lets:start         │
│  List?      /lets:worktree list │
└─────────────────────────────────┘
```

**If staying on current branch (cmux launcher, `launched=true`):**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} beads={worktree.beads_symlinked}

Opened cmux workspace {launch.workspace_name} → it's running `claude '/lets:start {task-id}'`.

┌─ LETS ──────────────────────────┐
│  List?  /lets:worktree list     │
└─────────────────────────────────┘
```

On `launched=false` (cmux absent / not macOS / cmux error), fall back to the terminal block above, prefixed with a one-line `{launch.reason}` note and the `{launch.fallback_command}`.

Recommended scripted idiom (e.g. tmux composition):

```bash
WT=$(lets worktree create my-feature --print-cd) || exit 1
cd "$WT" && claude
```

**If switching to worktree:**

```
Worktree created: {worktree.path}
Branch: {worktree.branch} ({worktree.branch_mode})
Symlinks: lets={worktree.lets_symlinked} beads={worktree.beads_symlinked}

┌─ LETS ──────────────────────────┐
│  Start?  /lets:start            │
│  Info?   /lets:worktree info    │
└─────────────────────────────────┘
```

---

## List

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets worktree list --json --quiet
```

Parse the JSON envelope's `worktrees[]` and `main` blocks. Each row exposes: `name`, `path`, `branch`, `kind` (`interactive` | `agent` | `other`), `lets_symlinked`, `beads_symlinked`, `changes_clean`, `changes_modified`, `changes_untracked`, optional `locked` / `prunable` / `detached`.

### Output

```
## Worktrees

| Path | Branch | Kind | LETS | Beads | Changes |
|------|--------|------|------|-------|---------|
| {worktrees[i].path} | {worktrees[i].branch} | {worktrees[i].kind} | {symlinked / -} | {symlinked / -} | clean / N modified / M untracked |

{count} worktrees (main: {main.branch})

┌─ LETS ──────────────────────────┐
│  Create?  /lets:worktree create │
│  Remove?  /lets:worktree remove │
└─────────────────────────────────┘
```

`.claude/worktrees/` rows are agent worktrees (native Claude Code) — surface with `kind=agent`. `.worktrees/` rows are interactive (this command) — `kind=interactive`.

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

On either **Force remove**, retry with `--force`. On `error.kind=worktree_not_found`, exit cleanly. Capture `removed.branch` from the success envelope for R3.

### Step R3: Branch Cleanup (optional)

Ask:

```
AskUserQuestion(
  questions=[{
    question: "Delete branch {removed.branch} too?",
    header: "Branch",
    options: [
      { label: "Delete", description: "Branch deletion (-d, refuses if unmerged)" },
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
Beads: {shared / local}
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
- **Location:** `.worktrees/` at project root (NOT `.claude/worktrees/` — that's for agents).
- **`.gitignore` invariants:** `lets worktree create` calls `initcmd.EnsureGitignore` (race-safe via flock + integrity check). Both `.worktrees/` and `.lets` (no slash — matches dir AND symlink) are appended if absent.
- **Branch lifecycle:** worktrees attach to existing branches by default; new branches are prefixed `worktree-<name>`. Refuses to attach the branch currently checked out in main (override with `--switch-main-if-needed` + clean tree).
- **Never force-remove without user approval.** `--force` and `--force-branch` always pass through an AskUserQuestion gate.
- **Each worktree = separate terminal = separate Claude Code session.**
- **Credential threat model:** `.beads/.env` symlink means the same credential is shared across all worktrees. Don't store cross-context secrets there; the file is `chmod 0o600` on disk and main `.beads/` is `chmod 0o700` (hardened by Create).
- Respond in user's language.
