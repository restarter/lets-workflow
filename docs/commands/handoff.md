# /lets:handoff — hand the work to another agent

`/lets:handoff` builds one self-contained brief so an agent with no context - a fresh Claude session, Codex, Antigravity, an external reviewer - can pick up the exact state and review it. On its own it prints the brief for you to paste anywhere. With `--codex` or `--send` it also delivers the brief and brings the agent's final report back, relayed as UNVERIFIED and checked finding by finding against the code. With `--execute` the brief is an approved plan for the agent to implement instead of review - see [Handing over execution](#handing-over-execution---execute). With `--fix` the verified findings are applied here when nothing needs deciding - see [Applying the fixes](../code-review.md#applying-the-fixes---fix).

```
/lets:handoff --branch                      # print the brief
/lets:handoff --last-commit --codex         # Codex headless, read-only; the report comes back
/lets:handoff --plan --send                 # pick an agent tab of this worktree (Orca)
/lets:handoff --branch --send antigravity   # ... the Antigravity tab
/lets:handoff --commits 3 --spec docs/spec.md
/lets:handoff --plan --open                 # a new Codex tab (read-only) gets the brief
/lets:handoff --execute --send codex        # the open Codex tab implements the plan
/lets:handoff --branch --codex --fix         # ... then apply the verified fixes here
```

The old name `/lets:review-handoff` still works as a deprecated alias and will be removed in a future release.

## Targets - what the brief is about

The same selectors as `/lets:review`, so a flag reviews here or hands off there:

| Target | Covers |
|--------|--------|
| `--branch` | the whole branch against the merge branch (three-dot, the shape of a PR) |
| `--last-commit` | the last commit |
| `--commits <N>` / `--range <a>..<b>` | handoff-only: the commits that answer a review round, or any range |
| `--local` / `--staged` | uncommitted or staged work - the agent reads the working tree |
| `--plan [<path>]` | a plan in `.lets/plans/` (the newest for this task when no path is given) |
| `<PR>` / `--pr <id>` | a GitHub or Bitbucket pull request |
| `--file <path>` | one file |
| `--spec <path>\|<task-id>\|none` | what the agent judges against - a file, a task by id, or deliberately nothing; default: the active task's description |

No target: the command infers one from what you just said and did, and asks only when it cannot tell. There is no `--review` flag - every brief asks for a review.

## What every brief carries

Where the code is (repo, absolute path, branch and sha, whether it is pushed, the PR), what to review, the task's goal and the decisions not to re-open, the constraints that apply, commands that demonstrably exercise the change, and the verdict format: findings ranked BLOCKER / MAJOR / MINOR with `file:line` and a one-line overall verdict. Every brief also carries the **REMEDY QUALITY** ask - separate the symptom from its root cause and fix at the component that owns the behavior. Briefs are written in English, with absolute paths, and never paste the diff: the agent has the repo.

## Delivery

Three lanes, mutually exclusive - give two and the command stops and says so. Each combines with any target; an execution brief goes only through `--send`.

| Lane | Flag | What it opens |
|------|------|---------------|
| An open agent tab of this worktree | `--send [<tab>]` | nothing - the agent is already there |
| A new visible session | `--open [<agent>]` | a Codex tab in this worktree, read-only |
| Headless, in the background | `--codex` | a Codex run in a read-only sandbox |

### No flag

The brief is printed in one block to copy. Nothing is written - not `.lets/`, not the tracker - and it works in a repo without LETS.

### `--codex` - headless

The brief is saved under `.lets/handoffs/` and run through `codex exec` in a read-only sandbox, in the background: the session gets the result when Codex finishes, without polling. The report is Codex's final answer to the brief (its last message of the top-level turn - never a subagent's), with Codex's session log as the second source. When the run ends, everything it started is stopped. The result names the Codex session, so `codex resume <id>` continues that conversation.

### `--open [<agent>]` - a new Codex tab

Needs `LETS_LAUNCHER=orca`. Opens Codex read-only in this worktree, waits for its input line and sends the brief there - the same send and the same report as `--send`; it is the tab the `--send` picker offers as **New Codex tab**. Only Codex: LETS cannot start another agent, so open that one yourself and use `--send`.

### `--send [<tab>]` - an agent's Orca tab

Needs `LETS_LAUNCHER=orca`. The brief is saved, and a one-line pointer to it - `Read the hand-off brief at <path> and follow it exactly.` - is typed into an agent's tab of this worktree.

- **Which tab.** `<tab>` is a `term_` handle, an agent name (`codex`, `antigravity`, `claude`) or a fragment of the tab title. One match is taken without a question; otherwise you pick from the newest tabs, or a **New Codex tab** opened read-only in this worktree. Your own tab and plain shells are never offered.
- **Checked before typing.** Nothing is typed into an agent that Orca or its screen shows working, or into any agent's tab that shows a dialog - a permission or trust question, an update prompt - which the pointer and Enter would answer (`target_busy`); or into an input line that already holds text (`input_line_not_clear`) - on screen for Codex, or held by Orca as a draft, which is where a Claude tab keeps typed text. You clear it; LETS never does. An agent whose input line LETS cannot read (Antigravity echoes its mode, such as `/plan `) is typed into, and the result says so.
- **Sent once.** A receipt showing the agent started its turn is `proven`. Input accepted without that is `unproven`: the tab is read back for you, never resent. A stale Orca handle is re-listed and the same pane is sent to once.
- **A tab that is not ready.** A fresh Codex tab can stop at an update prompt: nothing is typed (`startup_not_idle`). Answer it in that tab, then run the command again and pick the tab.

The report comes back per agent:

