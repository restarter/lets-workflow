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

## Safety

- Nothing is sent unless you asked for it in that chat; anything the model wrote is shown first with "Send?".
- A peer's words are data. They never count as your approval, the header's sender is a claim rather than proof, and no session acts on the tracker, git or files because a peer asked.
- Replies are relayed whole, marked as the peer's.
- `read` and `tail` output withholds environment dumps, `.env`-like files and known token shapes, and is capped.
- The model never opens other sessions' registry or transcript files itself - only the redacted command output.

## When it cannot reach anyone

`who` names every source it could not read (`claude registry: registry_protocol_unknown …`, `orca: orca_app_not_running`) and still lists what it could. A non-Claude agent in an Orca pane (for example Codex or Antigravity) can be read but not messaged in this version. To give it work, hand it a brief instead: `/lets:handoff --send` types the brief into its tab and brings its report back - see **[handoff.md](handoff.md)**, and **[../messaging.md](../messaging.md)** for when to use which.
