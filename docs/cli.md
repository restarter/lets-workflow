# The `lets` CLI

The `lets` binary does the filesystem, git and process work behind the `/lets:*` commands. You rarely call it yourself - the commands do - but a few subcommands are useful by hand, and they show up in error messages. This page is only an index: the flags, JSON envelopes and exit codes live in **[cli/README.md](../cli/README.md)**, one section per subcommand.

| Subcommand | You meet it when | Details |
|------------|------------------|---------|
| `lets init` | `/lets:init` runs it; `lets init --user` installs the global rules and `~/.lets/.env`; `--launcher`, `--tracker` and friends set config without the prompts | [lets init](../cli/README.md#lets-init) |
| `lets update` | `/lets:update` runs it - `.env`, rules and tracker-adapter sync plus the binary / plugin version check | [lets update](../cli/README.md#lets-update) |
| `lets worktree` | `/lets:worktree` runs `create` / `list` / `info` / `remove`; `adopt` and `release` are the Orca hooks in `orca.yaml`; `sweep` deletes merged task branches | [lets worktree](../cli/README.md#lets-worktree) |
| `lets cmux` / `lets tmux` / `lets orca` | The worktree launchers behind `LETS_LAUNCHER` - `open` a session, `notify` a gate. None of them hard-fails: an absent binary falls back to the terminal flow | [lets cmux](../cli/README.md#lets-cmux) · [lets tmux](../cli/README.md#lets-tmux) · [lets orca](../cli/README.md#lets-orca) |
| `lets notify` | The launcher-neutral gate notification the autonomous pipeline sends | [lets notify](../cli/README.md#lets-notify) |
| `lets peers` | Underneath `/lets:orc` and `/lets:hub` - who is live, role registry, message delivery | [lets peers](../cli/README.md#lets-peers) |
| `lets handoff` | Underneath `/lets:handoff --codex` / `--send` - `targets`, `send`, `codex`, `await` (ships next release) | [lets handoff](../cli/README.md#lets-handoff) |
| `lets statusline` | Claude Code runs it on every render; `lets statusline config` is what `/lets:statusline` saves through | [lets statusline](../cli/README.md#lets-statusline) · [config](../cli/README.md#lets-statusline-config) |
| `lets hook` | The SessionStart / PreCompact hooks that inject `## LETS Config` | [lets hook](../cli/README.md#lets-hook) |
| `lets version` | Checking what is installed | - |

Every `--json` subcommand prints one JSON object, valid even on failure, with a typed `error.kind` - see [JSON envelope conventions](../cli/README.md#json-envelope-conventions).

## See also

- **[installation.md](installation.md)** — installing and updating the binary
- **[commands.md](commands.md)** — the `/lets:*` commands that drive it
- **[statusline.md](statusline.md)** — the statusline flags in user terms
