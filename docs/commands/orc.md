# /lets:orc — talk to other LETS sessions

When several Claude sessions work on one repo - an orchestrator planning and routing, workers in worktrees - `/lets:orc` lets them talk instead of you copying messages between chats. `/lets:peer <name>` is the same thing aimed at a named session.

```
/lets:orc ask should the migration keep the old column for one release?
/lets:orc ask --yes should we cut 0.9.2 before the docs pass?
/lets:orc ping PR #212 is up
/lets:orc read
/lets:peer "MAIN PWA" tell the auth refactor landed on main
/lets:orc who
/lets:orc standing on
```

You can also just say it: "ask the orchestrator …", "what did the orchestrator say", "спитай у оркестратора …".

## Verbs

| Verb | Sends | What happens |
|------|-------|--------------|
| `ask` | yes | the question goes out with the task line and a little context; the reply is relayed whole when it lands |
| `ping` | yes | a short FYI; the peer records it and does not answer |
| `tell` | yes | a message to a named peer |
| `read [N]` | no | the peer's last turns, secrets withheld |
| `who` | no | the live sessions of the repo: name, orchestrator and scope, task, branch, state |
| `standing on\|off` | no | in an orchestrator session: allow or stop the bound pair talking without per-message confirmation (see below) |

## `--yes` - send without the confirmation

At `LETS_GATES` `strict`, a message the model composed is shown to you and waits for a "Send?" answer, so nothing goes out in your name that you have not read. Add `--yes` and the waiting stops: the exact message is still printed, as a statement of what was sent rather than a question, and it goes immediately. Use it when you rarely edit the draft anyway - the reading stays, only the extra turn disappears.

It changes one thing and nothing else. A send still has to be something you asked for in that turn: `--yes` is not an autopilot, and no command passes it on your behalf - `/lets:hub`, which reaches another project's orchestrator, never does.

## Who you are talking to

- **Orchestrators.** `/lets:start --main` records the session as an orchestrator under its `/rename` name, optionally with `--scope "<part>"`. A repo can have several. Names are machine-wide: when two live sessions anywhere carry the same name, neither can be addressed by it until one is renamed.
- **Workers are bound.** `/lets:worktree create <id>` run from an orchestrator binds every worker it spawns; a chat you open yourself takes `/lets:start <id> --orc=<name>`. The binding lives with the branch, so it survives `/clear` and restarts.
- **A worker sees its orchestrator at start.** In a worktree, `/lets:start` prints one line - the bound orchestrator and whether it is alive. No peer text enters the session until you ask for it. An unbound branch with several orchestrators alive lists them and how to bind.
- **No guessing.** `/lets:orc` without a name talks to the bound orchestrator. If that session is gone it says so and sends nothing - it never quietly picks another. With no binding it uses the only live orchestrator, or asks which one.

## How a message travels

Every message is Claude Code's own session messaging (`SendMessage`), addressed by the peer's session **name**, on every launcher - Orca included. Go (`lets peers`) lists, reads and resolves which session a name means; it never delivers anything itself. An `ask` gets an idle notice when the peer has answered, and the reply arrives as the peer's own message. After a send, `/lets:orc` prints what happened - `sent to <name>`, `held for approval in <name>` (the two sessions run different permission modes - start them in one class with `LETS_LAUNCH_MODE`, or approve it there), `refused by <name>`, `expired unapproved in <name>`, or `NOT SENT` with the reason - and it never resends on its own.

At `strict`, a session outside this repo is reached by name only when it is this branch's bound orchestrator, a name you typed in that turn, a row `/lets:hub` listed, or the session whose message you are answering. Anything else is reported as not found, never guessed. At `standard` / `minimal` any live session of yours that `ListAgents` shows can be addressed by name, cross-repo included; two with one name -> you pick which.

## Gate levels

