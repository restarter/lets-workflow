---
description: Add a note to the active task - progress, decisions, context. --pre-compact writes a resume-grade session snapshot before /compact.
---

# Task Note

Add a note to the active beads task. For mid-work documentation - progress updates, research findings, decisions, context for future sessions.

**This is a utility, not a flow step.** Main flow handles documentation automatically:
- `/lets:commit` records commits to the task
- `/lets:done` documents task completion
- `/lets:end` saves session progress

Use `/lets:note` when you want to add extra context that doesn't fit those flows.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```
/lets:note                  # interactive - pick a note type, add to the active task
/lets:note <free text>      # use the text directly as the note (infer type)
/lets:note --pre-compact    # RESUME snapshot of the whole session before /compact
```

## Step 0: Argument Parsing

**If `--pre-compact` (alias `--resume`) is passed** -> run **Pre-Compact Resume Mode** (below) INSTEAD of Steps 3-4: no type prompt, produce ONE recovery-grade snapshot. Still run Step 1 (detect-task), Step 6 (verify), and the Output box.

**Otherwise** -> normal flow (Steps 1-6).

## When to Use

- Research findings: "Investigated X, found Y"
- Design decisions: "Chose approach A because..."
- Blocker discovered: "Can't proceed until Z"
- Context dump: "Important info for next session"

## Step 1: Active Task Detection

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If no active task or multiple tasks found - ask user which task to add a note to.

## Step 2: Review Current State

```bash
bd show <task-id>
bd comments <task-id>
```

Read the full description and all comments. Check existing comments to avoid duplicating info.

## Step 3: Ask What to Note

If not obvious from conversation context, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What kind of note?",
    header: "Note type",
    options: [
      { label: "Progress", description: "What's done and what remains" },
      { label: "Decision", description: "Record a choice and reasoning" },
      { label: "Research", description: "Findings and recommendations" },
      { label: "Blocker", description: "What's blocking and possible solutions" }
    ],
    multiSelect: false
  }]
)
```

**Other** (free text) -> use as note content directly, infer type.

## Step 4: Add Note

```bash
bd comments add <task-id> "## {Note type} {YYYY-MM-DD}

{content based on type}"
```

### Note Templates

**Progress:**
```markdown
## Progress {date}

### Done
- {what was completed}

### Remaining
- {what's left to do}
```

**Decision:**
```markdown
## Decision {date}

**Chose:** {option}
**Over:** {alternatives}
**Because:** {reasoning}
```

**Research:**
```markdown
## Research {date}

### Findings
- {key discoveries}

### Recommendation
- {suggested approach}
```

**Blocker:**
```markdown
## Blocker {date}

**Issue:** {what's blocking}
**Impact:** {what can't proceed}
**Options:** {possible solutions}
```

## Step 5: Record Scope Change (if needed)

If the task scope or understanding changed significantly, record it as a comment (never overwrite the description):

```bash
bd comments add <task-id> "[scope-change] <what changed and why>"
```

## Step 6: Verify

```bash
bd show <task-id>
```

## Pre-Compact Resume Mode (`--pre-compact`)

Run this right before `/compact` (or when a long session is about to be auto-summarized). Goal: ONE recovery-grade comment on the active task so a future you / another agent can fully reconstruct the working context after compaction — what is decided, where everything lives, and the exact next step. (Same shape as the resume note a review round produces.)

**Skip Step 3** (no type prompt). Gather the snapshot from the session + git, then write it as a single comment.

```bash
git branch --show-current
git log --oneline -5
git status --short          # uncommitted / untracked
```

Write the snapshot to the active task (English; one continuous line per paragraph - no hard wrap):

```bash
bd comments add <task-id> "## RESUME {YYYY-MM-DD} - {short label}

### Where things live
- repo / branch: {branch} @ {short-sha}; key paths touched: {file:line, ...}
- external sources: {PR #, links, other-project paths, index / recovery commands}

### State
- committed/merged: {...}; uncommitted/untracked: {git status}; frozen artifacts + SHAs: {...}

### Decided (do NOT re-litigate)
- {decision -> reasoning}
- verified vs code: {claim -> file:line}

### Remaining + NEXT STEP
- {open items}
- NEXT: {the single concrete next action + how to resume it}

### Compaction
- snapshot taken before /compact; resume with: bd show <task-id> + bd comments <task-id>"
```

**No active task (fallback) - never drop the snapshot.** If Step 1 found none, write the same RESUME block to a session file and tell the user where it landed:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/sessions"
BRANCH_SLUG=$(git branch --show-current | tr '/' '-')
# Write the RESUME block (above) via the Write tool to:
#   $LETS_PROJECT_ROOT/.lets/sessions/<YYYY-MM-DD-HHMM>-precompact-${BRANCH_SLUG}.md
```

Then run Step 6 (verify) and show the Output box.

## Output

```
Note added to **{task title}** ({task-id})

┌─ LETS ─────────────────────────┐
│  Check?   /lets:check          │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

## Rules

- **Be specific** - "Fixed bug" is useless, "Fixed null check in PaymentService.process()" is useful
- **Record decisions** - future you will thank you
- **Use markdown** - comments support formatting
- Respond in user's language
