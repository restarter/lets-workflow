---
description: Worktree lifecycle management - create, list, remove interactive worktrees
---

# Worktree Management

Create and manage interactive worktrees for parallel work sessions. Each worktree is an isolated working copy where you can run a separate Claude Code session with full LETS workflow.

**This is for interactive parallel sessions.** Agent worktrees (isolation: worktree) use native Claude Code behavior and don't need this command.

## Step 1: Determine Subcommand

**If argument provided** (e.g., `/lets:worktree create auth-feature`), parse it:
- `create <name>` -> go to Create
- `list` -> go to List
- `remove <name>` -> go to Remove
- `info` -> go to Info

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What do you want to do with worktrees?",
    header: "Worktree",
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

Create an interactive worktree with full LETS workflow support.

### Step C1: Guard - Not Already in Worktree

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
```

If `$GIT_DIR` is not `.git` (contains `worktrees/`), you're already in a worktree:
- Show: "Already in a worktree. Open a new terminal in the main repo to create another."
- Stop.

### Step C2: Get Name

If name not provided via argument, ask:

```
AskUserQuestion(
  questions=[{
    question: "Name for the worktree? (lowercase, no spaces - used for directory and branch)",
    header: "Name",
    options: [
      { label: "From task", description: "Auto-generate from current or selected beads task" },
      { label: "Custom", description: "Enter a custom name" }
    ],
    multiSelect: false
  }]
)
```

**From task:** Run `bd ready --limit 5` and let user pick a task or use the current in-progress task. Generate name as `<task-id>-<slugified-title>` (e.g., `lets-hpi.3-worktree-start`).

**Custom:** Use provided text. Slugify: lowercase, spaces to hyphens, remove special chars, max 50 chars.

### Step C3: Verify .gitignore

```bash
ROOT=$(git rev-parse --show-toplevel)
git check-ignore -q "${ROOT}/.worktrees/test" 2>/dev/null
echo $?
```

If exit code is not 0 (not ignored):
- Add `.worktrees/` to `.gitignore`
- Inform user: "Added .worktrees/ to .gitignore"

### Step C4: Create Worktree

```bash
ROOT=$(git rev-parse --show-toplevel)
WORKTREE_PATH="${ROOT}/.worktrees/${NAME}"
BRANCH_NAME="worktree-${NAME}"
```

**With beads (preferred):**

```bash
cd "$ROOT"
bd worktree create "$WORKTREE_PATH" --branch "$BRANCH_NAME"
```

This handles: `git worktree add` + `.beads/redirect` for shared beads database.

**Without beads (fallback):**

```bash
cd "$ROOT"
git worktree add -b "$BRANCH_NAME" "$WORKTREE_PATH"
# If branch already exists, checkout it into the worktree
# git worktree add "$WORKTREE_PATH" "$BRANCH_NAME"
```

### Step C5: Symlink .lets/

```bash
ROOT=$(git rev-parse --show-toplevel)
WORKTREE_PATH="${ROOT}/.worktrees/${NAME}"

if [ -d "${ROOT}/.lets" ] && [ ! -e "${WORKTREE_PATH}/.lets" ]; then
  ln -s "${ROOT}/.lets" "${WORKTREE_PATH}/.lets"
fi
```

This gives the worktree access to: config.yaml, session history, plans, reviews, execution state.

### Step C6: Verify

```bash
ROOT=$(git rev-parse --show-toplevel)
WORKTREE_PATH="${ROOT}/.worktrees/${NAME}"

# Worktree exists
ls -la "$WORKTREE_PATH" | head -5

# .lets/ symlink works
ls -la "${WORKTREE_PATH}/.lets" 2>/dev/null

# .beads/ redirect works (if beads)
cat "${WORKTREE_PATH}/.beads/redirect" 2>/dev/null

# Branch
cd "$WORKTREE_PATH" && git branch --show-current
```

### Step C7: Output

```
Worktree created: .worktrees/{name}/
Branch: worktree-{name}
Beads: {shared via redirect / not available}
LETS: {symlinked / not available}

## Next Steps

Open a new terminal and run:

  cd {absolute-worktree-path}
  claude

Then use /lets:start to pick a task and begin working.

┌─ LETS ─────────────────────────┐
│  List?    /lets:worktree list  │
│  Info?    /lets:worktree info  │
└────────────────────────────────┘
```

---

## List

Show all active worktrees with status.

### Step L1: Get Worktree List

```bash
git worktree list
```

### Step L2: Annotate Each Worktree

For each worktree (skip the main one):

```bash
# Check .beads/redirect (use -f test, NOT cat - cat output breaks chaining)
[ -f "${WORKTREE_PATH}/.beads/redirect" ] && echo "beads: shared" || echo "beads: local/stale"

# Check .lets/ symlink
[ -L "${WORKTREE_PATH}/.lets" ] && echo "lets: symlinked" || echo "lets: not available"

# Check for uncommitted changes
cd "${WORKTREE_PATH}" && git status --short 2>/dev/null | head -5
```

### Step L3: Output

```
## Worktrees

| Path | Branch | Beads | LETS | Changes |
|------|--------|-------|------|---------|
| .worktrees/{name} | worktree-{name} | shared | symlinked | clean / N files |
| .worktrees/{name2} | worktree-{name2} | shared | symlinked | 3 files |
| .claude/worktrees/{x} | worktree-{x} | stale | - | clean |

