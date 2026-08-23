---
description: Generate a self-contained hand-off brief so ANOTHER agent (a fresh session, Codex, any external reviewer) can pick up the exact state and review it - a plan, a branch, the last commits, a PR, or one file. Produces one pasteable brief; reviews nothing itself.
argument-hint: "[PR-url-or-number|--pr <id>|--local|--staged|--last-commit|--branch|--plan [<path>]|--file <path>|--commits <N>|--range <a>..<b>] [--spec <path>|none]"
---

# Review Handoff - Brief for an External Reviewer

Produce ONE message the user copies into another agent. That agent has NO context: not this conversation, not the task, not even which repo. The brief must be complete on its own.

- The brief is the deliverable - do not review anything yourself, do not edit any file
- Target selectors mirror `/lets:review`, so the same flag reviews here or hands off there; works in any git repo, with or without LETS

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```bash
/lets:review-handoff                      # infer the target from context; asks only if genuinely ambiguous
/lets:review-handoff <PR-url-or-number>   # GitHub or Bitbucket PR
/lets:review-handoff --pr <id-or-url>     # alias of the bare form above
/lets:review-handoff --local              # uncommitted changes
/lets:review-handoff --staged             # staged changes only
/lets:review-handoff --last-commit        # last commit
/lets:review-handoff --branch             # full branch vs the merge branch (three-dot, like a PR)
/lets:review-handoff --plan               # newest plan in .lets/plans/
/lets:review-handoff --plan <path>        # a specific plan file
/lets:review-handoff --file <path>        # one file
/lets:review-handoff --commits <N>        # handoff-only: the last N commits (a fix round)
/lets:review-handoff --range <a>..<b>     # handoff-only: an explicit range
/lets:review-handoff --spec <path>        # what the reviewer judges against (or a bare task id)
/lets:review-handoff --spec none          # deliberately no spec - no spec line, no caveat
```

Selectors match `/lets:review`, with two deliberate differences: `--commits` / `--range` are **handoff-only** (review has no target for "the commits that answer a review round"), and `--pr` is kept as an alias because it is the spelling this tool shipped with. Review's output modifiers `--json` and `--workflow` are **not** implemented here.

## Step 1: Determine the target

- PR URL/number, or `--pr <id-or-url>` -> **PR mode**. `--local` / `--staged` / `--last-commit` / `--branch` / `--commits N` / `--range a..b` -> **local mode**. `--plan [path]` -> **plan mode**. `--file <path>` -> **file mode**.
- **Host resolution.** A `github.com` URL -> `gh`; a `bitbucket.org` URL -> `bbb`, whose PR number sits in the `/pull-requests/<n>` segment, not github's `/pull/<n>`; a bare number -> `{LETS_PR_FLOW}`. **When `{LETS_PR_FLOW}` is empty** - the normal state outside a LETS project, where the hook emits only four keys - fall through to the forge named by the `origin` URL Step 2 prints. Only when neither names a github or bitbucket host: stop and say a PR hand-off needs one.
- **No argument -> infer, do not ask by default.** Take the target from the user's sentence next to the command ("цих правок" -> the just-committed fixes; "цієї гілки" -> `--branch`; "план" -> `--plan`; "коміта" -> `--last-commit`) and from what just happened in the session. Only when genuinely ambiguous, ask **one** `AskUserQuestion` (header `Target`, `multiSelect: false`) offering Local changes / Branch / Plan / Last commit. Otherwise decide, and name the choice in the closing line.
- The flag-only targets (`--staged`, `--commits`, `--range`, `--file`) are not in the interactive menu.

## Step 2: Locate

```bash
ROOT=$(git rev-parse --show-toplevel); GIT_DIR=$(git rev-parse --git-dir)
echo "root=$ROOT branch=$(git branch --show-current)"
echo "head=$(git rev-parse HEAD) $(git log -1 --format=%s)"
git remote -v | head -2
case "$GIT_DIR" in *worktrees/*) echo "worktree=yes main_repo=$(git rev-parse --path-format=absolute --git-common-dir | sed 's#/.git$##')";; *) echo "worktree=no";; esac
git status --short | head -20; echo "dirty_files=$(git status --short | wc -l | tr -d ' ')"
# The hook resolves this per-repo, correctly, including in a repo with no LETS. Do NOT add a
# fallback resolver here: it would mask a config-layer defect for every other consumer of the key.
MB="{LETS_MERGE_BRANCH}"
BASE=$(git merge-base HEAD "origin/$MB" 2>/dev/null || git merge-base HEAD "$MB" 2>/dev/null)
echo "merge_branch=$MB base=$BASE ahead=$(git rev-list --count "$BASE"..HEAD 2>/dev/null)"
git log --oneline "$BASE"..HEAD 2>/dev/null | head -30
```

`{LETS_MERGE_BRANCH}` is substituted by the orchestrator before the block runs - never `$LETS_MERGE_BRANCH`, which yields empty in a bash block. When `BASE` comes back empty, say so in the brief and name the branch that was tried, rather than emitting a base the reviewer cannot act on. Also carry into the brief: whether `.lets/` exists, and a nested-repo layout (a repo under `code/<name>/` of a parent workspace - name BOTH paths).

## Step 3: Target facts

**Run the commands, never recall from memory** - the same rule as Step 2, and it binds hardest here, because this is where a session that already did the work is most tempted to narrate it. A commit's intent comes from its message; whether a file is new or modified, and how many lines moved, comes from `git show --stat` / `git diff --stat`. A brief that calls an appended-to file "new" costs the reviewer their trust in every other line of it.

