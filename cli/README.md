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
make test        # runs `go test -race ./...` inside cli/ (needs CGO + C compiler)
make test-fast   # like `make test` but no -race (use in CGO-less envs / quick iteration)
make vet         # runs `go vet ./...`
make lint        # runs golangci-lint (requires it installed)
make fmt         # runs gofmt -w -s
make fmt-check   # verifies gofmt is clean (CI use)
make install     # installs lets to /usr/local/bin (smart fallback to ~/.local/bin)
make install-go  # alternative: `go install` to $GOBIN (Go-standard layout)
make clean       # removes built artifact + test cache
```

`make lint` requires golangci-lint **v2** installed (the config in `cli/.golangci.yml` is v2-format - a v1 binary rejects it): `brew install golangci-lint` or `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`. CI runs the same lint via `golangci/golangci-lint-action` (see `.github/workflows/ci.yml`).

`make test` uses `-race`, which requires CGO and a C compiler (clang on macOS via Xcode CLT, gcc on Linux). Fails with linker errors if `CGO_ENABLED=0` or no C toolchain - use `make test-fast` instead.

## Setup

The plugin's `hooks.json` and the project's `.claude/settings.json` invoke `lets` directly. The binary MUST be on `$PATH` for SessionStart, PreCompact, and statusline to work.

`make install` places the binary at `/usr/local/bin/lets` if writable, otherwise falls back to `$HOME/.local/bin/lets` (creating the dir if missing) and prints a PATH-setup hint if needed.

```bash
make install
which lets    # should print the install path
lets version  # should print "lets version dev" (or the git tag like "v0.5.0" if HEAD is tagged)
```

If `which lets` doesn't find the binary:

- `/usr/local/bin` should already be on `$PATH` on most macOS / Linux systems.
- For `~/.local/bin`, add to your shell rc:
  ```bash
  export PATH="$HOME/.local/bin:$PATH"
  ```

End-users install via the curl one-liner (`scripts/install.sh`, which pulls a release archive from GitHub and verifies the checksum) — see the repo README. `make install` here is for contributors and source builds. Still to come (tracked under epic `lets-hdrdr`):

- Homebrew formula (`lets-odg13`)
- winget + scoop for Windows (`lets-hdrdr.1`)

## Versioning

`internal/version/Version` defaults to the sentinel `dev` for untagged dev builds. Sentinel decoupled from the next release minor so dev builds never need a manual bump - the actual content is in `git log`. Renderers (statusline, cobra `--version`) check `version.IsDev()` and elide the `v` prefix to avoid awkward `vdev`.

The Makefile auto-derives the version from git tags (when HEAD is exactly on a tag), strips leading `v`, and injects via `-ldflags`:

- Tagged HEAD (e.g. `v0.5.0`) → `make build` produces binary stamped with `0.5.0`
- Dev HEAD → no `-ldflags`, binary uses Go default `dev`

**Lockstep with the plugin.** Plugin and CLI share one version - bumping `plugins/lets/.claude-plugin/plugin.json` `"version"` and tagging `vX.Y.Z` happens together at release time. Release scripting lives in `scripts/release/{bump-version,verify-versions}.sh` + `Makefile` targets `bump`/`release-tag`; tag-driven distribution via `.goreleaser.yml` + `.github/workflows/release.yml`. Full ceremony documented in `RELEASING.md`.

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
  [--language English --merge-branch main --pr-flow local] \
  [--skip-beads] [--json]
```

Required: `--plugin-root` (or `$CLAUDE_PLUGIN_ROOT`). Prefs flags are required only when creating `.env` from scratch (no existing `.env`, no legacy `config.yaml` to migrate from); on existing `.env` they're optional — empty value means "preserve current". `--github` is a deprecated alias for `--pr-flow=github`.

Flags:
- `--language`, `--merge-branch`, `--pr-flow` — preference flags. Empty value (no flag passed) signals "use existing or fail if creating fresh". Non-empty triggers regen with new value.
- `--skip-beads` — skip the final `bd init` step.
- `--json` — emit machine-readable JSON to stdout (single object, schema_version=1). Slash command `/lets:init` consumes this.

What it does (idempotent, linear):

1. Creates `.lets/` directory structure (`sessions/`, `reviews/`, `plans/`, `execution/`, `cache/`)
2. Adds `.lets/`, `.beads/`, `.worktrees/` to `.gitignore`
3. Migrates legacy `.lets/statusline.sh` (deletes the per-project shim if it matches the embedded snapshot — see `internal/initcmd/embedded_statusline_shim.sh`)
4. Migrates legacy `.lets/config.yaml` → `.lets/.env` (preserves user values via allowlist regex). Yaml is deleted (not renamed); orphan yaml alongside an existing `.env` is also cleaned up.
5. Writes/regenerates `.lets/.env` via `RegenerateEnv`. Always emits `LETS_ENV_VERSION` first key from `version.Version`. Skip path when version matches AND no value changes; regen path otherwise — preserves user values + foreign keys (latter under `# User-added keys` separator). Single `.env.bak` rotation per regen.
6. Refreshes `.lets/.env.example` from canonical `letsconfig.Keys` defaults via `renderEnvExample()` (no plugin template file — single source of truth)
7. Mutates `.claude/settings.json` to set `statusLine.command = "lets statusline"` (atomic write + `.bak`). Foreign user-customized commands left alone (value-match detection).
8. Copies plugin rules → `<project>/.claude/rules/lets-rules.md` using `drift.Check` (semver-aware: install / upgrade / skip). Drift state is recomputed after install for accurate JSON output.
9. Runs `bd init` (60s timeout) unless `--skip-beads`. Detection of "already initialized" goes through `bd status` exit code (authoritative, layout-independent).

