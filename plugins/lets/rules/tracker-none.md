---
name: tracker-none
version: 0.7.0
---

<!-- DO NOT EDIT installed copies in .claude/rules/ - managed by `lets init` / `lets update`. Edit the source in plugins/lets/rules/. -->

# Tracker adapter: none (null adapter)

No task tracker. Every verb is a no-op; the project runs without cross-session task state. This is the deliberate "no-beads" stance - commands degrade to a no-tracker mode rather than erroring.

- Verb resolution is ORCHESTRATOR-ONLY (subagents never call tracker verbs).
- Identity: there is no tracker-side id - the git branch (and the `.task` session file) is the only task reference. `detect-task` falls through to its branch-name / `.task` parse and never resolves an id against a store (no tracker `show`).

## Neutral statuses

None tracked. `set-status` / `close` are no-ops.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | no | no-op - tell the user no tracker is configured; the branch is the only handle |
| show           | CORE | no | no-op - nothing to show (no store) |
| comment-add    | CORE | no | no-op - the comment is not persisted to a task |
| set-status     | CORE | no | no-op - no status to set |
| close          | CORE | no | no-op - nothing to close |
| comment-list   | OPT  | no | absent |
| list-by-status | OPT  | no | absent |
| search         | OPT  | no | absent |
| ready/stats    | OPT  | no | absent |
| label          | OPT  | no | absent |
| assignee       | OPT  | no | absent |
| set-field      | OPT  | no | absent |

## Degradation

Every verb is unsupported, so every tracker action degrades: the command continues and states plainly that no task tracker is configured (`LETS_TRACKER=none`). Nothing crashes, nothing is silently dropped as if it had been recorded. A flow that requires a task (e.g. `/lets:start`'s task gate) tells the user there is no tracker rather than fabricating one.
