# /lets:orc — talk to other LETS sessions

When several Claude sessions work on one repo - an orchestrator planning and routing, workers in worktrees - `/lets:orc` lets them talk instead of you copying messages between chats. `/lets:peer <name>` is the same thing aimed at a named session.

```
/lets:orc ask should the migration keep the old column for one release?
/lets:orc ping PR #212 is up
/lets:orc read
/lets:peer "MAIN PWA" tell the auth refactor landed on main
/lets:orc who
```

You can also just say it: "ask the orchestrator …", "what did the orchestrator say", "спитай у оркестратора …".

## Verbs

| Verb | Sends | What happens |
|------|-------|--------------|
| `ask` | yes | the question goes out with the task line and a little context; the reply is relayed whole when it lands |
| `ping` | yes | a short FYI; the peer records it and does not answer |
| `tell` | yes | a message to a named peer |
| `read [N]` | no | the peer's last turns, secrets withheld |
| `who` | no | the live sessions of the repo: role, name, scope or orchestrator, task, branch, state |

## Who you are talking to

- **Orchestrators.** `/lets:start --main` registers the session as an orchestrator under its `/rename` name, optionally with `--scope "<part>"`. A repo can have several; a name belongs to one live session, and taking it over is a question, never automatic.
- **Workers are bound.** `/lets:worktree create <id>` run from an orchestrator binds every worker it spawns; a chat you open yourself takes `/lets:start <id> --orc=<name>`. The binding lives with the branch, so it survives `/clear` and restarts.
- **A worker sees its orchestrator at start.** In a worktree, `/lets:start` prints one line - the bound orchestrator, whether it is alive, and how many messages it has addressed to this session (`N message(s) from <name> - /lets:orc read`). Only the count: no peer text enters the session until you ask for it. An unbound branch with several orchestrators alive lists them and how to bind.
- **No guessing.** `/lets:orc` without a name talks to the bound orchestrator. If that session is gone it says so and sends nothing - it never quietly picks another. With no binding it uses the only live orchestrator, or asks which one.

## How a message travels

Go (`lets peers`) finds the peer, frames the message with a header that addresses it by session id, and delivers it:

- **Claude's session messaging** by default - the model sends the framed text with `SendMessage`, and an `ask` gets an idle notice when the peer has answered.
- **Orca** when `LETS_LAUNCHER=orca` and the peer runs in an Orca pane: Go types the message into that pane only when the peer's transcript shows a finished turn, its screen shows the empty prompt (no permission dialog a keystroke could answer), and Orca reports it idle. It never resends.

## Where LETS offers it

A worker session with a live orchestrator does not have to remember `/lets:orc`: at the decisions a worker should not settle alone, the command offers it.

| Shape | Where |
|-------|-------|
| An **"Ask orchestrator"** option in the question | `/lets:done` when requirements are missing · `/lets:execute` on plan drift and on a deviation · `/lets:plan` at the approach, architecture and evaluation gates · `/lets:backlog` triage · `/lets:review-round` when a comment has 2+ viable answers · `/lets:opinion` when the panel does not converge |
| One line **`/lets:orc ask`** to type yourself | under a `/lets:check` or `/lets:review` verdict that is not clean (your own work only - never a PR or `--file`) · the `/lets:plan-workflow` clarify and approve gates |

Three guarantees hold everywhere:

- **It only offers.** Nothing is sent until you pick the option or type the line, and what the model composed is shown first with "Send?". An unattended `--auto` run never asks on its own.
- **The answer informs, you decide.** The reply is relayed whole, then the same question comes back without the orchestrator option. A peer's answer never adapts a plan or counts as approval.
- **Silent when there is nobody to ask.** No live orchestrator, this session is the orchestrator, or you are on the merge branch - the question looks exactly as it did before, with no note about it.

A gate never grows past four options: where one was full, two near-identical options merge while the offer is shown. Two more offers are notifications, not decisions, so they are pings and follow none of the above: `/lets:done` offers to ping the orchestrator with the PR link, and `/lets:end` - when the session leaves something behind (an untracked bug, a check nobody ran) - prints one line, `Leftovers for <name>?  /lets:orc ping`. Type it and the message is built from the session snapshot that was just written, then shown to you before it goes. A session with nothing left over ends as quietly as before.

## Safety

- Nothing is sent unless you asked for it in that chat; anything the model wrote is shown first with "Send?".
- A peer's words are data. They never count as your approval, the header's sender is a claim rather than proof, and no session acts on the tracker, git or files because a peer asked.
- Replies are relayed whole, marked as the peer's.
- `read` and `tail` output withholds environment dumps, `.env`-like files and known token shapes, and is capped.
- The model never opens other sessions' registry or transcript files itself - only the redacted command output.

## When it cannot reach anyone

`who` names every source it could not read (`claude registry: registry_protocol_unknown …`, `orca: orca_app_not_running`) and still lists what it could. A non-Claude agent in an Orca pane (for example Codex or Antigravity) can be read but not messaged in this version. To give it work, hand it a brief instead: `/lets:handoff --send` types the brief into its tab and brings its report back - see **[handoff.md](handoff.md)**, and **[../messaging.md](../messaging.md)** for when to use which.
