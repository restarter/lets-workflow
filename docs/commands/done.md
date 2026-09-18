# `/lets:done` — finishing a task

`/lets:done` closes out the **task**, not the session: it checks the work against what the task asked for, records it, and ships it the way your project's `LETS_PR_FLOW` says. Run it when every change is committed.

## What it checks first

| Check | What happens |
|-------|--------------|
| Active task | Found from the branch's task-state file. No task - nothing to finish. An **epic** is not closed automatically: epics stay open for future tasks. |
| Uncommitted changes | Commit first (`/lets:commit`), skip, or cancel. |
| Already merged (GitHub) | If the branch's PR was merged elsewhere - a parallel session, the browser - it skips the push and the PR and goes straight to the completion comment and close. |
| Scope | Each requirement in the task description is ticked against the code. Anything missing asks: **Fix first** (stop and implement it), **Update scope** (the task changes to match what was built) or **PR only, keep open** (ship this part, the task stays open for the rest). |
| CHANGELOG | With a `CHANGELOG.md` at the project root and a user-visible change, it drafts a one-line `[Unreleased]` entry from the task title and commits - **Add entry**, **Edit first** or **Skip** - and commits it so it lands in the same PR. No file, or pure infra / tests / refactor: skipped, and it says so. |

Then one confirmation - **Finish** or **Keep working** - worded for the flow below, and a completion comment on the task: commits, summary, key decisions, files changed.

## How it ships

| Flow | What **Finish** does | The task afterwards |
|------|----------------------|---------------------|
| `github` | Pushes the branch and opens a PR with `gh` | Stays open until the PR merges |
| `bitbucket` | Pushes the branch and opens a PR with `bbb` | Stays open until the reviewer merges |
| `local` (or any unrecognized value) | Merges into the merge branch locally and deletes the branch | Closed |
| Trunk-mode (you are on the merge branch) | Pushes the merge branch - no PR, since source and target are the same | Closed |

**The PR is read back.** A URL proves a PR exists, not that it can merge. After creating a GitHub PR, `/lets:done` looks at it and tells you what it found: mergeable and clean; mergeable but checks running or failing; or blocked, behind, or conflicting - and a PR born with conflicts gets no merge ref, so its CI will not run at all until the conflict is resolved.

**Waiting on review.** On a tracker whose board has an `in_review` status, the task moves there once the PR is open. beads has no such status, so there the task simply stays `in_progress`.

If `gh` is not authenticated, it offers a local merge for this task or cancelling to fix the login.

## After the PR

| Option | What it does |
|--------|--------------|
| Merge & close (GitHub) | Merges the PR, closes the task, switches back to the merge branch |
| Stay on branch / Stay here | Keeps you on the branch for PR fixes or follow-up work |
| Next task | Back to the merge branch, then claims another task |
| End session | Runs `/lets:end` |

A Bitbucket PR has no **Merge & close** - the reviewer merges it in Bitbucket. In a worktree there is no branch switch; after a local merge it offers to remove the worktree. When the session is bound to an orchestrator, it also offers to ping it with the PR link - nothing is sent unless you pick that.

Picking an option that names a `/lets:*` command runs that command, with its own gates.

## What it never does

- It never pushes, merges or closes without the **Finish** confirmation - including under AUTO MODE.
- It never reports a PR it has not read back, or a close that did not happen: a tracker call that fails is reported as a failure.
- `/lets:end` never runs it for you - it only refers you here.

## See also

- **[../workflow.md](../workflow.md)** — the daily loop
- **[../sessions.md](../sessions.md)** — what `/lets:end` settles, and why it leaves the task alone
- **[../configuration.md](../configuration.md)** — `LETS_PR_FLOW` and the forges
- **[../tasks.md](../tasks.md)** — trunk-mode and the task lifecycle
