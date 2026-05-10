---
description: Initialize LETS in current project - creates .lets/ structure, config, statusline, beads
---

# Project Initialization

Per-project LETS setup. Bridges to `lets init --json` (Go binary) for all filesystem mutation. Slash command captures user prefs via AskUserQuestion, exec's the binary, parses JSON stdout, renders to user.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

> **MANDATORY:** Execute every Step's bash block **literally as written**. Do not substitute output from earlier `ls`/`cat`/`bd show` in this conversation — `.env` and other dotfiles are invisible to plain `ls`. The `test -f` / `test -d` checks below ARE the contract for branching first-time-vs-re-run paths; shortcutting them produces wrong branches.

## Step 1: Pre-checks

```bash
command -v lets >/dev/null 2>&1 || { echo "NO_LETS_BINARY"; exit 0; }
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || echo "NOT_GIT_REPO"
if [ -n "$LETS_PROJECT_ROOT" ]; then
  test -f "$LETS_PROJECT_ROOT/.lets/.env" && echo "ENV_EXISTS" || echo "ENV_ABSENT"
  test -d "$LETS_PROJECT_ROOT/.beads" && echo "BEADS_EXISTS" || echo "BEADS_ABSENT"
fi
```

Branch on output:
- `NO_LETS_BINARY` → tell user "lets binary not found. Run `make install` from the lets-workflow repo or check `$PATH`." NO LETS box. STOP.
- `NOT_GIT_REPO` → ask user via AskUserQuestion (Step 1a below). If user picks "Init git" → run `git init`, recompute LETS_PROJECT_ROOT, ENV_ABSENT/BEADS_ABSENT (both will be absent for fresh repo). If "Cancel" → stop, NO LETS box.
- `ENV_ABSENT` → first-time path (Step 2). Use `BEADS_ABSENT`/`BEADS_EXISTS` to decide whether to ask about beads init in Step 2c-bis.
- `ENV_EXISTS` → re-run path (Step 3). Don't re-ask about beads — `lets init` self-heals (StepSkip if already inited; StepWarn if bd not on PATH).

### 1a. Git init prompt (only if NOT_GIT_REPO)

AskUserQuestion(
  questions=[{
    question: "This directory is not a git repository. Initialize git here?",
    header: "LETS",
    options: [
      { label: "Init git", description: "Run `git init` here and continue with /lets:init (LETS workflow assumes git)" },
      { label: "Cancel", description: "Stop. Run `git init` manually first or cd to a git repo." }
    ],
    multiSelect: false
  }]
)

If "Init git":
```bash
git init -q
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
```
Then proceed with first-time path (ENV_ABSENT, BEADS_ABSENT both true for fresh repo).

If "Cancel" → STOP. NO LETS box.

### 1b. Plugin install scope check

```bash
PLUGINS_FILE="$HOME/.claude/plugins/installed_plugins.json"
SCOPE=""
if [ -f "$PLUGINS_FILE" ] && command -v python3 >/dev/null 2>&1; then
  SCOPE=$(python3 -c "
import json, sys
try:
    d = json.load(open('$PLUGINS_FILE'))
    entries = d.get('plugins', {}).get('lets@lets-workflow', [])
    print(entries[0].get('scope', '') if entries else '')
except Exception:
    pass
" 2>/dev/null)
fi
echo "SCOPE=${SCOPE:-unknown}"
```

Branch on `$SCOPE`:
- `project` → no notice. Best case.
- `user` → surface a one-time notice (one short line), then continue:
  > ℹ️ Plugin installed at **user scope** (only you). For team adoption, re-install at project scope: `/plugin uninstall lets` → `/plugin install lets` → pick "Install for all collaborators on this repository".
- `local` → no notice. User picked deliberately ("this repo only for me").
- `unknown` (empty / file missing / dev `--plugin-dir` mode) → no notice.

The notice is informational, not a blocker. Continue with Step 2/3 regardless.

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
      { label: "Spanish", description: "Español" },
      { label: "Chinese", description: "中文" }
    ],
    multiSelect: false
  }]
)

Bind selected label to `$LANG`. "Other" free-text (auto-added by tool) → use as-is for any other language (Polski, Deutsch, Français, Русский, 日本語, Português, etc.).

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

### 2c-bis. Beads init prompt (only if BEADS_ABSENT from Step 1)

If `BEADS_ABSENT`:

AskUserQuestion(
  questions=[{
    question: "Initialize beads (cross-session task tracking)?",
    header: "LETS",
    options: [
      { label: "Init beads", description: "Recommended: LETS workflow uses beads for task IDs, dependencies, and persistent memory" },
      { label: "Skip beads", description: "Don't init now. You can run `bd init` later — `lets init` self-heals on next run" }
    ],
    multiSelect: false
  }]
)

If "Init beads" → set `$SKIP_BEADS_FLAG=""`.
If "Skip beads" → set `$SKIP_BEADS_FLAG="--skip-beads"`.

If `BEADS_EXISTS` (re-init scenario where someone removed .lets/ but kept .beads/) → set `$SKIP_BEADS_FLAG=""` (binary will report StepSkip "already initialized" — no harm).

### 2d. Exec

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW" \
  $SKIP_BEADS_FLAG
```

Capture stdout (JSON object).

### 2e. Render

Parse JSON. For each `steps[]` entry, render `[<status>] <message>`.

Show summary line: `<ok_count> ok · <skip_count> skip · <migrate_count> migrate · <warn_count> warn · <err_count> err`.

If `drift.detected: true` AND `drift.message != ""`, show `drift.message` directly (canonical wording from binary, no slash command formatting needed).

**Restart hint** — scan `steps[]` for messages containing `statusLine ->`, `.claude/rules/lets-rules.md installed`, or `.claude/rules/lets-rules.md updated`. If ANY match → show hint right before the LETS box:

(Note: `.lets/.env regenerated` is intentionally NOT in the scan — env regen only changes the file's header/comment and version marker, but the SessionStart hook injects only canonical user-facing keys to model context. The values themselves don't change unless `changed_keys` is non-empty AND a session restart isn't required for that.)

```
⚠️  Restart Claude Code to apply statusline + rules changes — run `/exit`, then reopen Claude Code in this directory.
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
  --plugin-root="${CLAUDE_PLUGIN_ROOT}"
```

No prefs flags passed. The binary reads existing values from `.env` and:
- If file's `LETS_ENV_VERSION` matches running binary AND no values changed → `env_action.kind=skip`
- If version mismatches → `env_action.kind=regenerated` (header refreshed, values preserved)

Other steps (settings.json, rules drift, beads) self-heal as needed.

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
      { label: "Ukrainian", description: "Українська" },
      { label: "Spanish", description: "Español" }
    ],
    multiSelect: false
  }]
)

If "Keep current" picked, substitute `$LANG = $CURRENT_LANG`. Else use selected label. "Other" free-text (auto-added by tool) for Chinese / Polski / Deutsch / etc. — use as-is.

Repeat for MergeBranch (`$BRANCH`) and PRFlow (`$FLOW`).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW"
```

Passing the prefs flags triggers `env_action.kind=regenerated` (binary detects values differ from existing .env, regenerates while preserving foreign keys + user-customized `LETS_TRACKER`).

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
- Binary backs up .env automatically when regenerating (single .env.bak, overwriting any previous)
