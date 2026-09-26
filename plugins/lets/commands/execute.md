---
description: Execute implementation plan from /lets:plan - inline in native plan mode, or delegated to named implementer subagents you review and correct
argument-hint: "[task-id|plan-path] [--status] [--step|--straight|--auto|--implementers] [--parallel] [--pipelined] [--gate per-commit|high-only|at-end]"
---

# Execute Plan

Load an implementation plan and execute it - inline, in Claude Code's native plan mode, or delegated: each commit-point chunk goes to a named implementer subagent, its diff is reviewed - by you or, under the gate policy you approve at Start, by the team's check - and corrections go back to that same agent. The plan provides the roadmap; the gates are plan mode (inline) or Step 5-D's Start, review and run-review gates (delegated).

**Plan is a roadmap, not a script.** Read real files before every change. Adapt cosmetically (a renamed variable, a line that moved). Anything that changes the plan's APPROACH is a **deviation** - see the Deviation gate in Step 5 - and STOPS the run; a silently adapted plan is a new, unapproved plan.

**Another agent instead of this session:** `/lets:handoff --execute --send [<tab>]` hands the plan to an agent tab of this worktree (Codex, Antigravity, Claude) as a self-contained brief; its commits come back UNVERIFIED for `/lets:review --branch`.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## --auto mode

`/lets:execute --auto` runs the approved plan **autonomously**: skip the per-step "review before moving on" gate and `/lets:commit` at each plan commit point WITHOUT re-asking (matches the established "autonomous commits during execute after plan approval" preference). `--auto` is execution-speed for an *already-approved* plan — it does NOT bypass the hard-stops.

**`--auto` = run under AUTO MODE** (see `.claude/rules/lets-rules.md` `## AUTO MODE`) — do NOT restate the contract here. What still applies under `--auto`: push / PR / close (tracker `close`) / external stay gated; a 3×-fail halts; fabrication halts; and **`--auto` on `$LETS_MERGE_BRANCH` REFUSES** (Step 1) rather than auto-entering trunk-mode. When a hard-stop is hit, write the `blocked` marker and fire the execute-blocked notify (Step 6 / the gate-notification block) so an unattended session surfaces instead of stalling.

Parse `--auto` from the argument (it composes with a plan path; it is mutually exclusive with `--status`).

## Pipeline-state marker (under `--auto`)

