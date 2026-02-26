# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Landing the Plane (Session Completion)

**When ending a work session**, complete these steps:

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Sync beads** - `bd sync --flush-only` (export to JSONL)
5. **Commit changes** - Use `/lets:commit` to commit with proper review
6. **Ask about push** - Ask user if they want to push to remote
7. **Hand off** - Provide context for next session via `/lets:end`

**CRITICAL RULES:**
- NEVER push without explicit user approval
- NEVER commit without explicit user approval
- Always suggest `/lets:end` for proper session closure
