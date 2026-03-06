---
description: First-time setup - install plugins, explain workflow, verify environment
---

# Workspace Setup

One-time setup for new developers or new projects.

## Step 1: Check Environment

```bash
# Claude Code version
claude --version

# Installed plugins
claude plugin list

# GitHub CLI (optional - for GitHub PR workflow)
gh --version 2>/dev/null || echo "gh CLI not found (optional - needed for github: true)"
gh auth status 2>/dev/null || echo "gh not authenticated (optional)"
```

## Step 2: Install Required Plugins

Install these plugins (check Claude Code docs for current installation method):

**Required:**
- **beads** - task tracking across sessions (by steveyegge)
**After installation, restart Claude Code.**

## Step 3: Initialize Project (if needed)

```bash
# Check if beads is initialized
ls -la .beads/ 2>/dev/null || echo "Beads not initialized"

# Initialize beads if missing
bd init

# Set hash length to 5 (default 4 is too short for readability)
bd config set hash_length 5
```

### Create .lets/ directory and gitignore

```bash
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/sessions" "$ROOT/.lets/reviews" "$ROOT/.lets/plans"

# Add .lets/ to .gitignore if not already there
if ! grep -q '^\.lets/' "$ROOT/.gitignore" 2>/dev/null; then
  echo '.lets/' >> "$ROOT/.gitignore"
fi
```

### GitHub Workflow (optional)

If `gh auth status` succeeded in Step 1, ask:

> "Enable GitHub PR workflow? When enabled:
> - `/lets:done` creates PR instead of local merge
> - `/lets:status` shows open PRs
>
> Requires `gh` CLI to stay authenticated."

If user agrees:

```bash
# ROOT = project-root from LETS Config
CONFIG="$ROOT/.lets/config.yaml"
if [ -f "$CONFIG" ]; then
  # Replace existing github line or append if missing
  if grep -q '^github:' "$CONFIG"; then
    sed 's/^github:.*/github: true/' "$CONFIG" > "$CONFIG.tmp" && mv "$CONFIG.tmp" "$CONFIG"
  else
    echo "github: true" >> "$CONFIG"
  fi
else
  cp "${CLAUDE_PLUGIN_ROOT}/hooks/config-template.yaml" "$CONFIG"
  sed 's/^github:.*/github: true/' "$CONFIG" > "$CONFIG.tmp" && mv "$CONFIG.tmp" "$CONFIG"
fi
```

If user declines or gh not available: skip (default `github: false` or omitted = false).

## Step 4: Verify Setup

Run these checks:

```bash
bd ready        # Should show tasks (or empty list)
bd status       # Should show database stats
```

## Step 5: Explain Workflow

Present this to the developer:

### Session Flow

```
/lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done -> /lets:end

Worktree: /lets:worktree create -> terminal -> /lets:start -> Work -> /lets:done -> /lets:end -> /lets:worktree remove

Team:     /lets:brainstorm -> /lets:team run -> monitor -> /lets:review --local -> /lets:done
```

### Essential Skills

| Skill | Category | When to use |
|-------|----------|-------------|
| `/lets:start` | Session | Beginning of session |
| `/lets:end` | Session | End of session |
| `/lets:done` | Task | Task is complete (creates PR if github mode, or merges locally) |
| `/lets:commit` | Code | Ready to commit changes |
| `/lets:check` | Code | Quick sanity check (~30 sec) |
| `/lets:review` | Code | Full deep review (~2-3 min) |
| `/lets:pr` | Code | PR review lifecycle (review, respond, follow-up, approve) |
| `/lets:opinion` | Expert | Technical decision needed |
| `/lets:ask` | Expert | Quick question to one expert |
| `/lets:worktree` | Utility | Create/manage worktrees for parallel sessions |
| `/lets:team` | Utility | Parallel implementation with Agent Teams |
| `/lets:status` | Utility | Task overview anytime |
| `/lets:note` | Utility | Add note to active task |

### Planning Skills (for bigger tasks)

| Skill | When to use |
|-------|-------------|
| `/lets:brainstorm` | Unclear goal, need architecture + plan ("Improve Z", "Not sure how...") |
| `/lets:execute` | Have a plan from /lets:brainstorm, ready to implement step by step |

**Rule of thumb:** Can you write a 1-sentence requirement?
- YES, small task -> work directly
- YES, medium/large -> `/lets:brainstorm` then `/lets:execute`
- NO -> `/lets:brainstorm` first

### Key Rules

1. **Every session has a task** - no random work without tracking
2. **Big tasks need planning** - use `/lets:brainstorm` + `/lets:execute` for 2+ hour tasks
3. **Document everything** - beads is the source of truth
4. **End properly** - `/lets:end` saves context for next session

### Beads Commands

| Command | Purpose |
|---------|---------|
| `bd ready` | Tasks ready to work on |
| `bd show <id>` | Task details |
| `bd update <id> --status=in_progress` | Claim task |
| `bd close <id>` | Complete task |
| `bd create --title="..."` | New task |

## Step 6: Configure Settings

### Disable Auto Compact

Auto compact reduces available context by automatically compressing conversation history. Disabling it gives you more control and more context to work with.

In Claude Code settings, disable auto compact:

```bash
claude config set --global autoCompact false
```

This keeps the full conversation history until you manually compact with `/compact`.

## Checklist

Run through and verify:

- [ ] Claude Code installed and running
- [ ] Marketplace added (`claude plugin marketplace list`)
- [ ] `beads` plugin installed
- [ ] Claude Code restarted after plugin install
- [ ] `.beads/` directory exists
- [ ] `.lets/` directory exists and gitignored
- [ ] `bd ready` works
- [ ] Auto compact disabled
- [ ] (Optional) `gh` CLI installed for GitHub workflow
- [ ] (Optional) `gh auth status` passes
- [ ] (Optional) `github: true` set in `.lets/config.yaml`

**Setup complete when all checked.**

## Rules

- Respond in user's language

## Output

After setup complete:

```
┌─ LETS ─────────────────┐
│  Start?  /lets:start   │
└────────────────────────┘
```
