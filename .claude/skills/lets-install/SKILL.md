---
name: lets-install
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
```

## Step 2: Add Plugin Marketplaces

```bash
# Add marketplaces (one-time setup)
claude plugin marketplace add steveyegge/beads
claude plugin marketplace add obra/superpowers-marketplace
```

## Step 3: Install Required Plugins

```bash
# beads - task tracking across sessions
claude plugin install beads

# superpowers - development workflows (brainstorming, TDD, debugging)
claude plugin install superpowers

# feature-dev - guided feature development (part of superpowers)
claude plugin install feature-dev
```

**After installation, restart Claude Code.**

## Step 4: Initialize Project (if needed)

```bash
# Check if beads is initialized
ls -la .beads/ 2>/dev/null || echo "Beads not initialized"

# Initialize beads if missing
bd init
```

## Step 5: Verify Setup

Run these checks:

```bash
bd ready        # Should show tasks (or empty list)
bd status       # Should show database stats
```

## Step 6: Explain Workflow

Present this to the developer:

### Session Flow

```
/lets-start → Pick task → Work → /lets-beads-finish → /lets-commit → /lets-end
```

### Essential Skills

| Skill | When to use |
|-------|-------------|
| `/lets-start` | Beginning of session |
| `/lets-end` | End of session |
| `/lets-commit` | Ready to commit changes |
| `/lets-check` | Quick sanity check (~30 sec) |
| `/lets-review` | Full deep review (~2-3 min) |
| `/lets-beads-finish` | Task done, need to document |
| `/lets-beads-status` | Check tasks anytime |
| `/lets-opinion` | Technical decision needed |

### Planning Skills (for bigger tasks)

| Skill | When to use |
|-------|-------------|
| `/feature-dev` | Clear goal, need implementation plan ("Add X to Y") |
| `/brainstorming` | Unclear goal, need to figure out what to build ("Improve Z", "Not sure how...") |

**Rule of thumb:** Can you write a 1-sentence requirement?
- YES → `/feature-dev`
- NO → `/brainstorming` first

### Key Rules

1. **Every session has a task** - no random work without tracking
2. **Big tasks need planning** - use `/feature-dev` for 2+ hour tasks
3. **Document everything** - beads is the source of truth
4. **End properly** - `/lets-end` saves context for next session

### Beads Commands

| Command | Purpose |
|---------|---------|
| `bd ready` | Tasks ready to work on |
| `bd show <id>` | Task details |
| `bd update <id> --status=in_progress` | Claim task |
| `bd close <id>` | Complete task |
| `bd create --title="..."` | New task |

### Workflow Rules

Full workflow is defined in `.claude/rules/lets-workflow.md` - Claude reads this automatically.

## Checklist

Run through and verify:

- [ ] Claude Code installed and running
- [ ] Marketplace added (`claude plugin marketplace list`)
- [ ] `beads` plugin installed
- [ ] `superpowers` plugin installed
- [ ] `feature-dev` plugin installed
- [ ] Claude Code restarted after plugin install
- [ ] `.beads/` directory exists
- [ ] `bd ready` works

**Setup complete when all checked.**

## Output

After setup complete:

```
┌─ LETS ─────────────────┐
│  Start?  /lets-start   │
└────────────────────────┘
```
