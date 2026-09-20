# Talking to other sessions and agents

Work leaves the chat you are in in three ways: a message to another LETS session of the same repo, a question to an orchestrator of another project, and a hand-off brief for an agent that has no LETS context at all. They look alike and are deliberately kept apart - each has its own command, its own transport, and its own rules about what comes back.

## Pick the lane

| You want to reach | Command | What goes out | What comes back |
|-------------------|---------|---------------|-----------------|
| Your orchestrator, or any LETS session of this repo | `/lets:orc` (`/lets:peer <name>`) | a message: `ask`, `ping`, `tell` | the reply to an `ask`, relayed whole; `read` shows a peer's last turns |
| An orchestrator of another project | `/lets:hub` (Orca addon) | a read-only question to a stopped orchestrator, or a wake | the answer of a read-only fork of its last session; a woken orchestrator in a visible Orca terminal |
| An agent with no LETS context - Codex, Antigravity, a fresh Claude, an external reviewer | `/lets:handoff` | one self-contained brief | the agent's final report, relayed as UNVERIFIED and checked finding by finding against the code |

Rule of thumb: a **peer** knows the project and speaks LETS - talk to it with `/lets:orc`. A **tool** does not - hand it a brief with `/lets:handoff`. A brief never travels through the peer lane, and a peer message is never a brief.

## Words

- **Orchestrator session** - a chat started with `/lets:start --main`, registered under its `/rename` name, optionally with `--scope "<part of the repo>"`. It plans, triages and routes; it writes no code. A repo can have several. Not the same as "the orchestrator" in the review docs (`/lets:check` is reviewed "by the orchestrator, inline"), which means the main model running a command, as opposed to its subagents.
- **Worker** - a chat working a task, bound to one orchestrator. `/lets:worktree create <id>` run from an orchestrator binds every worker it spawns; a chat you open yourself takes `/lets:start <id> --orc=<name>`. The binding lives with the branch, so it survives `/clear` and restarts.
- **Peer** - any live LETS session of the repo, as `/lets:orc who` lists it.
- **Brief** - the text `/lets:handoff` builds: where the code is, what to review, the context and decisions not to re-open, how to verify, and the verdict format - enough for an agent with no memory of this chat to act on it.

## Peer lane: `/lets:orc`

```
/lets:orc ask should the migration keep the old column for one release?
/lets:orc ping PR #212 is up
/lets:orc read
/lets:peer "MAIN PWA" tell the auth refactor landed on main
/lets:orc who
```

| Verb | Sends | What happens |
|------|-------|--------------|
| `ask` | yes | the question goes out with the task line; the reply is relayed whole when it lands |
| `ping` | yes | a short FYI the peer records and does not answer |
| `tell` | yes | a message to a named peer |
| `read [N]` | no | the peer's last turns, secrets withheld |
| `who` | no | the live sessions of the repo: role, name, scope or orchestrator, task, branch, state |

