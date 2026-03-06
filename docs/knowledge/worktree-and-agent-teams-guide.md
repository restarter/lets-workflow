# Worktree & Agent Teams - Complete Research Guide

Research conducted 2026-03-01 for task `lets-dnk.4` (Add worktree support to task workflow).

## Table of Contents

1. [Three Worktree Mechanisms in Claude Code](#1-three-worktree-mechanisms-in-claude-code)
2. [Agent Teams](#2-agent-teams)
3. [Beads Worktree Support (bd worktree)](#3-beads-worktree-support-bd-worktree)
4. [.lets/ State Sharing Problem](#4-lets-state-sharing-problem)
5. [Superpowers Reference Patterns](#5-superpowers-reference-patterns)
6. [Integration Points with LETS Plugin](#6-integration-points-with-lets-plugin)
7. [bd swarm - DAG-based Task Waves](#7-bd-swarm---dag-based-task-waves)
8. [Security Considerations](#8-security-considerations)
9. [Practical Combinations](#9-practical-combinations)
10. [Known Limitations & Gotchas](#10-known-limitations--gotchas)

---

## 1. Three Worktree Mechanisms in Claude Code

### 1.1 CLI Flag: `claude --worktree <name>`

**Source:** Official docs (code.claude.com/docs/en/common-workflows)

Start a Claude Code session in an isolated git worktree:

```bash
# Named worktree
claude --worktree feature-auth
# Creates: <repo>/.claude/worktrees/feature-auth/
# Branch: worktree-feature-auth

# Auto-named
claude --worktree
# Creates: <repo>/.claude/worktrees/bright-running-fox/ (random name)

# Short flag
claude -w bugfix-123
```

**Path:** `<repo>/.claude/worktrees/<name>/`
**Branch naming:** `worktree-<name>` (branched from default remote branch)

**Cleanup on session exit:**
- No changes made -> worktree + branch auto-removed
- Changes/commits exist -> prompt: keep or remove
- Keeping preserves directory + branch for later return
- Removing deletes worktree directory + branch + all uncommitted changes

**Important:** Add `.claude/worktrees/` to `.gitignore`.

**Can also be triggered mid-session:** Ask Claude to "work in a worktree" or "start a worktree" and it uses the `EnterWorktree` tool.

### 1.2 Agent Frontmatter: `isolation: worktree`

**Source:** Claude Code changelog v2.1.49-2.1.50, official docs (code.claude.com/docs/en/sub-agents)

Static declaration in agent `.md` file:

```yaml
---
name: implementer
description: Implements code changes in isolation
isolation: worktree
tools: Read, Edit, Write, Bash, Grep, Glob
model: sonnet
---

System prompt here...
```

When this agent is dispatched via `Agent()` tool, Claude Code automatically:
1. Creates a temporary git worktree
2. Fires `WorktreeCreate` hook (if configured)
3. Runs the agent in the worktree
4. On completion: fires `WorktreeRemove` hook
5. If no changes were made -> auto-cleanup
6. If changes exist -> worktree path + branch returned in result

**Key difference from `--worktree`:** This is automatic and tied to the agent lifecycle. The worktree is temporary and scoped to one agent invocation.

**Hook events for customization:**

```json
// hooks.json
{
  "WorktreeCreate": [{
    "hooks": [{
      "type": "command",
      "command": "./scripts/setup-worktree.sh"
    }]
  }],
  "WorktreeRemove": [{
    "hooks": [{
      "type": "command",
      "command": "./scripts/cleanup-worktree.sh"
    }]
  }]
}
```

These hooks enable custom setup (install deps, create symlinks, run `bd worktree create`) and teardown (cleanup, `bd worktree remove`).

### 1.3 Manual: `git worktree add` / `bd worktree create`

Standard git worktrees with full control over location and branch:

```bash
# Raw git
git worktree add ../project-feature-a -b feature-a
cd ../project-feature-a && claude

# With beads (preferred in beads projects)
bd worktree create .worktrees/feature-auth --branch feature/auth
```

**When to use:** When you need specific branch names, specific locations, or integration with beads task tracking.

### 1.4 Comparison Table

| Feature | `--worktree` flag | `isolation: worktree` | Manual |
|---------|-------------------|----------------------|--------|
| Created by | User (CLI) | Automatic (agent dispatch) | User (shell) |
| Location | `.claude/worktrees/<name>/` | `.claude/worktrees/<name>/` (native) | Anywhere |
| Branch name | `worktree-<name>` | `worktree-agent-<id>` (auto) | Any |
| Lifecycle | Session-scoped | Agent-scoped | Manual |
| Cleanup | Prompt on exit | Auto if no changes | Manual |
| Beads support | Needs manual `bd worktree` | None (agents use stale JSONL copy) | Via `bd worktree create` |
| `.lets/` support | Needs symlink for LETS commands | Not needed (agents don't run LETS commands) | Not available (gitignored) |
| Hooks | WorktreeCreate/Remove (optional) | None needed (native works) | N/A |
| Best for | Interactive parallel sessions | Automated parallel agents | Full control |

---

## 2. Agent Teams

**Source:** Official docs (code.claude.com/docs/en/agent-teams)

**Status:** Available out of the box (as of 2026-03). No env var needed.

### 2.1 Enable

No configuration needed - Agent Teams are available by default.

Previously required (no longer needed):
```json
// settings.json - NOT REQUIRED ANYMORE
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  }
}
```

### 2.2 Architecture

```
Lead Session (your Claude Code)
  ├── TeamCreate -> creates shared TaskList + Mailbox
  ├── Agent(team_name: "my-team", name: "frontend") -> Teammate 1
  ├── Agent(team_name: "my-team", name: "backend")  -> Teammate 2
  └── Agent(team_name: "my-team", name: "tester")   -> Teammate 3
```

| Component | Role |
|-----------|------|
| **Team lead** | Main session. Creates team, spawns teammates, coordinates |
| **Teammates** | Separate Claude Code instances. Own context window each |
| **Task list** | Shared work items. Teammates claim and complete |
| **Mailbox** | Messaging system for inter-agent communication |

**Storage:**
- Team config: `~/.claude/teams/{team-name}/config.json`
- Task list: `~/.claude/tasks/{team-name}/`

Config contains `members` array with each teammate's `name`, `agentId`, `agentType`.

### 2.3 Communication

**SendMessage types:**
- `message` - DM to specific teammate (by name)
- `broadcast` - to ALL teammates (expensive, use sparingly)
- `shutdown_request` - ask teammate to shut down gracefully
- `shutdown_response` - teammate approves/rejects shutdown
- `plan_approval_response` - approve/reject teammate's plan

**Automatic delivery:** Messages arrive automatically. No polling needed. Lead gets notified when teammates go idle.

**Teammate discovery:** Read `~/.claude/teams/{team-name}/config.json` to find other teammates by name.

### 2.4 Task Management

Shared task list with file locking for race-condition prevention:

```
TaskCreate -> creates task (pending)
TaskUpdate -> claim (owner + in_progress), complete, add dependencies
TaskList   -> see all tasks and status
TaskGet    -> full task details
```

Task states: `pending` -> `in_progress` -> `completed`
Dependencies: `addBlockedBy` / `addBlocks` - blocked tasks can't be claimed until deps resolve.

### 2.5 Display Modes

| Mode | How | Requirement |
|------|-----|-------------|
| **In-process** (default) | All in main terminal. Shift+Down to cycle. Ctrl+T for task list | Any terminal |
| **Split panes** | Each teammate gets own pane | tmux or iTerm2 |

Configure:
```json
{ "teammateMode": "in-process" }  // or "tmux" or "auto"
```

Or per-session: `claude --teammate-mode in-process`

### 2.6 Plan Approval for Teammates

Force teammate to plan before implementing:

```
"Spawn an architect teammate to refactor auth module.
Require plan approval before they make any changes."
```

Flow:
1. Teammate works in read-only plan mode
2. Sends `plan_approval_request` to lead
3. Lead approves -> teammate exits plan mode, begins implementation
4. Lead rejects with feedback -> teammate revises plan

### 2.7 Quality Gate Hooks

```json
// settings.json
{
  "hooks": {
    "TeammateIdle": [{
      "hooks": [{ "type": "command", "command": "./scripts/check-idle.sh" }]
    }],
    "TaskCompleted": [{
      "hooks": [{ "type": "command", "command": "./scripts/verify-task.sh" }]
    }]
  }
}
```

- `TeammateIdle`: fires when teammate about to go idle. Exit code 2 = send feedback, keep working
- `TaskCompleted`: fires when task marked complete. Exit code 2 = prevent completion, send feedback

### 2.8 Best Practices (from official docs)

- **3-5 teammates** for most workflows
- **5-6 tasks per teammate** keeps everyone productive
- **Avoid file conflicts** - each teammate owns different files
- **Give enough context** in spawn prompt (teammates don't inherit lead's history)
- **Start with research/review** tasks if new to teams
- **Monitor and steer** - check in on progress, redirect if needed

### 2.9 Agent Teams vs Subagents

| | Subagents | Agent Teams |
|---|---|---|
| Context | Own window, results return to caller | Own window, fully independent |
| Communication | Report back to main only | Teammates message each other |
| Coordination | Main agent manages all | Shared task list, self-coordination |
| Best for | Focused tasks, result matters | Complex work, discussion needed |
| Token cost | Lower (results summarized) | Higher (each is separate instance) |

**Use subagents** when: quick focused workers that report back.
**Use agent teams** when: teammates need to share findings, challenge each other, coordinate.

### 2.10 Limitations

- No session resumption for in-process teammates (`/resume` doesn't restore them)
- Task status can lag (teammates may forget to mark complete)
- Shutdown can be slow (teammates finish current request first)
- One team per session
- No nested teams (teammates can't spawn own teams)
- Lead is fixed (can't transfer leadership)
- All teammates start with lead's permission mode
- Split panes need tmux/iTerm2 (not VS Code terminal, not Ghostty)

---

## 3. Beads Worktree Support (bd worktree)

**Source:** `reference/beads/cmd/bd/worktree_cmd.go`, `reference/beads/docs/WORKTREES.md`

### 3.1 Commands

```bash
bd worktree create <path> [--branch <name>]
bd worktree list
bd worktree remove <path> [--force]
bd worktree info [<path>]
```

### 3.2 How `bd worktree create` Works

Step by step (from `worktree_cmd.go:142-251`):

1. Resolves `<path>` to absolute path
2. Validates `.beads/` exists in main repo
3. Determines branch: `--branch` flag or `filepath.Base(path)`
4. Runs `git worktree add -b <branch> <path>`
   - If branch exists, retries without `-b`: `git worktree add <path> <branch>`
5. Creates `<path>/.beads/` directory
6. Writes redirect file: `<path>/.beads/redirect`
   - Content: relative path from worktree root to main `.beads/`
   - Example: `../../.beads` (for `.worktrees/foo/` -> `../../.beads`)
7. If worktree is inside repo root -> adds `<path>/` to root `.gitignore` under `# bd worktree` comment

### 3.3 The Redirect Mechanism

**File:** `.beads/redirect` (gitignored, per-machine)

**Content:** Single line with relative or absolute path to main `.beads/` directory.

**Resolution (from `beads.go:38-92`, `FollowRedirect()`):**
- Relative paths resolved from **parent of `.beads/`** (worktree root), NOT from `.beads/` itself
- Only one level of redirect followed - chains blocked with warning
- If target doesn't exist -> fallback to original path with warning
- `BD_DEBUG_ROUTING=1` env var enables verbose routing output

**Example:**
```
# Worktree at <repo>/.worktrees/feature-auth/
# Redirect file: <repo>/.worktrees/feature-auth/.beads/redirect
# Content:
../../.beads
# Resolves: .worktrees/feature-auth/ + ../../.beads = <repo>/.beads
```

**Critical bug fixes:**
- GH#1266: redirect must be relative to worktree root, not `.beads/` dir
- GH#1098: main `.beads/` path must be absolute before computing `filepath.Rel()`

### 3.4 How `bd worktree remove` Works

1. Resolves worktree path (absolute, relative to cwd, relative to repo root, or from `git worktree list`)
2. Safety checks (skippable with `--force`):
   - `git status --porcelain` - fails if uncommitted changes
   - `git log @{upstream}.. --oneline` - fails if unpushed commits
   - Stashes NOT checked (git stashes are global)
3. `git worktree remove <path>` (or `--force` variant)
4. Removes `# bd worktree` + `<path>/` from root `.gitignore`
5. `.beads/redirect` file deleted as part of worktree removal

### 3.5 Beads State in Worktrees

| State | Meaning |
|-------|---------|
| `shared` | Main repo, has the real `.beads/` database |
| `redirect` | Worktree with redirect file pointing to main |
| `local` | Has `.beads/` but it's not the main (stale git-tracked copy) |
| `none` | No `.beads/` at all |

**Without `bd worktree create`:** `.beads/` is git-tracked, so worktrees get a stale JSONL copy. `bd` commands work but use stale data. The Dolt database is NOT shared.

**With `bd worktree create`:** Redirect file points to shared main `.beads/`. All `bd` commands use the live database.

### 3.6 Concurrent Access

- Single-writer: embedded mode is safe
- Multi-writer (multiple worktrees): needs Dolt server mode (`bd dolt start`)
- Multiple worktrees share one `.beads/` database via redirect

---

## 4. `.lets/` State Sharing Problem

### 4.1 The Problem

`.lets/` is in `.gitignore`. When a worktree is created:
- `.lets/` directory does NOT exist in the worktree
- `ROOT=$(git rev-parse --show-toplevel)` returns worktree path, not main repo
- All commands that do `$ROOT/.lets/...` look in worktree's non-existent `.lets/`

### 4.2 What Breaks

| Command | What breaks | Impact |
|---------|------------|--------|
| `start.md` | Reads recent session files via `ls -t` glob | No previous session context (resolved: dated files + branch slug) |
| `start.md` | Writes `.session-start-ref` | Written to worktree (isolated) |
| `end.md` | Reads `.session-start-ref`, writes summaries | Isolated from main session history |
| `brainstorm.md` | Saves plans to `$ROOT/.lets/plans/` | Plans invisible from main repo |
| `execute.md` | Reads/writes execution state | State not shared |
| `review.md` | Saves reports to `$ROOT/.lets/reviews/` | Reports isolated |
| `pr.md` | PR state in `$ROOT/.lets/execution/pr-N/` | PR state split |
| `statusline` | Creates `$ROOT/.lets/cache/` | Separate cache per worktree |

### 4.3 Detection: Am I in a Worktree?

```bash
# In worktree: returns .git/worktrees/<name>
# In main repo: returns .git
git rev-parse --git-dir

# Always returns the main .git directory
git rev-parse --git-common-dir

# Get main repo root from worktree:
MAIN_ROOT=$(cd "$(git rev-parse --git-common-dir)/.." && pwd)
```

### 4.4 Solution Options

**Option A: Redirect (like beads)**
Create `.lets/redirect` file in worktree pointing to main `.lets/`. Requires modifying all commands.

**Option B: Symlink**
Transparent to all commands - no code changes needed. But creates circular path if worktrees are inside `.lets/`.

**Option C: Detect and resolve in ROOT derivation**
Change canonical ROOT to use `git rev-parse --git-common-dir`. Requires updating all commands.

**Option D: WorktreeCreate hook does setup**
The hook creates symlink or redirect automatically.

**Option E: No .lets/ in worktrees (CHOSEN for agents)**
Agents with `isolation: worktree` do code work only - they don't run LETS commands (/lets:start, /lets:review, etc.). They only need code. No `.lets/` access needed at all.

**Option F: Native Claude Code worktrees (FINAL for agents, 2026-03-01)**
Don't register any hooks. Let Claude Code handle worktree creation/cleanup natively at `.claude/worktrees/`. Custom hooks (bd worktree create + .lets/ symlink) add files that git sees as "changes", blocking auto-cleanup. Native behavior = clean git status = auto-cleanup works.

For interactive worktrees (`/lets:worktree` command): use `.worktrees/` at project root with .lets/ symlink + bd worktree create. This is done by the command, not by hooks.

### 4.5 Race Conditions

If two worktree sessions write to shared `.lets/`:
- `last-summary.md` - last writer wins (acceptable?)
- Execution state - could corrupt if same task executed in parallel (unlikely)
- Session summaries - use unique filenames (YYYY-MM-DD-HHMM.md), safe

---

## 5. Superpowers Reference Patterns

**Source:** `reference/superpowers/skills/`

### 5.1 Worktree Creation (`using-git-worktrees/SKILL.md`)

**Directory selection - 3-step priority:**
1. Check existing `.worktrees/` or `worktrees/` (existing dir wins; `.worktrees/` preferred)
2. Check CLAUDE.md for worktree directory preference
3. Ask user: `.worktrees/` (local) vs `~/.config/superpowers/worktrees/<project>/` (global)

**Safety: .gitignore verification (mandatory):**
```bash
git check-ignore -q .worktrees 2>/dev/null
# If not ignored: add to .gitignore + commit BEFORE creating worktree
```

**Creation:**
```bash
git worktree add "$path" -b "$BRANCH_NAME"
```

**Auto-detect project setup after creation:**
- `package.json` -> npm install
- `Cargo.toml` -> cargo build
- `requirements.txt` / `pyproject.toml` -> pip/poetry
- `go.mod` -> go mod download

**Baseline test verification (mandatory):**
Run test suite. If failures found -> report and ask before proceeding. Can't distinguish new bugs from pre-existing if baseline is dirty.

### 5.2 Branch Finishing (`finishing-a-development-branch/SKILL.md`)

**4 options presented to user:**

| Option | Action | Worktree cleanup |
|--------|--------|-----------------|
| 1. Merge locally | `git checkout main && git merge` | `git worktree remove` - YES |
| 2. Create PR | `git push && gh pr create` | Keep worktree alive - NO |
| 3. Keep as-is | Nothing | Keep worktree alive - NO |
| 4. Discard | Delete branch + changes | `git worktree remove` - YES |

**Discard requires typed confirmation:** Must type exact string `'discard'` before `git branch -D`.

**Test gate before options:** Tests must pass before presenting merge/PR options.

### 5.3 Parallel Agents (`dispatching-parallel-agents/SKILL.md`)

**ONLY for independent domains** - different files, no shared state:
```
Task("Fix auth tests")      # touches src/auth/
Task("Fix payment tests")   # touches src/payment/
Task("Fix notification tests") # touches src/notify/
```

**After all agents return:** Verify fixes don't conflict. Run full test suite.

### 5.4 Subagent-Driven Development (`subagent-driven-development/SKILL.md`)

**STRICTLY sequential** - one implementer at a time:
```
For each task:
  1. Dispatch implementer subagent via Task()
  2. Spec reviewer (skeptical!) checks work
  3. Fix loop if needed
  4. Code quality reviewer checks
  5. Fix loop if needed
  6. Mark complete
  7. Next task
```

**Red flags (explicitly forbidden):**
- Never dispatch multiple implementation subagents in parallel (conflicts!)
- Subagent must NOT read plan file (inject full task text in prompt)
- Don't start code quality review before spec compliance is green

**Spec reviewer's skeptical stance:**
> "The implementer finished suspiciously quickly. Their report may be incomplete, inaccurate, or optimistic."

Reviewer reads ACTUAL code, not the implementer's report.

### 5.5 Executing Plans (`executing-plans/SKILL.md`)

- Default batch: 3 tasks at a time
- Checkpoint-based with human review between batches
- Wraps up with `finishing-a-development-branch`
- Creates TodoWrite for tracking

---

## 6. Integration Points with LETS Plugin

### 6.1 Commands That Need Worktree Awareness

**Must modify:**

| Command | What changes | Why |
|---------|-------------|-----|
| `start.md` | Step 6: offer worktree option alongside branch creation | Entry point for worktree workflow |
| `done.md` | Step 7: add `git worktree remove` / `bd worktree remove` | Cleanup after merge/PR |
| `hooks/rules-context.md` | Skill Quick Reference table | New command documentation |
| `commands/install.md` | Essential Skills table | New command documentation |

**Should modify (for worktree compatibility):**

| Command | Issue | Fix |
|---------|-------|-----|
| `end.md` | ~~`last-summary.md` race condition~~ (resolved) | Uses dated files with branch slug: `YYYY-MM-DD-HHMM-{branch}.md` |
| `commit.md` | Task ID parsing from branch name | Verify works with `bd worktree create` branch naming |
| `execute.md` | Execution state in `.lets/execution/` | Needs shared `.lets/` |
| `brainstorm.md` | Plans in `.lets/plans/` | Needs shared `.lets/` |

**No changes needed:**

| Command | Why it's fine |
|---------|--------------|
| `check.md` | Reads CLAUDE.md only, no `.lets/` |
| `review.md` | Agents are read-only, reports in `.lets/reviews/` (shared via symlink) |
| `opinion.md` | Agents are read-only |
| `ask.md` | Agent is read-only |
| `note.md` | Uses `bd` commands (shared via beads redirect) |
| `status.md` | Uses `bd` commands (shared via beads redirect) |

### 6.2 `git rev-parse --show-toplevel` Behavior

This is the ROOT derivation used in every command. In a worktree:

```bash
# In main repo:
git rev-parse --show-toplevel  # -> /Users/umbo/projects/lets-workflow

# In worktree:
git rev-parse --show-toplevel  # -> /Users/umbo/projects/lets-workflow/.worktrees/feature-auth
```

**This is why `.lets/` sharing is critical** - without it, every command looks for `.lets/` in the wrong place.

### 6.3 `project-root` Injection by SessionStart Hook

`hooks/session-start.sh` line 9:
```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
```

In a worktree, `project-root` injected into session context = worktree path. This is correct for git operations (the worktree IS the working directory) but wrong for `.lets/` access.

**rules-context.md** instructs AI: "Use `project-root` instead of `git rev-parse --show-toplevel`." This means in worktree context, the AI stays scoped to the worktree - which is correct behavior for the Boundaries rule ("Stay inside `project-root`").

### 6.4 Statusline in Worktrees

`hooks/lets-statusline.sh` derives cache dir from Claude Code's `workspace.current_dir`:
```bash
CACHE_DIR="${dir}/.lets/cache"
```

In a worktree, this creates a separate cache. Mostly harmless but wasteful. Could show worktree identity in statusline (currently shows branch name only).

### 6.5 Beads Integration

`.beads/` is git-tracked (not in `.gitignore`). Worktrees get a checked-out copy.

**Without `bd worktree create`:** `bd` commands work but use stale JSONL snapshot. Dolt database NOT shared.

**With `bd worktree create`:** Redirect file in `.beads/redirect` points to shared main `.beads/`. All `bd` commands use live database.

**Concurrent access:** Multiple worktrees need Dolt server mode (`bd dolt start`) for safe multi-writer.

---

## 7. bd swarm - DAG-based Task Waves

**Source:** `reference/beads/cmd/bd/swarm.go`

`bd swarm` analyzes task dependency graph using Kahn's algorithm and produces "waves" (`ReadyFront`) - groups of tasks that can execute in parallel.

```bash
bd swarm status
# Wave 1: [task-a, task-b, task-c]  (all independent)
# Wave 2: [task-d]                   (depends on task-a)
# Wave 3: [task-e, task-f]           (depend on task-d)
```

**Commands:**
- `bd swarm validate` - check for cycles, disconnected subgraphs, structural inversions
- `bd swarm status` - show current wave analysis
- `bd swarm create` - create swarm configuration
- `bd swarm list` - list swarms

**Current state:** Analytics/tracking only. No automatic agent dispatch. The dispatch-to-agents step is manual.

**Potential for LETS:** Lead could use `bd swarm status` to automatically:
1. Get current wave of independent tasks
2. Spawn teammates (one per task)
3. Each teammate works in own worktree
4. When wave completes -> get next wave -> spawn new teammates

---

## 8. Security Considerations

### 8.1 The Incident

During brainstorm exploration (this research session), an explorer agent with Bash access ran `bd worktree create` which:
- Initialized a Dolt database in project root (`dolt/` directory)
- Started a Dolt server process (PID file, lock file, log file, port file)
- Created server activity tracking files

All in the project root, polluting the working directory.

**Task created:** `lets-z3y.26` (Add agent sandboxing - prevent destructive operations from explorer agents)

### 8.2 Risk: Read-Only Agents with Bash

Current explorer agent has `tools: Read, Grep, Glob, Bash`. Bash allows:
- `bd worktree create` (creates directories, databases, starts servers)
- `bd create` (creates beads issues)
- `git checkout`, `git commit` (modifies repo state)
- `dolt init` (initializes databases)
- `rm -rf` (destructive operations)
- Any shell command

### 8.3 Mitigation Options

**Option 1: Remove Bash from explorer**
Simplest but loses `git log`, `find`, `wc -l` etc.

**Option 2: PreToolUse hook for agents**
Block destructive commands:
```bash
#!/bin/bash
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
if echo "$COMMAND" | grep -iE '\b(bd worktree|bd create|bd init|dolt|git checkout|git commit|git push|rm -rf)\b' > /dev/null; then
  echo "Blocked: destructive command not allowed for read-only agent" >&2
  exit 2
fi
exit 0
```

**Option 3: Agent-scoped hooks (frontmatter)**
```yaml
---
name: explorer
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "./hooks/validate-readonly.sh"
---
```

**Option 4: Explicit rules in agent prompt**
Add to system prompt: "NEVER run commands that modify state (bd create, bd worktree, git commit, etc.)"
Least reliable - LLM may still do it.

**Recommended:** Option 3 (agent-scoped hooks) + Option 4 (prompt rules) as defense in depth.

### 8.4 Agent Tool Restrictions

From official docs, subagents support:
- `tools` field: allowlist of permitted tools
- `disallowedTools` field: denylist
- `Agent(type1, type2)`: restrict which subagents can be spawned
- `permissionMode: "plan"` or `"dontAsk"`: restrict permissions

For LETS explorer agents that need Bash for `git log`, `find`, etc. but not for writes:
agent-scoped PreToolUse hook is the right approach.

---

## 9. Practical Combinations

### 9.1 Single Developer, Parallel Tasks

```
Terminal 1: claude --worktree feature-auth
Terminal 2: claude --worktree bugfix-123
Terminal 3: claude (main repo, coordinating)
```

Each terminal is independent. No automatic coordination.
Beads shared via `bd worktree create` redirect.

### 9.2 Agent Team with Worktree Isolation

```
Lead: creates team, creates tasks from bd swarm wave
  -> Teammate "frontend" (isolation: worktree) -> works on UI task
  -> Teammate "backend" (isolation: worktree)  -> works on API task
  -> Teammate "tests" (isolation: worktree)    -> writes tests
```

WorktreeCreate hook handles `bd worktree create` + `.lets/` symlink.
Teammates communicate via SendMessage.
Lead synthesizes and reviews.

### 9.3 LETS Workflow Integration

```
/lets:start -> pick task -> branch created
/lets:worktree -> creates worktree for parallel work
  -> bd worktree create + .lets/ symlink
  -> work in worktree
  -> /lets:commit -> commits in worktree
  -> /lets:done -> merge/PR + bd worktree remove + cleanup
```

### 9.4 Full Swarm Mode (future)

```
/lets:swarm
  -> bd swarm status -> get wave 1
  -> TeamCreate
  -> For each task in wave:
       -> Spawn teammate with isolation: worktree
       -> WorktreeCreate hook: bd worktree create + .lets/ symlink
       -> Teammate: claim task, implement, /lets:commit
  -> Wait for wave 1 complete
  -> bd swarm status -> get wave 2
  -> Spawn new teammates for wave 2
  -> Repeat until all waves done
  -> Lead: review all changes, merge
```

---

## 10. Known Limitations & Gotchas

### 10.1 Worktree-specific

- **`.lets/` doesn't exist in worktrees** - gitignored, must be symlinked/redirected
- **`git rev-parse --show-toplevel` returns worktree path** - not main repo
- **Branch can't be checked out in two worktrees** - git limitation
- **`git stash` is global** - stashes from one worktree visible in all
- **`git worktree remove` while branch checked out fails** - must checkout different branch first (handled by `bd worktree remove`)

### 10.2 Agent Teams-specific

- **No session resumption** - `/resume` doesn't restore in-process teammates
- **One team per session** - can't manage multiple teams
- **No nested teams** - teammates can't spawn own teams
- **Token-intensive** - each teammate is a full Claude Code session
- **Experimental** - API may change

### 10.3 Beads-specific

- **Without `bd worktree create`** - stale JSONL snapshot in worktree, not live DB
- **Multi-writer needs server mode** - `bd dolt start` required for parallel worktree writes
- **Redirect chains blocked** - only one level of redirect
- **`bd worktree remove` doesn't prune git registry** - may need `git worktree prune`

### 10.4 LETS Plugin-specific

- **~~`last-summary.md` race condition~~** - resolved: `end.md` now writes dated files with branch slug, `start.md` reads via `ls -t` glob
- **Session start ref is per-worktree** - correct behavior but different from main
- **Plans, execution state, reviews** - all in `.lets/`, need sharing mechanism
- **`session-start.sh` injects worktree path as `project-root`** - correct for git, wrong for `.lets/`
- **Statusline creates separate cache per worktree** - wasteful but harmless
- **No worktree identity in statusline** - can't tell which worktree you're in

---

## 11. Lessons Learned (2026-03-01 prototype)

### Don't fight native behavior

Custom WorktreeCreate/WorktreeRemove hooks were built to add .beads/redirect and .lets/ symlink to agent worktrees. But these additions made git see "changes" in the worktree, which blocked Claude Code's auto-cleanup. The hooks solved a problem that didn't exist (agents don't need .beads/ or .lets/) while creating a real problem (broken cleanup).

**Rule:** start with native behavior, only customize when there's a proven need.

### Agent frontmatter hooks don't work

Despite our research docs listing `hooks:` as a supported agent frontmatter field, runtime testing proved it's silently ignored. Official Claude Code docs only document hooks in:
1. Plugin `hooks/hooks.json`
2. User `.claude/settings.json`

**Rule:** verify features with runtime tests, not just documentation research.

### .gitignore trailing slash matters

`.lets/` (with trailing slash) ignores directories but NOT symlinks. When our hook created a `.lets` symlink in a worktree, git saw it as untracked. Fix: add `.lets` (without slash) alongside `.lets/`.

**Rule:** when gitignoring something that might be both a directory and a symlink, add both patterns.

### WorktreeRemove fires during removal, not to trigger it

WorktreeRemove hook fires as part of the removal process. If Claude Code decides to keep the worktree (because of changes), WorktreeRemove never fires. This is by design - the hook is for cleanup during removal, not for deciding whether to remove.

### Two worktree use cases need different solutions

| Use case | What's needed | Solution |
|----------|--------------|----------|
| Agent isolation (isolation:worktree) | Code only | Native Claude Code (.claude/worktrees/) |
| Interactive parallel sessions (claude --worktree) | Code + .lets/ + .beads/ | /lets:worktree command with manual setup |

Don't try to unify them with a single hook - the requirements are different.

---

## 12. Real-World Multi-Agent Setup (Beadbox, 2026-03)

Source: reddit.com/r/vibecoding - "I Ship Software with 13 AI Agents"

Production setup running 13 Claude Code agents building Beadbox (a dashboard for monitoring AI agent fleets). Uses beads as coordination backbone.

### Architecture

Not Agent Teams - 13 separate `claude` sessions in tmux panes, each with its own CLAUDE.md.

| Group | Agents | What they own |
|-------|--------|---------------|
| Coordination | super, pm, owner | Work dispatch, product specs, business priorities |
| Engineering | eng1, eng2, arch | Implementation, system design, test suites |
| Quality | qa1, qa2 | Independent validation, release gates |
| Operations | ops, shipper | Platform testing, builds, release execution |
| Growth | growth, pmm, pmm2 | Analytics, positioning, public content |

### Coordination tools

- **beads** (`bd`) - task tracking, claims, comments. `bd update --claim --actor eng2`, `bd comments add --author eng2 "PLAN: ..."`
- **gn/gp/ga** - tmux messaging. gn sends, gp peeks, ga queues async messages
- **super patrol loop** - every 5-10 min checks all agents for stalls, dispatches work
- **CLAUDE.md protocols** - identity paragraph + boundary section per agent. Defines what each agent owns and can't touch

### Key insight: boundaries, not hooks

Sandboxing through CLAUDE.md protocols, not technical enforcement:
> "eng2 can't close issues. qa1 doesn't write code. pmm never touches the app source."

Works at scale (13 agents, production). Validates prompt-level rules over hook-based enforcement.

### Key insight: QA uses separate repo clone

QA tests pushed code, not working tree. Independent validation = catches bugs eng missed.

### What breaks at scale

- Rate limits with 13 concurrent agents on same API account
- Context windows fill up (65% threshold = "save your work" protocol)
- Agents get stuck in error loops (super's patrol loop catches this)
- QA bounces add cycles but catch real bugs

### Minimum viable fleet

"Two engineers and a QA agent, coordinated through beads, will change how you think about what a single developer can ship."
