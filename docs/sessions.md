# Session continuity

A **session** is one conversation (`/lets:start` … `/lets:end`); a **task** usually outlives it. This page is about what carries over between sessions - what `/lets:end` settles, what a snapshot holds, and what `/lets:start` reads back.

## `/lets:end` — the settlement pass

`/lets:end` looks at what the session leaves unsettled and asks **once**, with only the options that apply. A tidy session gets no question at all.

| Found | Offered | What picking it does |
|-------|---------|----------------------|
| Uncommitted changes | Commit | Runs `/lets:commit` |
| The task is still open | Finish task | Writes the snapshot, then refers you to `/lets:done` and stops - `/lets:end` never finishes or closes a task itself |
| Commits not on the remote | Push | Pushes the current branch (no PR, never a force push) |
| A task with progress worth recording | Post progress | Adds a Session progress comment to the task |

The picks run in a fixed order: commit → finish-task hand-off → push → snapshot → progress comment. The snapshot is always written, picks or no picks; the progress comment comes last because it points at the snapshot file.

## Snapshots

A snapshot is a recovery-grade record of the session: what was done, what remains, the next step, decisions, and the commit range. It is a file in `.lets/sessions/`, named `{date}-{HHMM}-{task-id}-snapshot.md` (a session with no task uses `{branch}-{6hex}` in place of the id; a name collision adds `-vN`). When a task is unambiguously active, the task also gets a one-line pointer to the file.

`.lets/` is gitignored, so snapshots are **local**: a teammate on another machine, or you on another laptop, recover from the task's comments alone.

**Banking a snapshot mid-session.** `/lets:end --session` and `/lets:note --session` (aliases `--snapshot`, `--pre-compact`, `--compact` - one path, four spellings) write the same snapshot and nothing else: no settlement, no commit or push offers, and the session keeps going. Use it before a `/compact`, or any time a long session holds state you would not want to lose.

## Commit ranges carry a trust level

A snapshot and a progress comment both say which commits the session produced. That range is only as good as the boundary it is measured from, so it always travels with a label:

| Trust | The boundary is | What the number means |
|-------|-----------------|-----------------------|
| `exact` | recorded by this session | this session's commits |
| `prior-session` | recorded by an earlier session on this branch | a real boundary, best effort |
| `estimate` | not recorded; derived from where the branch left the merge branch | what the branch added, not what the session did |
| `none` | not usable | no number is claimed |

The boundary lives in the per-branch task-state file `.lets/sessions/.task-<branch-slug>`, which `/lets:start` writes when you claim a task.

## Coming back

| Command | What it restores |
|---------|------------------|
| `/lets:start` | Shows the last few snapshots, then the task picker |
| `/lets:start <task-id>` | Jumps to the task: its full description and every comment, plus the latest snapshot for it |
| `/lets:start --continue` | Resumes the in-progress task - with exactly one, it skips the picker and goes straight to recovery |

Recovery reads the latest snapshot file first, then the task's comments; where they disagree, the file wins. After a `/compact` the same applies - what survived in the conversation is used, and the snapshot fills in what compaction dropped.

## See also

- **[workflow.md](workflow.md)** — where sessions sit in the daily loop
- **[commands/done.md](commands/done.md)** — finishing the task, which `/lets:end` deliberately leaves alone
- **[tasks.md](tasks.md)** — notes and the task lifecycle
- **[configuration.md](configuration.md)** — the `.lets/` layout
