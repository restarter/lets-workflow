# Gotchas & Lessons Learned

Things we discovered the hard way while building skills and workflows.

## 1. Relative Paths in Skills

**Problem:** Skills use bash code blocks as examples for Claude. If Claude's working directory changed (e.g., after `cd code/letsbrowse`), relative paths like `.lets/sessions/` resolve to wrong location.

**Fix:** Always use `git rev-parse --show-toplevel` for repo-root-relative paths:
```bash
ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$ROOT/.lets/sessions"
```

**Affected:** `lets-end` (session file creation), `lets-start` (session file reading).

## 2. Symlink Directory vs Symlinked Files

**Problem:** Symlinking a whole directory (`.claude/rules/ -> other/rules/`) means ALL files created in that dir appear in both locations. Can't have project-specific files.

**Fix:** Create a real directory with individual symlinks to shared files:
```
.claude/rules/                    # Real directory
|-- architecture.md -> shared     # Symlink to shared
|-- workflow.md -> shared         # Symlink to shared
|-- android-standards.md          # Local file, only here
```

## 3. Hardlinks Are Invisible

**Problem:** Hardlinked files have same inode - deleting from one location deletes from both. `ls -la` doesn't show they're linked (unlike symlinks which show `->` target).

**Fix:** Use symlinks instead of hardlinks for cross-project file sharing. Check with `stat -f "%i"` to compare inodes.

## 4. "ALWAYS" Rules Cause Over-triggering

**Problem:** Rule saying "ALWAYS suggest the next skill after completing an action" caused Claude to show LETS boxes even when waiting for user input (e.g., task selection after `/lets-start`).

**Fix:** Removed the "ALWAYS" rule. Phase-based triggers (active work, work done, after commit) are specific enough. Added implicit "no box when waiting for user input" behavior.

## 5. Subagent Type Selection

**Problem:** `lets-review` skill doesn't specify which `subagent_type` to use for review agents. Claude picks one itself (e.g., `superpowers:code-reviewer`), which may not match skill intent.

**Fix:** Skills that launch agents should specify the subagent_type explicitly, or the plugin should provide dedicated agent types (e.g., `lets:compliance-reviewer`).

## 6. Skills Loaded from Base Directory

**Problem:** Each skill has a "Base directory" header injected by Claude Code. If the skill uses relative paths, they're relative to the skill's base directory, not the project root.

**Awareness:** This is metadata only - Claude interprets bash blocks as examples, not as literal execution. But it can confuse the AI about context.

## 7. PHP Rules in Android Project

**Problem:** Shared rules (hardlinked/symlinked) included `php-standards.md` which is irrelevant for Android project. Claude loads all rules in `.claude/rules/` regardless.

**Fix:** Only symlink relevant rules per project. Keep project-specific rules as local files (e.g., `android-standards.md`).
