# LETS Scripts

Per-project scripts for the LETS plugin.

## Statusline

Branded status bar for Claude Code showing git branch, context window, and API usage stats.

```
🌱 LETS Workflow » feature/my-task
Opus 4.6 » window 38% (76k/200k) · 5h 12% (4h 32m) · 7d 45% (5d 18h)
```

- Color-coded usage: green < 50%, yellow 50-80%, red > 80%
- Reset countdown timers for 5h and 7d windows
- Background cache refresh (5min TTL) - never blocks rendering
- Per-project cache in `.lets/cache/`

### Install

**Via plugin (recommended):**

```bash
/lets:init
```

**Manual:**

```bash
# From any project root (must be a git repo)
bash path/to/lets-workflow/scripts/lets/init.sh
```

The init script:
1. Creates `.lets/` directory structure
2. Copies `statusline.sh` to `.lets/statusline.sh`
3. Adds `.lets/`, `.beads/`, `.worktrees/` to `.gitignore`
4. Configures `.claude/settings.json` with statusLine command
5. Creates `.lets/config.yaml` with defaults
6. Initializes beads (if `bd` is available)

Restart Claude Code after install.

### Requirements

- `jq` - JSON processor (`brew install jq` / `apt install jq`)
- `git` - for branch detection and project root
- Claude Code OAuth token - auto-detected from macOS Keychain or `~/.claude/.credentials.json`

### How it works

Claude Code pipes JSON context (model, context window, workspace) to the statusline command via stdin. The script:

1. Parses model name, git branch, context window from stdin JSON
2. Reads usage cache from `.lets/cache/usage` (4 lines: 5h%, 7d%, 5h_reset, 7d_reset)
3. If cache is stale (> 5min), spawns background fetch via Anthropic OAuth API
4. Renders two formatted lines with ANSI colors

### Files

| File | Location | Purpose |
|------|----------|---------|
| `statusline.sh` | `scripts/lets/` (source) | Source script, copied by install |
| `statusline.sh` | `.lets/statusline.sh` (installed) | Per-project copy, referenced by settings.json |
| `usage` | `.lets/cache/usage` | Cached API usage stats (4 lines) |
| `init.sh` | `scripts/lets/` | Per-project setup script |

### Updating

To update the statusline after a plugin update:

```bash
# Re-run init (safe to run multiple times)
bash path/to/lets-workflow/scripts/lets/init.sh
```

Or copy manually:

```bash
cp path/to/lets-workflow/scripts/lets/statusline.sh .lets/statusline.sh
```
