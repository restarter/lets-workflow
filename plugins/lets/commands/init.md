---
description: Initialize LETS in current project - creates .lets/ structure, config, statusline, beads
---

# Project Initialization

Per-project LETS setup. Bridges to `lets init --json` (Go binary) for all filesystem mutation. Slash command captures user prefs via AskUserQuestion, exec's the binary, parses JSON stdout, renders to user.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Pre-checks

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || { echo "NOT_GIT_REPO"; exit 0; }
command -v lets >/dev/null 2>&1 || { echo "NO_LETS_BINARY"; exit 0; }
test -f "$LETS_PROJECT_ROOT/.lets/.env" && echo "ENV_EXISTS" || echo "ENV_ABSENT"
```

Branch on output:
- `NOT_GIT_REPO` → tell user "Run `git init` first" and stop. NO LETS box.
- `NO_LETS_BINARY` → tell user "lets binary not found. Run `make install` from the lets-workflow repo or check `$PATH`." NO LETS box.
- `ENV_ABSENT` → first-time path (Step 2)
- `ENV_EXISTS` → re-run path (Step 3)

## Step 2: First-time path

### 2a. Detect language

Detect from user's most recent message language. If unclear, ask:

AskUserQuestion(
  questions=[{
    question: "Response language for this project?",
    header: "LETS",
    options: [
      { label: "English", description: "Default" },
      { label: "Ukrainian", description: "Українська" },
      { label: "Italian", description: "Italiano" }
    ],
    multiSelect: false
  }]
)

Bind selected label to `$LANG`. "Other" free-text → use as-is.

### 2b. Detect PR flow

```bash
gh auth status >/dev/null 2>&1 && echo "GH_AUTH" || echo "GH_NONE"
```

AskUserQuestion(
  questions=[{
    question: "PR workflow for this project?",
    header: "LETS",
    options: [
      { label: "GitHub", description: "Recommended if gh authenticated; /lets:done creates PR" },
      { label: "Bitbucket", description: "Bitbucket PR workflow" },
      { label: "Local", description: "Local merge workflow (default)" }
    ],
    multiSelect: false
  }]
)

Bind label (lowercased) to `$FLOW`: "GitHub"→"github", "Bitbucket"→"bitbucket", "Local"→"local".

### 2c. Merge branch

AskUserQuestion(
  questions=[{
    question: "Target branch for merges and PRs?",
    header: "LETS",
    options: [
      { label: "main", description: "Default modern Git convention" },
      { label: "dev", description: "Develop branch as default merge target" },
      { label: "master", description: "Legacy default" }
    ],
    multiSelect: false
  }]
)

Bind to `$BRANCH`. "Other" free-text → use as-is.

### 2d. Exec

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW"
```

Capture stdout (JSON object).

### 2e. Render

Parse JSON. For each `steps[]` entry, render `[<status>] <message>`.

Show summary line: `<ok_count> ok · <skip_count> skip · <migrate_count> migrate · <warn_count> warn · <err_count> err`.

If `drift.detected: true` AND `drift.message != ""`, show `drift.message` directly (canonical wording from binary, no slash command formatting needed).

**Restart hint** — scan `steps[]` for messages containing `statusLine ->`, `_letsManaged marker added`, `.claude/rules/lets-rules.md installed`, or `.claude/rules/lets-rules.md updated`. If ANY match → show hint right before the LETS box:

```
⚠️  Restart Claude Code to apply statusline + rules changes (Cmd+R, or quit and reopen).
```

If no match (everything skipped or only `.env` updated) → no hint, just LETS box.

If `ok: false`, show `error` field; do NOT show LETS box.

If `ok: true`, render LETS box (Step 4).

## Step 3: Re-run path

### 3a. Read current config

Read `.lets/.env` via Read tool. Extract:
- `$CURRENT_LANG` from `LETS_LANGUAGE=...` line
- `$CURRENT_BRANCH` from `LETS_MERGE_BRANCH=...` line
- `$CURRENT_FLOW` from `LETS_PR_FLOW=...` line

Show user a one-line summary: "Current: $CURRENT_LANG / $CURRENT_BRANCH / $CURRENT_FLOW".

### 3b. Confirm intent

AskUserQuestion(
  questions=[{
    question: "Re-run /lets:init - what to do?",
    header: "LETS",
    options: [
      { label: "Refresh", description: "Self-heal (rules drift, statusline, etc.). Keep current config." },
      { label: "Change config", description: "Update language / merge branch / PR flow. Backs up current .env." },
      { label: "Cancel", description: "Stop, no changes." }
    ],
    multiSelect: false
  }]
)

Branch:
- **Refresh** → Step 3c
- **Change config** → Step 3d
- **Cancel** → stop. NO LETS box.

### 3c. Refresh exec

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$CURRENT_LANG" \
  --merge-branch="$CURRENT_BRANCH" \
  --pr-flow="$CURRENT_FLOW"
```

(Binary skips .env step → `env_action.kind=skip`. Other steps self-heal.)

Render per Step 2e.

### 3d. Change config exec

For each of Language / MergeBranch / PRFlow ask AskUserQuestion. First option is "Keep current" with description showing the actual current value:

AskUserQuestion(
  questions=[{
    question: "Response language for this project?",
    header: "LETS",
    options: [
      { label: "Keep current", description: "Currently: $CURRENT_LANG" },
      { label: "English", description: "Default" },
      { label: "Ukrainian", description: "Українська" }
    ],
    multiSelect: false
  }]
)

If "Keep current" picked, substitute `$LANG = $CURRENT_LANG`. Else use selected label.

Repeat for MergeBranch (`$BRANCH`) and PRFlow (`$FLOW`).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json --force-env \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW"
```

Render per Step 2e. JSON's `env_action.changed_keys` shows what changed; `env_action.backup_path` shows `.env.bak` location — surface both to user.

## Step 4: Output

If `ok: true`:

```
┌─ LETS ─────────────────┐
│  Start?  /lets:start   │
└────────────────────────┘
```

If `ok: false` or user picked Cancel: NO LETS box. Plain-text status only.

## Rules

- Respond in user's language ($LETS_LANGUAGE)
- Idempotent: re-running on the same project is safe
- Binary backs up .env automatically when --force-env runs (single .env.bak, overwriting any previous)