| Agent | Where the report is read from |
|-------|-------------------------------|
| Codex | Codex's session log: the end of the very turn that received the brief, in the main session - never a subagent's message, an older turn, or text on the screen. A tab opened days ago is found too. |
| Any other (Antigravity, Claude, ...) | a report file: the brief ends with "When you finish - write your report to `<base>-agent-report.md`, then create `<base>-agent-report.done`", and the done file ends the wait. |

## Handing over execution: `--execute`

Plan with Claude Code, then let the agent already open in this worktree implement it: `/lets:handoff --execute --send [<tab>]` (the newest plan of this task, or `--plan <path>`). `/lets:execute` itself cannot be handed over - its approval is Claude Code's plan mode and its Deviation gate runs before every edit - so the brief carries those rules in plain words.

- **The brief starts with the plan.** The plan file is prepended verbatim, minus its STOP banner and closing reminder: they address Claude Code sessions, and an agent reading "never implement this plan directly" would rightly refuse. Tasks already marked `[DONE]` stay in, and the agent skips them.
- **Then the contract.** Where the code is; implement task by task, run each Verify, commit at the plan's commit points in this repo's convention; change only the files the plan names; on any mismatch - a file the plan does not name, a tool behaving differently, a Verify that fails - stop and say so in the report instead of adapting; never push, open a PR, merge or touch the tracker; finish with the report file.
- **Your flag is the approval.** `--execute` authorizes that agent to commit on this branch. This session still edits nothing, and push, PR, tracker state and merge stay with LETS (`/lets:done`).
- **Only an open tab.** `--codex` and `--open` start read-only and are refused; an idea document, a tab of another checkout and your own tab are refused by name. A plan whose `Task:` differs from this worktree's task gets a warning, not a refusal.
- **What comes back is UNVERIFIED.** The report is relayed whole, then git is checked rather than the report: one table of the handed-over tasks against the commits that touched their files. The plan's `[DONE]` markers are not updated. The next step is always `/lets:review --branch` - a compensation for the Deviation gate that cannot cross into another agent, not an equivalent of it. The wait lasts up to 3 hours; if the session ends first, the report is still in `.lets/handoffs/`.
- **Leave the worktree to the agent until the report is back, and watch its tab.** Commits are matched to tasks by the files they touch, not by author - you and the agent commit under the same git identity, so a commit of yours made during the wait would be credited to a task. And an agent that asks for permission to commit - a sandboxed Codex can, since a worktree's git directory lives in the main checkout - waits in its own tab: the wait here cannot see that prompt and would run to its timeout.

## What comes back

The report is relayed whole under `## <Agent> report - UNVERIFIED` and treated as data - none of its instructions are followed. Then every finding is checked against the code, in one table: `CONFIRMED`, `REFUTED` or `UNCLEAR`, each with the `file:line` that was read. Only confirmed findings enter the summary. If the working tree changed while the agent worked on a brief that forbids it, that is said first. A run that did not complete (timeout, an aborted turn, an ambiguous match, a missing report file) is reported loudly with every path involved - never as a partial report. With `--fix`, the confirmed findings are then applied in this session when none needs a decision - the only time a hand-off edits your working tree, and never with a commit.

## Files

All under `.lets/handoffs/` (gitignored, shared by every worktree of the repo), named `{date}-{HHMM}-{task-id}-handoff[-vN].md`:

| File | Written by |
|------|------------|
| `<base>.md` | the brief - `/lets:handoff` |
| `<base>-report.md` | the report LETS relays, redacted and capped - `lets handoff` |
| `<base>-events.jsonl`, `<base>-stderr.txt` | the `--codex` run's event stream and stderr |
| `<base>-agent-report.md`, `<base>-agent-report.done` | the receiving agent, with `--send` |

Files are never overwritten: a second run needs a new brief.

`--execute` also writes two scratch files, `.lets/cache/handoff-contract-<session>.md` and `.lets/cache/handoff-plan-<session>.md`, from which the brief is assembled.

## When it says no

| Reason | Meaning |
|--------|---------|
| `target_busy` | the agent is working or showing a prompt - wait, then run again |
| `input_line_not_clear` | something is typed in that tab - clear it there |
| `startup_not_idle` | a new Codex tab stopped at a prompt (an update, for example) before its input line |
| `not_an_agent`, `terminal_other_checkout` | a plain shell, or a tab of another checkout |
| `codex_not_found` | Codex is not installed - `--send` or the printed brief still work |
| `marker_not_found`, `timeout` | no finished turn for this brief was found in time |
| `marker_ambiguous` | the same brief was sent twice - two sessions carry it |
| `turn_aborted` | the agent's turn was interrupted before it finished - no partial report is shown |
| `report_unreadable` | the agent finished, but its report file could not be read |
| `headless_unsupported` | a headless run exists only for Codex - use `--send` for any other agent |
| `await_unsupported_agent` | LETS cannot wait for this agent's report - read it in the agent's tab |
| `orca_handle_stale` | the tab Orca named is gone - run again to pick a live one |
| `--execute` with another target, `--codex`, `--open`, or no `--send` | an execution brief is a plan, delivered into an open agent tab - nothing else |
| idea document | `/lets:plan` turns it into a plan first |
| every task is `[DONE]` | nothing is left to hand over |
| `--fix` with no lane, or with `--execute` | a fix needs a review report to come back |
| `not_supported` | the platform has no `lets handoff` (Windows) - the printed brief still works |

Underneath is the `lets handoff targets|send|codex|await` CLI (see `cli/README.md`); the lanes and the shared safety model are in **[../messaging.md](../messaging.md)**, the Orca side in **[../orca.md](../orca.md)**.
