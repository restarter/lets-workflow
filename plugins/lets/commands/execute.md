---
description: Execute implementation plan from /lets:plan - inline in native plan mode, or delegated to named implementer subagents you review and correct
argument-hint: "[task-id|plan-path] [--status] [--step|--straight|--auto|--implementers]"
---

# Execute Plan

Load an implementation plan and execute it - inline, in Claude Code's native plan mode, or delegated: each commit-point chunk goes to a named implementer subagent, you review its diff, and corrections go back to that same agent. The plan provides the roadmap; the gates are plan mode (inline) or Step 5-D's Start and review gates (delegated).

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

How the approved plan runs is ONE up-front choice. **A mode flag pre-answers it - skip the picker entirely when any is present:** `--auto` (Here · auto), `--implementers` or its alias `--team` (Implementers), `--step` / `--step-by-step` (Here · step-by-step), `--straight` / `--straight-through` (Here · straight-through). `--auto` keeps all its AUTO MODE semantics (Step 1 refuses on `$LETS_MERGE_BRANCH`; hard-stops preserved). Default locus is **Here** (this session, native plan mode); **Implementers** is the only locus switch. **`--auto` together with `--implementers` / `--team` is REFUSED** with one line - a delegated run is interactive by design (every diff waits for your review, and an unattended run has nobody to review it): drop `--auto`, or run inline.

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
      { label: "Implementers", description: "Named agents implement chunk by chunk; you review and correct each diff" }
    ],
    multiSelect: false
  }]
)
```

**Commit cadence is DERIVED from the mode - never a separate question:** step-by-step -> confirm each commit; straight-through -> auto-commit at each plan commit point; auto -> auto-commit; implementers -> one commit per chunk, only after you accept its diff.

**Handle response (sets the mode that Step 5 obeys):**
- **Straight-through** -> Step 5 (native plan mode); after the plan-mode approval, implement all tasks with NO per-task pause and `/lets:commit` at each plan commit point without re-asking.
- **Step-by-step** -> Step 5 (native plan mode); after the plan-mode approval, implement one task, pause for user review before the next, and confirm each `/lets:commit`.
- **Auto** -> proceed exactly as `--auto` (Step 5's `--auto` behavior + pipeline-state marker + execute-blocked notify). **Guard:** if on `$LETS_MERGE_BRANCH`, REFUSE Auto here too (same rule as Step 1's `--auto` refuse - AUTO MODE never edits the merge-branch); tell the user to pick step-by-step / straight-through or take a feature branch.
- **Implementers** -> do NOT enter native plan mode: Step 4.6, then Step 5-D. `/lets:team run` remains its own entry point for several *tracker* tasks; this locus runs one *plan*.

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

**2. Group chunks.** A caller task is a barrier: nothing before it may still be open when it runs, and nothing after it starts before it is done. Within each stretch between barriers, two chunks share a group when their allowlists share a path, directly or through another chunk of the stretch. Groups keep plan order.

**3. Show the split** - in the Start gate itself, as the `preview` of its Start option (5-D.2), so the user approves the split they are looking at (a table printed as prose before a gate is skipped in practice). Every task exactly once:

```
### Plan split

| Task | Owner    | Group | Allowlist             |
|------|----------|-------|-----------------------|
| 0    | caller   | -     | none                  |
| 1-2  | chunk c1 | A     | a.md, b.md, cli/x.go  |
| 3    | chunk c2 | A     | a.md                  |
```

A task missing from the table, or listed twice, is a derivation error: stop and say which. The Allowlist column names every path, so a dropped file is visible before Start.

**4. Propose the shape.** Delegated runs are **solo**: one implementer at a time, in this tree, chunks and caller tasks in plan order. When a stretch holds more than one group, add one line: "{N} groups share no file; running groups in parallel is not available yet (lets-7dwc1)." Groups are shown so the split is honest about what could be independent - file-disjoint is not proof of independence (a chunk can call code another chunk adds in a different file).

**Refuse delegation** - one line saying why, then the Step 4.5 picker again without the Implementers option - when any task is malformed (name it); when the plan has no chunk at all; when no task has a `**Files:**` block (a hand-written plan cannot be split); or when any task's execution itself (not one conditional sub-step inside it) depends on something learned during the run - read each whole task section; a heading or opening line such as `only if`, `only when`, `only on` (any case) or `run this task if ...` is the usual sign. A delegated run dispatches every chunk, so such a plan runs inline, where the condition is judged when the plan reaches it.

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
    question: "Hand {N} chunk(s) to implementer agents, one at a time in this tree? {M} caller task(s) run here. Nothing is committed until you accept each diff.",
    header: "Start work",
    options: [
      { label: "Start (Recommended)", description: "Pick the model, then spawn the first implementer", preview: "{the Step 4.6 split table}" },
      { label: "Run inline instead", description: "Execute this plan here in native plan mode (Step 5)" },
      { label: "Cancel", description: "Stop; nothing is spawned and nothing is edited" }
    ],
    multiSelect: false
  }]
)
```

