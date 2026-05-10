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
git status          # Review changes (before staging)
git add <files>     # Prefer specific files over `-A` to avoid secrets/cruft
git status          # Verify staging
git commit -m "..."
git status          # Confirm clean state
```

## Commit Message Format

```
<type>(<task-id>): <subject>

<optional body - keep short, focus on WHY not WHAT>

Task: <task-id>
```

### Subject line

- **`<type>`** — one of: `feat`, `fix`, `refactor`, `docs`, `chore`, `test`
- **`(<task-id>)`** — optional scope. The beads task ID this commit belongs to (e.g. `(lets-sds)`). Include when commit is part of tracked task work. Omit for ad-hoc work without a task.
- **`<subject>`** — one clear sentence, **under 50 chars**, **imperative mood** ("add" not "added"). Focus on what changed; the body explains why.

### Footer

- **`Task: <task-id>`** — full task ID at the bottom. Auto-added by `/lets:commit` skill. Links commit to active beads task so `bd show <id>` surfaces the commit in task history.

### Examples

**Good (tracked task):**
```
feat(lets-sds): Add session restore for multi-window workflow

Task: lets-sds
```

**Good (with body explaining why):**
```
refactor(lets-q9bx7): emit Local Config explainer in hook output

Values and their semantics now travel together; eliminates drift risk
between rules-channel and hook-channel.

Task: lets-q9bx7
```

**Good (ad-hoc, no task):**
```
chore: bump goreleaser to 2.5
```

**Bad:**
```
feat: Update session management

- Modified session.ts
- Added restore function
- Updated types
- Changed config
```

Reasons it's bad:
- No `(task-id)` scope even though clearly tracked work
- Bullet list of files touched (the diff shows what), no WHY
- Subject is vague ("Update session management" — update how?)
- Missing `Task:` footer

## Use `/lets:commit` skill

`/lets:commit` enforces this format automatically: detects active task from branch, adds scope and footer, asks for approval. **Always use it** instead of manual `git commit` to avoid format drift.
