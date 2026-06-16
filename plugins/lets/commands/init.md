---
description: Initialize LETS in current project - creates .lets/ structure, config, statusline, beads; offers user-scope global-rules install when the plugin is user-scoped
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
- `NO_LETS_BINARY` → tell user "`lets` binary not found on `$PATH`. Install it — `! curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash` (the leading `!` runs it in this session; or the same command without `!` in a terminal). See the README → Quick Start." NO LETS box. STOP.
- `NOT_GIT_REPO` → ask user via AskUserQuestion (Step 1a below). If user picks "Init git" → run `git init`, recompute LETS_PROJECT_ROOT, ENV_ABSENT/BEADS_ABSENT (both will be absent for fresh repo). If "Cancel" → stop, NO LETS box.
- `ENV_ABSENT` → first-time path (Step 2). Use `BEADS_ABSENT`/`BEADS_EXISTS` to decide whether to ask about beads init in Step 2c-bis.
- `ENV_EXISTS` → re-run path (Step 3). Don't re-ask about beads — `lets init` self-heals (StepSkip if already inited; StepWarn if bd not on PATH).

### 1a. Git init prompt (only if NOT_GIT_REPO)

AskUserQuestion(
  questions=[{
    question: "This directory is not a git repository. Initialize git here?",
    header: "Git init",
    options: [
      { label: "Init git", description: "Run `git init` here, then continue with this init flow (LETS workflow assumes git)" },
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

Branch on `$SCOPE`. When the plugin is found (`project` / `user` / `local`) **lead** with the **auto-update recommendation** - this is the single biggest update-UX win, so present it prominently (not as a footnote):
  > ✅ **Do this once - it removes the manual plugin update forever:** `/plugin` → **Marketplaces** → `lets-workflow` → **Enable auto-update**. The plugin then tracks every LETS release on its own, so ongoing updates collapse to a single `/lets:update` loop (run it, do the one thing it says, re-run until `✓ Everything on vX.Y.Z`) - no `/plugin marketplace update` step to remember. (Claude Code has no setting the `lets` binary can flip - this is a one-time UI action only you can do.)

Additionally:
- `project` → just the auto-update recommendation. Best case.
- `user` → the auto-update recommendation **plus** the global-rules check:

  ```bash
  test -f "$HOME/.claude/rules/lets-rules.md" && echo "GLOBAL_RULES_PRESENT" || echo "GLOBAL_RULES_ABSENT"
  ```

  - `GLOBAL_RULES_PRESENT` → one line: "ℹ️ User-scope install detected — global rules active (`~/.claude/rules/lets-rules.md`); `/lets:update` keeps them current." Continue.
  - `GLOBAL_RULES_ABSENT` → offer the install (AskUserQuestion, header `User scope`):
    - "Install global rules (Recommended)" - description: "Writes ~/.claude/rules + ~/.lets/.env defaults; every project gets LETS rules without per-project init"
    - "Project-only" - description: "Skip global install; this project gets its own rules copy below"

    If installed → run, then render its `steps[]` like Step 2e renders project steps:

    ```bash
    lets init --user --json --plugin-root="${CLAUDE_PLUGIN_ROOT}" --language={LANGUAGE}
    ```

    (`{LANGUAGE}` is an orchestrator placeholder — substitute the English language name BEFORE running. If no language is bound yet (Step 2a hasn't run), ask Step 2a's language question first. NEVER leave a bash variable here: `$LANG` is the POSIX locale env var (`en_US.UTF-8`) and bash would expand it, silently poisoning `~/.lets/.env` in every future session. Add `--launcher={LAUNCHER}` only if the user customized the launcher this session.)
  - One-line note stays for teams: "For team adoption, project scope is still preferable (`/plugin install` → 'all collaborators')."
- `local` → just the auto-update recommendation. User picked the scope deliberately, so no scope-change notice.
- `unknown` (empty / file missing / dev `--plugin-dir` mode) → no notice at all.

These are informational, not blockers. Continue with Step 2/3 regardless.

## Step 2: First-time path

### 2a. Detect language

Detect from user's most recent message language. If unclear, ask:

AskUserQuestion(
  questions=[{
    question: "Response language for this project?",
    header: "Language",
    options: [
      { label: "English", description: "Default" },
      { label: "Ukrainian", description: "Українська" },
      { label: "Spanish", description: "Español" },
      { label: "Chinese", description: "中文" }
    ],
    multiSelect: false
  }]
)

Bind selected label to `$LANG`. "Other" free-text (auto-added by tool) → use the **ENGLISH name** of the language (Polish, German, French, Russian, Japanese, Portuguese, ...). If the user types a native-script name (`Русский`, `Українська`, `日本語`, `中文`, `Deutsch`, ...), **normalise it to the English name** before binding to `$LANG` — every value in `.lets/.env` is in English so the model honours it via the rules' language priority.

### 2b. Detect PR flow

```bash
gh auth status >/dev/null 2>&1 && echo "GH_AUTH" || echo "GH_NONE"
git remote -v 2>/dev/null | head -1 | grep -q . && echo "HAS_REMOTE" || echo "NO_REMOTE"
```

Pick the GitHub option's `description` from the four cases below — branch on (`GH_AUTH` / `GH_NONE`) × (`HAS_REMOTE` / `NO_REMOTE`):

- `GH_AUTH` + `HAS_REMOTE` → `"PR workflow via gh CLI (lets:done will push the branch and open a PR)"` (the safe-default case; per Rule 3 of `## AskUserQuestion Conventions` the `(Recommended)` marker stays in the label, not here. Future-tense "will push" + the `lets:done` mention as a parenthetical noun phrase keeps Rule 7 from auto-firing on PR-flow setup — the user is configuring, not invoking)
- `GH_AUTH` + `NO_REMOTE` → `"⚠ gh authenticated but this repo has no git remote — add one (git remote add origin ...) before /lets:done, or it fails at push"`
- `GH_NONE` + `HAS_REMOTE` → `"Needs gh auth (gh auth login) first"`
- `GH_NONE` + `NO_REMOTE` → `"Needs gh auth (gh auth login) AND a git remote"`

Then:

AskUserQuestion(
  questions=[{
    question: "PR workflow for this project?",
    header: "PR flow",
    options: [
      { label: "GitHub", description: "<conditional, picked above>" },
      { label: "Bitbucket", description: "Bitbucket PR workflow" },
      { label: "Local", description: "Local merge workflow (default)" }
    ],
    multiSelect: false
  }]
)

Bind label (lowercased) to `$FLOW`: "GitHub"→"github", "Bitbucket"→"bitbucket", "Local"→"local".

**If the user picks `GitHub` while `NO_REMOTE`:** accept it (write `LETS_PR_FLOW=github` to `.env`) but explicitly warn after the question — `/lets:done` will fail at the push step until they run `git remote add origin <url>`. Don't try to add the remote here; that's the user's call.

### 2c. Merge branch

AskUserQuestion(
  questions=[{
    question: "Target branch for merges and PRs?",
    header: "Merge branch",
    options: [
      { label: "main", description: "Default modern Git convention" },
      { label: "dev", description: "Develop branch as default merge target" },
      { label: "master", description: "Legacy default" }
    ],
    multiSelect: false
  }]
)

Bind to `$BRANCH`. "Other" free-text → use as-is.

### 2c-ter. Worktree launcher

AskUserQuestion(
  questions=[{
    question: "How should LETS open new worktree sessions?",
    header: "Launcher",
    options: [
      { label: "Terminal (Recommended)", description: "Print a cd command to run in a new terminal. Works everywhere." },
      { label: "cmux", description: "Open in a cmux workspace automatically (macOS + cmux only; falls back to terminal when absent)" }
    ],
    multiSelect: false
  }]
)

Bind label (lowercased, first word) to `$LAUNCHER`: "Terminal"→"terminal", "cmux"→"cmux". `cmux` needs no extra setup — it degrades to the terminal flow on non-macOS or when cmux isn't installed (the `lets cmux` launcher handles the fallback).

### 2c-bis. Beads init prompt (only if BEADS_ABSENT from Step 1)

If `BEADS_ABSENT`:

AskUserQuestion(
  questions=[{
    question: "Initialize beads (cross-session task tracking)?",
    header: "Beads",
    options: [
      { label: "Init beads (Recommended)", description: "Cross-session task IDs, dependencies, and persistent memory" },
      { label: "Skip beads", description: "Don't init now. You can run `bd init` later — `lets init` self-heals on next run" }
    ],
    multiSelect: false
  }]
)

If "Init beads" → set `$SKIP_BEADS_FLAG=""`.
If "Skip beads" → set `$SKIP_BEADS_FLAG="--skip-beads"`.

If `BEADS_EXISTS` (re-init scenario where someone removed .lets/ but kept .beads/) → set `$SKIP_BEADS_FLAG=""` (binary will report StepSkip "already initialized" — no harm).

### 2d. Exec

**Global-rules check (decides `$RULES_SCOPE_FLAG`):**

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
SCOPE=$(grep -E '^LETS_RULES_SCOPE=' "$LETS_PROJECT_ROOT/.lets/.env" 2>/dev/null | cut -d= -f2)
GLOBAL="ABSENT"; test -f "$HOME/.claude/rules/lets-rules.md" && GLOBAL="PRESENT"
PROJECT="ABSENT"; test -f "$LETS_PROJECT_ROOT/.claude/rules/lets-rules.md" && PROJECT="PRESENT"
echo "SCOPE=${SCOPE:-unset} GLOBAL=$GLOBAL PROJECT=$PROJECT"
```

- `SCOPE` already `project`/`user` → no question, `$RULES_SCOPE_FLAG=""` (the persisted value rules; the binary reads it from `.env`).
- `SCOPE=unset` AND `GLOBAL=PRESENT` AND `PROJECT=ABSENT` → ask (AskUserQuestion, header `Rules scope`):
  - "Rely on global (Recommended)" - description: "Persisted choice - your ~/.claude/rules copy covers this project; /lets:update won't re-create a project copy" → set `$RULES_SCOPE_FLAG="--rules-scope=user"`.
  - "Copy to project" - description: "Git-trackable team copy; right call for shared repos" → set `$RULES_SCOPE_FLAG="--rules-scope=project"`.
- Any other combination → no question, `$RULES_SCOPE_FLAG=""` (project copy present syncs as usual; no global rules = nothing to rely on).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW" \
  --launcher="$LAUNCHER" \
  $SKIP_BEADS_FLAG $RULES_SCOPE_FLAG
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
- `$CURRENT_RULES_SCOPE` from `LETS_RULES_SCOPE=...` line (absent → `project`)

Show user a one-line summary: "Current: $CURRENT_LANG / $CURRENT_BRANCH / $CURRENT_FLOW".

### 3b. Confirm intent

AskUserQuestion(
  questions=[{
    question: "Re-run /lets:init - what to do?",
    header: "Re-init",
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

Run the **Global-rules check** from Step 2d first (same bash block, same question rule — prompt fires only when `SCOPE=unset` AND `GLOBAL=PRESENT` AND `PROJECT=ABSENT`; on a typical re-run the scope is persisted or the project copy exists, so no question, no flag).

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  $RULES_SCOPE_FLAG
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
    header: "Language",
    options: [
      { label: "Keep current", description: "Currently: $CURRENT_LANG" },
      { label: "English", description: "Default" },
      { label: "Ukrainian", description: "Українська" },
      { label: "Spanish", description: "Español" }
    ],
    multiSelect: false
  }]
)

If "Keep current" picked, substitute `$LANG = $CURRENT_LANG`. Else use selected label. "Other" free-text (auto-added by tool) → use the **ENGLISH name** of the language (Chinese, Polish, German, Russian, Japanese, ...). If the user types a native-script name (`Русский`, `Українська`, `日本語`, `中文`, `Deutsch`, ...), **normalise it to the English name** before binding — same rule as Step 2a; every value in `.lets/.env` is in English.

Repeat for MergeBranch (`$BRANCH`), PRFlow (`$FLOW`), and Launcher (`$LAUNCHER` — "Keep current" shows `$LETS_LAUNCHER` from LETS Config, plus options terminal / cmux).

**Rules scope** — only ask when `GLOBAL=PRESENT` (otherwise there's nothing to rely on; bind `$RULES_SCOPE_FLAG=""`):

AskUserQuestion(
  questions=[{
    question: "Where should this project's rules come from?",
    header: "Rules scope",
    options: [
      { label: "Keep current", description: "Currently: $CURRENT_RULES_SCOPE" },
      { label: "Rely on global", description: "No project copy; rules from ~/.claude/rules (/lets:update won't re-create one)" },
      { label: "Copy to project", description: "Git-trackable team copy" }
    ],
    multiSelect: false
  }]
)

Bind: "Keep current" → `$RULES_SCOPE_FLAG="--rules-scope=$CURRENT_RULES_SCOPE"`; "Rely on global" → `--rules-scope=user`; "Copy to project" → `--rules-scope=project`.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
lets init --json \
  --plugin-root="${CLAUDE_PLUGIN_ROOT}" \
  --language="$LANG" \
  --merge-branch="$BRANCH" \
  --pr-flow="$FLOW" \
  --launcher="$LAUNCHER" \
  $RULES_SCOPE_FLAG
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