Only **Start** continues. The model is chosen by the first spawn (the `implementer-run` panel), after this approval - never before it.

### 5-D.3 Run record

Before the first spawn, write `.lets/cache/delegated-run-{TASK_ID}.json` (`.lets/` is shared by every worktree, so the name carries the task id). `.lets/` is a symlink into the main checkout, so the Write / Edit tools ask a permission on every write there, and the record is rewritten at every transition. So every file this run writes under `.lets/` whose content THIS session authors - the record, briefs, amendments, the plan's `[DONE]` marks - goes through Bash (`cat > <file> <<'EOF'`, or `sed -i` for a mark). Reports are not authored by this session: the implementer writes its own REPORT_FILE.

```json
{
  "task": "{TASK_ID}",
  "plan": "{plan path from Step 2}",
  "run": "{RUN}",
  "model": null,
  "caller": [ { "task": 0, "state": "pending" } ],
  "chunks": [
    { "id": "c1", "tasks": [1, 2], "group": "A", "allowlist": ["path/a", "path/b"], "agent": "impl-{RUN}-c1", "generation": 1, "base": null, "phase": "pending", "round": 0, "report": null, "status": null, "reason": null, "received": null, "patch_sha": null, "commit": null, "nudged": false }
  ]
}
```

`phase` moves `pending` -> `running` -> `review` -> (`correcting` -> `running` -> `review`)* -> `committing` -> `accepted`. A Stop at a review gate records `paused` (it reopens as `review`); a Stop while an agent works goes through `stopping` to `blocked`; `blocked` always carries a `reason` - `stopped`, `re-plan`, `unreachable`, `missing-report` or `unrecognized-commits`. `nudged` records that this round's one report nudge was sent; it resets to `false` whenever `round` or `generation` changes. Caller tasks move `pending` -> `running` -> `done`. `report` is the path of the latest round's saved report, `status` and `reason` are read from it, `received` counts reports in arrival order across the run, and `generation` is 1 for the first agent of a chunk and grows with each replacement. Rewrite the record at every transition BEFORE acting on it, so an interrupted session always finds the state it was in.

### 5-D.4 Dispatch - in plan order

Walk the Step 4.6 split:

- **Caller task** -> record its `state: running`, run it here, now, exactly as the plan writes it, then record `state: done`.
- **Chunk** -> record `base` = `git rev-parse HEAD` and `phase: running`, write its brief, spawn it, and end the turn - the report arrives as the agent's message (5-D.5 says what counts as one).

The brief, `.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md`:

