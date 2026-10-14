# `/lets:done` — finishing a task

`/lets:done` closes out the **task**, not the session: it checks the work against what the task asked for, records it, and ships it the way your project's `LETS_PR_FLOW` says. Run it when every change is committed. Typing it is the approval for what it names - the push and PR, or the local merge and close; how many questions it still asks follows `LETS_GATES` ([workflow.md](../workflow.md#what-asks-you-first---gate-levels)).

## What it checks first

| Check | What happens |
|-------|--------------|
| Active task | Found from the branch's task-state file. No task - nothing to finish. An **epic** is not closed automatically: epics stay open for future tasks. |
| Uncommitted changes | At `strict`: commit first (`/lets:commit`), skip, or cancel. At `standard` / `minimal`, changes to tracked files are committed through `/lets:commit` and named in one line; new untracked files still ask, since which of them belong to the task is your call. |
| Existing PR (GitHub, Bitbucket) | Before anything is pushed it asks the forge for every PR of this branch - only those from the repository it pushes to (origin's push URL); a PR from another fork with the same branch name is ignored - and compares commits, not the branch name. An **open** PR is reused: new commits are pushed to it, no second PR is opened, and **Merge & close** acts on it. When the branch has more than one open PR, or its only open PR targets another base (a stacked PR), it stops and names them instead of reusing one or opening another. A PR **merged** into the merge branch that already holds every commit means the work shipped (a parallel session, the browser): after the scope check (and **Finish**, at `strict`) it is closed - nothing pushed. A **declined** PR, one merged into another branch, or a merged one the branch has moved past (new work on a reused branch) gets a fresh PR, and the confirmation says why. If the lookup runs and fails, or comes back incomplete, it stops before pushing; without `gh` / `bbb` it skips the check, since nothing can open a PR then. |
| Scope | Each requirement in the task description is ticked against the code. Anything missing asks, at every level: **Fix first** (stop and implement it), **Update scope** (the task changes to match what was built) or **PR only, keep open** (ship this part, the task stays open for the rest). |
| CHANGELOG | With a `CHANGELOG.md` at the project root and a user-visible change, it drafts a one-line `[Unreleased]` entry from the task title and commits; at `strict` it asks - **Add entry**, **Edit first** or **Skip** - otherwise it writes the entry and shows it. Either way it is committed so it lands in the same PR. No file, or pure infra / tests / refactor: skipped, and it says so. On a re-run whose `[Unreleased]` already has an entry for this task it offers only **Edit first** or **Skip** at `strict` and leaves it as it is otherwise - never a second entry; when the work already shipped in a merged PR the step is skipped. |

Then, at `strict`, one confirmation - **Finish** or **Keep working** - worded for the flow below; at `standard` / `minimal` one line says what happens next instead. A park commit in the range (`wip: park`) asks at every level: once pushed, only a force push removes it. Then a completion comment on the task: commits, summary, key decisions, files changed.

## How it ships

| Flow | What **Finish** does | The task afterwards |
|------|----------------------|---------------------|
| `github` | Pushes the branch and opens a PR with `gh` - or pushes to the branch's open PR | Stays open until the PR merges |
| `bitbucket` | Pushes the branch and opens a PR with `bbb` - or pushes to the branch's open PR | Stays open until the reviewer merges |
| `local` (or any unrecognized value) | Merges into the merge branch locally and deletes the branch | Closed - unless the scope check chose **PR only, keep open** |
| Trunk-mode (you are on the merge branch) | Pushes the merge branch - no PR, since source and target are the same | Closed - unless the scope check chose **PR only, keep open** |

**The PR is read back.** A URL proves a PR exists, not that it can merge. After creating a GitHub PR, or pushing to the branch's open one, `/lets:done` looks at it and tells you what it found: mergeable and clean; mergeable but checks running or failing; or blocked, behind, or conflicting - and a PR born with conflicts gets no merge ref, so its CI will not run at all until the conflict is resolved.

**Waiting on review.** On a tracker whose board has an `in_review` status, the task moves there once the PR is open. beads has no such status, so there the task simply stays `in_progress`.

If `gh` is not authenticated, it offers a local merge for this task or cancelling to fix the login.

## After the PR

| Option | What it does |
|--------|--------------|
| Merge & close (GitHub) | Merges the PR, then closes the task and switches back to the merge branch only once GitHub reports the PR `MERGED` - a PR added to a merge queue, or a merge that did not complete, leaves the task open and says so; a later `/lets:done` closes it after the merge. At `strict` it is one of the options; at `standard` it is its own question (merging is never automatic there); at `minimal` `/lets:done` waits up to 30 minutes for the PR's checks and merges it only when every check passed, it is mergeable and its head is still the commit the checks ran on (`--match-head-commit`) - a branch that requires a merge queue is never merged automatically - otherwise the PR and the task stay open and it says why. It never arms GitHub auto-merge, and never merges a PR whose scope check chose **PR only, keep open** |
| Stay on branch / Stay here | Keeps you on the branch for PR fixes or follow-up work. At `standard` / `minimal` the next steps are a box of commands to type rather than a question |
| Next task | Back to the merge branch, then claims another task |
| End session | Runs `/lets:end` |

A Bitbucket PR has no **Merge & close** - the reviewer merges it in Bitbucket. In a worktree there is no branch switch; after a local merge it offers to remove the worktree. When the session is bound to an orchestrator, it also offers to ping it with the PR link - nothing is sent unless you pick that.

Picking an option that names a `/lets:*` command runs that command, with its own gates.

## What it never does

- It never pushes, merges or closes beyond what you typed and what `LETS_GATES` lets through (at `strict`, never without the **Finish** confirmation) - AUTO MODE lifts nothing.
- It never closes a task the scope check kept open (**PR only, keep open**) - on any path, local merge and trunk-mode included.
- It never opens a second PR for a branch that already has an open one, on either forge - and when a lookup fails or comes back incomplete, it stops before pushing rather than guess.
- It never reports a PR it has not read back, or a close that did not happen: a tracker call that fails is reported as a failure.
- `/lets:end` never runs it for you - it only refers you here.

## See also

- **[../workflow.md](../workflow.md)** — the daily loop
- **[../sessions.md](../sessions.md)** — what `/lets:end` settles, and why it leaves the task alone
- **[../configuration.md](../configuration.md)** — `LETS_PR_FLOW` and the forges
- **[../tasks.md](../tasks.md)** — trunk-mode and the task lifecycle
