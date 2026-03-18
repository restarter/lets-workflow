---
description: One-time migration from .claude/sessions/ to .lets/ folder
---

# Migrate Storage Folder

One-time migration. Run once per project, then delete this command.

## Step 1: Check State

```bash
# ROOT = project-root from LETS Config
ls -la "$ROOT/.claude/sessions/" 2>/dev/null
ls -la "$ROOT/.lets/" 2>/dev/null
```

- If `.claude/sessions/` doesn't exist - nothing to migrate, exit
- If `.lets/` already exists - already migrated, exit

## Step 2: Migrate

```bash
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/sessions" "$ROOT/.lets/reviews" "$ROOT/.lets/plans"

# Add .lets/ to .gitignore if not already there
if ! grep -q '^\.lets/' "$ROOT/.gitignore" 2>/dev/null; then
  echo '.lets/' >> "$ROOT/.gitignore"
fi

# Move session files
mv "$ROOT/.claude/sessions/"*.md "$ROOT/.lets/sessions/" 2>/dev/null

# Move reviews
if [ -d "$ROOT/.claude/sessions/reviews" ]; then
  mv "$ROOT/.claude/sessions/reviews/"*.md "$ROOT/.lets/reviews/" 2>/dev/null
fi

# Remove old folder
rm -rf "$ROOT/.claude/sessions"
```

## Step 3: Verify

```bash
# ROOT = project-root from LETS Config
echo "Sessions:" && ls "$ROOT/.lets/sessions/" 2>/dev/null
echo "Reviews:" && ls "$ROOT/.lets/reviews/" 2>/dev/null
echo "Old folder:" && ls "$ROOT/.claude/sessions/" 2>/dev/null || echo "removed"
```

## Rules

- Respond in user's language

## Output

```
Migrated .claude/sessions/ -> .lets/
  Sessions: {count} files
  Reviews: {count} files
  Old folder: removed

┌─ LETS ─────────────────────────┐
│  Start?  /lets:start           │
└────────────────────────────────┘
```
