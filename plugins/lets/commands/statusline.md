---
description: Manage & persist statusline appearance - light/dark, compact, and which rows to show
---

# Statusline Appearance

Thin dispatcher for the statusline's persisted appearance. All file work lives in the Go subcommand `lets statusline config` (`cli/internal/statuslinecmd/`); this command captures intent via `AskUserQuestion`, shells out with `--json`, and renders the result.

The choice is written to `.claude/settings.local.json` (personal, gitignored) - NOT the tracked `.claude/settings.json` - so your `--light`/`--compact` preference never gets forced onto collaborators. **A changed statusLine command is re-read by Claude Code only on session start - tell the user to restart.**

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own - the tool invocation is part of the contract. This is critical.

## Step 0: Parse Argument

| Argument | Action |
|----------|--------|
| `show` | go to **Show** (read-only, no prompt) |
| `reset` | go to **Reset** (persist defaults) |
| *(none)* / anything else | go to **Configure** (interactive) |

## Show

```bash
lets statusline config --show --json
```

Parse the envelope and report the current appearance in one line (palette + which rows are hidden + compact), plus the `settings_path`. Then stop (show the box).

## Reset

```bash
lets statusline config --reset --json
```

Parse the envelope; on `ok=true` confirm "reset to defaults". Add the restart note. Show the box.

## Configure

### Step C1: Load current state

```bash
lets statusline config --show --json
```

Parse `appearance` (`light`, `compact`, `no_tip`, `no_dir`, `no_task`) from the envelope. Use it to tell the user what's currently set (the prompts below select the **absolute** desired state, not a delta). If the envelope is `ok=false` with `error.kind == "foreign_statusline"`, the local file has a custom statusLine - surface `error.message` and ask whether to overwrite it (if yes, add `--force` to the apply in Step C3); otherwise stop.

### Step C2: Ask appearance

```
AskUserQuestion(
  questions=[
    {
      question: "Statusline palette? (currently: {current palette})",
      header: "Palette",
      options: [
        { label: "Dark", description: "Dark-terminal colors (default)" },
        { label: "Light", description: "Light-terminal colors (--light)" }
      ],
      multiSelect: false
    },
    {
      question: "Which layout options? (currently: {current layout})",
      header: "Layout",
      options: [
        { label: "Compact", description: "Legacy 2-line statusline instead of the rich box (--compact)" },
        { label: "Hide tip", description: "Drop the rotating bottom tip line (--no-tip)" },
        { label: "Hide location", description: "Drop the location/worktree pill (--no-dir)" },
        { label: "Hide task", description: "Drop the active-task line (--no-task)" }
      ],
      multiSelect: true
    }
  ]
)
```

### Step C3: Apply

Map selections to flags (absolute set):
- Palette **Light** -> `--light` (Dark -> omit).
- **Compact** -> `--compact`; **Hide tip** -> `--no-tip`; **Hide location** -> `--no-dir`; **Hide task** -> `--no-task`.

Build and run (append `--force` only if Step C1 surfaced a foreign command and the user approved overwrite):

```bash
# When at least one flag resolved:
lets statusline config {flags} --json
# When the resolved set is empty (Dark + nothing hidden = defaults):
lets statusline config --reset --json
```

Parse the envelope. On `ok=false` surface `error.message` + `error.remediation`. On `ok=true`:

```
Saved statusline appearance: {command}
  {settings_path}
```

**Always add:** "Restart Claude Code (or start a new session) to apply - the statusLine command is read on session start."

## Output

```
Preview: `echo '{}' | lets statusline`. Restart Claude Code (or start a new session) to apply.
```

## Rules

- NEVER edit `settings.json` / `settings.local.json` directly - always go through `lets statusline config` (it owns the merge, the foreign-guard, and atomic write).
- The flag set is absolute - always pass the full desired state, never a partial delta.
- Respond in the user's language.