```
MODE: solo
TASK: {TASK_ID}   PLAN: {plan path}   RUN: {RUN}   CHUNK: {chunk} (group {group})
GOAL: {the plan's Goal line}
APPROACH: {the plan's Approach line}
KEY DECISIONS (all bind you):
{the plan's Key Decisions section, verbatim}
OUT OF SCOPE:
{the plan's Non-Goals section, verbatim}
YOU MAY WRITE ONLY:
{the chunk's allowlist, one path per line}
CORRECTIONS: your reviewer may send corrections for this chunk. Each arrives as an AMENDMENT to this brief and changes what it names, within YOU MAY WRITE ONLY; one that needs any other file is a deviation.
BASE: {base sha}
TASKS:
{the chunk's ### Task sections verbatim, each **Commit:** block removed - this session commits after review}
PROJECT RULES: read the repository's CLAUDE.md before editing.
REPORT_FILE: {absolute path of .lets/cache/report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round}.md}
REPORT: fill in exactly this skeleton and write it to REPORT_FILE (last line REPORT-END); your final message is REPORT_WRITTEN <path> and the report's **Status:** line. Status is one of `complete`, `deviation-stopped`, `blocked`; keep only the block that matches it:
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

Spawn: `Skill(skill: "lets:implementer-run", args: "op=spawn name={agent} chunk-file=.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}.md")`, adding ` model=<m>` once the record holds a model. After the first spawn of the run, write the returned model into the record.

One chunk is live at a time.

**One writer.** While a chunk is `running` or `correcting`, this session writes no code. A request to edit meanwhile -> say that `{agent}` (the record's current agent for the chunk) is still working, and offer to wait for its report or to stop it (5-D.6).

### 5-D.5 Review - one report at a time

**What is a report.** On EVERY notification from `{agent}` after its latest spawn or correction - a message, an idle notification, a completion, EMPTY ones included - inspect the round's REPORT_FILE first, without concluding: `Skill(skill: "lets:agent-report", args: "op=peek dir={abs .lets/cache} names=report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round}")` - READ EVERY REPORT IN FULL. `OK` -> that file IS the report, whatever the message says or whether it is empty: `status` and `reason` come from its **Status** line and its **Blocked** / **Deviation** block (a message Status that disagrees is ignored and named in the review block). A file whose Status line is missing or holds any other value -> `blocked`, reason `malformed-report` - never infer a status from prose. Not `OK` -> the report is DUE only when the message carries `REPORT_WRITTEN` or a `**Status:**` line: then `Skill(skill: "lets:agent-report", args: "op=collect dir={abs .lets/cache} names=report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round} retry=no")` records the `GAP`, and the report is status `blocked`, reason `missing-report`, with the message text quoted UNVERIFIED. Not `OK` and neither marker: an interim message while `{agent}` is still working -> not a report yet, no gap, keep the phase and end the turn. An IDLE or COMPLETION notification (the agent has stopped) -> the report is due now, in this session: when the chunk's record has no `nudged: true` for this round, record it and `SendMessage({to: "{agent}", message: "Write your report for chunk {chunk} to REPORT_FILE {path} now and send REPORT_WRITTEN."})` - a SendMessage that fails (the agent is gone) settles the round at once: `op=collect ... retry=no`, `blocked`, `missing-report`; otherwise end the turn. A messaged agent takes a turn, so its NEXT notification - a reply or another idle - settles the round under this same rule; and the round never outlives the nudge: whenever this session is next invoked for any OTHER reason (a user turn, another agent's notification, a `/lets:execute` resume) while a nudged round still has no `OK` file, `op=peek` it and check `ListAgents`: `{agent}` busy -> it is still answering, leave the round pending; `{agent}` idle (it has had its turn since the nudge) or not listed -> settle now: `op=collect ... retry=no`, `blocked`, `missing-report`, and the review gate. When the round was already nudged, the same settlement applies. Never wait for a later `/lets:execute` to notice a stopped agent. Anything later in the same round is a repeat: record nothing.

When `{agent}` reports, record `phase: review`, `report` (the REPORT_FILE path), `status` and `reason` (read from its **Status** and its **Blocked** / **Deviation** block) and `received` (one more than the highest in the record). One agent is live at a time, so no report interrupts an open gate.

Check the real tree - the report is the agent's claim, the diff is the fact:

```bash
git rev-parse HEAD                                    # must equal the chunk's recorded base
git status --porcelain --untracked-files=all          # every listed path must be in the chunk's allowlist; new directories are expanded to their files
git diff --cached --name-only                         # must print nothing: an implementer never stages
git diff HEAD                                         # the patch of every tracked file, index and worktree alike
git ls-files --others --exclude-standard -z           # untracked files, NUL-separated so any file name is safe
```

For each untracked path, show it as a patch: `git diff --no-index -- /dev/null "<path>"` (read-only; nothing is staged). Its exit status 1 means a patch was printed; only a status above 1 is an error. HEAD moved, anything staged, or a path outside the allowlist changed -> the diff is NOT attributed to the agent: name the paths and use the `blocked` gate. An amendment cannot clean a path outside the allowlist (amendments stay inside it), so say plainly that those paths are the user's to remove or restore; once they are gone, `/lets:execute` re-checks the tree and, for a `complete` report, offers Accept.

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

- **Accept** -> re-run the tree check above and recompute the patch sha first; a failed check, or a sha that differs from the recorded `patch_sha` -> Render review again (it records the new sha), never commit what was not reviewed. Then record `phase: committing`, stage exactly the chunk's allowlist paths (untracked ones included) and `Skill(skill: "lets:commit", args: "approved=review-accept")` - the Accept pick is its approval, so the skill does not ask again. Then record `phase: accepted` and `commit`, append `[DONE]` to each of the chunk's `### Task` headings in the plan file, and continue 5-D.4 with the next item.
- **Correct** -> ask in words what to change. Write it to `.lets/cache/correct-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round+1}.md` framed as an amendment to the brief:

  ```
  AMENDMENT to chunk {chunk}, round {round+1} - it changes what it names and nothing else:
  REPORT_FILE: {absolute path of .lets/cache/report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r{round+1}.md}
  {the correction, verbatim}
  ```

  Record `phase: correcting` and `round+1`, then `Skill(skill: "lets:implementer-run", args: "op=correct name={agent} correct-file=<that file>")`. It returns `corrected {agent}` -> record `phase: running` and end the turn. It returns `agent_gone` -> record `phase: blocked` and `reason: unreachable`, then 5-D.7. A correction that needs a file outside the chunk's allowlist is not a correction: by the agent's own deviation rule it would stop - choose Re-plan instead.