| Mode | Gather |
|---|---|
| `--plan` | absolute path, title line, task count, `[DONE]` markers, which code it will touch, whether execution started (commits since the plan's date). An unspecified path resolves task-id-first (`*<task-id>*.md`), then `*<branch-slug>*.md`, both scoped - `.lets/plans` is shared across worktrees. **Name the file taken**; it may be a superseded revision |
| `--branch` | `git diff --stat <base>...HEAD`, the commit list, base sha, whether pushed (`git rev-parse origin/<branch>`), open PR id if any |
| `--local` / `--staged` | `git diff --stat` (or `--staged`), the file list, and that the work is uncommitted - the reviewer reads the working tree, not a ref |
| `--last-commit` / `--commits N` / `--range` | exact shas, `git show --stat` per commit, and **why** they exist - which findings they answer, quoting the finding ids or the reviewer's wording |
| `<PR>` / `--pr` | PR id + URL, host, target branch, head sha; for a fix round, each reviewer point mapped to its answering commit, plus points deliberately NOT addressed and why |
| `--file` | absolute path, what the file is, what the reviewer should judge it against |

## Step 4: Task and spec context

`Skill(skill: "lets:detect-task", args: "fallback=no")` yields the id; the neutral `show` yields the metadata. Two calls, two independent outcomes - report them as such, and derive an identifier **no other way**.

| detect-task | `show` | Brief carries |
|---|---|---|
| an id | succeeded | `Task: <tracker> <id> "<title>"` + the goal inlined in 2-4 lines from the description |
| an id | failed / unsupported (`LETS_TRACKER=none`) / no adapter | `Task: <id> (tracker metadata unavailable - title and description could not be read)`. The id came from the branch name or the `.task-<slug>` file and is real; a second, independent call failing is no reason to discard it |
| None | not called | the `Task:` line is **omitted entirely**. Never emit a guessed or fabricated id |

Never render a `url` - beads returns none; include a link only when the adapter's `show` declares it. `--spec <path>` is named in the brief as an absolute path, a bare task id resolves through the tracker, `none` means no spec line and no caveat; with no `--spec` the active task's description is the spec. A spec that is a **file** is named by path - the external agent opens it; a spec that is the **tracker task** is inlined, because that agent cannot reach the tracker.

## Step 5: Compose the brief

```
# Review hand-off: <one-line what>

## Where the code is
- Repo: <name> (<remote url>) - <what it is, one line>
- Path: <absolute path>  [worktree of <main repo path> | standalone | nested under <parent workspace path>]
- Branch: <branch> @ <sha> (<ahead> commits over <merge-branch>@<base sha>); pushed: yes/no; PR: <id/url or none>
- Working tree: clean | N uncommitted files (list)
- Task: <tracker> <id> "<title>"

## What to review
<mode-specific: the plan path / the commit list with shas and one-line intent each / the diff range / the PR + fix mapping>

## Context the reviewer needs
- Goal of the task in 2-4 lines
- Decisions already taken (do NOT re-litigate): ...
- Constraints / hard rules from CLAUDE.md or the spec that apply: ...
- Known/expected failures and why (e.g. a named RED test): ...

## What we want from the review
- <the specific question(s): correctness of X, spec conformance vs <doc path>, did the fixes answer the reviewer's points, risk in Y>
- REMEDY QUALITY: for each finding, separate the observed symptom from its root cause, and propose the smallest coherent fix at the component that canonically owns the behavior. A local workaround is acceptable only when the systemic correction is disproportionate - say why. Do not widen a local issue into speculative refactoring.
- Verdict format requested: findings ranked BLOCKER / MAJOR / MINOR with file:line, plus a one-line overall verdict

## How to verify locally
<exact commands: build, unit, the relevant gated target, any prerequisite (docker, env vars)>

## Do NOT
- push, commit, or modify files; touch zones: <list>; re-open decisions above
```

The `REMEDY QUALITY` line is **standing text, not a per-run judgement call** - every brief, every mode; the hand-off is the only moment this command gets to set the external reviewer's contract. `How to verify locally` carries commands that **demonstrably exercise the change**, not plausible-looking ones. Name a test by its actual function name rather than a guessed `-run` filter - a filter that matches nothing exits 0, so the reviewer is handed a green run that never touched the thing under review. Carry any caveat the repo documents for those commands (this repo: `-count=1` on Go tests that read `plugins/`, or the cache serves a stale PASS).

Composition rules, stated here and nowhere else in this file: the brief is in **English** regardless of conversation language; paths absolute; shas full or 12+ chars; no "as discussed"; under ~80 lines; drop any section that has nothing; **never paste the diff** - the reviewer has the repo, so pointers and verification commands beat a dump.

## Step 6: Deliver

Print the brief inside ONE fenced block so it copies cleanly. Then one line naming what was inferred as the target, so the user can correct it.

## Keeping this file consistent with `/lets:review`

Two things here are restatements of `/lets:review`, not independent decisions: the **target selectors** (every flag except the handoff-only `--commits` / `--range` and the `--pr` alias) and the **forge host resolution** (`github.com` -> `gh`, `bitbucket.org` -> `bbb` with the PR number in `/pull-requests/<n>`, bare number -> `{LETS_PR_FLOW}`). Change one and change the other - no test holds them together, so a forge change landing in `review.md` alone breaks a Bitbucket hand-off silently.

## Rules

- The brief is the deliverable - review nothing, edit nothing, write no file (not `.lets/`, not the tracker)
- Conversation in the user's language; the brief's English is stated at Compose
- Absolute paths and internal task ids **belong** in the brief - the audience is an agent on the same machine, not an external channel
- No `.lets/` -> skip plan lookup; `--plan <path>` with an explicit path still works

## Output

Close footer: one prose line, no LETS box. The next step is the user pasting the brief into another agent, which is not a `/lets:*` command.
