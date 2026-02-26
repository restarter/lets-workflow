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
```

## Step 2: Install Required Plugins

Install these plugins (check Claude Code docs for current installation method):

**Required:**
- **beads** - task tracking across sessions (by steveyegge)
- **superpowers** - development workflows: brainstorming, TDD, debugging (by obra)

**Recommended:**
- **feature-dev** - guided feature development with codebase understanding

**After installation, restart Claude Code.**

## Step 3: Initialize Project (if needed)

```bash
# Check if beads is initialized
ls -la .beads/ 2>/dev/null || echo "Beads not initialized"

# Initialize beads if missing
bd init
```

### Create .lets/ directory and gitignore

```bash
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$ROOT/.lets/sessions" "$ROOT/.lets/reviews" "$ROOT/.lets/plans"

# Add .lets/ to .gitignore if not already there
if ! grep -q '^\.lets/' "$ROOT/.gitignore" 2>/dev/null; then
  echo '.lets/' >> "$ROOT/.gitignore"
fi
```

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
```

### Essential Skills

| Skill | Category | When to use |
|-------|----------|-------------|
| `/lets:start` | Session | Beginning of session |
| `/lets:end` | Session | End of session |
| `/lets:done` | Task | Task is complete (creates PR or merges) |
| `/lets:commit` | Code | Ready to commit changes |
| `/lets:check` | Code | Quick sanity check (~30 sec) |
| `/lets:review` | Code | Full deep review (~2-3 min) |
| `/lets:pr` | Code | PR review lifecycle (review, respond, follow-up, approve) |
| `/lets:opinion` | Expert | Technical decision needed |
| `/lets:ask` | Expert | Quick question to one expert |
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
- [ ] `superpowers` plugin installed
- [ ] `feature-dev` plugin installed
- [ ] Claude Code restarted after plugin install
- [ ] `.beads/` directory exists
- [ ] `.lets/` directory exists and gitignored
- [ ] `bd ready` works
- [ ] Auto compact disabled

**Setup complete when all checked.**

## Output

After setup complete:

```
┌─ LETS ─────────────────┐
│  Start?  /lets:start   │
└────────────────────────┘
```
