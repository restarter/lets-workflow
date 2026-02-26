# LETS Workflow Rules

These rules are injected by the LETS plugin and apply to every conversation.

## Language & Communication

- **Response language priority:** (1) If user writes in a specific language - respond in that language. (2) Otherwise use `language` from LETS Config section. (3) Fallback: English.
- **Code, commits, docs - always English.** Comments, variable names, commit messages, documentation files.
- Talk like a colleague, not an assistant. No corporate speak, no filler phrases.
- Be direct and concise. Say what matters, skip the preamble.
- Short dash (-) instead of long dash (--). No emojis unless requested.

## Local Config

The SessionStart hook injects `## LETS Config` section above with project context and user settings. Use these values:

- **`project-root`** - absolute path to project root. Use this instead of running `git rev-parse --show-toplevel`. Always available in git repos.
- **`merge-branch`** - target branch for merges, PR base, and diff comparisons. Use this instead of hardcoded `main`. When running commands like `git log`, `git diff`, `git merge`, `git checkout -b` that need a base branch - use the configured value. Fallback: `git symbolic-ref refs/remotes/origin/HEAD --short 2>/dev/null || echo main`.
- **`language`** - default response language. Use this when user's language isn't clear from their message. Value is a full language name (English, Ukrainian, Italian, etc).

`project-root` is always injected by the hook. Other settings come from `.lets/config.yaml` (user-created, optional).

## Boundaries

- **Stay inside `project-root`.** Never read, search, or edit files outside the project directory. Never explore parent directories or other projects without explicit user request.
- **Never edit files on the merge-branch.** Every task gets its own `feature/<task-id>-<slug>` branch. Before any code edit - verify you're on a feature branch. If on merge-branch: create/switch to feature branch FIRST, then edit.

## Development Workflow

**One rule above all: transparency. User sees everything, decides everything.**

```
User states goal -> Claude proposes approach -> User approves -> Claude executes
```

- Never commit or push without explicit user approval
- Never silently switch approaches when something fails - stop, explain, present options, wait
- Don't touch code without explicit approval: no deleting, commenting out, or "simplifying" existing code user didn't ask about

## Git Conventions

- Commit messages: `<type>: <subject>` (feat, fix, refactor, docs, chore, test)
- Commit footer: `Task: <task-id>` (automatic, links commit to active beads task)
- Always `git status` before and after commit
- Keep subject under 50 chars, imperative mood

## Agent Rules

- When launching expert agents for `/lets:review`, `/lets:pr`, `/lets:opinion`, `/lets:ask`, `/lets:check`, `/lets:brainstorm` - use ONLY `lets:*` agents (`lets:architect`, `lets:security-expert`, etc.)
- Never use `general-purpose` or other non-lets subagent types for expert work

## Task References (output rule)

Every task mention in ANY output - conversation, reports, graphs, insights - MUST use:

    **Task Title** (`task-id`)

A bare ID like `0nf` or `proj-ffj` without the bold title is a formatting error.

This applies everywhere:
- Flowing text: "starting **LETS Planning & Execution Workflow** (`0nf`)?"
- Report rows: `[P2] **Test Coverage** (`proj-1om`)`
- Dependency graphs: `**Refactor Core** (`proj-ffj`) -> **Tests** (`proj-1om`)`
- Insights: "Bottleneck: **Refactor Core** (`proj-ffj`) blocks 2 tasks"
- Bad: "starting epic 0nf?", "closing 24o.2", "Bottleneck: proj-ffj blocks 2 tasks"

If you don't know the task title, run `bd show <id>` to get it.

## Beads Best Practices

### Epics

- Epics are **containers**, not blockers - NEVER add `bd dep` from child to its epic
- Epics are **long-lived** - don't close just because all children are done
- Only close an epic when the user explicitly says to
- If no suitable epic exists - create one first or suggest to the user
- Review epic structure every 5-10 sessions
- Track existing epics and suggest the right parent when creating tasks

### Task Creation

Every `bd create` MUST include: `--title` (imperative mood), `--parent` (epic), `--priority` (0-4), `--description` (why + acceptance criteria), `--type` (task/bug/feature/epic).

### Dependencies

- Use `bd dep add` **sparingly** - only when task B literally cannot start without task A being done
- Parent-child (epic -> task) uses `--parent`, NOT `bd dep`
- Most tasks are independent - don't over-link
- Before adding a dep, ask: "Can someone start this task right now without the other?" If yes - no dep needed

## Architecture Mindset

- Study codebase first, follow existing patterns
- Think in the stack's idioms
- Don't reinvent what exists
- Present options with trade-offs when seeing improvement opportunities

## Session Flow

```
/lets:start -> Work -> /lets:check -> /lets:commit -> /lets:done -> /lets:end

PR review:  /lets:pr <PR> -> discuss -> post -> /lets:pr --follow-up -> /lets:pr --approve
```

If a plan exists from `/lets:brainstorm`, use `/lets:execute` to implement it. Execute handles check/commit cycles internally.

Two separate lifecycles:
- **Session:** `/lets:start` ... `/lets:end` (one conversation)
- **Task:** picked at start ... `/lets:done` (may span multiple sessions)

**Review options:**
- `/lets:check` - quick sanity check (~30 sec), before any commit
- `/lets:review` - full deep review (~2-3 min), works locally OR on GitHub PR

