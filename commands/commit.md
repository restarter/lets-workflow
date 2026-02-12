---
description: Commit changes with proper review and conventional commit message
---

# Git Commit

Review and commit changes with conventional commit format.

## Step 1: Check Status

```bash
git status --short
git diff --stat
```

If no changes - inform user and exit.

## Step 2: Review Changes

```bash
git diff
```

Summarize what changed:
- Files modified/added/deleted
- Key changes in each file

## Step 3: Confirm with User

**Ask:**
> "Ready to commit? Here's what changed: {summary}"

Wait for approval before committing.

## Step 4: Commit

```bash
git add -A
git status  # Verify staging
git commit -m "<type>: <description>"
git status  # Verify clean
```

### Commit Message Format

```
<type>: <short description>

<optional body - why, not what>
```

**Types:**
- `feat` - new feature
- `fix` - bug fix
- `refactor` - code restructure
- `docs` - documentation
- `chore` - maintenance, deps
- `test` - tests

### Good Examples

```
feat: Add user authentication with JWT
fix: Resolve null pointer in PaymentService
refactor: Extract PostbackHandler from controller
docs: Update API documentation
chore: Upgrade Laravel to 10.x
```

### Bad Examples

```
BAD: update stuff
BAD: fix bug
BAD: WIP
BAD: feat: Add user authentication system with JWT tokens and refresh logic and middleware (too long)
```

## Rules

- **NEVER** commit without user approval
- **ALWAYS** run `git status` before and after commit
- Keep subject line under 50 chars
- Use imperative mood ("Add" not "Added")
- Body explains WHY, diff shows WHAT

## Output

After successful commit:

```
Committed: <hash> <message>
  Files: X changed, Y insertions, Z deletions

┌─ LETS ─────────────────┐
│  End?   /lets:end      │
│  Push?  git push       │
└────────────────────────┘
```
