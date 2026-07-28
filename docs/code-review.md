# Code review

LETS has three levels of review, from a 30-second sanity check to a full PR lifecycle.

| Need | Command | Time | What happens |
|------|---------|------|--------------|
| Quick pre-commit check | `/lets:check` | ~30s | Inline 6-lens review: bugs, security, performance, quality, project conventions, docs. No subagents. |
| Full code review | `/lets:review` | ~2-3 min | Dynamic agent selection — only the experts relevant to your changes. |
| Full PR lifecycle (GitHub) | `/lets:github-pr <PR>` | Interactive | Analyze, discuss, post inline comments, follow up on fixes, approve. |

Both `/lets:check` and `/lets:review` accept the same targets: working tree (default), staged changes, last commit, the full branch vs the merge branch (`--branch` — three-dot diff against `$LETS_MERGE_BRANCH`, the same shape GitHub renders for a PR), a PR, a specific file (`--file <path>`), or a plan (`--plan`).

**Rule of thumb:** small change → `/lets:check` → commit. Significant change → `/lets:check` → `/lets:review --local` → fix → commit → PR. Multi-commit branch heading for a push → add `/lets:review --branch` for a final PR-equivalent pass before pushing. PR already open → `/lets:review <PR>`, or the full `/lets:github-pr` lifecycle.

## `/lets:check` — quick sanity check

A fast inline pass before you commit. Six perspectives — bugs, security, performance, quality, project conventions, docs — no subagents, ~30 seconds. It's the lighter version of `/lets:review`, with the same target flags. Run it before any commit, or as a first pass on a PR.

## `/lets:review` — full code review

A deep multi-agent review. The command analyzes what actually changed and brings in only the relevant experts — the security agent for auth code, the database agent for migrations, the architect for structural changes. A pure docs update gets docs + compliance and skips the rest; a full-stack feature can pull in up to 12 agents, each focused on its domain.

Findings come back tiered:

- `[BLOCKER]` — must fix
- `[SUGGESTION]` — should fix
- `[NIT]` — nice to have

Agents are tuned to skip the obvious and focus on what matters — the goal is signal, not a wall of nitpicks.

For plan reviews (`/lets:review --plan`), agents are selected from signals in the plan content (mentions of migrations, API endpoints, Docker configs, …).

See **[agents.md](agents.md)** for the agent roster and how selection works.

### The reviewers know what you were building *(ships next release)*

Both `/lets:check` and `/lets:review` pull the task's description and put it in front of the reviewers — the expert agents for `/lets:review`, the inline pass for `/lets:check`. Without it a reviewer sees code that nothing calls yet and confidently reports it as dead — when the wiring may simply be the next PR. Work the spec describes is planned work, not scope creep. When no spec can be resolved the review still runs and says so on any scope finding — but it does **not** downgrade that finding. A missing spec is missing information about intent, not evidence that the code is fine, so the severity stays whatever the reviewer judges it to be. That matters most if you work without a task tracker at all: your reviews are exactly as sharp as everyone else's.

**`/lets:review` asks where it is.** Once, before the agents start, in the same prompt as the PR-branch question — so it's one interruption, not two. The options name what it actually found rather than asking you to type a path: your task by id and title, the newest plan file matching this branch by name, or **no spec**. "Other" takes a task id or any path you like.

"No spec" is a real answer, not a failure — pick it and the review runs with no spec block and no caveat about running without one. That's for projects that genuinely don't keep specs; being reminded on every single review would be noise. A lookup that *failed* is different: there the review says so, because something may exist that it couldn't read.

`--spec <path>`, `--spec none` or `--spec <task-id>` answers it up front and skips the question — useful when your specs always live in the same place. It's also how `/lets:check` gets the same control: check never asks, because you fire it repeatedly while writing code; review asks, because it's the considered checkpoint you run once or twice.

**A PR description is never treated as the spec.** It's the author's account of what they built, which is a different thing from what was supposed to be built — so it goes in its own block, honestly labelled, and a PR with no resolvable task simply has no spec. The reviewers still read the description and judge for themselves; they just aren't told "this was planned, don't report it" on the author's say-so.

Where the spec comes from by default depends on the command and the target. `/lets:review` uses your branch's active task locally, and on a PR the task behind the PR's own branch. `/lets:check` uses your branch's active task locally and has no spec on a PR unless you pass `--spec` — resolving a task from a PR branch is `/lets:review`'s job, so that rule lives in one place. `--file` reviews get the spec too: a file is often exactly the task, and with acceptance criteria in hand ("a CSV of every US state, skip none") the review becomes a completeness check rather than a taste test.

