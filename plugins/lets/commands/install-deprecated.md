---
description: First-time global setup - install plugins, configure environment, explain workflow
---

# Global Setup

One-time setup for new developers. Checks environment, installs plugins, then delegates to `/lets:init` for project setup.

## Step 1: Check Environment

```bash
# Claude Code version
claude --version

# lets CLI binary (REQUIRED - hooks and statusline invoke it directly)
lets version 2>/dev/null || echo "lets CLI not found - install before continuing"

# GitHub CLI (optional)
gh --version 2>/dev/null || echo "gh CLI not found (optional - needed for LETS_PR_FLOW=github)"
gh auth status 2>/dev/null || echo "gh not authenticated (optional)"
```

If `lets version` is missing, the plugin's hooks and statusline will silently fail. Install before continuing:

```bash
# From a clone (current canonical install method)
git clone https://github.com/restarter/lets-workflow
cd lets-workflow
make install
which lets       # verify $PATH
lets version
```

Future installers (tracked under epic `lets-hdrdr`): Homebrew (`lets-odg13`), curl install.sh (`lets-2vb2b`), winget+scoop (`lets-hdrdr.1`). After those ship, this step becomes a one-liner per platform.

## Step 2: Install Required Plugins

Install these plugins (check Claude Code docs for current installation method):

**Required:**
- **beads** - task tracking across sessions (by steveyegge)
**After installation, restart Claude Code.**

## Step 3: Global Settings

### Disable Auto Compact

```bash
claude config set --global autoCompact false
```

## Step 4: Initialize Current Project

Delegate to `/lets:init` for per-project setup (`.lets/` structure, config, statusline, beads).

Run: `/lets:init`

## Step 5: Explain Workflow

### Session Flow

```
/lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done -> /lets:end

Worktree: /lets:worktree create -> terminal -> /lets:start -> Work -> /lets:done -> /lets:end -> /lets:worktree remove

Team:     /lets:plan -> /lets:team run -> monitor -> /lets:review --local -> /lets:done
```

### Essential Skills

| Skill | Category | When to use |
|-------|----------|-------------|
| `/lets:start` | Session | Beginning of session |
| `/lets:end` | Session | End of session |
| `/lets:done` | Task | Task is complete (creates PR if LETS_PR_FLOW=github, or merges locally) |
| `/lets:commit` | Code | Ready to commit (also triggers automatically on "commit") |
| `/lets:check` | Code | Quick sanity check - code (~30s) or plan (--plan) |
| `/lets:review` | Code | Full deep review (~2-3 min) |
| `/lets:github-pr` | Code | GitHub PR review lifecycle (review, respond, follow-up, approve) |
| `/lets:opinion` | Expert | Technical decision needed |
| `/lets:ask` | Expert | Quick question to one expert |
| `/lets:init` | Setup | Initialize LETS in a new project; re-run for self-heal or config change |
| `/lets:update` | Setup | Sync project with the current release - `.lets/.env` + rules self-heal, plus `lets` binary / plugin version status |
| `/lets:worktree` | Utility | Create/manage worktrees for parallel sessions |
| `/lets:team` | Utility | Parallel implementation with Agent Teams |
| `/lets:status` | Utility | Task overview anytime |
| `/lets:note` | Utility | Add note to active task |
| `/lets:brainstorm` | Planning | Interactive ideation - review backlog, explore ideas, quick brainstorm, cleanup |

### Auto-triggered Skills

These fire automatically when you describe the action in conversation - no slash command needed:

| Skill | Triggers on |
|-------|-------------|
| `create-task` | "create task", "new task", "bd create" and variations |
| `commit` | "commit", "закоміть", "git commit" and variations |
| `take-task` | "take task X", "візьми таск", "work on X", "claim task" and variations |

### Planning Skills (for bigger tasks)

| Skill | When to use |
|-------|-------------|
| `/lets:plan` | Task needs architecture + implementation plan |
| `/lets:execute` | Have a plan from /lets:plan, ready to execute via native plan mode |

**Rule of thumb:** Can you write a 1-sentence requirement?
- YES, small task -> work directly
- YES, medium/large -> `/lets:plan` then `/lets:execute`
- NO -> `/lets:brainstorm` to explore and clarify, then `/lets:plan`

### Key Rules

1. **Every session has a task** - no random work without tracking
2. **Big tasks need planning** - use `/lets:plan` + `/lets:execute` for 2+ hour tasks
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

## Checklist

### Global
- [ ] Claude Code installed and running
- [ ] `lets` CLI binary on `$PATH` (`lets version` works)
- [ ] `beads` plugin installed (`bd ready` works)
- [ ] Claude Code restarted after plugin install
- [ ] Auto compact disabled
- [ ] (Optional) `gh` CLI installed
- [ ] (Optional) `gh auth status` passes

### Per-project (via /lets:init)
- [ ] `.lets/` directory exists and gitignored
- [ ] `.lets/.env` configured (with `LETS_ENV_VERSION` as first key)
- [ ] `.claude/settings.json` has `statusLine.command = "lets statusline"` (value-match; no provenance marker)
- [ ] `.claude/rules/lets-rules.md` installed (with frontmatter `version`)
- [ ] `.beads/` initialized
- [ ] `bd ready` works

**Setup complete when all checked.**

## Rules

- Respond in user's language

## Output

```
┌─ LETS ─────────────────┐
│  Start?  /lets:start   │
└────────────────────────┘
```
