# Git Workflow

## Permission Required

**Never commit or push without explicit user approval.** Also applies to any command that sends data to a remote (`bd dolt push`, etc).

```
Claude: "Done. Ready to commit?"
User: "yes" / "+" / "commit"
Claude: [only now commits]
```

## Commit Process

```bash
git status          # Review changes
git add -A          # Stage everything
git status          # Verify staging
git commit -m "..."
git status          # Confirm clean state
```

## Commit Messages

```
<type>: <subject>

<optional body - keep short>
```

**Types:** `feat`, `fix`, `refactor`, `docs`, `chore`, `test`

**Good:** one clear sentence about what and why.
**Bad:** bullet list of every file touched.

```
# Good
feat: Add session restore for multi-window workflow

# Bad
feat: Update session management

- Modified session.ts
- Added restore function
- Updated types
- Changed config
```
