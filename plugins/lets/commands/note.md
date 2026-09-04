---
description: Add a note to the active task - progress, decisions, context. --session (aliases --snapshot, --pre-compact, --compact) writes a resume-grade session snapshot without ending the session.
---

# Task Note

Add a note to the active task. For mid-work documentation - progress updates, research findings, decisions, context for future sessions.

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
/lets:note --session        # RESUME snapshot of the whole session (aliases: --snapshot, --pre-compact, --compact)
```

## Step 0: Argument Parsing

**If `--session` (aliases `--snapshot`, `--pre-compact`, `--compact`) is passed** -> run **Session Snapshot Mode** (below) INSTEAD of Steps 3-4: no type prompt, produce ONE recovery-grade snapshot. Do NOT run Step 1's interactive task prompt - the `session-snapshot` skill owns task detection (file-primary: ambiguity -> file only, no prompt), keeping this byte-identical to `/lets:end --session`. Then show the output below, NOT the generic Output box (no separate verify - the skill returns the path + task id, and `/lets:end` likewise just delegates and stops).

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

```lets-tracker
show task=<task-id>          # returns {id,title,status,url,description}
comment-list task=<task-id>
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

The orchestrator composes the note body into a temp file, then submits it via `body-file=` (lets-rules "Tracker Adapters"):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel); mkdir -p "$LETS_PROJECT_ROOT/.lets/cache"
cat > "$LETS_PROJECT_ROOT/.lets/cache/note-<task-id>.md" <<EOF
## {Note type} $(date +%Y-%m-%d)

{content based on type}
EOF
```

```lets-tracker
comment-add task=<task-id> body-file=.lets/cache/note-<task-id>.md
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

```lets-tracker
comment-add task=<task-id> body="[scope-change] <what changed and why>"
```

## Step 6: Verify

```lets-tracker
show task=<task-id>
```

## Session Snapshot Mode (`--session`)

One path, four spellings - `--session`, `--snapshot`, `--pre-compact`, `--compact` all write the same recovery-grade `## RESUME` snapshot. There is no separate pre-compaction variant on purpose: the file is permanent and `/lets:start` reads it back, so it records what HAPPENED (a snapshot written mid-session) rather than what the caller intended next, which is true whether or not a `/compact` follows.

**Skip Step 3** (no type prompt) - delegate to the internal **session-snapshot** skill, which ALWAYS writes the snapshot to a `.lets/sessions/` file and adds a one-line pointer to the active task only when one is unambiguously active. Single source of truth, shared with `/lets:end --session` (byte-identical), so the template never drifts:

`Skill(skill: "lets:session-snapshot", args: "kind=session pointer=auto")`

The skill gathers session + git state, writes the file, and returns the snapshot path (plus the task id if a pointer was written) - report directly from that return; no separate verify round-trip. It resolves the session range through `session-boundary`, so the snapshot carries a qualified `### Range` block; the block is omitted entirely, never stubbed, when no range could be resolved. Then show the output below - NOT the generic `## Output` box:

```
## Session Snapshot

Snapshot -> .lets/sessions/{date}-{HHMM}-{task-id}-snapshot.md
Task pointer -> {task-id}  (only if a task is unambiguously active; else "none - file only")
Branch: {branch}
Range: {RANGE_DESC returned by the skill, or "none - no valid session boundary"}

Recorded - the session continues, and a /compact may safely follow. Resume later: /lets:start reads the snapshot file.
```

Then STOP - the session continues. (Identical output contract to `/lets:end --session`.)

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