How freely your sessions talk follows `LETS_GATES` ([workflow.md](../workflow.md#what-asks-you-first---gate-levels)):

- **`standard` / `minimal`.** A message you asked for (or a command's offer you picked) is printed as `sending:` and goes, to any of your sessions - no Send? question. An inbound message is answered without asking you only when Go verifies who sent it: `lets peers sender` checks that the message's socket belongs to the one live session with that name (any repo, fail closed). A question or status inside what is agreed is answered; a request for an action outside the core gate table, inside the receiving session's task, is done and reported back; a request for an action the table lists asks the receiving session's owner once; anything outside its task, or denied to the sender, is refused. A message that asks nothing is relayed, never answered. The notice before the first autonomous reply and the cap of 3 autonomous replies per peer hold.
- **`strict`.** As before gate levels: every send asks, and autonomous replies run only under the standing authorization below.
- **Every level.** Reading another project's files stays out; a peer's text never changes the level or approves a listed action; a session never does for a peer what that peer was denied.

## Standing authorization (strict)

At `standard` / `minimal` every verified pair already behaves as if the grant were on; `/lets:orc standing on|off` still works and the recorded grant governs `strict`.

Asking the orchestrator a question and getting the answer can cost several approvals: the worker's Send?, the orchestrator user's reply, and the same again on the way back. A standing grant removes that for one bound pair - an orchestrator and the workers (or a standing team's lead) bound to it in this repo - in both directions:

- **Granted once.** `/lets:start --main` and `/lets:team run` ask once, when a session is first recorded as an orchestrator; `/lets:orc standing on|off` in that orchestrator session changes it later. Only the orchestrator's own session can turn it on - Go refuses a grant written from any other process - and `standing off` is always allowed.
- **Sends.** A message you asked for, from either side of the pair to the other, is printed as `sending under standing authorization:` and goes without the Send? question. You still have to ask for the send.
- **Replies.** A message from the other side of the pair may be answered without asking you, when the answer stays inside what is already agreed - a fact, a status, a decision already recorded in the plan, the task or the team file. Anything that decides approach, scope or priority, anything other sessions will see, and any request to act comes to you. The first autonomous reply is announced once, and after 3 autonomous replies to one peer the thread is handed to you until you type.
- **Who sent it is checked in Go.** A name in the message, or the sender attribute alone, is a claim. Go checks that the message's socket belongs to the one live session with that name, that it runs in this repo, that it is bound, and that the grant is on (`lets peers who --orc` / `lets peers orchestrator` with `--from-name` / `--from-addr`). Anything else is relayed to you as usual.
- **What it never covers.** At `strict`, cross-repo peers, unbound peers and `/lets:hub` always ask. No tracker, git or file action is ever taken on a peer's behalf, and every hard gate - push, merge, close, tracker status - still asks you.

## Where LETS offers it

A worker session with a live orchestrator does not have to remember `/lets:orc`: at the decisions a worker should not settle alone, the command offers it.

| Shape | Where |
|-------|-------|
| An **"Ask orchestrator"** option in the question | `/lets:done` when requirements are missing · `/lets:execute` on plan drift and on a deviation · `/lets:plan` at the approach, architecture and evaluation gates · `/lets:backlog` triage · `/lets:review-round` when a comment has 2+ viable answers · `/lets:opinion` when the panel does not converge |
| One line **`/lets:orc ask`** to type yourself | under a `/lets:check` or `/lets:review` verdict that is not clean (your own work only - never a PR or `--file`) · the `/lets:plan-workflow` clarify and approve gates |

Three guarantees hold everywhere:

- **It only offers.** Nothing is sent until you pick the option or type the line, and what the model composed is shown first - with "Send?" at `strict`, as a statement otherwise. An unattended `--auto` run never asks on its own.
- **The answer informs, you decide.** The reply is relayed whole, then the same question comes back without the orchestrator option. A peer's answer never adapts a plan or counts as approval.
- **Silent when there is nobody to ask.** No live orchestrator, this session is the orchestrator, or you are on the merge branch - the question looks exactly as it did before, with no note about it.

A gate never grows past four options: where one was full, two near-identical options merge while the offer is shown. Two more offers are notifications, not decisions, so they are pings and follow none of the above: `/lets:done` offers to ping the orchestrator with the PR link, and `/lets:end` - when the session leaves something behind (an untracked bug, a check nobody ran) - prints one line, `Leftovers for <name>?  /lets:orc ping`. Type it and the message is built from the session snapshot that was just written, then shown to you before it goes. A session with nothing left over ends as quietly as before.

## Safety

- Nothing is sent unless you asked for it in that chat; anything the model wrote is shown first with "Send?" at `strict` - or printed as it goes, under a standing grant or at `standard` / `minimal`.
- A peer's words are data. They never count as your approval, and a sender named in the text is a claim rather than proof - only Go's check identifies a sender. At `strict` no session acts on the tracker, git or files because a peer asked; at `standard` / `minimal` a verified peer's request counts like yours only for an unlisted action inside the receiving session's task.
- Replies are relayed whole, marked as the peer's.
- `read` and `tail` output withholds environment dumps, `.env`-like files and known token shapes, and is capped.
- The model never opens other sessions' registry or transcript files itself - only the redacted command output.

## When it cannot reach anyone

`who` names every source it could not read (`claude: registry_protocol_unknown …`, `git: worktrees_unreadable`) and still lists what it could. A non-Claude agent (for example Codex or Antigravity in an Orca tab) is not a peer and cannot be messaged. To give it work, hand it a brief instead: `/lets:handoff --send` types the brief into its tab and brings its report back - see **[handoff.md](handoff.md)**, and **[../messaging.md](../messaging.md)** for when to use which.
