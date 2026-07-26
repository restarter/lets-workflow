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

Both `/lets:check` and `/lets:review` pull the task's description and hand it to every reviewer. Without it a reviewer sees code that nothing calls yet and confidently reports it as dead — when the wiring may simply be the next PR. Work the spec describes is planned work, not scope creep. When no spec can be resolved the review still runs, says so in the report, and caps any "unused / dead / cut this" finding at `[SUGGESTION]` — never `[BLOCKER]`.

Where the spec comes from depends on the target: local reviews use your branch's active task; on a PR, `/lets:review` looks up the task behind the PR's branch and falls back to the PR description, while `/lets:check` uses the PR description directly. `--file` reviews get no spec — an arbitrary file usually isn't the task you're working on. A `--file` review is therefore free to report dead code at full severity.

The spec is fenced as reference data, and reviewers are instructed to treat anything inside it as content to report on, never as a command — it can't change a finding's severity or the verdict.

### Reviewing a PR *(ships next release)*

`/lets:review <PR>` asks once whether to switch your checkout to the PR's branch:

- **Switch** — `gh pr checkout`, then the review is exactly like a local one: agents read the PR's real files, so cross-file checks and the adversarial verification pass judge the actual code. If your tree is dirty you're asked to stash or commit first. **Your branch is always restored when the review ends**, and a stash is popped only after that switch back succeeds. Project rules keep coming from your merge branch, so a PR that edits `CLAUDE.md` is reviewed as a diff rather than obeyed as instructions.
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