- **Re-plan** -> record `phase: blocked` and `reason: re-plan`, then `Skill(skill: "lets:plan")`.
- **Stop** -> record `phase: paused`, keeping `report`, `status` and `reason` as they are. Say that the uncommitted diff is still in the tree and that `/lets:execute` reopens this same review. Nothing is discarded.
- **Ask orchestrator** -> `Skill(skill: "lets:orc", args: "verb=ask footer=none text={agent} reports {status} on {chunk}: {one-line summary}. {question}")` where `{question}` is the one the shown gate asks - "Accept, correct or stop?" for `complete`, "Correct, re-plan or stop?" for `deviation-stopped`, "Correct or stop?" for `blocked`; then show the same gate again without that option. A peer's answer never decides.

### 5-D.6 Stop while an agent is running

A request to stop - or to edit code - while a chunk is `running` or `correcting`: record `phase: stopping`, then `TaskStop(task_id="{agent}")`. The one-writer rule holds until the stop is confirmed: `TaskStop` reports success, or `ListAgents` no longer shows `{agent}` as busy. Then record `phase: blocked` and `reason: stopped`, and show what the agent left (`git status --porcelain --untracked-files=all`). Nothing it wrote is discarded. `TaskStop` fails and the agent still shows busy -> say so and keep `stopping`; start no other code-writing flow.

### 5-D.7 Recovery (a run record exists)

