# Smoke: parallel implementers and team runs

The live checks behind lets-7dwc1 (merged; ships next release): what a Go test cannot prove because it depends on the running Claude Code harness - how it names, isolates and restores agents, and what it does with a session's id. Run them on a scratch branch, with the owner's yes, whenever Claude Code or this area changes; record every run in the results table at the end. A check that fails is a **deviation**: stop, record what happened, and decide with the owner - never adapt the procedure to make it pass.

The design facts these checks rest on are pinned statically by `TestSmokeFacts` (`cli/internal/initcmd/smokefacts_test.go`): no `team_name` / `mode=` in the execute or team commands, `BASE` in every isolated brief, member-run never messaging a gone member, and `lets integrate` running before Accept.

Before any check: a clean tree, `make build`, and the plugin loaded from this checkout (`make dev` from a host terminal) when the change is not released yet. Note the Claude Code version (`claude --version`).

## 1. Session-owner guard (Task 4)

1. **Plain worktree, pane teammate.** In a task worktree whose `.task-<slug>` records this session, open a second, named Claude session in the same worktree. Pass: the task-state `session:` line still names the first session, and the second session shows the held Notice naming the holder and "run /lets:start in that chat to rewrite the session".
2. **Plain worktree, no role file, `/clear`.** In a worktree where no peer role file was ever registered, run `/clear` in the owning session, then open a pane. Pass: the CarrySession Notice naming the missing peer role file appears when `.lets/sessions/peers` exists (and nothing when it does not). Do NOT assert `session:` - this residual gap is accepted.
3. **Team worktree.** In a worktree a team file claims, with a recorded lead (`lets members lead --scope <c> --json`), open a pane. Pass: `session:` unchanged, and the Notice names the recorded lead.

## 2. member-run in a team (Task 5)

In a team worktree with a recorded lead, spawn `<c>-architect` through member-run (`/lets:team spawn architect`). Pass: `lets peers who --json` shows exactly one `<c>-architect`, and `lets members status --scope <c> --json` shows it `live` with `link: team`.

## 3. Delivered SendMessage and the fallback line (Task 5b)

From a worker, `/lets:orc ask` the lead while the lead is mid-turn. Pass: the worker prints `delivered via SendMessage fallback - orca refused: <reason> (<state>)` on its own line, and `lets peers tail --to-session <lead sid> --since-message <msgid> --json` on the receiver shows the message as seen.

## 4. Parallel isolated groups (Task 10)

On a scratch branch, a plan with two groups, one of them two sequential one-file chunks; run `/lets:execute --implementers --parallel`.

Pass, all of:

- both groups run at the same time;
- each isolated agent's spawn guard passes, and the harness lets `git switch -C <branch> {BASE}` through (a refusal is a deviation: stop);
- `git rev-parse HEAD` == BASE in both agent worktrees before the first chunk;
- the second chunk of the two-chunk group arrives by `op=next`, and its `lets integrate` uses `--since` = that group's `integrated_source`;
- the integrations run one after another, with a clean caller tree between them;
- after completion the agent branches still hold their commits, and `git worktree list` shows no `lets-integrate` entry.

## 5. Pipelined run (Task 10b)

On a scratch branch, a plan with three one-file chunks; run `/lets:execute --implementers --pipelined`. Pass: c2 starts before c1 is accepted; a correction on c1 lands as a `fixup!` commit; the closing autosquash (after `lets worktree pushed` reports `not_pushed`) leaves three commits with the plan's messages, and `git diff <pre_rebase_head> HEAD` is empty.

## 6. Team run on a launcher (Task 11)

With `LETS_LAUNCHER=tmux` or `cmux` and two scratch tasks, run `/lets:team run --tasks <a>,<b>` from the main checkout. Pass: two sessions named `<repo>-<id>` appear in `lets peers who --orc <lead> --json`, bound to this lead; the run record goes `launched` -> `open`; `/lets:team stop` asks both workers. Any miss is a deviation.

## 7. Standing-team members (Task 12)

In a team worktree: dismiss a pane member, then `/lets:team spawn --roster`. Pass: a new `<c>-architect` is live, a surviving pane under the same name is refused or renamed (`<name>-2`), and the lead's task-state `session:` is untouched.

## Results

| Date | Claude Code version | Check | Result (pass / deviation + what happened) |
|---|---|---|---|
| | | | |