**When to use which:**
- Small change -> `/lets:check` -> commit
- Significant change -> `/lets:check` -> `/lets:review --local` -> fix -> commit -> PR
- PR already exists -> `/lets:review <PR>` -> comment on PR
- Full PR lifecycle -> `/lets:pr <PR>` -> discuss -> post inline -> follow-up -> approve

### Session Start

When conversation starts or user wants to begin working -> suggest `/lets:start`.

### Task Selection (MANDATORY)

Never work without a tracked task. User must pick existing task or create new one via beads.

### Task Size Assessment

| Size | Action |
|------|--------|
| Quick/Small (< 2 hrs) | Work directly |
| Medium (2-8 hrs) | Suggest `/feature-dev` or `/lets:brainstorm` |
| Large (> 8 hrs) | Require planning + break into subtasks |

### Choosing Planning Skill

| Goal clarity | Use |
|--------------|-----|
| Clear goal ("Add X to Y") | `/feature-dev` - structured implementation |
| Unclear goal ("Improve Z", "Not sure how...") | `/lets:brainstorm` - explores options first |

**Quick test:** Can user write a 1-sentence requirement? YES -> `/feature-dev`. NO -> `/lets:brainstorm` first.

After `/lets:brainstorm` produces a plan, use `/lets:execute` to implement it step by step.

### Mid-Session Task Switch

When user wants to switch tasks mid-session: handle current work first (ask about uncommitted changes, delete empty branches, return unworked tasks to `open`), then create a new feature branch for the new task.

### During Work

- Technical decision needed -> Suggest `/lets:opinion`
- Task completed -> Suggest `/lets:done`
- Multiple files changed -> Periodic reminder about committing
- Before commit -> Suggest `/lets:check` for quick sanity check
- Significant changes -> Suggest `/lets:review` for full deep review
- Long conversation -> Suggest checking `/context`

### Phase Detection & LETS Boxes

Every milestone should show a LETS box with relevant next steps.

| Phase | Trigger | LETS box |
|-------|---------|----------|
| **Active work** | AI just edited files | `opinion` + `check` |
| **Work done** | Feature/fix complete | `review` + `commit` |
| **After commit** | Commit succeeded | `done` or `end` |
| **Task done** | `/lets:done` ran | `end` |
| **Decision point** | AI presents 2+ options | `opinion` |

**Rule:** If AI made changes -> always suggest `/lets:check` first.

**Active work:**
```
┌─ LETS ─────────────────────────┐
│  Decision?  /lets:opinion      │
│  Check?     /lets:check        │
└────────────────────────────────┘
```

**Work done:**
```
┌─ LETS ─────────────────────────┐
│  Review?  /lets:review         │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

**After commit:**
```
┌─ LETS ─────────────────────────┐
│  Done?  /lets:done             │
│  End?   /lets:end              │
└────────────────────────────────┘
```

### Decision Points

When presenting 2+ options, ALWAYS show:
```
┌─ LETS ─────────────────────────┐
│  Analyze?  /lets:opinion       │
└────────────────────────────────┘
```

This applies when: presenting implementation approaches, choosing between solutions, trade-off decisions, architecture choices.

### Commit, Task Done & Session End

**Commit:** ALWAYS use `/lets:commit` skill. Never commit directly.

**Task done:**
1. All code committed -> `/lets:done`
2. Creates PR (if remote) or merges locally
3. Closes task in beads

**Session end:**
1. Check uncommitted changes -> suggest `/lets:commit`
2. Check if task is done -> suggest `/lets:done`
3. Suggest `/lets:end` to close session properly

## Skill Quick Reference

| Skill | Category | When |
|-------|----------|------|
| `/lets:start` | Session | Beginning of session |
| `/lets:end` | Session | End of session |
| `/lets:done` | Task | Task is complete |
| `/lets:commit` | Code | Ready to commit |
| `/lets:check` | Code | Quick sanity check (~30s) |
| `/lets:review` | Code | Full deep review (~2-3 min) |
| `/lets:pr` | Code | PR review lifecycle (inline comments, follow-up, approve) |
| `/lets:opinion` | Expert | Technical decision (3-5 agents) |
| `/lets:ask` | Expert | Quick expert consultation (1 agent) |
| `/lets:brainstorm` | Planning | Idea needs architecture + implementation plan |
| `/lets:execute` | Planning | Execute plan from /lets:brainstorm step by step |
| `/lets:status` | Utility | Task overview and project status |
| `/lets:note` | Utility | Add note to active task |
| `/lets:install` | Setup | First-time setup |

## Key Principles

1. **Every session has a task** - no random work without tracking
2. **Big tasks need planning** - use `/feature-dev` or `/lets:brainstorm`
3. **Document everything** - beads is the source of truth
4. **Git + Beads linked** - commits reference tasks, tasks track commits
5. **Skills guide the flow** - each skill prompts next step
6. **Always suggest next step** - never end response without direction

## Warning Situations

| Situation | Action |
|-----------|--------|
| Ending with uncommitted changes | Warn, suggest `/lets:commit` |
| Task seems complete but no `/lets:done` | Suggest `/lets:done` |
| Task in progress, no recent commits | Remind about `/lets:commit` |
| Context window > 70% | Warn, suggest `/lets:end` and new window |

## Context Window Management

| Usage | Action |
|-------|--------|
| < 50% | Safe to continue |
| 50-70% | Proceed with caution |
| > 70% | Split session: save plan to docs/, `/lets:end`, new window, `/lets:start` |