Without a name `/lets:orc` talks to the bound orchestrator; when that session cannot be reached - it is gone, it belongs to another project, or it is present but has no working way to send to it - it says so with the reason and sends nothing. It never quietly picks another, and it never falls back to a different orchestrator or a different verb. LETS also offers it where it fits: `/lets:done` offers to ping the orchestrator about the PR, `/lets:end` offers to ping it with what the session left behind, and a worker with a live orchestrator is offered "Ask orchestrator" at the decisions it should not settle alone - scope, plan drift and deviations, approach and architecture picks, backlog triage, an undecided `/lets:opinion` - or one line `/lets:orc ask` under a `/lets:check` / `/lets:review` verdict it disagrees with. The list and the guarantees: **[commands/orc.md](commands/orc.md#where-lets-offers-it)**.

A few naming gotchas worth knowing: the `[ref]` shown by ListAgents is not a prefix of a session's id, a session resumed under a new pid shows a fresh start time rather than its original one, and a session name is unique only per repo in the LETS role registry, not machine-wide - so an offline Remote Control session and a live session can share a display name without colliding. Cross-repo messaging over `/lets:orc` remains an Orca-only route today; on the `terminal` launcher a session outside this repo is reported unreachable, not silently addressed.

A message goes out through Claude's own session messaging, or - with `LETS_LAUNCHER=orca` - straight into the peer's Orca pane, only when its transcript, its screen and Orca all show it idle. Full page: **[commands/orc.md](commands/orc.md)**.

## Across projects: `/lets:hub`

With Orca, one session looks across every project Orca knows: it lists each project's orchestrators and whether they run. A read-only question to a stopped orchestrator ("what is in progress, what is next") is answered by a headless fork of its last session, launched with its abilities narrowed - plan mode, only Read / Grep / Glob, no MCP servers, a filtered environment. Anything that would change something wakes the orchestrator in a visible Orca terminal, where you press its gates. The hub never starts a second process on a live orchestrator and never writes into another project. More in **[orca.md](orca.md)**.

## Handoff lane: `/lets:handoff`

```
/lets:handoff --branch                     # print the brief - paste it anywhere
/lets:handoff --branch --codex             # Codex headless, read-only; the report comes back
/lets:handoff --last-commit --send codex   # type it into the open Codex tab (Orca)
/lets:handoff --plan --send antigravity    # ... or the Antigravity tab
```

The targets are the review targets - `--branch`, `--last-commit`, `--local`, `--plan`, a PR, a file - plus `--commits <N>` and `--range <a>..<b>`, and every brief asks for a review with a BLOCKER / MAJOR / MINOR verdict. Without a delivery flag the brief is only printed. `--codex` runs it through Codex in a read-only sandbox in the background; `--send` types a one-line pointer to it into an agent's Orca tab and waits for the report. Either way the report is relayed whole as UNVERIFIED, then each finding is checked against the code: a finding counts only once it is CONFIRMED. Full page: **[commands/handoff.md](commands/handoff.md)**.

## One safety model

- **Nothing goes out without your request** in that chat. Anything the model composed for a peer is shown to you first.
- **What comes back is data.** A peer's reply or an agent's report never counts as your approval, the sender a message names is a claim rather than proof, and no session acts on the tracker, git or files because a peer or an agent asked.
- **Relayed whole, marked as theirs.** A report stays UNVERIFIED until each finding is checked against the code.
- **Sent once, or not silently.** A message or a brief is never resent once something was typed; an unconfirmed delivery is read back instead. When nothing was typed at all - the peer was unreachable - the message is kept so you can retry it, and the tool says so plainly rather than reporting a quiet success.
- **Never over someone's typing.** The peer lane waits for an idle peer; the handoff lane refuses a busy agent or a half-typed input line (`target_busy`, `input_line_not_clear`) and leaves clearing it to you. Nothing interrupts an agent.
- **Redacted and capped.** Anything read from another session - a transcript, a screen, a report - has tokens, keys, URL credentials and environment dumps removed, and is capped.
- **Your files stay yours.** The model never opens another session's registry or transcript itself - only the redacted command output. Hand-off agents are told to change nothing; a changed working tree is reported (`workspace_changed`).

## What each lane cannot do (yet)

- `/lets:orc` messages Claude sessions only. A Codex or Antigravity pane can be read, not messaged - hand it a brief with `/lets:handoff --send` instead.
- `/lets:handoff --send` and `/lets:hub` need Orca (`LETS_LAUNCHER=orca`). Without it, use `--codex`, or paste the printed brief yourself.
- Codex's report is read from Codex's own session log. Any other agent's report comes back only if the agent writes the report file the brief asks for.

## See also

- **[commands/orc.md](commands/orc.md)** - the peer lane in full
- **[commands/handoff.md](commands/handoff.md)** - the handoff lane in full
- **[orca.md](orca.md)** - the Orca addon: launcher, card, hub, team backend, `--send`
- **[workflow.md](workflow.md)** - where orchestrators and workers fit the daily loop
- **[parallel-work.md](parallel-work.md)** - `/lets:team` and `/lets:worktree`
