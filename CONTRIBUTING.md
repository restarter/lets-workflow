# Contributing to LETS Workflow

Thanks for taking the time. This repo is a Claude Code plugin (`plugins/lets/`) plus its companion Go CLI (`cli/`) and some infrastructure scripts. It's small and opinionated — read this and `CLAUDE.md` before a non-trivial change.

## Repo layout

Monorepo (beads-style):

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

## Editing rules — the one thing that bites people

**Never edit `.claude/rules/lets-rules.md`.** That's the *installed copy*, plugin-managed; it's regenerated from the canonical source by `/lets:init` / `/lets:update`. Edit `plugins/lets/rules/lets-rules.md` instead. Editing the installed copy bypasses our own drift detection and silently desyncs.

Likewise: the **frontmatter `version`** in `lets-rules.md` (and `plugin.json`, `marketplace.json`) is bumped **once per release** at ceremony time (`scripts/release/bump-version.sh`) — not per change. A rules edit on a feature branch accumulates under the current target version. Don't bump it in your PR.

## Audience of plugin source

`commands/`, `skills/`, `agents/`, `rules/` are read by Claude (the model), never by humans. Write for the model: terse, structured, parseable; tables and bullets over prose; `MANDATORY` / `NEVER` / `IMPORTANT` markers where a constraint must be locked onto. Match the existing style. Human-facing docs live in `README.md` and `CLAUDE.md`.

When adding a config key, command, skill, or agent, follow the checklists in `CLAUDE.md` ("Adding a new config key", "When Adding/Modifying Commands, Skills, or Agents", "Command Checklist") — they list every file that needs to stay in sync.

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