{N} active worktrees

┌─ LETS ─────────────────────────┐
│  Create?  /lets:worktree create│
│  Remove?  /lets:worktree remove│
└────────────────────────────────┘
```

Note: `.claude/worktrees/` are agent worktrees (native Claude Code). `.worktrees/` are interactive (this command).

---

## Remove

Remove an interactive worktree and clean up.

### Step R1: Identify Worktree

If name not provided via argument:

```bash
# List worktrees in .worktrees/
ls -d .worktrees/*/ 2>/dev/null
```

If multiple exist, ask user which one to remove. If only one, confirm it.

If none exist in `.worktrees/`, check `git worktree list` and show all non-main worktrees.

### Step R2: Safety Check

```bash
ROOT=$(git rev-parse --show-toplevel)
WORKTREE_PATH="${ROOT}/.worktrees/${NAME}"
cd "$WORKTREE_PATH"
git status --short
git log @{upstream}.. --oneline 2>/dev/null
```

If uncommitted changes or unpushed commits exist, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Worktree has uncommitted changes. Remove anyway?",
    header: "Worktree",
    options: [
      { label: "Force remove", description: "Delete worktree and all uncommitted changes" },
      { label: "Cancel", description: "Keep the worktree - commit or stash changes first" }
    ],
    multiSelect: false
  }]
)
```

**Force remove** -> proceed with `--force`
**Cancel** -> stop

### Step R3: Remove

```bash
ROOT=$(git rev-parse --show-toplevel)
WORKTREE_PATH="${ROOT}/.worktrees/${NAME}"

# Remove .lets/ symlink first (points outside worktree)
[ -L "${WORKTREE_PATH}/.lets" ] && rm "${WORKTREE_PATH}/.lets"

cd "$ROOT"

# Remove via bd (handles git worktree remove + beads cleanup)
if command -v bd &>/dev/null && [ -d ".beads" ]; then
  bd worktree remove "$WORKTREE_PATH" --force 2>/dev/null || \
    git worktree remove "$WORKTREE_PATH" --force
else
  git worktree remove "$WORKTREE_PATH" --force
fi

# Prune stale worktree entries
git worktree prune
```

### Step R4: Clean Up Branch (optional)

```bash
BRANCH_NAME="worktree-${NAME}"
# Check if branch has been merged
git branch --merged | grep -q "$BRANCH_NAME"
```

If branch is merged or has no unique commits, offer to delete it:

```
AskUserQuestion(
  questions=[{
    question: "Delete branch worktree-{name} too?",
    header: "Branch",
    options: [
      { label: "Delete", description: "Branch has no unique commits - safe to delete" },
      { label: "Keep", description: "Keep the branch for reference" }
    ],
    multiSelect: false
  }]
)
```

**Delete** -> `git branch -d "$BRANCH_NAME"` (safe delete, fails if unmerged)
**Keep** -> skip

### Step R5: Output

```
Worktree removed: .worktrees/{name}/
Branch: {deleted / kept}

┌─ LETS ─────────────────────────┐
│  List?  /lets:worktree list    │
└────────────────────────────────┘
```

---

## Info

Show worktree status for the current directory.

### Step I1: Detect Worktree State

```bash
GIT_DIR=$(git rev-parse --git-dir 2>/dev/null)
TOPLEVEL=$(git rev-parse --show-toplevel 2>/dev/null)
COMMON_DIR=$(git rev-parse --git-common-dir 2>/dev/null)
MAIN_ROOT=$(cd "$COMMON_DIR/.." 2>/dev/null && pwd)
BRANCH=$(git branch --show-current 2>/dev/null)
```

### Step I2: Check Integrations

```bash
# Beads
if [ -f ".beads/redirect" ]; then
  echo "beads: shared (redirect -> $(cat .beads/redirect))"
elif [ -d ".beads" ]; then
  echo "beads: local (stale snapshot)"
else
  echo "beads: not available"
fi

# LETS
if [ -L ".lets" ]; then
  echo "lets: symlinked -> $(readlink .lets)"
elif [ -d ".lets" ]; then
  echo "lets: local (main repo)"
else
  echo "lets: not available"
fi
```

### Step I3: Output

**If in a worktree:**

```
## Worktree Info

Location: {absolute path}
Main repo: {main root path}
Branch: {branch name}
Beads: {shared via redirect / local (stale) / not available}
LETS: {symlinked / not available}
Changes: {clean / N uncommitted files}

┌─ LETS ─────────────────────────┐
│  List?    /lets:worktree list  │
│  Remove?  /lets:worktree remove│
└────────────────────────────────┘
```

**If in main repo:**

```
## Worktree Info

You are in the main repository (not a worktree).
Path: {toplevel}

┌─ LETS ─────────────────────────┐
│  Create?  /lets:worktree create│
│  List?    /lets:worktree list  │
└────────────────────────────────┘
```

---

## Rules

- **Interactive worktrees only** - agents use native Claude Code worktrees (isolation: worktree)
- **Location:** `.worktrees/` at project root (NOT `.claude/worktrees/` - that's for agents)
- **.gitignore:** verify `.worktrees/` is ignored before creating
- **Never force-remove without user approval**
- **Each worktree = separate terminal = separate Claude Code session**
- `.lets/` symlinked for shared config, sessions, plans, reviews
- `.beads/redirect` via `bd worktree create` for shared task database
- Respond in user's language
