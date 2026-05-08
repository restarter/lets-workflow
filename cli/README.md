# lets CLI

Go binary that ships alongside the LETS Claude Code plugin (`../plugins/lets/`).

The plugin's `hooks.json` and slash commands invoke `lets <subcommand>` for cross-platform behavior. Eventually replaces all bash scripts under `plugins/lets/hooks/` and `plugins/lets/scripts/lets/`.

## Layout

````
cli/
├── cmd/lets/main.go              # Entry point - thin wrapper
├── internal/
│   ├── cli/                      # Cobra command factories (one file per subcommand)
│   │   ├── root.go, version.go
│   │   ├── hook.go, hook_session_start.go, hook_precompact.go
│   │   ├── statusline.go, init.go
│   │   └── *_test.go             # Black-box tests (package cli_test)
│   ├── envfile/                  # .lets/.env reader (whitelist + parser, mirrors bash semantics)
│   ├── frontmatter/              # YAML frontmatter `version` reader (drift check via x/mod/semver)
│   ├── hook/sessionstart/        # SessionStart + PreCompact output (LETS Config + Notice + drift check)
│   ├── initcmd/                  # `lets init` orchestration (init.go + migrate.go + jsonmerge.go +
│   │                             #   state.go + render.go + embed.go + embedded_statusline_shim.sh)
│   ├── statusline/               # Render loop, OAuth fetch, cache (build-tag splits:
│   │                             #   keychain_darwin.go vs keychain_other.go,
│   │                             #   spawn_unix.go vs spawn_windows.go)
│   └── version/version.go        # CLI version (var, ldflags-overridable)
├── go.mod, go.sum
├── .golangci.yml
└── .gitignore
````

## Build / Test (from repo root)

```bash
make build       # produces cli/lets (with -trimpath, ldflags from git tag if present)
make test        # runs `go test -race ./...` inside cli/
make vet         # runs `go vet ./...`
make lint        # runs golangci-lint (requires it installed)
make fmt         # runs gofmt -w -s
make fmt-check   # verifies gofmt is clean (CI use)
make install     # installs lets to $GOBIN
make clean       # removes built artifact + test cache
```

`make lint` requires golangci-lint installed: `brew install golangci-lint` or `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`.

`make test` uses `-race`, which requires CGO and a C compiler (clang on macOS via Xcode CLT, gcc on Linux). Fails with linker errors if `CGO_ENABLED=0` or no C toolchain.

## Setup

After `make install`, the `lets` binary lives at `$(go env GOPATH)/bin/lets` (or `$GOBIN/lets` if set). The plugin's hooks (Phase 3+) will invoke `lets` from `$PATH` - this is required.

Verify:

```bash
make install
which lets    # should print path
lets version
```

If `which lets` fails, add `$(go env GOPATH)/bin` to your `$PATH`.

## Versioning

`internal/version/Version` defaults to `0.4.0-dev` for untagged dev builds.

The Makefile auto-derives the version from git tags (when HEAD is exactly on a tag), strips leading `v`, and injects via `-ldflags`:

- Tagged HEAD (e.g. `v0.5.0`) → `make build` produces binary stamped with `0.5.0`
- Dev HEAD → no `-ldflags`, binary uses Go default `0.4.0-dev`

**Lockstep with the plugin.** Plugin and CLI share one version - bumping `plugins/lets/.claude-plugin/plugin.json` `"version"` and tagging `vX.Y.Z` happens together at release time. Release scripting handled by `lets-pplgq`.

**Module path is load-bearing.** `go install github.com/restarter/lets-workflow/cli/cmd/lets@latest` depends on this exact path. Renaming the `cli/` directory or moving the repo requires a deprecation cycle (old import paths fail).

## Adding a subcommand

1. Create `internal/cli/<name>.go` with `func New<Name>Cmd() *cobra.Command`
2. Register in `internal/cli/root.go`: `cmd.AddCommand(New<Name>Cmd())`
3. Add `<name>_test.go` (use `package cli_test` for black-box tests; `package cli` only when testing unexported helpers)
4. Use `cmd.OutOrStdout()` (not `fmt.Printf`) for testability
5. Domain logic goes in `internal/<name>/` (see `initcmd/`, `sessionstart/`, `statusline/`, `frontmatter/` for patterns)

Example: `lets init` lives in `internal/cli/init.go` (cobra factory) and `internal/initcmd/` (orchestration, migration helpers, embedded shim).

## `lets init`

Internal subcommand. Designed to be invoked by the `/lets:init` slash command, which captures user preferences via `AskUserQuestion` in Claude Code and shells out with explicit flags. Direct shell invocation works for CI / dev override but requires all flags up front — there is **no TUI**.

```bash
lets init \
  --plugin-root "${CLAUDE_PLUGIN_ROOT}" \
  --language English --merge-branch main --pr-flow local \
  [--skip-beads]
```

Required: `--plugin-root` (or `$CLAUDE_PLUGIN_ROOT`). Other flags default sensibly. `--github` is a deprecated alias for `--pr-flow=github`.

What it does (idempotent, linear):

1. Creates `.lets/` directory structure (`sessions/`, `reviews/`, `plans/`, `execution/`, `cache/`)
2. Adds `.lets/`, `.beads/`, `.worktrees/` to `.gitignore`
3. Migrates legacy `.lets/statusline.sh` (deletes the per-project shim if it matches the embedded snapshot — see `internal/initcmd/embedded_statusline_shim.sh`)
4. Migrates legacy `.lets/config.yaml` → `.lets/.env` (preserves user values via allowlist regex)
5. Writes `.lets/.env` (if absent) with chosen prefs
6. Refreshes `.lets/.env.example` from plugin template
7. Mutates `.claude/settings.json` to set `statusLine.command = "lets statusline"` with `_letsManaged.statusLine: true` provenance marker (atomic write + `.bak`)
8. Copies plugin rules → `<project>/.claude/rules/lets-rules.md` (frontmatter-version-aware: install / upgrade / skip)
9. Runs `bd init` (60s timeout) unless `--skip-beads`

Refuses: from a worktree (`--git-dir != --git-common-dir`), in `$HOME`, or in filesystem root `/`.
