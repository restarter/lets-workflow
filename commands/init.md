---
description: Initialize LETS in current project - creates .lets/ structure, config, statusline, beads
---

# Project Initialization

Per-project LETS setup. Creates `.lets/` structure, config, statusline, and initializes beads.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Check Current State

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
ls -d "$LETS_PROJECT_ROOT/.lets" 2>/dev/null && echo "INITIALIZED" || echo "NOT_INITIALIZED"
ls "$LETS_PROJECT_ROOT/.lets/.env" 2>/dev/null && echo "CONFIG_EXISTS" || echo "NO_CONFIG"
```

If already initialized AND config exists:
> "Project already initialized. Re-running will update statusline and skip existing config."

Proceed regardless (idempotent).

## Step 2: Gather Preferences (only if no .env)

If `.lets/.env` does NOT exist, gather settings:

### Language

Detect from user's message language. If unclear, ask:

```
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
```

### GitHub PR Workflow

```bash
gh auth status 2>/dev/null
```

If gh is authenticated, ask:

```
AskUserQuestion(
  questions=[{
    question: "Enable GitHub PR workflow?",
    header: "LETS",
    options: [
      { label: "Yes", description: "/lets:done creates PR instead of local merge" },
      { label: "No", description: "Local merge workflow (default)" }
    ],
    multiSelect: false
  }]
)
```

If gh not available: skip, default to false.

### Merge Branch

Default to `main`. Mention: "Edit `.lets/.env` to change `LETS_MERGE_BRANCH` if needed."

## Step 3: Run Init Script

Build and execute the command:

```bash
bash "${CLAUDE_PLUGIN_ROOT}/scripts/lets/init.sh" \
  --language "$LANGUAGE" \
  --merge-branch "$MERGE_BRANCH" \
  [--github]
```

Show script output to user.

## Step 4: Handle Result

**Exit 0:** "Project initialized successfully. Restart Claude Code to see the statusline."

**Exit 2 (partial):** "Project initialized but beads is not available. Install the beads plugin, then run `/lets:init` again."

**Exit 1 (fatal):** Show error, suggest fixing the issue.

## Rules

- Respond in user's language
- Idempotent: safe to re-run anytime

## Output

```
┌─ LETS ─────────────────────────┐
│  Start?  /lets:start           │
└────────────────────────────────┘
```
