# Contributing to LETS Workflow

Thanks for taking the time. This repo is a Claude Code plugin (`plugins/lets/`) plus its companion Go CLI (`cli/`) and some infrastructure scripts. It's small and opinionated — read this and `CLAUDE.md` before a non-trivial change.

## Repo layout

Monorepo layout:

- `plugins/lets/` — the plugin payload. `commands/` (slash commands), `agents/` (expert subagents), `skills/` (reusable + internal), `rules/lets-rules.md` (workflow rules, frontmatter-versioned), `hooks/` (SessionStart + PreCompact).
- `cli/` — the Go CLI (`lets`). Go module root is `cli/`, not the repo root — all `go` commands run from there (or via the repo-root `Makefile`).
- `scripts/release/` — release tooling (`bump-version.sh`, `verify-versions.sh`).
- `scripts/remote/` — VPS deployment for the shared task-tracker backend (not part of the plugin).
- `docs/` — `installation.md` + images.
- `CLAUDE.md` — the authoritative architecture/conventions doc. **Read it.**

## Local development

From the repo root:

```bash
make build    # build the lets binary
make test     # run the Go test suite
make vet      # go vet
make lint     # golangci-lint (config in cli/.golangci.yml)
make fmt      # gofmt + goimports
make install  # build + install to /usr/local/bin or ~/.local/bin
```

CLI changes **must** keep `make test` and `make build` green, and update testdata goldens when behaviour changes (see `cli/internal/initcmd/testdata/`).

To run the plugin from a local checkout in Claude Code: `/plugin marketplace add ./lets-workflow` then `/plugin install lets`.

### Dev binary: `make dev` / `make dev-tmux`

`make dev` in a worktree builds `cli/lets` (version `dev-<branch>-<sha>[-dirty]`), prepends `<worktree>/cli/` to PATH, and execs `claude --plugin-dir <worktree>/plugins/lets` — self-contained, no global install, no marketplace mutation. `make dev-tmux` auto-discovers `.worktrees/*/` and spawns one Claude pane per worktree (`WORKTREES="a b"` to limit). Implementation: `scripts/dev/run.sh`. The most important gotcha (run `make dev` from a host terminal, never from a Bash tool inside Claude) is in `CLAUDE.md` "## Local Development"; the rest:

**`LETS_ENV_VERSION` stamping.** Running `lets init` from a dev binary writes `LETS_ENV_VERSION=dev-<branch>-<sha>[-dirty]` into `.lets/.env` (because `initcmd/render.go` writes `version.Version` literally). Reversible by running prod `lets init`, which restores the proper semver stamp. If you don't want to churn `.env`, skip `lets init` on the dev binary.

**Old worktrees.** Both `make dev` and `make dev-tmux` require `scripts/dev/run.sh` plus the corresponding Makefile targets to exist in the worktree's branch HEAD. Worktrees created before this tooling shipped (or on feature branches that haven't pulled main) will fail with `bash: scripts/dev/run.sh: No such file or directory` or `make: *** No rule to make target 'dev'`. Three fixes (cheapest first): (a) `git checkout main -- Makefile scripts/dev/` from inside the affected worktree — fast but leaves the working tree dirty (uncommitted staged files), so plan to `git restore --staged Makefile scripts/dev/` or commit the migration on the worktree's branch; (b) rebase the worktree's branch onto main; (c) `lets worktree remove <name> && lets worktree create <name>`. `make dev-tmux` is the more dangerous case — it opens panes in ALL worktrees, any of which might be stuck on an old branch. Use `make dev-tmux WORKTREES="up-to-date-only"` to limit, or migrate all worktrees first.

**Production install unaffected.** The production `lets` at `~/.local/bin/lets` is untouched — the dev binary lives at `<worktree>/cli/lets` and only wins on PATH for the `make dev` exec'd process.

