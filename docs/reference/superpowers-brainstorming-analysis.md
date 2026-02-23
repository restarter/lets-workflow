# Planning Skills Research Notes

Research for task aak: inform design of /lets:brainstorm and /lets:execute.
Covers superpowers:brainstorming, superpowers:executing-plans, and feature-dev:feature-dev.

## superpowers:brainstorming

### Process (6 phases)
1. Explore project context (files, docs, recent commits)
2. Ask clarifying questions ONE AT A TIME
3. Propose 2-3 approaches with trade-offs
4. Present design in sections scaled to complexity
5. Write design doc to `docs/plans/YYYY-MM-DD-<topic>-design.md`
6. Invoke writing-plans skill (only next skill allowed)

### Key Patterns
- **HARD-GATE**: forbids ANY code until design is approved
- **One question at a time** - prefer multiple choice over open-ended
- **Scale sections** from few sentences to 200-300 words based on complexity
- **Ask approval after each section** of design
- **Commits design doc to git**

### Skill Chain
brainstorming -> writing-plans -> executing-plans -> finishing-a-development-branch

## superpowers:executing-plans

### Process (5 steps)
1. Load and critically review plan (raise concerns BEFORE starting)
2. Execute tasks in batches (default: first 3 tasks)
3. Report what was implemented + verification output, say "Ready for feedback"
4. Apply feedback, execute next batch, repeat
5. Use finishing-a-development-branch skill to complete

### Key Patterns
- **Batch execution** with checkpoint between batches
- **Stop conditions** - never force through blockers, ask for help
- **Verification runs** after each task (don't skip)
- **Never start on main/master** without consent
- **Requires git worktree** for isolation

### Skill Chain
- Requires: using-git-worktrees
- Created by: writing-plans
- Transitions to: finishing-a-development-branch

## feature-dev:feature-dev (BEST REFERENCE)

### Process (7 phases)
1. **Discovery** - clarify feature, confirm understanding
2. **Codebase Exploration** - 2-3 code-explorer agents in parallel
3. **Clarifying Questions** (CRITICAL) - ALL questions at once, WAIT for answers
4. **Architecture Design** - 3 code-architect agents in parallel (minimal/clean/pragmatic)
5. **Implementation** - only after explicit approval
6. **Quality Review** - 3 code-reviewer agents in parallel
7. **Summary** - what was built, decisions, files, next steps

### Key Patterns
- **5 approval gates**: clarify, questions, arch choice, impl start, review
- **Specialized agents per phase**: explorer -> architect -> reviewer
- **3 architecture options**: minimal changes, clean architecture, pragmatic balance
- **Consolidate ALL questions** in Phase 3 (not one at a time like brainstorming)
- **Review built-in** as Phase 6 (3 reviewer agents)
- **TodoWrite** for progress tracking
- **HARD-GATE** before implementation (Phase 5)

### Agents
- **code-explorer** (Sonnet) - traces execution paths, maps architecture, returns key files list
- **code-architect** (Sonnet) - designs architecture, provides implementation blueprint
- **code-reviewer** (Sonnet) - reviews code for bugs, quality, conventions (confidence >= 80%)

## Comparison: brainstorming vs feature-dev

| Aspect | brainstorming | feature-dev |
|--------|--------------|-------------|
| Structure | 6 phases, exploratory | 7 phases, strict gates |
| Questions | One at a time, open-ended | Phase 3: ALL questions together, WAIT |
| Architecture | 2-3 approaches informally | 3 parallel agents with distinct strategies |
| Agents | None specialized | explorer -> architect -> reviewer pipeline |
| Review | None | Phase 6: 3 reviewer agents |
| Approval gates | 1 (plan) | 5 (clarify, questions, arch, impl, review) |
| Output | Design doc file | Modified files + summary |

## What to Adopt for LETS

### /lets:brainstorm (hybrid: brainstorming clarity + feature-dev structure)

FROM FEATURE-DEV:
- ADOPT: Phase 1 Discovery (clarify intent, confirm understanding)
- ADOPT: Phase 2 Codebase Exploration via lets:* agents
- ADOPT: Phase 3 ALL clarifying questions at once (better than one-at-a-time)
- ADOPT: Phase 4 Multiple architecture options via lets:architect + lets:pragmatist
- ADOPT: Multiple approval gates
- ADOPT: HARD-GATE before any code

FROM BRAINSTORMING:
- ADOPT: AskUserQuestion for structured interaction (multiple choice)
- ADOPT: Scale output to complexity

LETS-SPECIFIC:
- Save plan to beads task AND .lets/plans/ file
- Use lets:* agents (not feature-dev's code-explorer/architect/reviewer)
- End with clear plan ready for /lets:execute
- No implementation in brainstorm (that's execute's job)

### /lets:execute (from executing-plans + feature-dev Phase 5-7)

FROM EXECUTING-PLANS:
- ADOPT: Load and review plan before starting
- ADOPT: Batch execution with checkpoints
- ADOPT: Stop conditions and "ask for help"
- ADOPT: Never work on main/master

FROM FEATURE-DEV:
- ADOPT: Phase 6 quality review concept (use /lets:check or /lets:review)
- ADOPT: Phase 7 summary (what was built, decisions, files)

LETS-SPECIFIC:
- Create feature branch (not worktree)
- Track progress in beads (not TodoWrite)
- Use /lets:commit and /lets:end at finish
- Instruct user to restart Claude for clean context
- /lets:start picks up context from beads + plan file
