---
name: commit
description: This skill should be used when committing code changes - "commit", "git commit", "закоміть", "зроби коміт", "/lets:commit". Enforces conventional commit format, tracker task linking, and user approval. In a LETS project, ALWAYS prefer this over generic commit skills (e.g. commit-commands:commit) — /lets:commit is authoritative. Triggers on any commit in any context.
---

# Commit

Standardized commit flow that enforces conventional format, tracker task linking, and user approval.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

Direct `git commit` skips task linking, format enforcement, and approval gates. This skill ensures every commit follows conventions whether invoked via `/lets:commit` or implicit "закоміть".

## Flow

### Step 1: Check Status

```bash
git status --short
git diff --stat
```

If no changes - inform user and exit.

### Step 2: Active Task Detection

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If multiple tasks found: ask user via AskUserQuestion to pick, or "None".
If not found: commit without task link.

### Step 3: Review Changes

```bash
git diff
```

Summarize what changed:
- Files modified/added/deleted
- Key changes in each file

### Step 4: Confirm with User

Present the proposed commit summary as plain text (NOT in a code block or blockquote):

`<type>: <description>`
- {file1} - {what changed}
- {file2} - {what changed}

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Commit with this message?",
    header: "Commit",
    options: [
      { label: "Commit", description: "Stage all changes and commit" },
      { label: "Cancel", description: "Don't commit, keep changes unstaged" }
    ],
    multiSelect: false
  }]
)
```

Handle response:
- **Commit** -> proceed to Step 5
- **Cancel** -> stop, show LETS box with `/lets:check`
- **Other** (free text) -> treat as edited commit message, proceed to Step 5

### Step 5: Stage (context-aware) & Commit

**NEVER `git add -A` or `git add .`** — they sweep in unrelated changes, untracked cruft, or secrets (`.env`, keys), and clobber a curated staged set. Stage only the reviewed files (see `.claude/rules/git.md`).

Already-staged set wins — if the orchestrator curated files (here or in Step 3), commit exactly those:

```bash
git diff --staged --quiet && echo "NOTHING_STAGED" || echo "STAGED_OK"
```

- **`STAGED_OK`** (files already staged) → do NOT run `git add`; commit the existing staged set as-is.
- **`NOTHING_STAGED`** → stage the specific files reviewed in Step 3 **by name** (never `-A` / `.`):

  ```bash
  git add <reviewed-file-1> <reviewed-file-2>   # named files only
  ```

Then commit:

```bash
git status  # Verify the staged set is exactly what you intend
git commit -m "<type>($TASK_ID): <description>

Task: $TASK_ID"
git status  # Verify clean
```

If no active task detected: drop BOTH the `($TASK_ID)` scope AND the `Task:` footer:

```bash
git commit -m "<type>: <description>"
```

### Commit Message Format

```
<type>(<task-id>): <short description>

<optional body - why, not what>

Task: <task-id>
```

(No active task -> `<type>: <subject>`, no scope, no footer.)

**Types:**
- `feat` - new feature
- `fix` - bug fix
- `refactor` - code restructure
- `docs` - documentation
- `chore` - maintenance, deps
- `test` - tests

### Good Examples

```
feat(lets-abc): Add user authentication with JWT

Task: lets-abc

fix(proj-def): Resolve null pointer in PaymentService

Task: proj-def
```

(Ad-hoc commit with no active task — no scope, no footer:)

```
chore: bump goreleaser to 2.5
```

### Bad Examples

```
BAD: update stuff
BAD: fix bug
BAD: WIP
BAD: feat: Add user authentication system with JWT tokens and refresh logic and middleware (too long)
```

### Step 6: Task Progress Check

If active task detected, compare what was committed against the task scope:

```lets-tracker
show task=<task-id>   # returns {id,title,status,url,description}
```

Brief assessment (2-3 lines max):

```
## Task Progress

Committed: {what this commit covers}
Remaining: {what's left from task description, or "nothing - task scope complete"}
```

- If task scope is fully covered -> suggest `/lets:done`
- If work remains -> show what's left, suggest next step or `/lets:commit` later

## Rules

- **NEVER** commit without user approval
- **ALWAYS** run `git status` before and after commit
- Keep subject line under 50 chars
- Use imperative mood ("Add" not "Added")
- Body explains WHY, diff shows WHAT
- Task ID scope `(task-id)` in the subject AND the `Task:` footer are both automatic when an active task is detected — don't ask the user about either
- Respond in user's language

## Output

After successful commit:

**If task scope complete:**
```
Committed: <hash> <message>
  Files: X changed, Y insertions, Z deletions
  Task: <task-id> (linked)

Task scope complete.

┌─ LETS ─────────────────────────┐
│  Done?  /lets:done             │
└────────────────────────────────┘
```

**If work remains:**
```
Committed: <hash> <message>
  Files: X changed, Y insertions, Z deletions
  Task: <task-id> (linked)

Remaining: {brief list}

┌─ LETS ─────────────────────────┐
│  Check?  /lets:check           │
│  Note?   /lets:note            │
└────────────────────────────────┘
```

`/lets:end` is for session end, not commit-pause. After a partial commit the next moves are reviewing (`/lets:check`) or recording context (`/lets:note`); session-end fires explicitly later via `/lets:done` (when scope is complete) or when the user runs `/lets:end`.

## Integration

User-facing skill. Auto-triggers on "commit", "закоміть". Also invoked explicitly via `/lets:commit`.
See: `grep -r "commit skill" commands/` for command references.