### On a PR, the reviewers also read the PR itself *(ships next release)*

Three different things reach a reviewer and only one of them is the spec. The spec says what was *supposed* to be built. The PR description says what the author *claims* they built. The discussion says what people have *already said* about it. `/lets:review <PR>` now hands over all three — previously the description arrived only when no spec could be found, and the discussion never arrived at all.

`/lets:check <PR>` gets the same two blocks - it has no verification pass to withhold them from, and the description was already being fetched there anyway. The only difference is that check never asks how much discussion to load: it takes the anchored parts plus recent comments and leaves the rest, since asking is the thing check does not do.

The discussion is the one that saves you re-reading the same objection twice. If a thread on `foo.go:42` already says "raised → fixed in abc1234 → verified", a reviewer that can see it says so instead of filing the finding again. Gathering it takes three separate calls on GitHub: comments under the PR, review summaries, and the inline comments anchored to lines — the last of which appear in none of the obvious ones. On a busy PR you're asked whether to load the whole thread history or just the anchored parts.

None of it reaches the adversarial verification pass. Everything in that block is written by the author of the code being judged, or by people commenting on it, and a verifier can only answer "real or not" — so one "we agreed to ignore this" in a thread would delete a finding outright. Reviewers can weigh it and still report; a verifier can't.

A spec is only used to decide whether a finding of the shape "dead / unrelated / cut this" is planned work. It never softens a correctness, security, or logic finding, and the adversarial verification pass gets a deliberately narrower version of it — a verifier that can only answer "real or not" must not be handed a rule about severity. On a PR the verifiers get no spec at all: whatever it came from, it is the PR author's own task or their own plan file, so it is their account of their work either way.

### Reviewing a PR *(ships next release)*

`/lets:review <PR>` asks once whether to switch your checkout to the PR's branch:

- **Switch** — checks out the PR at a detached HEAD, and the review is then exactly like a local one: agents read the PR's real files, so cross-file checks and the adversarial verification pass judge the actual code. If your tree is dirty you're asked to stash or commit first. **Your branch is restored when the review ends**, and a stash is popped only after that switch back succeeds. Project rules (`CLAUDE.md`) are still read from your merge branch, not from the PR — the PR's own edits to them show up in the diff, where a reviewer judges them instead of obeying them.

  If a session dies mid-review, the return trip is `git checkout <your branch>` (the review prints that command when it switches) plus `git stash pop` if it stashed. The next `/lets:review <PR>` and the next `/lets:start` both notice the leftover and tell you.
- **Review from diff** — nothing on disk changes. The reviewers work from the diff alone and are told plainly that the files on disk are the base branch, so they don't mistake old file contents for the PR's.

It **never creates a worktree** — where you review is your call. Run it from your main checkout or from any worktree; the question is the same in both. `--json` never touches your working tree at all.

## `/lets:github-pr` — the PR lifecycle

This is where LETS shines: reviewing a PR from the terminal with expert agents instead of in a browser. GitHub only — Bitbucket and local flows finish tasks with `/lets:done` and don't have a PR review lifecycle.

```
/lets:github-pr https://github.com/owner/repo/pull/42
```

1. **Analyze** — agents review the PR based on what changed (security agent for auth code, database for migrations, …).
2. **Discuss** — you see each finding with its code context and decide what to do with it: post it as an inline comment, fold it into the summary, or drop it.
3. **Post** — batch-posts the inline comments to the exact lines on GitHub, with a review summary.
4. **Follow up** — after the author pushes fixes, `/lets:github-pr --follow-up` checks each finding: fixed, not fixed, or needs discussion.
5. **Approve** — `/lets:github-pr --approve` when it's ready, or request changes.

### Responding to a review

If you're the PR author, `/lets:github-pr --respond <PR>` triages the comments on your PR, auto-fixes the mechanical ones, and posts replies.

## Dynamic agent selection

Agents aren't hardcoded into a review. Each command looks at your changes and picks only the relevant experts:

- Changed auth code? Security + backend + architect join.
- Pure docs update? Docs + compliance, nothing else.
- Full-stack feature? Up to 12 agents, each on their domain.

The same idea applies to plan reviews — agents are chosen from signals in the plan text.

## See also

- **[agents.md](agents.md)** — the 14 agents and what triggers each
- **[plan-execute.md](plan-execute.md)** — reviewing a plan before executing it
- **[commands.md](commands.md)** — `/lets:check`, `/lets:review`, `/lets:github-pr` flags