Read the record. Its `plan` is not this run's plan -> ask whether to resume that run or discard its record (deleting the record file changes no code).

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
| `running` / `correcting` | `ListAgents` shows `{agent}` idle | FIRST the 5-D.5 `op=peek` on the round's REPORT_FILE - `OK` -> handle it as 5-D.5 (the report was written, only the message was lost); otherwise the SAME one-nudge rule as 5-D.5, on the same per-round `nudged` state: not yet nudged -> record `nudged: true` and `SendMessage({to: "{agent}", message: "Report on chunk {chunk} now: write your report to REPORT_FILE {path} and send REPORT_WRITTEN, or, if you have not finished, write a report saying why."})`, handle the reply as 5-D.5; already nudged -> `op=collect ... retry=no`, record `blocked`, `reason: missing-report`. A resumed session never nudges a round twice |
| `running` / `correcting` | `{agent}` not listed | FIRST the same `op=peek` - `OK` -> handle as 5-D.5 (the agent finished before it vanished); otherwise `op=collect ... retry=no` (the report is due and will not come), then record `phase: blocked`, `reason: unreachable`; the Replacement gate |
| `stopping` | - | 5-D.6 again |
| `review` / `paused` | the recorded `report` file peeks `OK` | Render review (5-D.5) from the saved report, with its ORIGINAL status gate - Accept included when it was `complete` and the check is clean; a recorded `patch_sha` of `null` is simply replaced. `ListAgents` does not list `{agent}` -> a correction needs the agent that wrote the diff: `complete` shows its gate without **Correct** (Accept, Stop), `deviation-stopped` without **Correct** (Re-plan, Stop), and `blocked` goes to the Replacement gate instead |
| `review` / `paused` | the recorded `report` file does NOT peek `OK` (MISSING, EMPTY, UNTERMINATED or UNREADABLE since it was recorded) | a **rehydration** of the SAME round, not a new report: `{agent}` listed -> `SendMessage({to: "{agent}", message: "Your report for chunk {chunk}, round {round}, is no longer readable at {path}: write it there again, unchanged, and send REPORT_WRITTEN."})`; on its reply `op=peek` the same file - `OK` -> read it in full, overwrite the record's `status` / `reason` for THIS round (`round` and `received` unchanged - it is the same report), then render the review gate from it; still not `OK`, or `{agent}` not listed -> `op=collect ... retry=no`, record `blocked`, `reason: missing-report`, then the Replacement gate. 5-D.5's "anything later in the same round is a repeat" does not apply to a reply this row asked for |
| `committing` | `git log --format=%H {base}..HEAD` | exactly one new commit whose message carries `Task: {TASK_ID}` and whose `git show --name-only --format= <sha>` lists only allowlist paths -> record `accepted` and `commit`, mark `[DONE]`; no new commit -> record `review` and show the gate again; anything else -> record `blocked`, `reason: unrecognized-commits` |
| `blocked`, `reason: re-plan` | - | say the chunk went to re-planning; after `/lets:plan`, `/lets:execute` starts a run of the new plan |
| `blocked`, `reason: stopped`, `unreachable` or `missing-report` | - | the Replacement gate |
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

- **Start a replacement** -> `generation+1`; the new name is `impl-{RUN}-{chunk}-r{generation}`; write `.lets/cache/chunk-{TASK_ID}-{RUN}-{chunk}-g{generation}.md` = the original brief plus an `AMENDMENTS SO FAR:` section holding every correction file of the chunk, oldest first - drop every `REPORT_FILE:` line from the copied brief and amendments, and put one operative line `REPORT_FILE: {absolute path of .lets/cache/report-{TASK_ID}-{RUN}-{chunk}-g{generation}-r0.md}` (the NEW generation, round 0) directly above `REPORT:` - a replacement never writes an earlier generation's report; record `agent`, `generation`, `round: 0`, `phase: running`, and clear the first agent's `report`, `status`, `reason` and `patch_sha` to `null` so no stale round describes the new one; spawn it through `implementer-run`. Say plainly that it is a new agent.
- **Discard it and replace** -> ask in words, listing every file, before touching anything (destructive). On yes: `git restore --staged --worktree -- <each changed tracked allowlist path>` and `rm -- "<each untracked allowlist path>"`, check the tree is clean, then **Start a replacement**.
- **Keep it and stop** / **Stop** -> leave the record as it is.

A chunk `accepted` with any of its `### Task` headings not yet `[DONE]` -> mark the missing ones; nothing else. Never present a replacement as the same agent, and never discard a diff without the separate confirmation above.

### 5-D.8 Completion

Every chunk `accepted` and every caller task `done` -> delete `.lets/cache/delegated-run-{TASK_ID}.json` and go to Step 6.

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
- **Delegated: the code-write approval is Step 5-D's Start gate** - nothing is spawned before it, and every commit waits for that chunk's Accept
- **Delegated: one writer per tree** - a run starts only on a clean tree, this session writes no code while an implementer is running, and a stop is confirmed before anything else writes
- **Delegated: implementers never commit, push, or touch the tracker** - this session does all three, after review
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