A spawned autonomous session records its phase in a **per-task** marker file so N parallel worktrees don't clobber each other and the gate-notify (Step 6) can tell autonomous from interactive runs. **Per-task filename** (NOT a shared `pipeline-state` — `.lets/` is a symlink shared across worktrees): `.lets/cache/pipeline-state-<task-id>`, single line `<task-id>|<phase>|<iso>`, phase ∈ `planning | gate-clarify | gate-approve | executing | blocked | done`. Write helper (run at each transition; only under `--auto` — interactive execute writes no marker, so it triggers no notify):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
printf '%s|%s|%s\n' "{TASK_ID}" "{PHASE}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$LETS_PROJECT_ROOT/.lets/cache/pipeline-state-{TASK_ID}"
```

`execute --auto` writes `executing` when implementation starts (Step 5), `blocked` on a hard-stop (with the notify), `done` on completion (Step 6). **Keep in sync:** the same contract + helper live in `plan-workflow.md` (which writes `planning`/`gate-clarify`/`gate-approve`).

**Execute-blocked notify.** On a hard-stop under `--auto` (3×-fail / fabrication / a gated op reached / `$LETS_MERGE_BRANCH` refused), after writing the `blocked` marker, fire the **marker-gated gate-notification** so an unattended session surfaces instead of stalling. Use the authoritative snippet documented in `plan-workflow.md` "## Gate notifications" (don't re-paraphrase), incl. its single-quote rule for substituted values: `lets notify --cwd "$LETS_PROJECT_ROOT" --title 'Execute blocked — needs you' --body '<reason>' --json 2>/dev/null || true` (launcher-neutral — dispatches on `LETS_LAUNCHER`), guarded by the `pipeline-state-{TASK_ID}` marker existing. Best-effort — the run also halts visibly in-band. With the blocked marker, also mirror it onto the Orca card:

```bash
[ "{LETS_LAUNCHER}" = "orca" ] && lets orca card --phase blocked --comment '{reason}' --json 2>/dev/null || true
```

## Step 1: Active Task Detection

**Positional argument** — if it is a path (ends in `.md` or contains `/`), it is a **plan-path**: bind `{PLAN_ARG}` to it (Step 2 consumes it and skips slug derivation) and skip the task resolve-and-claim below. Otherwise treat it as a `<task-id>`.

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If not on a feature/worktree branch and no in-progress task found - ask user which task to execute.

If invoked with an explicit `<task-id>` argument and it is not already the active/in-progress task (e.g. a spawned `execute --auto <id>` in a fresh worktree), resolve-and-claim it per the **detect-task** *explicit task-id argument* convention before resolving the plan - don't re-paraphrase the logic; that section is the single source of truth (incl. the AUTO-MODE entry-claim carve-out). `--orc` is stripped per that convention before the task-id test.

**If on `$LETS_MERGE_BRANCH`** — `/lets:execute` usually expects a feature branch.

**Under `--auto`: REFUSE, do NOT show the soft-gate.** Autonomous editing of the merge-branch must never be auto-authorized — trunk-mode is a deliberate human opt-in (via the take-task picker), and an unattended session cannot answer an `AskUserQuestion`. Write the `blocked` marker, fire the execute-blocked notify ("Execute blocked — needs a feature branch"), and stop. A human re-runs on a feature branch, or opts into trunk-mode interactively (without `--auto`). `--auto` speeds an *authorized* path; it is not authorization to start editing `$LETS_MERGE_BRANCH`.

Without `--auto`, soft-gate before proceeding:

```
AskUserQuestion(
  questions=[{
    question: "You're on {LETS_MERGE_BRANCH}. /lets:execute usually wants a feature branch. Proceed?",
    header: "Trunk-mode",
    options: [
      { label: "Take a branch (Recommended)", description: "Stop, take a feature branch via /lets:start" },
      { label: "Continue here", description: "Proceed in trunk-mode (plan lookup uses task-id)" },
      { label: "Cancel", description: "Stop, return to work" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Take a branch (Recommended)** -> stop. Inform user: "Run `/lets:start <id>` and pick **Branch**, then `/lets:execute` again."
- **Continue here** -> proceed to Step 2 (trunk-mode is detected at runtime by Step 2's `[ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]` check; no flag passed).
- **Cancel** -> stop, return to user.

## Step 2: Load Plan

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
PLAN=""; IDEA_REFUSED=""

# Explicit plan-path argument wins: `/lets:execute <path-to-plan>.md` skips slug derivation
# entirely (the escape hatch for detached HEAD / unresolved task-id / cross-worktree cases).
# {PLAN_ARG} = the orchestrator-substituted path argument, empty when none was passed.
if [ -n "{PLAN_ARG}" ] && [ -f "{PLAN_ARG}" ]; then
  PLAN="{PLAN_ARG}"
  # An idea document is not a plan: never execute one, even by explicit path.
  if printf '%s' "$PLAN" | grep -q -E -- '-idea(-v[0-9]+)?\.md$'; then
    echo "This is an idea document - run /lets:plan to turn it into a plan."; PLAN=""; IDEA_REFUSED=1
  fi
fi

if [ -z "$PLAN" ] && [ -z "$IDEA_REFUSED" ]; then
  # Derive slug: trunk-mode uses task-id (plan.md saves <date>-<task-id>.md on the merge-branch);
  # otherwise the branch slug (covers feature/* and worktree-* branches).
  # ${TASK_ID} is substituted by the orchestrator from the Step 1 detect-task result.
  if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
    SLUG="${TASK_ID}"
  else
    SLUG="${BRANCH#feature/}"
  fi

  # Guard: an empty slug (detached HEAD, or unresolved task-id in trunk-mode) would collapse the
  # glob to *.md -> global latest -> another worktree's plan (the exact bug this task fixes).
  if [ -z "$SLUG" ]; then
    echo "Could not derive a plan slug (detached HEAD or unresolved task-id). Pass a plan path (/lets:execute <path>.md) or run /lets:start."
  else
    # Latest plan for this slug - matches date-prefixed (YYYY-MM-DD-HHMM-<slug>.md) AND legacy bare
    # <slug>.md. Slug-scoped, NOT global latest: .lets/plans is shared across worktrees via symlink.
    # task-id first (artifact-path naming, lets-05c4s); branch slug = legacy fallback
    [ -n "${TASK_ID}" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${TASK_ID}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
    [ -z "$PLAN" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
  fi
fi

[ -n "$PLAN" ] && cat "$PLAN"
```

If the explicit path was an idea document (`IDEA_REFUSED`), stop after that one line.

If no plan found:
> "No plan found for this task. Run `/lets:plan` first to create one."

Exit with LETS box suggesting `/lets:plan`.

## Step 3: Handle --status (exit early)

If argument is `--status`:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
# Show plan info (re-resolve: each bash block is a fresh shell). Same guarded lookup as Step 2.
SLUG=${BRANCH#feature/}; [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && SLUG="${TASK_ID}"
PLAN=""
# task-id first (artifact-path naming, lets-05c4s); branch slug = legacy fallback
[ -n "${TASK_ID}" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${TASK_ID}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
[ -z "$PLAN" ] && [ -n "$SLUG" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
echo "Plan: ${PLAN:-(none found)}"
```

```lets-tracker
show task=<task-id>
```

Resolve the session boundary through the shared reader - `Skill(skill: "lets:session-boundary")` - and read its echoed `SESSION_BOUNDARY` / `SESSION_RANGE_DESC`. Do NOT read the `session:` line here: this used to be a second, unvalidated copy of that read (no shape guard, no ancestor check, no floors), reporting an unqualified count of exactly the kind lets-370mx was filed about. Report the range with its qualifier, never a bare number.

```bash
# {SESSION_BOUNDARY} is the ref the skill echoed; `none` means it resolved no boundary.
git log --oneline {SESSION_BOUNDARY}..HEAD
```

```
## Execution Status: **{task title}** (`{task-id}`)

Plan: {resolved plan path}
Commits this session: {N}

{commit list}
```

```
┌─ LETS ─────────────────────────┐
│  Continue?  /lets:execute      │
│  End?       /lets:end          │
└────────────────────────────────┘
```

Exit after showing status.

## Step 4: Plan Validation

Quick check before entering plan mode:

1. Read the plan file
2. Extract file paths from `Create:` and `Modify:` sections
3. Check if `Modify:` targets exist
4. Check if `Create:` targets already exist (partial execution?)

**If all OK:**
Present plan summary (title, task count, key files), then proceed to Step 4.5.

**If drift detected** (files missing, already created, etc.), use AskUserQuestion. Not under `--auto`, add the "Ask orchestrator" option per lets-rules `### Orchestrator offer` (Act shape; rule not loaded -> no offer).

```
AskUserQuestion(
  questions=[{
    question: "Plan drift detected: {details}. How to proceed?",
    header: "Drift",
    options: [
      { label: "Execute anyway", description: "Adapt implementation to current state" },
      { label: "Re-plan", description: "Run /lets:plan to update the plan" },
      { label: "Cancel", description: "Don't execute" },
      { label: "Ask orchestrator", description: "Stay at this gate; /lets:orc ask with the drift details" }  /* only per Orchestrator offer */
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Execute anyway** -> proceed to Step 4.5
- **Re-plan** -> invoke `Skill(skill: "lets:plan")`
- **Cancel** -> stop, return to the user
- **Ask orchestrator** -> `Skill(skill: "lets:orc", args: "verb=ask footer=none text=Plan drift before executing: {details}. Execute anyway or re-plan?")`. After the reply is relayed, show this gate again without that option - the user picks.

## Step 4.5: Choose Execution Mode

How the approved plan runs is ONE up-front choice. **A mode flag pre-answers it - skip the picker entirely when any is present:** `--auto` (Here · auto), `--implementers` or its alias `--team` (Implementers), `--step` / `--step-by-step` (Here · step-by-step), `--straight` / `--straight-through` (Here · straight-through). `--auto` keeps all its AUTO MODE semantics (Step 1 refuses on `$LETS_MERGE_BRANCH`; hard-stops preserved). Default locus is **Here** (this session, native plan mode); **Implementers** is the only locus switch. **`--auto` together with `--implementers` / `--team` is REFUSED** with one line - a delegated run is interactive by design (its gates wait for you, and an unattended run has nobody to review it): drop `--auto`, or run inline. **`--gate <per-commit|high-only|at-end>`** sets a delegated run's gate policy (Step 4.7 proposes it, 5-D.5 dispatches on it); under `--auto` any value other than `per-commit` is REFUSED with one line - `--auto` runs keep today's gates. **`--parallel`** fixes the parallel shape (Step 4.6, 4.7); it needs `--implementers` / `--team` - alone it is REFUSED with one line - and under `--auto` it is REFUSED like every delegated run. **`--pipelined`** fixes the commit policy `pipelined` (implementers commit locally and move on; 5-D.5 "Pipelined run"), solo or with `--parallel`; it too needs `--implementers` / `--team`, and under `--auto` it is REFUSED.

**Bare `/lets:execute` (no mode flag) - ask exactly once:**

```
AskUserQuestion(
  questions=[{
    question: "How should I execute this plan?",
    header: "Run mode",
    options: [
      { label: "Straight-through (Recommended)", description: "Here, one approval, run all tasks, auto-commit at plan points" },
      { label: "Step-by-step", description: "Here, pause for review after each task; confirm each commit" },
      { label: "Auto", description: "Here, AUTO MODE - unattended; hard-stops + push/PR/close/external still gated" },
      { label: "Implementers", description: "Named agents implement chunk by chunk; reviewed by the gate policy you approve" }
    ],
    multiSelect: false
  }]
)
```

**Commit cadence is DERIVED from the mode - never a separate question:** step-by-step -> confirm each commit; straight-through -> auto-commit at each plan commit point; auto -> auto-commit; implementers -> one commit per chunk, after the Accept its gate policy names (5-D.5).

**Handle response (sets the mode that Step 5 obeys):**
- **Straight-through** -> Step 5 (native plan mode); after the plan-mode approval, implement all tasks with NO per-task pause and `/lets:commit` at each plan commit point without re-asking.
- **Step-by-step** -> Step 5 (native plan mode); after the plan-mode approval, implement one task, pause for user review before the next, and confirm each `/lets:commit`.
- **Auto** -> proceed exactly as `--auto` (Step 5's `--auto` behavior + pipeline-state marker + execute-blocked notify). **Guard:** if on `$LETS_MERGE_BRANCH`, REFUSE Auto here too (same rule as Step 1's `--auto` refuse - AUTO MODE never edits the merge-branch); tell the user to pick step-by-step / straight-through or take a feature branch.
- **Implementers** -> do NOT enter native plan mode: Step 4.6, Step 4.7, then Step 5-D. `/lets:team` manages a standing team; parallel implementation of a plan runs here, in the parallel shape.

(A remembered default / `LETS_EXECUTE_MODE` to skip the picker on every run is a deferred follow-up - this ships the picker + flag shortcuts only.)

## Step 4.6: Split the plan (Implementers only)

Everything here is derived from the plan's existing `### Task N` sections - their `**Files:**` Create/Modify paths and their `**Commit:**` blocks. No other marker is read and none is invented.

**1. Classify every task, in document order:**

| kind | rule | who runs it |
|---|---|---|
| **caller task** | no `Create:` / `Modify:` path (`**Files:** none`, a check, a smoke, a tracker step) | this session, inline, in plan order |
| **chunk** | a maximal run of consecutive non-caller tasks ending at the first one whose `**Commit:**` is a real commit (not `none`); a non-caller task with `**Commit:** none` extends the run | one implementer |
| **malformed** | a run of non-caller tasks with no real commit before the next caller task or the end of the plan | nobody - refuse delegation (below) |

Chunk ids are `c1`, `c2`, ... in plan order. A chunk's **allowlist** is the union of EVERY path its tasks' Create/Modify items name - one item may name several (`Modify: a.md (:10) and b.md (:20)`), and taking only the first drops a planned file. A target that cannot be read as a path unambiguously refuses delegation (below).

A chunk's **Risk** is `high` when any of its tasks says `**Risk:** high` or its commit task has no `**Risk:**` line - missing = high; otherwise `low`. Review depth follows it.

**Files audit - a GATE.** Every path a task's step text creates, edits, renames or deletes must appear in that task's `**Files:**` (a path named only to read or cite is not a touch). Resolve the Files: lines first:

- expand `{a,b}` brace sets (`memberscmd/{members.go,lead.go}` -> two paths);
- a bare name (no `/`) inherits the directory of the previous path on the same Files line (`info.go`, `info_test.go` -> both in `info.go`'s directory); with no previous path on the line it is a repo-root path (`CLAUDE.md`);
- a task's `Files add:` Amendment lines count as its Files.

Only a FULL path a step's text touches that is still missing from Files after this expansion refuses delegation (below), naming each task and path - it would put the implementer outside its allowlist, a certain deviation; fixing the plan's Files: lines, or `/lets:plan`, clears it. A name that cannot be resolved, or resolves ambiguously, is a warning in the preview's `### Warnings` block, never a refusal.

**Removed-symbol check - a WARNING only.** Best-effort: for each symbol (function, type, flag, file, heading) a commit point's text removes or renames, grep the text of every LATER commit point for it. A later use is listed as a warning in the Start preview; it never refuses delegation - a grep over prose cannot prove a use.

**Anchor check against BASE - a WARNING only.** `BASE` = `git rev-parse HEAD` now. Mechanically check every `file:line` anchor the plan names (the line still holds what the plan says it holds), every pin counter (a count the plan or its tests fix, e.g. "exactly 3 Review gates" - count it in the file) and every path a Verify command names (it exists, or a task creates it). The checker is an explorer already live for this run or team, through `member-run` (`op=next`, `role=lets:explorer`), when there is one - never spawned for this, nothing is spawned before Start - else this session inline. A mismatch is a warning listed in the Start preview; it adds no gate and no option.

**CI checks.** Read the repo's CI workflow files (e.g. `.github/workflows/*.yml`) and the Makefile targets they call, and take the commands CI runs on a change (e.g. `make build`, `make test`, `make lint`). They go into every chunk brief as `CI CHECKS:`. No CI workflow -> the brief's list reads `none found`, and the Start preview's warnings say so.

**2. File-sharing sets.** A caller task is a barrier: nothing before it may still be open when it runs, and nothing after it starts before it is done. Within each stretch between barriers, two chunks share a set when their allowlists share a path, directly or through another chunk of the stretch. Sets keep plan order; groups (step 4) are built from them.

**3. Show the split** - in the Start gate itself, in the `preview` of its Start option after the Step 4.7 launch plan (5-D.2), so the user approves the split they are looking at (a table printed as prose before a gate is skipped in practice). Every task exactly once, then the warnings:

```
### Plan split

| Task | Owner    | Group | Risk | Allowlist             |
|------|----------|-------|------|-----------------------|
| 0    | caller   | -     | -    | none                  |
| 1-2  | chunk c1 | a     | high | a.md, b.md, cli/x.go  |
| 3    | chunk c2 | a     | low  | a.md                  |

### Warnings
- removed symbol: `oldName` removed by c1, still used by Task 3
- anchor: `a.md:120` no longer holds "Step 2" at BASE
```

A task missing from the table, or listed twice, is a derivation error: stop and say which. The Allowlist column names every path, so a dropped file is visible before Start. No warnings -> `### Warnings` reads `none`.

**4. Declare the groups (parallel shape).** The shape is never inferred. In the solo shape every chunk belongs to one group, `a`. In the parallel shape (`--parallel`, or a launch plan the owner accepts with it) this session DECLARES the groups - which chunks each implementer owns - in the split table's Group column, each group a union of the step-2 file-sharing sets, in plan order inside it. Group ids are `a`, `b`, ... **Files-disjoint gate across groups:** no path may sit in the allowlists of two groups; a declared split that breaks it is not offered - name the shared paths and declare again, or run solo. Every declared split carries the warning, in the preview's `### Warnings`: "file-disjoint is not independence" - a chunk can call code another group adds in a different file; a dependency like that puts both chunks in one group. A caller task stays a barrier for every group.

**Refuse delegation** - one line saying why, then the Step 4.5 picker again without the Implementers option - when any task is malformed (name it); when the Files audit fails (name each task and path); when the plan has no chunk at all; when no task has a `**Files:**` block (a hand-written plan cannot be split); or when any task's execution itself (not one conditional sub-step inside it) depends on something learned during the run - read each whole task section; a heading or opening line such as `only if`, `only when`, `only on` (any case) or `run this task if ...` is the usual sign. A delegated run dispatches every chunk, so such a plan runs inline, where the condition is judged when the plan reaches it.

## Step 4.7: Launch plan (proposal)

Before the 5-D.2 Start gate, read the plan and PROPOSE how to run it. This is reasoning about THIS plan, not a fixed rule - no allocation is applied by default. The inputs:

- the plan's blocks (its `**Unit:**` lines or an execution-order table, when it has them) and the Step 4.6 chunks and groups;
- each chunk's Risk (Step 4.6);
- the Files audit: which groups are file-disjoint;
- dependencies: a chunk that needs another chunk's output (a function, a flag, a file it creates) runs serially after it, whatever the files say - file-disjoint is not independence.

The proposal names six choices, each with a one-line reason:

| Choice | What it names |
|---|---|
| Implementers | how many, each by its name, and the blocks or chunks each one owns |
| Isolation | which implementers run isolated (only file-disjoint groups that run at the same time) and which run in the task worktree |
| Integration order | the order in which chunks land in the task branch |
| Pipelined | whether an implementer starts its next chunk before the previous one is accepted - the run's commit policy, `pipelined` or `lead`, which the Start preview names |
| Gate policy | the default: `per-commit` (your Review gate before every commit), `high-only` (your gate on Risk high or missing only) or `at-end` (the team flow accepts, you review the finished run) - 5-D.5 dispatches on it |
| Overrides | which choices an owner flag fixed |

```
### Launch plan

| Choice            | Proposal                                  | Why                                           |
|-------------------|-------------------------------------------|-----------------------------------------------|
| Implementers      | 1: impl-{RUN} owns c1-c4                  | c1-c4 all touch a.md                          |
| Isolation         | impl-{RUN} in the task worktree           | nothing runs at the same time                 |
| Integration order | c1, c2, c3, c4                            | plan order; c3 calls what c2 adds             |
| Pipelined         | no                                        | c1 and c2 are Risk high - review each first   |
| Gate policy       | per-commit                                | two high-risk chunks                          |
| Overrides         | none                                      | -                                             |
```

**Owner overrides.** `--parallel`, `--pipelined` and `--gate <policy>` each fix one choice (the parallel shape, pipelining, the gate policy); the proposal keeps the fixed value and its Why reads `fixed by --<flag>`. The proposal never contradicts a flag - a flag that cannot hold for this plan (e.g. `--parallel` with no file-disjoint groups) is said in one line, and the run does not start until the owner drops or changes it.

**The proposal is the Start option's `preview`**, above the Step 4.6 split table and its warnings, so the owner accepts exactly what they see - the launch plan, the split and the warnings in one view.

**Change the launch plan** (the Start gate's option for it, 5-D.2) -> follow-up questions change the implementers and their groups, the isolation, the pipelining and the gate policy; recompute the preview (every changed choice's Why reads `set by you`), then show the Start gate again. Nothing is spawned before Start.

## Step 5: Enter Native Plan Mode

Present the plan summary to the user, then enter plan mode.

```
## Ready to Execute

**Plan:** {plan title}
**Tasks:** {N} implementation tasks
**Key files:** {top 3-5 files from plan}

Entering plan mode to create an execution strategy.
```

Call `EnterPlanMode`.

**In plan mode**, Claude:
1. Reads the existing plan from `.lets/plans/`
2. Reads the current state of files referenced in the plan (to detect drift)
3. Creates an execution strategy - adapting the plan's tasks to the actual codebase state (restructuring HERE is legitimate because `ExitPlanMode` puts it in front of the user; after approval, any approach change is a deviation - gate below)
4. Writes the execution strategy to the plan file (the file specified by plan mode)
5. Calls `ExitPlanMode` when ready for user approval

**After user approves**, Claude implements per the **Step 4.5 mode**: *step-by-step* implements one task, pauses for review before the next, and confirms each commit; *straight-through* runs all tasks and `/lets:commit`s at each plan commit point without re-asking; *auto* is the `--auto` behavior below. Commit only at the plan's commit points.

**Under `--auto` (inline runs only - Step 4.5 refuses `--auto` for Implementers):** write the `executing` pipeline-state marker, then — the plan-mode approval IS the gate — implement straight through without a per-step "review before moving on" pause, and `/lets:commit` at each plan commit point WITHOUT re-asking (one approval covers the run). Hard-stops still halt (push/PR/close/external gated; 3×-fail; fabrication; `$LETS_MERGE_BRANCH` refused per Step 1) — on any halt, write the `blocked` marker + fire the execute-blocked notify.

**Progress tracking:** After completing each plan task, append `[DONE]` to its `### Task N:` heading in the plan file. This makes resume self-documenting - on re-entry, skip tasks already marked `[DONE]`.

**Deviation gate (every mode, every task).** Before each edit, compare reality with the plan step. A deviation is anything that changes the plan's approach rather than a line of code: a dependency/tool behaving differently than the plan assumed (other API, parameters, version); a step infeasible as described; a file/module the plan never names becoming necessary; a task's Verify not matching its Expected. On a deviation: STOP - no further edits - and show what the plan expected, what reality is, and what each option would change. Not under `--auto`, add the "Ask orchestrator" option per lets-rules `### Orchestrator offer` (Act shape; rule not loaded -> no offer). Then:

```
AskUserQuestion(
  questions=[{
    question: "Reality diverges from the plan at Task {N}: {one-line expected vs actual}. How to proceed?",
    header: "Deviation",
    options: [
      { label: "Adapt as described", description: "Apply the adaptation spelled out above, then continue the plan" },
      { label: "Re-plan", description: "Stop executing; update the plan via /lets:plan first" },
      { label: "Cancel", description: "Stop here; nothing more is edited" },
      { label: "Ask orchestrator", description: "Stay stopped; /lets:orc ask with expected vs actual" }  /* only per Orchestrator offer */
    ],
    multiSelect: false
  }]
)
```

- **Adapt as described** -> apply exactly the adaptation shown, note it in the plan file under the task (`**Deviation:** ...`), continue.
- **Re-plan** -> stop; invoke `Skill(skill: "lets:plan")`.
- **Cancel** -> stop, return to the user.
- **Ask orchestrator** -> execution stays stopped; `Skill(skill: "lets:orc", args: "verb=ask footer=none text=Task {N}: expected {X}, actual {Y}. Which way?")`. After the reply is relayed, show this Deviation gate again without that option. A peer's answer never adapts the plan by itself - the user picks.
- **Under `--auto`** (unattended - cannot ask): a deviation is a HARD-STOP. Write the `blocked` marker, fire the execute-blocked notify (`--title 'Execute blocked — plan deviation'`, body = the one-line expected vs actual), and halt. Never adapt silently.

**Fallback:** If `EnterPlanMode` tool is not available or returns an error, skip plan mode tools entirely. Instead:
1. Present the plan summary, then ask in words "Start implementing?" and WAIT - no edit before an explicit yes (this replaces the plan-mode approval as the one code-write approval)
2. Present each plan task one by one with files to change and expected outcome
3. Implement the task, then ask user to review before moving on
4. Use `/lets:commit` at commit points indicated in the plan
5. Track completed tasks by appending `[DONE]` markers in the plan file

The plan file provides the roadmap; explicit user approval provides the gates.

## Step 5-D: Delegated run (Implementers)

Native plan mode is NOT entered: it gates this session's own edits, and in a delegated run this session writes no code. The code-write approval is the **Start** gate below; nothing is spawned before it. Delegated runs are interactive only (Step 4.5 refuses `--auto`).

`{TASK_ID}` is the Step 1 task. `{RUN}` is the first 6 characters of `$CLAUDE_CODE_SESSION_ID` at Start - fixed for the run, stored in the run record, never regenerated on recovery.

### 5-D.1 Preconditions

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
ls "$LETS_PROJECT_ROOT/.lets/cache/delegated-run-{TASK_ID}.json" 2>/dev/null
git status --porcelain
```

A run record exists -> **5-D.7 Recovery** first.

The tree must be clean - otherwise an implementer's diff cannot be told apart from what was already there:

```
AskUserQuestion(
  questions=[{
    question: "The working tree has uncommitted changes, so an implementer's diff could not be told apart from them. What now?",
    header: "Dirty tree",
    options: [
      { label: "Commit them first (Recommended)", description: "Run /lets:commit for the existing changes, then check again" },
      { label: "Run inline instead", description: "Execute this plan here in native plan mode (Step 5)" },
      { label: "Cancel", description: "Stop; nothing is spawned" }
    ],
    multiSelect: false
  }]
)
```

### 5-D.2 Start - the one code-write approval

```
AskUserQuestion(
  questions=[{
    question: "Run the launch plan: {N} chunk(s) by {K} implementer(s), gate policy {gate_policy}? {M} caller task(s) run here.",
    header: "Start work",
    options: [
      { label: "Start (Recommended)", description: "Pick the model, then spawn; commits follow the gate policy shown", preview: "{the Step 4.7 launch plan, then the Step 4.6 split table and its warnings}" },
      { label: "Change the launch plan", description: "Change implementers, isolation, pipelining or the gate policy; this gate returns" },
      { label: "Run inline instead", description: "Execute this plan here in native plan mode (Step 5)" },
      { label: "Cancel", description: "Stop; nothing is spawned and nothing is edited" }
    ],
    multiSelect: false
  }]
)
```

Four options is the cap: the parallel shape, pipelining and the gate policy are flags or launch-plan choices, never another option, and this gate carries no orchestrator offer (run mode is session mechanics).

- **Start** -> record the run's `gate_policy`. With `high-only` or `at-end`, this pick is the owner's explicit, recorded approval for the lead's commits in this run - the commits 5-D.5 makes without a Review gate. With `--pipelined`, Start is also the owner's approval for the implementer's LOCAL commits on the branch it works on (the lead's integration commits, for an isolated group) - the recorded exception to "committed only after Accept"; record `commit_policy: pipelined`. Nothing is ever pushed. The model is chosen by the first spawn (the `member-run` panel), after this approval - never before it.
- **Change the launch plan** -> the Step 4.7 follow-up questions, then this gate again with the recomputed preview.
- **Run inline instead** -> Step 5.
- **Cancel** -> stop; nothing is spawned.

### 5-D.3 Run record

Before the first spawn, write `.lets/cache/delegated-run-{TASK_ID}.json` (`.lets/` is shared by every worktree, so the name carries the task id). `.lets/` is a symlink into the main checkout, so the Write / Edit tools ask a permission on every write there, and the record is rewritten at every transition. So every file this run writes under `.lets/` whose content THIS session authors - the record, briefs, amendments, the plan's `[DONE]` marks - goes through Bash (`cat > <file> <<'EOF'`, or `sed -i` for a mark). A report is the one exception: its text is the agent's, and a line in it that matches the heredoc delimiter would end the heredoc and run the rest as shell. Write a report with the Write tool (one permission per report), never through a shell command:

```json
{
  "task": "{TASK_ID}",
  "plan": "{plan path from Step 2}",
  "run": "{RUN}",
  "shape": "solo",
  "gate_policy": "per-commit",
  "commit_policy": "lead",
  "pre_rebase_head": null,
  "members_scope": "run-{RUN}",
  "model": null,
  "caller": [ { "task": 0, "state": "pending" } ],
  "groups": [ { "id": "a", "agent": "impl-{RUN}", "generation": 1, "isolation": null, "base": null, "agent_id": null, "agent_branch": null, "agent_worktree_path": null, "integrated_source": null } ],
  "chunks": [
    { "id": "c1", "tasks": [1, 2], "group": "a", "risk": "high", "allowlist": ["path/a", "path/b"], "allowlist_amendments": [], "agent": "impl-{RUN}", "generation": 1, "base": null, "agent_branch": null, "agent_worktree_path": null, "commit_sha": null, "fixups": [], "review_sha": null, "accepted_sha": null, "picked_sha": null, "patch_path": null, "phase": "pending", "round": 0, "report": null, "status": null, "reason": null, "received": null, "patch_sha": null, "check": null, "skeptic": null, "accepted_by": null, "committed_by": null, "commit": null }
  ]
}
```

Run fields: `shape` is `solo` or `parallel` (the launch plan's); `gate_policy` is `per-commit` (default), `high-only` or `at-end` (the Start pick); `commit_policy` is `lead` (default) or `pipelined`, and `pre_rebase_head` is recorded before a 5-D.9 autosquash; `members_scope` names the `lets members` scope every agent of the run is registered in. Per group: the agent that owns it now and its `generation`; for an isolated group (`isolation: worktree`) its `base` (the task branch's HEAD at Start), and the `agent_id`, `agent_branch` and `agent_worktree_path` the spawn returned - `agent_id` mirrors the registry's for recovery only, `lets members` stays the record member-run reads; and `integrated_source` - the last agent-branch commit landed in the task branch; it moves only after the lead's commit, never on a report or a pick. Per chunk: `risk` from Step 4.6 (missing = high); `agent_branch`, `agent_worktree_path`, `commit_sha` (the latest `**Commit:**` sha the agent reported), `picked_sha` and `patch_path` belong to an isolated group's integration and stay `null` in the task worktree; under `pipelined` the record is keyed by sha: `commit_sha`, `fixups[]` (each `git commit --fixup` sha, in order), `review_sha` (the last sha the review showed) and `accepted_sha` (the commit and its fixups, as accepted); `check` and `skeptic` hold the 5-D.5 verdicts; `accepted_by` is `owner` or `team`; `committed_by` is `lead`, or `implementer` only under pipelining; `allowlist_amendments[]` holds `{path, by, reason}` for every path added to the allowlist mid-run (5-D.5 Correct). Liveness is never stored here - `lets members status` is its one judge.

`phase` moves `pending` -> `running` -> `review` -> (`correcting` -> `running` -> `review`)* -> `committing` -> `accepted`. A Stop at a review gate records `paused` (it reopens as `review`); a Stop while an agent works goes through `stopping` to `blocked`; `blocked` always carries a `reason` - `stopped`, `re-plan`, `unreachable` or `unrecognized-commits`. Caller tasks move `pending` -> `running` -> `done`. `report` is the path of the latest round's saved report, `status` and `reason` are read from it, `received` counts reports in arrival order across the run, and `generation` is 1 for the first agent of a chunk and grows with each replacement. Rewrite the record at every transition BEFORE acting on it, so an interrupted session always finds the state it was in.

### 5-D.4 Dispatch - in plan order

Walk the Step 4.6 split:

- **Caller task** -> wait until every group has no chunk open, record its `state: running`, run it here, now, exactly as the plan writes it, then record `state: done`.
- **Chunk** -> record `base` = `git rev-parse HEAD` and `phase: running`, write its brief, hand it to its group's agent (below), and end the turn - the report arrives as the agent's message (5-D.5 says what counts as one).

The brief, `.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md`:

```
MODE: {solo | isolated | pipelined}
TASK: {TASK_ID}   PLAN: {plan path}   RUN: {RUN}   CHUNK: {chunk} (group {group})
GOAL: {the plan's Goal line}
APPROACH: {the plan's Approach line}
KEY DECISIONS (all bind you):
{the plan's Key Decisions section, verbatim}
OUT OF SCOPE:
{the plan's Non-Goals section, verbatim}
YOU MAY WRITE ONLY:
{the chunk's allowlist, one path per line}
CORRECTIONS: your reviewer may send corrections for this chunk. Each arrives as an AMENDMENT to this brief and changes what it names, within YOU MAY WRITE ONLY - or within a path the AMENDMENT itself adds to it; one that needs any other file is a deviation.
BASE: {base sha}
{isolated only, and only in the group's FIRST brief:}
CALLER_TOPLEVEL: {git rev-parse --show-toplevel of this session}
MAIN_ROOT: {main_root from lets worktree info --json}
{isolated or pipelined, in every brief:}
COMMIT MESSAGE: {the chunk's plan commit message, with its Task: footer}
TASKS:
{the chunk's ### Task sections verbatim, each **Commit:** block removed - its message is COMMIT MESSAGE where the agent commits; in `solo` this session commits after review}
CI CHECKS: run each from the repository root before your report; a failing one is never `complete`:
{the Step 4.6 CI commands, one per line}
PROJECT RULES: read the repository's CLAUDE.md before editing.
REPORT: fill in exactly this skeleton and send it as your final message - nothing before it, nothing after it. Status is one of `complete`, `deviation-stopped`, `blocked`; keep only the block that matches it:
### Chunk: {chunk}
**Status:** `complete`
**Files changed**
- `path` - what changed
**Verify**
Command: `...`
Output:
(raw output)
Result: `pass`
**Tree**
Before: `...`
After: `...`
**Deviation** (deviation-stopped only) - Plan expected / Reality / Options
**Blocked** (blocked only) - What / Why
```

In an isolated brief `BASE` is the literal group base - the task branch's HEAD sha at Start - that the agent's guard switches to; `CALLER_TOPLEVEL` is the tree it must never write. A group's later briefs (`op=next`) carry `MODE: isolated` and no base instruction: the agent never switches or resets again. An isolated agent's report adds a `**Commit:** <sha>` line - its chunk's commit, or its fixup's on an amendment. Under `--pipelined` the in-tree group's briefs carry `MODE: pipelined`, and its reports carry the same line.

**One persistent implementer per group.** In the solo shape - the shape the 4.7 proposal usually picks when chunks share files or build on each other - the run has one group and one agent, `impl-{RUN}`, working in this tree (the task worktree). The group's agent is spawned once, for its first chunk; every later chunk of the group goes to the same agent as its next brief, so it keeps its context from chunk to chunk. Each brief is complete on its own - the agent's memory helps, the brief is the contract.

- The group's first chunk: `Skill(skill: "lets:member-run", args: "op=spawn scope=run-{RUN} role=lets:implementer name={agent} brief-file=.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md")`, adding ` model=<m>` once the record holds a model. After the first spawn of the run, write the returned model into the record.
- An isolated group's first chunk (parallel shape, a group the launch plan runs isolated): `Skill(skill: "lets:member-run", args: "op=spawn scope=run-{RUN} role=lets:implementer isolation=worktree name=impl-{RUN}-{group} brief-file=.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md")`. Record what it returns in the group (`agent_id`, `agent_branch`, `agent_worktree_path`) - the harness reaches an isolated agent only by its `agent_id`, and member-run addresses it by that.
- Every later chunk of the group: `Skill(skill: "lets:member-run", args: "op=next scope=run-{RUN} name={agent} brief-file=.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md")`. It returns `agent_gone` -> record `phase: blocked`, `reason: unreachable`, then the 5-D.7 Replacement gate - a replacement always runs under a visibly new name, never as the same agent.

One chunk per group is live at a time; in the parallel shape the groups run at the same time.

**One writer per tree.** The task worktree has one writer at a time: its in-tree group's agent while that chunk is `running` or `correcting`, or this session - which writes it only through `lets integrate` and the lead's commit, and only while the tree is clean. An isolated agent writes only its own worktree. A request to edit meanwhile -> say which `{agent}` is still working, and offer to wait for its report or to stop it (5-D.6).

### 5-D.5 Review - one report at a time

**What is a report.** The FIRST text from `{agent}` after its latest spawn, next brief or correction - a message, or an idle notification's `result` - that carries a `**Status:**` line holding `complete`, `deviation-stopped` or `blocked`. Anything later in the same round is a repeat of it, Status line or not: record nothing. An idle notification with no such line is NOT a report - the agent may still be working: keep the phase and end the turn. A message offered as the report whose Status line is missing or holds any other value IS a report with status `blocked`, reason `malformed-report` - never infer a status from prose.

When `{agent}` reports, save it verbatim to `.lets/cache/report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round}.md`, then record `phase: review`, `report` (that path), `status` and `reason` (read from its **Status** and its **Blocked** / **Deviation** block) and `received` (one more than the highest in the record). Reports are handled one at a time: a report that arrives while a gate is open is saved and recorded, and handled after that gate.

**Isolated chunk - integrate first.** An isolated group's `complete` report is landed in this tree before any check or gate, and only while the tree is clean (an in-tree chunk that is running waits for its commit first):

1. `lets members status --scope run-{RUN} --name {agent} --json` - `live` is the normal case; a gone agent's branch can still be integrated (its context is lost; a correction then needs the 5-D.7 replacement).
2. `lets integrate --from {commit} --since {since} --run {RUN} --chunk {chunk} --json`, where `{commit}` is the reported `**Commit:**` sha (recorded first as the chunk's `commit_sha`) and `{since}` is the group's integrated_source, or its base. Record `picked_sha` (the last of `picked`) and `patch_path` on the chunk, and `agent_branch` / `agent_worktree_path` from the group.
3. Exit 50 (conflict) -> the caller tree is untouched: the `blocked` gate with the conflicting files - **Correct** (an AMENDMENT to that agent) or **Stop**. Any other failure -> show the error kind as-is, the `blocked` gate.
4. Success -> run the brief's CI CHECKS here, in this tree, and add their output to the review block; then the tree check below and the Accept dispatch, as for any chunk. `integrated_source` moves to that `--from` sha only after the lead's commit (Accept).

A **Correct**, **Re-plan** or **Stop** that rejects an integrated, uncommitted chunk first takes its patch back out: `lets integrate --revert --patch {patch_path} --json`; exits 58 / 59 are shown as-is and nothing else runs until the tree is sorted out. The agent's fixup then comes back as a new report and is integrated from the same `--since`.

**Pipelined run (`commit_policy: pipelined`) - review by sha.** Start approved local commits, so the review reads commits, never a working diff:

- **Move on at once.** A `complete` report with a `**Commit:**` sha -> record `commit_sha`, then send the group's next chunk (`op=next`) right away - the next chunk starts before this one is accepted - and review this one.
- **In-tree group** (`MODE: pipelined`, `committed_by: implementer`). The implementer is the only writer of the tree, so this session writes and stages nothing there, and the tree check below is replaced by a check of the commits: the review block is `git show {commit_sha}`, then `git show {each fixup}` - read-only, by sha. Every file they touch must be in the chunk's allowlist, each commit's parent must be the previous chunk's last sha (or `base`), and each message must carry `Task: {TASK_ID}`; anything else -> the `blocked` gate. Record `review_sha` = the last sha shown; it takes the place of `patch_sha`.
- **Isolated group** (with `--parallel`). The agent commits per chunk and moves on; each report is integrated as the "Isolated chunk" steps above say and committed by the lead at once (`committed_by: lead`, `integrated_source` = that `--from` sha after the commit), then reviewed by `git show` of the lead's commit. An agent's fixup is integrated the same way and committed by the lead as `fixup! <that chunk's commit subject>`, recorded in `fixups[]`.
- **Correct** -> the AMENDMENT names the sha: "fix `{commit_sha}` - stage only the fix's paths and `git commit --fixup={commit_sha}`". A fix path that the agent's in-progress chunk also changes -> it reports `blocked` with reason `overlap`: wait for that chunk's report (its commit), then send the same amendment again.
- **Accept makes no commit**: record `accepted_sha` (the commit and its fixups), `accepted_by`, and `[DONE]` as usual. The team check and the Accept dispatch run as for any chunk, on the commits.
- **A reject no correction can fix** stops the pipeline: no further `op=next`. Once the running chunk has reported (or 5-D.6 stopped it), offer `git revert --no-edit <sha>` for the rejected commit and each of its fixups - a new commit, only on the owner's yes, never a reset.

Check the real tree - the report is the agent's claim, the diff is the fact:

```bash
git rev-parse HEAD                                    # must equal the chunk's recorded base
git status --porcelain --untracked-files=all          # every listed path must be in the chunk's allowlist; new directories are expanded to their files
git diff --cached --name-only                         # in-tree chunk: must print nothing (an implementer never stages); integrated chunk: exactly the integrate result's files
git diff HEAD                                         # the patch of every tracked file, index and worktree alike
git ls-files --others --exclude-standard -z           # untracked files, NUL-separated so any file name is safe
```

For each untracked path, show it as a patch: `git diff --no-index -- /dev/null "<path>"` (read-only; nothing is staged). Its exit status 1 means a patch was printed; only a status above 1 is an error. HEAD moved, anything staged (beyond an integrated chunk's own files), or a path outside the allowlist changed -> the diff is NOT attributed to the agent: name the paths and use the `blocked` gate. An amendment cannot clean a path outside the allowlist (amendments stay inside it), so say plainly that those paths are the user's to remove or restore; once they are gone, `/lets:execute` re-checks the tree and, for a `complete` report, offers Accept.

**Team check - every `complete` report whose tree check is clean, before any gate.** Save the full patch (as Render review builds it) to `.lets/cache/patch-{TASK_ID}-{RUN}-{chunk}-r{round}.diff`, then:

**Who checks.** In a team worktree (`lets worktree info --json` -> `worktree.team` set) the standing team's live `explorer` / `skeptic` do it, through `member-run` `op=next scope=<callsign> name=explorer|skeptic`. Only when the team has no live one - `lets members status --scope <callsign> --name <name>` not `live` / `rotated` - or outside a team worktree, the run's own `explorer-{RUN}` / `skeptic-{RUN}` do it (`member-run` `op=spawn scope=run-{RUN}` the first time, `op=next` after). Every hand-over is a brief file naming the chunk's brief, the saved report and the saved patch. No model is passed for either role: they inherit the session model (no upward name pin).

- **CHECK** - every policy, every Risk. The explorer (`role=lets:explorer`) checks the patch against the brief: every step done, nothing beyond it, the report's CI CHECKS output present and passing. Record `check`: `clean`, or its findings.
- **Skeptic** - Risk high or missing, every policy. The skeptic (`role=lets:skeptic`) tries to refute the patch against the brief. Record `skeptic`: `confirmed`, or its findings. Risk low -> `skeptic: not-run`.

**Accept dispatch - the ONE table.** `gate_policy` x the chunk's `risk` decides who accepts, who commits and where the owner stops:

| gate_policy | risk | CHECK | skeptic | accepted_by | committed_by | the owner stops |
|---|---|---|---|---|---|---|
| `per-commit` (default) | any | yes | yes on high / missing | owner (Review gate) | lead | at every chunk |
| `high-only` | high / missing | yes | yes | owner (Review gate) | lead | at this chunk |
| `high-only` | low | yes | no | team | lead | only on a deviation; the 5-D.8 run review |
| `at-end` | high / missing | yes | yes | team | lead | the 5-D.8 run review |
| `at-end` | low | yes | no | team | lead | the 5-D.8 run review |

The table is the extension point: a new policy is a new row, never a new code path. The skeptic runs whenever Risk is high or missing, in every policy. `committed_by: implementer` exists only under pipelining.

- **owner** -> Render review and the status gate below; its Accept records `accepted_by: owner`.
- **team** -> both verdicts clean (`check: clean`, and `skeptic: confirmed` where it ran) -> compute `patch_sha` as Render review does, without a gate, then the Accept steps below with `accepted_by: team`: the owner's Start pick of this policy is the approval for that commit. A finding the lead can fix inside the plan and the chunk's allowlist -> Correct, with the lead writing the AMENDMENT, and the team check runs again on the next report. A finding the lead cannot resolve inside the plan - a skeptic finding included - is a deviation: Render review and the `deviation-stopped` gate, now.
- **Hard stops and deviations halt at once in every policy**, `at-end` included: a `deviation-stopped` or `blocked` report, a failed tree check, an unresolvable finding or a hard stop (lets-rules `## AUTO MODE`) goes to the owner's gate now, never to the run review.

Present it in the gate itself - the block below is the `preview` of the status gate's first option, because a tool result is shown collapsed and prose before a gate is skipped in practice - under the agent's name, the name this run spawned it under, never a name the agent wrote about itself - with the report verbatim and then the full patch (`git diff HEAD`, then each untracked file):

```
### {agent} - {chunk} - {status}
{the report, verbatim}
{the full patch}
```

The stat is validation; the patch is what the user reviews. Record `patch_sha` - the sha256 of the full patch exactly as shown (`git diff HEAD`, then each untracked file's patch, in path order) - so Accept can prove it commits that patch and nothing else. Then ask the gate for that status.

**Render review** = the tree check, a fresh `patch_sha`, then the status gate carrying the heading, the report verbatim and the full patch as its first option's `preview` - always in that order. Every path that shows a review gate runs it: a new report, a 5-D.7 recovery of `review` / `paused`, and an Accept that found the patch changed. No path asks a Review gate without first recording the sha of what it showed.

- **Orchestrator offer** - when it applies (lets-rules `### Orchestrator offer`, Act shape; rule not loaded -> no offer), add as the last option `{ label: "Ask orchestrator", description: "Stay at this gate; /lets:orc ask with the report and the patch" }`

**`complete`, every Verify `pass`, diff check clean:**

```
AskUserQuestion(
  questions=[{
    question: "{agent} completed chunk {chunk}. Accept its diff?",
    header: "Review",
    options: [
      { label: "Accept (Recommended)", description: "Commit exactly this chunk's files now; the next item of the plan starts", preview: "{the review block}" },
      { label: "Correct", description: "Send {agent} an amendment; it keeps its context" },
      { label: "Stop", description: "Pause here; the diff stays uncommitted and /lets:execute reopens this review" }
    ],
    multiSelect: false
  }]
)
```

**`deviation-stopped`:**

```
AskUserQuestion(
  questions=[{
    question: "{agent} stopped at a deviation in chunk {chunk}: {one-line expected vs actual}. How to proceed?",
    header: "Review",
    options: [
      { label: "Correct", description: "Send the adaptation you choose as an amendment, within its allowed files", preview: "{the review block}" },
      { label: "Re-plan", description: "Stop; update the plan via /lets:plan" },
      { label: "Stop", description: "Pause here; the diff stays uncommitted and /lets:execute reopens this review" }
    ],
    multiSelect: false
  }]
)
```

**`blocked`, or a failed diff check:**

```
AskUserQuestion(
  questions=[{
    question: "{agent} is blocked on chunk {chunk}: {reason}. What now?",
    header: "Review",
    options: [
      { label: "Correct", description: "Send an amendment that gets it past the block; it keeps its context", preview: "{the review block}" },
      { label: "Stop", description: "Pause here; the diff stays uncommitted and /lets:execute reopens this review" }
    ],
    multiSelect: false
  }]
)
```

- **Accept** -> re-run the tree check above and recompute the patch sha first; a failed check, or a sha that differs from the recorded `patch_sha` -> Render review again (it records the new sha), never commit what was not reviewed. Then record `phase: committing`, stage exactly the chunk's allowlist paths (untracked ones included) and `Skill(skill: "lets:commit", args: "approved=review-accept")` - the owner's Accept pick, or under team acceptance the owner's Start pick of the policy, is its approval, so the skill does not ask again. Then record `phase: accepted`, `commit`, `accepted_by` and `committed_by: lead`, append `[DONE]` to each of the chunk's `### Task` headings in the plan file, and continue 5-D.4 with the next item.
- **Correct** -> ask in words what to change, then:

  ```
  AskUserQuestion(
    questions=[{
      question: "Does the fix need a file outside the chunk's allowlist?",
      header: "Allowlist",
      options: [
        { label: "No", description: "The amendment stays inside the chunk's allowed files" },
        { label: "Yes", description: "The architect weighs the path first; you confirm before it is added" }
      ],
      multiSelect: false
    }]
  )
  ```

  **Yes** -> ask in words for the path and why. Route it to an architect by the same rule as the team check - the standing team's live `architect` (`op=next scope=<callsign> name=architect`), else the run's `architect-{RUN}` (`op=spawn scope=run-{RUN} role=lets:architect` the first time, `op=next` after; no model passed) - with a brief naming the chunk, the path and the reason, then show its answer and confirm:

  ```
  AskUserQuestion(
    questions=[{
      question: "Add {path} to chunk {chunk}'s allowlist? The architect says: {one-line verdict}.",
      header: "Addendum",
      options: [
        { label: "Add it", description: "Record the addendum, then send the amendment" },
        { label: "Re-plan", description: "Stop; update the plan via /lets:plan" },
        { label: "Cancel", description: "Send nothing; this review gate returns" }
      ],
      multiSelect: false
    }]
  )
  ```

  **Add it** -> append to the plan file (through Bash) a `## Allowlist addendum` section when there is none, and under it `- {chunk}: {path} - by: {architect|owner} - {reason}` (`architect` when you took its recommendation, `owner` when you decided against or without it); record `{path, by, reason}` in the chunk's `allowlist_amendments[]` and add the path to its `allowlist`. Only then write the amendment, which names the added path. **Re-plan** -> as below. **Cancel** -> show the review gate again.

  Write it to `.lets/cache/correct-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round+1}.md` framed as an amendment to the brief:

  ```
  AMENDMENT to chunk {chunk}, round {round+1} - it changes what it names and nothing else:
  {the correction, verbatim}
  ```

  Record `phase: correcting` and `round+1`, then `Skill(skill: "lets:member-run", args: "op=correct scope=run-{RUN} name={agent} correct-file=<that file>")`. It returns `sent correct to {agent}` -> record `phase: running` and end the turn. It returns `agent_gone` -> record `phase: blocked` and `reason: unreachable`, then 5-D.7. A path outside the allowlist goes through the Allowlist question and its addendum - never a fifth option on a gate - or the plan is re-planned.
- **Re-plan** -> record `phase: blocked` and `reason: re-plan`, then `Skill(skill: "lets:plan")`.
- **Stop** -> record `phase: paused`, keeping `report`, `status` and `reason` as they are. Say that the uncommitted diff is still in the tree and that `/lets:execute` reopens this same review. Nothing is discarded.
- **Ask orchestrator** -> `Skill(skill: "lets:orc", args: "verb=ask footer=none text={agent} reports {status} on {chunk}: {one-line summary}. {question}")` where `{question}` is the one the shown gate asks - "Accept, correct or stop?" for `complete`, "Correct, re-plan or stop?" for `deviation-stopped`, "Correct or stop?" for `blocked`; then show the same gate again without that option. A peer's answer never decides.

### 5-D.6 Stop while an agent is running

A request to stop - or to edit code - while a chunk is `running` or `correcting`: record `phase: stopping`, then `TaskStop(task_id="{agent}")`. The one-writer rule holds until the stop is confirmed: `TaskStop` reports success, or `ListAgents` no longer shows `{agent}` as busy. Then record `phase: blocked` and `reason: stopped`, and show what the agent left (`git status --porcelain --untracked-files=all`). Nothing it wrote is discarded. `TaskStop` fails and the agent still shows busy -> say so and keep `stopping`; start no other code-writing flow.

### 5-D.7 Recovery (a run record exists)

Read the record. Its `plan` is not this run's plan -> ask whether to resume that run or discard its record (deleting the record file changes no code).

**Liveness comes from `lets members status --scope {members_scope} --json`**, never from the record: a member `gone` or `unknown` counts below as `{agent}` not listed; `ListAgents` only tells a live member busy from idle. **A pre-upgrade record** - no `members_scope`, or no `.lets/execution/members-run-{RUN}.json` - proves no member: treat every agent name it records as gone (`unknown_pre_upgrade`) and go to the Replacement gate; such a member is never messaged.

**Caller tasks first.** A caller task recorded `state: running` may have done its work before the interruption; it is never re-run by itself:

```
AskUserQuestion(
  questions=[{
    question: "Caller task {N} ({task title}) was running when the session ended and may already have run. What now?",
    header: "Caller task",
    options: [
      { label: "Mark it done", description: "It finished before the interruption; continue with the next item" },
      { label: "Run it again", description: "Repeat it now, exactly as the plan writes it" },
      { label: "Stop", description: "Leave the run as it is; nothing changes" }
    ],
    multiSelect: false
  }]
)
```

Then, for each chunk not `accepted`, in plan order:

| phase | check | then |
|---|---|---|
| `pending` | - | continue 5-D.4 from here |
| `running` / `correcting` | `ListAgents` shows `{agent}` busy | it is still working: end the turn and wait for its report |
| `running` / `correcting` | `ListAgents` shows `{agent}` idle | its report was lost with the old session, or it is waiting on something: write `.lets/cache/resend-{TASK_ID}-{RUN}-{chunk}.md` ("Report on chunk {chunk} now: resend your last report unchanged, or, if you have not finished, send a report saying why."), `member-run` `op=next` with it, and handle the reply as 5-D.5 |
| `running` / `correcting` | `{agent}` not listed | record `phase: blocked`, `reason: unreachable`; the Replacement gate |
| `stopping` | - | 5-D.6 again |
| `review` / `paused` | the `report` file exists | Render review (5-D.5) from the saved report, with its ORIGINAL status gate - Accept included when it was `complete` and the check is clean; a recorded `patch_sha` of `null` is simply replaced. `ListAgents` does not list `{agent}` -> a correction needs the agent that wrote the diff: `complete` shows its gate without **Correct** (Accept, Stop), `deviation-stopped` without **Correct** (Re-plan, Stop), and `blocked` goes to the Replacement gate instead |
| `committing` | `git log --format=%H {base}..HEAD` | exactly one new commit whose message carries `Task: {TASK_ID}` and whose `git show --name-only --format= <sha>` lists only allowlist paths -> record `accepted` and `commit`, mark `[DONE]`; no new commit -> record `review` and show the gate again; anything else -> record `blocked`, `reason: unrecognized-commits` |
| `blocked`, `reason: re-plan` | - | say the chunk went to re-planning; after `/lets:plan`, `/lets:execute` starts a run of the new plan |
| `blocked`, `reason: stopped` or `unreachable` | - | the Replacement gate |
| `blocked`, `reason: unrecognized-commits` | - | show `git log --oneline {base}..HEAD`; stop - the user sorts the commits out |

**Replacement gate.** A replacement agent refuses to start in a dirty tree (its own clean-start rule), so check first: `git rev-parse HEAD` against `base`, and `git status --porcelain --untracked-files=all`.

HEAD at `base` and the tree clean:

```
AskUserQuestion(
  questions=[{
    question: "{agent} is gone and chunk {chunk} has no changes in the tree. Start a replacement?",
    header: "Recovery",
    options: [
      { label: "Start a replacement (Recommended)", description: "A new agent with the brief and every amendment so far; it has no memory of the first" },
      { label: "Stop", description: "Leave the run blocked; nothing changes" }
    ],
    multiSelect: false
  }]
)
```

HEAD at `base`, nothing staged, and only allowlist paths changed - an unfinished diff the first agent left. Show that patch first (as 5-D.5 renders one, headed `### {agent} (gone) - {chunk} - unfinished`), then:

```
AskUserQuestion(
  questions=[{
    question: "{agent} is gone and left the unfinished diff above for chunk {chunk}. What now?",
    header: "Recovery",
    options: [
      { label: "Discard it and replace", description: "Remove this diff (asked again, naming every file), then start a replacement" },
      { label: "Keep it and stop", description: "Leave the diff and the run blocked; finish or discard it yourself" }
    ],
    multiSelect: false
  }]
)
```

HEAD moved, anything staged, or a path outside the allowlist changed -> offer neither: show what changed and stop.

- **Start a replacement** -> `generation+1` on the chunk and its group; the new name is `impl-{RUN}-r{generation}` (an isolated group's: `impl-{RUN}-{group}-r{generation}`), and it becomes the group's agent for its later chunks (`op=next`). An isolated group's replacement is spawned isolated in a new worktree: first integrate anything the gone agent reported and never integrated - `--from` the chunk's recorded `commit_sha`, never the branch tip (a sha outlives a removed branch until gc, and a tip can hold commits nobody reviewed) - then set the group's `base` to the task branch's HEAD, `integrated_source` to `null`, and its `agent_id` / `agent_branch` / `agent_worktree_path` from the new spawn; write `.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}-g{generation}.md` = the original brief plus an `AMENDMENTS SO FAR:` section holding every correction file of the chunk, oldest first; record `agent`, `generation`, `round: 0`, `phase: running`, and clear the first agent's `report`, `status`, `reason` and `patch_sha` to `null` so no stale round describes the new one; spawn it through `member-run`. Say plainly that it is a new agent.
- **Discard it and replace** -> ask in words, listing every file, before touching anything (destructive). On yes: `git restore --staged --worktree -- <each changed tracked allowlist path>` and `rm -- "<each untracked allowlist path>"`, check the tree is clean, then **Start a replacement**.
- **Keep it and stop** / **Stop** -> leave the record as it is.

A chunk `accepted` with any of its `### Task` headings not yet `[DONE]` -> mark the missing ones; nothing else. Never present a replacement as the same agent, and never discard a diff without the separate confirmation above.

### 5-D.8 Run review (`at-end` and `high-only`)

Every chunk `accepted` and every caller task `done`, under `gate_policy` `at-end` or `high-only` - before completion the owner sees the finished run (under `per-commit` every chunk was already reviewed: go to 5-D.9). The order is Run review -> Completion. Show, as the preview of the first option, the commits in `{start}..HEAD` (`{start}` = the first chunk's `base`: `git log --format='%h %s' {start}..HEAD`), each with its chunk's `risk`, `check`, `skeptic`, `accepted_by`, `committed_by` and any deviation it went through:

```
AskUserQuestion(
  questions=[{
    question: "The run is done: {N} commit(s), {T} accepted by the team. Accept the run?",
    header: "Run review",
    options: [
      { label: "Accept run (Recommended)", description: "Finish the run; nothing is pushed - that is /lets:done", preview: "{the run review table}" },
      { label: "Correct", description: "A fix chunk goes to the implementer; this review returns after it" },
      { label: "Stop", description: "The commits stay, local and unpushed; the run ends here" }
    ],
    multiSelect: false
  }]
)
```

- **Accept run** -> 5-D.9.
- **Correct** -> ask in words what to fix; it becomes a fix chunk (`f1`, `f2`, ... in the record, allowlist = the paths the fix names, `risk` as the owner says, missing = high) with its own brief, sent to the group's agent with `op=next` - the same flow, 5-D.4 and 5-D.5 under the run's gate policy - then this review again.
- **Stop** -> the commits stay, local and unpushed. Offer a revert of a named commit as `git revert --no-edit <sha>` (a new commit, nothing rewritten), run only on the owner's yes. The record stays; nothing is pushed.

Nothing here pushes - that is `/lets:done`'s job.

### 5-D.9 Completion

Every chunk `accepted`, every caller task `done`, and - under `at-end` / `high-only` - the 5-D.8 run review accepted -> autosquash (pipelined only), clean up, then delete `.lets/cache/delegated-run-{TASK_ID}.json` and go to Step 6. The order is Run review -> autosquash -> Completion.

**Autosquash (`commit_policy: pipelined`, any chunk with `fixups[]`).** `{branch}` = `git branch --show-current`; `{start}` = the first chunk's `base`; `{oldest}` = the oldest commit of `{start}..HEAD` (`git rev-list --reverse {start}..HEAD`, its first line).

0. `git rev-list --merges {start}..HEAD` must print nothing: a merge in the range (the lead merged the merge-branch mid-run) brings commits that are already on a remote, and a rebase would re-create them under new shas where the empty-diff check cannot see it. A merge -> stop, no rewrite (the same rule as `lets integrate`), and say why.
1. `lets worktree pushed --branch {branch} --commit {oldest} --json` (every configured remote) must return `state: not_pushed`. `pushed` or `unverified` -> stop: no rewrite, the fixups stay as commits; say why (`head` / `reason`).
2. `git version` must be 2.44 or newer (a non-interactive autosquash); older -> stop the same way.
3. Ask in words, naming `{start}..HEAD` and its fixups; only on the owner's yes: record `pre_rebase_head` = `git rev-parse HEAD`, then `git rebase --autosquash --no-autostash {start}` - non-interactive, never with an editor or a todo list, and never stashing (a `rebase.autoStash` would push onto the stash stack every worktree shares).
4. `git diff {pre_rebase_head} HEAD` must print nothing.

A rebase conflict -> `git rebase --abort`, stop and report it. A non-empty diff in step 4 -> stop and show it; `pre_rebase_head` is recorded, and nothing is reset.

**Isolated groups' worktrees.** For each isolated group, compare its `integrated_source` with the tip of its `agent_branch` (`git rev-parse {agent_branch}`). Not equal, or the branch is gone -> remove nothing; say which commits were never integrated (`git log --oneline {integrated_source}..{agent_branch}`). Equal -> ask in words, naming the branch and the worktree; only on the user's yes, each a separate command, in this order:

1. `member-run` `op=dismiss` FIRST - its stop must be confirmed, so the agent can commit nothing more.
2. Compare `integrated_source` with `git rev-parse {agent_branch}` again, then `git worktree remove {agent_worktree_path}` - plain, never with `--force`: a failure (a dirty agent tree) stops the cleanup here, and it is not retried.
3. Compare them once more, then `git branch -D {agent_branch}`.

Any mismatch at step 2 or 3 -> remove nothing more and list the unintegrated commits.

## Step 6: Record Completion

After implementation is complete (all plan tasks done). **Under `--auto`:** write the `done` pipeline-state marker (the Pipeline-state marker helper, phase `done`) so the statusline / a watcher sees the run finished.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}; [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && SLUG="${TASK_ID}"
PLAN=""  # task-id first (artifact-path naming, lets-05c4s); branch slug = legacy fallback
[ -n "${TASK_ID}" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${TASK_ID}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
[ -z "$PLAN" ] && [ -n "$SLUG" ] && PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | grep -v -E -- '-idea(-v[0-9]+)?\.md$' | head -1)
BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
# Plan execution is TASK-scoped (a plan can run across sessions), so anchor on the task boundary
# start:, NOT session: - session: would under-report every prior session's commits.
START_REF=$(sed -n 's/^start: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}" 2>/dev/null | head -1)
[ -z "$START_REF" ] && START_REF=$(cat "$LETS_PROJECT_ROOT/.lets/sessions/.session-start-ref-${BRANCH_SLUG}" 2>/dev/null)  # back-compat: legacy session ref
# Degrade loud - never silently record "Commits: 0" into a durable tracker comment on a missing/bad boundary.
if printf '%s' "$START_REF" | grep -Eq '^[0-9a-f]{7,40}$' && git rev-parse --verify --quiet "${START_REF}^{commit}" >/dev/null; then
  COUNT=$(git log --oneline "${START_REF}..HEAD" | wc -l | tr -d ' '); LOG=$(git log --oneline "${START_REF}..HEAD")
else
  COUNT="unknown (task boundary not found)"; LOG="(commit range unavailable - no valid start: boundary in .task-${BRANCH_SLUG})"
fi
mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
cat > "$LETS_PROJECT_ROOT/.lets/cache/exec-complete-<task-id>.md" <<EOF
## Plan execution complete $(date +%Y-%m-%d)

Plan: ${PLAN:-(none found)}
Commits: ${COUNT}

${LOG}
EOF
```

```lets-tracker
comment-add task=<task-id> body-file=.lets/cache/exec-complete-<task-id>.md
```

## Rules

- **NEVER execute blindly** - read actual file state before every change
- **NEVER commit without user approval** - use `/lets:commit` at plan commit points
- **Don't work on `$LETS_MERGE_BRANCH` without explicit opt-in** - Step 1 soft-gates with a prompt; user must pick "Continue here" to enable trunk-mode (plan lookup uses task-id)
- **Adapt cosmetically, never structurally** - plan intent matters more than plan text, but an approach change is a deviation, not an adaptation
- **Stop on deviation** - the Deviation gate (Step 5) runs before every edit; no answer = no edit; under `--auto` it is a hard-stop
- **NEVER edit before the code-write approval** - inline: `ExitPlanMode` approved by the user (the fallback path with no plan mode asks "Start implementing?" in words first); delegated: the Step 5-D Start gate, before which nothing is spawned
- **Delegated: the code-write approval is Step 5-D's Start gate** - nothing is spawned before it, and every commit waits for the Accept its gate policy names (5-D.5): the owner's, or the team check's under the `high-only` / `at-end` policy the owner picked at Start. The one exception is `--pipelined`: Start approves the implementer's local commits (the lead's integration commits for an isolated group), and Accept then records `accepted_sha` instead of committing
- **Delegated: one writer per tree** - a run starts only on a clean tree, this session writes no code while an implementer is running, and a stop is confirmed before anything else writes
- **Delegated: implementers never push or touch the tracker, and commit only in their own isolated worktree or, under `--pipelined`, locally in the task worktree** - otherwise the task branch gets its commits from this session, after review; a history rewrite (the autosquash) needs `lets worktree pushed` = `not_pushed` and the owner's yes
- **Delegated runs are interactive only** - `--auto` with `--implementers` / `--team` is refused
- Respond in user's language

## Output

**After completion:**
```
┌─ LETS ─────────────────────────┐
│  Check?   /lets:check          │
│  Review?  /lets:review --local │
│  Done?    /lets:done           │
└────────────────────────────────┘
```

**If no plan found:**
```
┌─ LETS ─────────────────────────┐
│  Plan?  /lets:plan             │
└────────────────────────────────┘
```