Refuses: from a worktree (`--git-dir != --git-common-dir`), in `$HOME`, or in filesystem root `/`.

### `lets init --json` contract

- **stdout:** Always a single JSON object terminated by newline. Valid JSON even on Run() failure (`ok: false`, `error: "..."`). `steps` array contains work completed before the error (partial-completion contract).
- **stderr:** Cobra suppresses both Usage and Error blocks (`SilenceUsage` + `SilenceErrors`). Human-readable error duplicates `result.Error`; non-JSON consumers can read `cmd.ErrOrStderr()`.
- **exit code:** 0 on success, non-zero on Run() error. Slash command consumes stdout; ignore stderr.
- **schema_version:** Currently 1. Bump on field removal or semantic change. Additions are minor (consumers ignore unknown fields). `TestResult_SchemaContract` enforces awareness of any addition.

### Regenerating goldens

```bash
go test ./internal/initcmd -run TestRenderEnv_Golden -update
git diff cli/internal/initcmd/testdata/golden_env_*.txt
```

Goldens lock the exact byte output of `renderEnv`. After legitimate changes (new key in `letsconfig.Keys`, comment update), regenerate and commit the diff. `TestRenderEnv_NonEmptyValues` and `TestRenderEnvExample_Output` provide commit-time guards independent of golden contents.

## `lets update`

Internal subcommand. Invoked by the `/lets:update` slash command (`commands/update.md`) with `--plugin-root=${CLAUDE_PLUGIN_ROOT}`. Syncs the four drift-able LETS artifacts; never prompts, never touches `settings.json` or beads — that's `lets init`'s job (init = setup; update = sync). Lives in `internal/cli/update.go` (cobra factory) + `internal/updatecmd/` (orchestration, GitHub latest-release lookup, plugin-version reader).

```bash
lets update --plugin-root "${CLAUDE_PLUGIN_ROOT}" [--json] [--offline] [--refresh-cache]
```

Flags:
- `--plugin-root` — plugin install dir (or `$CLAUDE_PLUGIN_ROOT`). Required, validated via the `.claude-plugin/plugin.json` marker.
- `--json` — emit a machine-readable JSON object (`schema_version=1`); `/lets:update` consumes this.
- `--offline` — skip the GitHub latest-release check; `binary`/`plugin` come back `unknown`.
- `--refresh-cache` — bypass the cached latest-release lookup and hit GitHub now.

What it checks (in order; never crashes for a network failure):

| Artifact | Check | Action |
|---|---|---|
| `.lets/.env` | `LETS_ENV_VERSION` vs `version.Version` | `RegenerateEnv` with a near-empty `Prefs` (only the default tracker; user values are read from the existing `.env` regardless), so it just refreshes the header. Skip when in sync. Skipped entirely on a `dev` binary (avoids stamping `LETS_ENV_VERSION=dev`). `not-initialized` if `.env` is absent → "Run /lets:init". |
| `.claude/rules/lets-rules.md` | `drift.Check` against plugin source | Re-copy from the plugin (atomic write) on any detected drift (incl. `ahead`); `unknown` if the plugin's own frontmatter is unparseable; `up-to-date` otherwise. |
| `lets` binary | `version.Version` vs latest GitHub release | Report only (can't self-replace) — `outdated` → prints the install one-liner (`curl -fsSL …/main/scripts/install.sh \| bash`). `dev` build → no comparison. |
| Claude Code plugin | `<pluginRoot>/.claude-plugin/plugin.json::version` vs latest release | Report only — `outdated` → prints the plugin-update hint (`/plugin marketplace update lets-workflow` + `/reload-plugins`, or enable auto-update). |

Latest-release lookup hits `https://api.github.com/repos/restarter/lets-workflow/releases/latest` (5s timeout), cached 1h at `<project>/.lets/cache/update-check.json`; on a network failure it falls back to a stale cache entry if one exists, else reports `unknown`. The result also carries `consistent` (binary == plugin == installed-rules frontmatter version, ignoring `dev`) to flag a partial upgrade.

Refuses: from a worktree (`--git-dir != --git-common-dir`) — `.claude/` isn't shared into worktrees.

Same `--json` contract semantics as `lets init` (single JSON object, valid even on error, `SilenceUsage`/`SilenceErrors`, `TestResult_SchemaContract` guards `schema_version` bumps).