**Trust the branch.** Because `make dev` prepends `<worktree>/cli/` to PATH, any executable in `<worktree>/cli/` — including a malicious `cli/git`, `cli/curl`, etc. shipped by an untrusted branch — would shadow the system binary for the duration of the inner Claude session. The exposure is limited to the dev process (the PATH change isn't exported), but treat `make dev` like `make` on the branch: only run it on branches you'd run arbitrary code from.

**`IsDev()` semantics.** `version.IsDev()` returns true for the literal string `"dev"` AND for any `"dev-<non-empty>"` form. Statusline + `lets update` already consume `IsDev()` correctly — dev-stamped binaries render without a `v` prefix and skip env regeneration as expected.

## Editing rules — the one thing that bites people

**Never edit `.claude/rules/lets-rules.md`.** That's the *installed copy*, plugin-managed; it's regenerated from the canonical source by `/lets:init` / `/lets:update`. Edit `plugins/lets/rules/lets-rules.md` instead. Editing the installed copy bypasses our own drift detection and silently desyncs.

Likewise: the **frontmatter `version`** in `lets-rules.md` (and `plugin.json`, `marketplace.json`) is bumped **once per release** at ceremony time (`scripts/release/bump-version.sh`) — not per change. A rules edit on a feature branch accumulates under the current target version. Don't bump it in your PR.

## Audience of plugin source

`commands/`, `skills/`, `agents/`, `rules/` are read by Claude (the model), never by humans. Write for the model: terse, structured, parseable; tables and bullets over prose; `MANDATORY` / `NEVER` / `IMPORTANT` markers where a constraint must be locked onto. Match the existing style. Human-facing docs live in `README.md` and `CLAUDE.md`.

When adding a config key, command, skill, or agent, follow the checklists in "## Editing commands, skills, agents, config keys" below — they list every file that needs to stay in sync. `CLAUDE.md` keeps the canonical *decisions*; the step-by-step *procedures* live here.

## Editing commands, skills, agents, config keys

> Paths below are relative to `plugins/lets/` (the plugin root) unless they start with `cli/`, `scripts/`, `docs/`, or are repo-root files (`README.md`, `CLAUDE.md`).

Update these files:

| File | What to update |
|------|----------------|
| `plugins/lets/rules/lets-rules.md` | Skill Quick Reference table. Edit ONLY here, never the installed `.claude/rules/lets-rules.md`. **Do NOT bump frontmatter `version` per change** — it's bumped once per release at ceremony time (`scripts/release/bump-version.sh`; see `RELEASING.md`). A rules edit on a feature branch accumulates under the current target version. |
| `commands/install-deprecated.md` | Essential Skills / Planning Skills tables |
| `CLAUDE.md` Key Concepts | If adding a new skill |
| `docs/commands.md` (+ the relevant `docs/<topic>.md`) | A new command, a renamed command, or a **new user-facing flag**. This row exists because it was missing: `--pre-compact`, `--session` and `/lets:start --main` all shipped while the human-facing reference kept claiming those commands take no flags. The plugin-source rows above do not cover `docs/` — nothing else does either. |
| `README.md` | Agent table, feature descriptions |
| All `agents/*.md` `## Constraints` sections | If changing the read-only Bash allowlist or constraint wording, sync the identical 1-line text across all 14 analyst agents (verify with `grep -h "You are read-only" agents/*.md \| sort -u` returning exactly one line) |
| `commands/end.md` + `commands/done.md` + `skills/session-snapshot/SKILL.md` session-id references | Channel per context (the rule): **inside a bash command** → `$CLAUDE_CODE_SESSION_ID` (Bash subprocess env var; bash expands it at runtime) — used by `done.md` Step 7, `end.md` Step 3b progress-comment heredoc (body-filed via the tracker `comment-add`), and the `session-snapshot` skill Step 2 (`SID=$CLAUDE_CODE_SESSION_ID` + `find ... "${CLAUDE_CODE_SESSION_ID}.jsonl"`) which writes the snapshot's `- ID:` line from that single bash channel. **The `${CLAUDE_SESSION_ID}` command-load-time template channel is intentionally NOT used in the snapshot skill** (fragile inside a multiline Write arg — lets-bdkvd QA #13). No `SESSION_ID=` alias anywhere. Verify with `grep -rn "SESSION_ID" plugins/lets/commands/ plugins/lets/skills/`. Background + remaining adoption scope (subagents, statusline, `/lets:team` records): see `lets-bdkvd`. |
| `cli/internal/initcmd/reviewspec_test.go` | If touching `commands/review.md`, `commands/check.md` or `skills/review-workflow/review.workflow.js`. Go tests read that markdown from `../../../plugins/` and pin the SPEC blocks, the `--workflow` args wiring, the one-shell PR switch (which they also EXECUTE against a stub `gh`), the restore fence and the no-worktree boundary. **Run with `-count=1`** — Go's test cache does not track files reached through `../../../plugins/`, so a markdown-only edit serves a stale PASS. Same caveat applies to `trackerbodies_test.go` / `trackerrules_test.go`. |
| `cli/internal/cli/<name>.go` + register in `cli/internal/cli/root.go` | If adding a Go subcommand. Add `<name>_test.go` (`package cli_test`). Use `cmd.OutOrStdout()`. Domain logic goes in `cli/internal/<name>/` (see `initcmd/`, `updatecmd/`, `worktreecmd/`, `sessionstart/`, `statusline/`, `frontmatter/` for patterns). If the package needs platform-specific primitives (`syscall.Flock` etc.), gate with `//go:build unix` and add a `<name>_stub.go` (`//go:build !unix`) per the `worktreecmd` pattern so cross-platform builds keep working. Update `cli/README.md` "Adding a subcommand" recipe if pattern changes. |
| Any `commands/*.md` or `skills/*/SKILL.md` invoking `AskUserQuestion` | Follow `## AskUserQuestion Conventions` in `plugins/lets/rules/lets-rules.md` (header chip 4-12 chars descriptive, `(Recommended)` in label, `multiSelect` per rule, follow-through via `Skill` tool per Rule 7, `{LETS_FOO}` substitution per Rule 9). Spec strings hardcoded in English; orchestrator translates to `$LETS_LANGUAGE` at runtime. |
| Any `commands/*.md` or `skills/*/SKILL.md` that **writes a file under `.lets/plans`, `.lets/reviews` or `.lets/sessions`** | Resolve the path via `Skill(skill: "lets:artifact-path", args: "kind=<kind> ext=<ext> [task=<id>]")` and write to the echoed `ARTIFACT_FILE` VERBATIM; register a new `<kind>` in `skills/artifact-path/SKILL.md`'s kinds table. Never `date`+`ls` a path by hand - `.lets/` is one symlinked dir shared by every worktree, and a hand-built name overwrote another session's file (lets-05c4s). |
| Adding a **Dynamic Workflow asset** (a `Workflow`-tool script) | Follow `## Dynamic Workflow Assets (authoring standard)` in `CLAUDE.md`. Ship as `skills/<name>-workflow/` (`<name>.workflow.js` + `user-invocable: false` `SKILL.md`); invoke from the command via `Workflow({scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/<name>-workflow/<name>.workflow.js", args})`; obey the 6 conventions + runtime must-obey list; validate via a live smoke test (no committed unit test — runtime-blocked); degrade gracefully when an `agentType` can't resolve. `skills/review-workflow/` is the reference example. |

### Command output requirements

Every command response ends with exactly ONE footer of the right type - the runtime rule is `plugins/lets/rules/lets-rules.md` `### Response Footer` (Act = AskUserQuestion / Nav = LETS box / Close = prose or nothing; never mix; Nav content is state-driven). This section covers authoring the **Nav** box.

```
┌─ LETS ─────────────────┐
│  [action]? [command]   │
└────────────────────────┘
```

**Box format:**
- Header `┌─ LETS ─` + `─` padding + `┐`; lines `│  ` + content + padding + ` │`; footer `└─` + `─` padding + `┘`. Min width 25.
- **All boxes in one file MUST be the same width**, measured in DISPLAY columns = Unicode code points, NOT bytes (the box-drawing glyphs are 3 bytes each, so a byte count like `awk length` lies). Verify:
  ```bash
  python3 -c "import sys;[print(len(l.rstrip())) for l in open(sys.argv[1]) if l.rstrip().endswith(('┐','│','┘'))]" <file> | sort -u | wc -l   # expect 1 (matches box lines by their right edge; skips tree diagrams)
  ```

**Content:**
- Short action word + `?` (e.g. "Commit?", "Fix?"), then a `/lets:*` command. **ONLY `/lets:*`** (exception: `git push` after `/lets:done`/`/lets:end`). If the next step is NOT a `/lets:*` command (a shell command, "restart to apply", etc.) → no box, use a Close prose line.
- **No command / internal invocation → no footer** (a `/lets:*` called by another command, or a Rule-7 follow-through — see `lets-rules.md` `## AskUserQuestion Conventions` Rule 7). Only the outermost user-invoked command emits one.
- **State-driven** (per the runtime rule): task active → NEVER `/lets:start` (it is only a no-task / bootstrap escape hatch); "reset context, keep the task" → `/clear` + the mid-task command, never `/lets:start`.

**Which shortcuts (pick in order):** (1) most-likely next step in the loop; (2) a lighter alt if one exists (`/lets:check` for `/lets:review`, `/lets:ask` for `/lets:opinion` — pass the matching flag); (3) one escape hatch (`/lets:start` / `/lets:end` / `/lets:status`). ≤4 lines; one line is correct when there is a single sensible step.

### Command checklist

- [ ] Response ends with exactly one footer of the right type (Act/Nav/Close - see the Response Footer rule)
- [ ] Nav-box shortcuts follow the guidance above (next step + lighter alt + escape hatch; no `/lets:start` mid-task); all boxes in the file are the same display-column width (python recipe)
- [ ] Updates Skill Quick Reference in `plugins/lets/rules/lets-rules.md` (do NOT bump frontmatter `version` per change — once per release at ceremony, see the version-coherence rule above)
- [ ] Updates `/lets:install` Essential Skills / Planning Skills tables
- [ ] Follows session flow (start -> work -> commit -> done -> end)
- [ ] Description is clear and actionable
- [ ] **If file invokes any deferred tool** (`AskUserQuestion`, `EnterPlanMode`, `WebFetch`, etc.), include the `> **IMPORTANT:**` deferred-tool callout right after the file's brief description, before the first `## Step` (or first major section). Wording: see existing commands/skills for the standard block (search for `IMPORTANT:** If the spec below`)
- [ ] **If file invokes `AskUserQuestion`**, follow `## AskUserQuestion Conventions` in `plugins/lets/rules/lets-rules.md` — header chip 4-12 chars descriptive (never `"LETS"`; command name is OK when it names the topic), `(Recommended)` in label not description, follow-through via `Skill` tool (Rule 7) when an option names a `/lets:*` command, **substitute `{LETS_FOO}` placeholders before tool call (Rule 9)** — never use `$LETS_FOO` inside `label`/`description`/`question` strings

### Adding a new config key

Single source of truth for canonical metadata: `cli/internal/letsconfig/keys.go::Keys`. Single source of truth for Prefs↔Key wiring: `cli/internal/initcmd/render.go::Prefs.AsValues()`.

Required edits:

1. Append `Key{Name, Comment, Default}` entry to `letsconfig.Keys`. Name MUST start with `LETS_`.
2. Add field to `Prefs` struct in `cli/internal/initcmd/render.go` AND add ONE entry to `Prefs.AsValues()` map (one-line addition right below the field).
3. Regenerate the env goldens (`go test ./internal/initcmd -run Golden -update`) and bump any hardcoded key-count assertions (`keys_test.go`, sessionstart tests, the "N canonical keys" prose in `CLAUDE.md` / `cli/README.md`). Do **NOT** bump the `lets-rules.md` frontmatter `version` per change — it's bumped once per release at ceremony time (see the version-coherence rule above); the SessionStart drift check picks the new key up on the next release.

If the key is exposed via the `/lets:init` slash command (most are):

4. Add a `--<key>` cobra flag in `cli/internal/cli/init.go` and wire it through `flagOrDefault(flag<X>, defaults["LETS_X"])` in prefs construction.
5. Add an AskUserQuestion in `plugins/lets/commands/init.md` (Step 2 first-time path + Step 3d "Keep current" option in change-config path).

Auto-derived (no edit needed):
- `.lets/.env` content (renderEnv → renderTemplate(Header, p.AsValues()))
- `.lets/.env.example` content (renderEnvExample → renderTemplate(ExampleHeader, Defaults()))
- SessionStart hook env-injection whitelist (sessionstart imports `letsconfig.Names()`)
- Regenerate wiring (`RegenerateEnv` uses `p.AsValues()`, iterates `letsconfig.Keys`)
- Future `/lets:doctor` validation + display

Then document in CLAUDE.md's "LETS Config keys" table + `README.md` Configuration block, and add consuming logic in the relevant commands.

### Add a tracker adapter

A **tracker adapter** binds the neutral task-tracker verbs to one concrete transport. `LETS_TRACKER` names the adapter; `lets init` installs exactly one `plugins/lets/rules/tracker-<name>.md` into the project's `.claude/rules/` (same drift-tracked, frontmatter-versioned mechanism as `lets-rules.md`). Adding an adapter is authoring one markdown file - no command forks, usually no Go.

1. Copy `plugins/lets/rules/tracker-TEMPLATE.md` to `tracker-<name>.md`. Set frontmatter `name: tracker-<name>` + `version:` matching the current plugin version (bumped only at release ceremony, like `lets-rules.md`). The release scripts (`scripts/release/bump-version.sh` + `verify-versions.sh`) **glob** `tracker-*.md` (excluding `tracker-TEMPLATE.md` and `*.board.md`), so your adapter is version-bumped, staged, and drift-checked automatically - no release-script edit needed. (Corollary: never name a real adapter `tracker-TEMPLATE.md` or end it `.board.md` - both are deliberately excluded.)
2. Fill the **Capabilities + bindings** table. Header is PINNED (`| verb | tier | supported | binding |`) - the contract test parses it; don't reorder columns. Bind the 5 CORE verbs (`create`, `show`, `comment-add`, `set-status`, `close`) - they MUST be `supported = yes`. OPTIONAL verbs may be `absent` (the command degrades gracefully). **Each binding cell is EXECUTED as written** when a command invokes its verb (command bodies carry neutral ` ```lets-tracker ` blocks, not inline `bd`) - keep bindings to the tracker's own transport, never a destructive/exfiltrating command; installing a shared adapter runs its bindings (the contract test pins table shape, not binding safety).
3. **Normalize output:** `show`/`list-by-status` MUST return status as a NEUTRAL name (`open`/`in_progress`/`closed`/...) so consuming commands stay adapter-agnostic. The adapter owns native↔neutral translation. `close` returns the status the task ended in - `closed` when it really closed, another neutral status when this board's terminal is gated and the legal move was an advance (the caller then reports a handoff, never a close).
4. **Declare your fields:** `create` and `set-field` open with `accepts:`, `show` with `returns:`, in NEUTRAL field names (the vocabulary is listed in the TEMPLATE's declaration bullet; a native rename is written `priority`→`severity`). **Close each declaration with a period** - that is the terminator, and without it the transport call's own backticks get read as field names. A verb marked unsupported still writes the marker with nothing in it (`accepts: nothing - absent`). A supported `show` must declare `id`/`title`/`status`/`description` because commands read them unconditionally; `url` only if your tracker really has per-task links. Keep the lists minimal - every declared field multiplies a read verb's payload.
5. **Prune `## Neutral statuses` to your board.** It is a declaration, not a vocabulary listing: naming `in_review` there is what authorizes `/lets:done` to move a task into review after opening a PR. Copying the TEMPLATE line unedited will push tasks into a column you do not have.
6. **Mark what you have not exercised** - `[ASSUMED]` / `[UNVERIFIED]` inline, `[VERIFIED <date>]` once confirmed. Scoped to claims you took from documentation or inferred; a binding you have actually run needs no marker. The file is auto-loaded instruction, and the agent acts on whatever it states flatly.
7. **Degradation:** OPTIONAL absent → continue with a message; a CORE verb unresolvable at runtime → HARD-FAIL loud (never a phantom success, esp. under AUTO MODE).
8. **Secrets:** never in `.lets/.env` (644, injected) or a `.board.md` (auto-loaded + git-shareable). A direct-API adapter reads its token from a gitignored chmod-0600 file (`.lets/trackers/<name>/.env`, written `0o600` + dir `0o700`); a transport that owns its own creds (an MCP server) keeps them there.
9. Optionally ship a `tracker-<name>.board.md` template (project-specific status-id map / transitions / principles) - `lets init` scaffolds it once and NEVER overwrites it.
10. The adapter contract test (`cli/internal/.../trackerrules_test.go`) auto-covers the new file (valid frontmatter + pinned header + 5 CORE rows each marked `supported = yes` (the `none` null adapter excepted) + `## Degradation` + `accepts:` on `create`/`set-field` and `returns:` on `show`, declared in neutral field names). No allowed-values list to edit - `lets init` validates `LETS_TRACKER` against `^[a-z0-9][a-z0-9-]*$` and skips with a warning if `tracker-<name>.md` is absent.

Command bodies invoke verbs via ` ```lets-tracker ` blocks (never inline `bd`); the orchestrator resolves them through the loaded adapter. The "no adapter file loaded" fallback lives in the rule, NOT the adapter file (an unloaded file can't instruct): `beads`/unset resolves via `tracker-beads.md`; a non-beads name with no file loaded behaves as `none` (no tracker ops, never `bd`) and nudges `/lets:update`.

## Commits & PRs

- **Conventional commits**: `feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`. Subject under 50 chars, imperative mood ("add" not "added"). Optional `(task-id)` scope when the work is tracked. See `.claude/rules/git.md`.
- Branch off `main`; open a PR. The `Verify version coherence` check must pass (source-tree version coherence), one approval, conversations resolved.
- Keep PRs focused — one theme per branch. Surgical changes are easier to review and revert.
- Update `CHANGELOG.md` under `## [Unreleased]` for user-visible changes.
- Don't bump versions (see above).

## Task tracking

Maintainers track work in [beads](https://github.com/steveyegge/beads) with a shared [Dolt](https://github.com/dolthub/dolt) remote. As an external contributor you don't need any of that — just describe your change in the PR body. If you're filing a bug or proposing a feature, use the issue templates.

## Where to ask

- **Bug / feature request** → open an issue (templates provided).
- **Security vulnerability** → see `SECURITY.md` (private reporting, *not* a public issue).
- **Design questions** → open an issue; reference the relevant section of `CLAUDE.md`.
