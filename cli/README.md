# lets CLI

Go binary that ships alongside the LETS Claude Code plugin (`../plugins/lets/`).

The plugin's `hooks.json` and slash commands invoke `lets <subcommand>` for cross-platform behavior (the former `plugins/lets/hooks/*.sh` and `plugins/lets/scripts/lets/` bash surfaces are gone).

## Layout

````
cli/
├── cmd/lets/main.go              # Entry point - thin wrapper
├── internal/
│   ├── cli/                      # Cobra command factories (one file per subcommand)
│   │   ├── root.go, version.go
│   │   ├── hook.go, hook_session_start.go, hook_precompact.go
│   │   ├── statusline.go, init.go, update.go
│   │   ├── worktree.go (+ worktree_stub.go), cmux.go (+ cmux_stub.go)
│   │   └── *_test.go             # Black-box tests (package cli_test)
│   ├── letsconfig/               # Canonical LETS_* key metadata + defaults (single source of truth)
│   ├── envfile/                  # .lets/.env reader (whitelist + parser, mirrors bash semantics)
│   ├── frontmatter/              # YAML frontmatter `version` reader (drift check via x/mod/semver)
│   ├── drift/                    # Rules-file drift check + user-facing drift messages
│   ├── gitutil/                  # Shared git helpers
│   ├── hook/sessionstart/        # SessionStart + PreCompact output (LETS Config + Notice + drift check)
│   ├── initcmd/                  # `lets init` orchestration (init.go + migrate.go + env.go +
│   │                             #   render.go + tracker.go + embed.go + …)
│   ├── updatecmd/                # `lets update` (order-aware next_action, deferred rules; SchemaVersion=2)
│   ├── worktreecmd/              # `lets worktree create/remove/list/info` (//go:build unix)
│   ├── cmuxcmd/                  # `lets cmux open/rename/notify` (optional macOS launcher, //go:build unix)
│   ├── statusline/               # Render loop, OAuth fetch, cache (build-tag splits:
│   │                             #   keychain_darwin.go vs keychain_other.go,
│   │                             #   spawn_unix.go vs spawn_windows.go)
│   ├── statuslinecmd/            # `lets statusline config` persistence (own SchemaVersion + envelope)
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
5. Domain logic goes in `internal/<name>/` (see `initcmd/`, `updatecmd/`, `worktreecmd/`, `sessionstart/`, `statusline/`, `frontmatter/` for patterns)

Example: `lets init` lives in `internal/cli/init.go` (cobra factory) and `internal/initcmd/` (orchestration, migration helpers, embedded shim).

### Platform-specific primitives (unix vs windows)

If the package needs Unix-only primitives (`syscall.Flock`, fifo, signals, etc.), gate the implementation with a build constraint and ship a Windows stub so cross-platform builds still link:

```go
// foo_unix.go
//go:build unix

package foocmd
func Run(...) error { /* real impl using syscall.Flock etc. */ }
```

```go
// foo_stub.go
//go:build !unix

package foocmd
func Run(...) error { return errors.New("not yet supported on this platform") }
```

The same pattern applies at the cobra factory layer (`internal/cli/<name>.go` + `internal/cli/<name>_stub.go`) when the subcommand should still appear in `--help` on Windows but return a structured error. See `worktreecmd/` for a worked example (filesystem + git operations + 4-subcommand surface) and `internal/cli/worktree_stub.go` for the Windows-side no-op.

## `lets init`

Internal subcommand. Designed to be invoked by the `/lets:init` slash command, which captures user preferences via `AskUserQuestion` in Claude Code and shells out with explicit flags. Direct shell invocation works for CI / dev override but requires all flags up front — there is **no TUI**.

```bash
lets init \
  --plugin-root "${CLAUDE_PLUGIN_ROOT}" \
  [--language English --merge-branch main --pr-flow local --tracker beads --launcher terminal] \
  [--rules-scope project] [--skip-beads] [--json]
  # or: lets init --user --language English   (user-scope global install)
```

Required: `--plugin-root` (or `$CLAUDE_PLUGIN_ROOT`). Prefs flags are required only when creating `.env` from scratch (no existing `.env`, no legacy `config.yaml` to migrate from); on existing `.env` they're optional — empty value means "preserve current". `--github` is a deprecated alias for `--pr-flow=github`.

Flags:
- `--language`, `--merge-branch`, `--pr-flow`, `--tracker`, `--launcher` — preference flags. Empty value (no flag passed) signals "use existing or fail if creating fresh". Non-empty triggers regen with new value. `--tracker` (`beads` | `planfix-mcp` | `none`) additionally installs the matching `.claude/rules/tracker-<name>.md` adapter on a fresh init (an existing `.env` value wins on re-init, with a warning).
- `--rules-scope` (`project` | `user`), `--user` — rules-install scope; `--user` does the machine-global install (`~/.claude/rules/` + `~/.lets/.env`) instead of a project init.
- `--skip-beads` — skip the final `bd init` step.
- `--json` — emit machine-readable JSON to stdout (single object, schema_version=1). Slash command `/lets:init` consumes this.

What it does (idempotent, linear):

1. Creates `.lets/` directory structure (`sessions/`, `reviews/`, `plans/`, `execution/`, `cache/`)
2. Adds `.lets/`, `.worktrees/`, and `.mcp.json` (a tracker adapter's MCP config can carry a secret token) to `.gitignore` via `EnsureGitignore` (`.beads/` is added separately by `bd init`)
3. Migrates legacy `.lets/statusline.sh` (deletes the per-project shim if it matches the embedded snapshot — see `internal/initcmd/embedded_statusline_shim.sh`)
4. Migrates legacy `.lets/config.yaml` → `.lets/.env` (preserves user values via allowlist regex). Yaml is deleted (not renamed); orphan yaml alongside an existing `.env` is also cleaned up.
5. Writes/regenerates `.lets/.env` via `RegenerateEnv`. Always emits `LETS_ENV_VERSION` first key from `version.Version`. Skip path when version matches AND no value changes; regen path otherwise — preserves user values + foreign keys (latter under `# User-added keys` separator). Single `.env.bak` rotation per regen.
6. Refreshes `.lets/.env.example` from canonical `letsconfig.Keys` defaults via `renderEnvExample()` (no plugin template file — single source of truth)
7. Mutates `.claude/settings.json` to set `statusLine.command = "lets statusline"` (atomic write + `.bak`). Foreign user-customized commands left alone (value-match detection).
8. Copies plugin rules → `<project>/.claude/rules/lets-rules.md` using `drift.Check` (semver-aware: install / upgrade / skip). Drift state is recomputed after install for accurate JSON output.
9. Runs `bd init` (60s timeout) unless `--skip-beads`. Detection of "already initialized" goes through `bd status` exit code (authoritative, layout-independent).

Refuses: from a worktree (`--git-dir != --git-common-dir`), in `$HOME`, or in filesystem root `/`.

- `--rules-scope=project|user` — where this project's rules come from. `user` persists `LETS_RULES_SCOPE=user` and skips step 8 when the project copy is missing (delegated to the global `~/.claude/rules` copy); `project` (default) keeps the own copy. The cobra flag is the only strict validator — a hand-edited `.env` value other than `user` degrades to `project` (fail-safe). Set by `/lets:init` when global rules already cover the project and the user picks "Rely on global".

### `lets init --user` (user-scope install)

```bash
lets init --user --plugin-root "${CLAUDE_PLUGIN_ROOT}" [--language Ukrainian] [--launcher cmux] [--json]
```

User-scope alternative (`internal/initcmd/init_user.go::RunUser`): installs the global rules + user-level defaults ONCE per machine instead of per project. Works from **any** directory — no git repo, no worktree guard (those are project-scope concerns). Project-scope flags (`--merge-branch`, `--pr-flow`, `--skip-beads`, `--rules-scope`, `--github`) are warned about and ignored.

What it does (idempotent; a deliberate SUBSET of project init — no git, no `.gitignore`, no migrations, no `settings.json` statusline, no beads, no `.env.example`):

1. Copies plugin rules → `~/.claude/rules/lets-rules.md` via `drift.Check` (install / upgrade / skip). **`ahead` = no-clobber:** a global copy NEWER than the plugin (user customization — the only per-project opt-out Claude Code offers, GH anthropics/claude-code#8395 — or a newer release's copy) is reported with a warn step and left untouched. `unknown` (unparseable frontmatter) IS overwritten — `lets-*` files are plugin-owned by convention.
2. Writes/regenerates `~/.lets/.env` via `RegenerateUserEnv` — manages only the user-level keys (`LETS_LANGUAGE`, `LETS_LAUNCHER`; `letsconfig.UserKeys()`). Empty flag = preserve existing value, else canonical default. Hand-added `LETS_*` keys survive as foreign lines (the hook whitelist still injects them). Single `~/.lets/.env.bak` per regen (best-effort, machine-shared).

JSON envelope: same `initcmd.Result` shape (`schema_version=1`); `project_root` carries the **home dir** (the scope root for a `--user` run). Guard: refuses an empty, relative, or filesystem-root home (`guardHomeDir`; root-refusal is best-effort on Windows — drive roots not special-cased). Platform-neutral otherwise: `os.UserHomeDir()` + `filepath.Join` give `%USERPROFILE%\.claude\rules` on Windows (compile-checked only; no behavioral CI). Security note: `~/.claude/rules` is created via `MkdirAll` and is not symlink-hardened; `AtomicWriteBytes`'s rename REPLACES a symlink at the target rather than writing through it — same trust model as the project-scope writer.

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

Internal subcommand. Invoked by the `/lets:update` slash command (`commands/update.md`) with `--plugin-root=${CLAUDE_PLUGIN_ROOT}`. Syncs the drift-able LETS artifacts (four core + the optional user-scope rules copy + the optional `tracker-rules` adapter row); never prompts, never touches `settings.json` or beads — that's `lets init`'s job (init = setup; update = sync). Lives in `internal/cli/update.go` (cobra factory) + `internal/updatecmd/` (orchestration, GitHub latest-release lookup, plugin-version reader).

```bash
lets update --plugin-root "${CLAUDE_PLUGIN_ROOT}" [--json] [--offline] [--refresh-cache]
```

Flags:
- `--plugin-root` — plugin install dir (or `$CLAUDE_PLUGIN_ROOT`). Required, validated via the `.claude-plugin/plugin.json` marker.
- `--json` — emit a machine-readable JSON object (`schema_version=2`); `/lets:update` consumes this.
- `--offline` — skip the GitHub latest-release check; `binary`/`plugin` come back `unknown`.
- `--refresh-cache` — bypass the cached latest-release lookup and hit GitHub now.

What it checks (never crashes for a network failure). Emission order is `.env → binary → plugin → rules → user-rules → tracker-rules` so the **order-aware deferral** below can read the plugin's status off the already-computed artifact rather than re-deriving it:

| Artifact | Check | Action |
|---|---|---|
| `.lets/.env` | `LETS_ENV_VERSION` vs `version.Version` | `RegenerateEnv` with a near-empty `Prefs` (only the default tracker; user values are read from the existing `.env` regardless), so it just refreshes the header. `in-sync` (tracks the binary) when already current, `updated` after a header refresh. Skipped entirely on a `dev` binary (avoids stamping `LETS_ENV_VERSION=dev`). `not-initialized` if `.env` is absent → "Run /lets:init". Note: `.env` tracks the **binary**, so it syncs even while rules are deferred. |
| `lets` binary | `version.Version` vs latest GitHub release | `outdated` → drives `next_action.kind=binary` carrying the install one-liner (`curl -fsSL …/main/scripts/install.sh \| bash`) as an execution-bound const; `/lets:update` can run it in-session (approval-gated). `dev` build → no comparison. |
| Claude Code plugin | `<pluginRoot>/.claude-plugin/plugin.json::version` vs latest release | Report only — `outdated` → drives `next_action.kind=plugin` (`/plugin marketplace update lets-workflow` + `/reload-plugins`, or enable auto-update; both user-only). |
| `.claude/rules/lets-rules.md` | `drift.Check` against plugin source | Re-copy from the plugin (atomic write) on detected drift; `unknown` if the plugin's own frontmatter is unparseable; `in-sync` (tracks the plugin) otherwise. **Order-aware deferral (lets-rlue4):** when the installed rules are `outdated` AND the plugin itself is behind (outdated vs latest, or locally < the binary), the row is `deferred` and the file is **NOT written** — syncing now would advance to a stale lower version (the half-step). Only `StateOutdated` defers: a *missing* file still installs (behind rules beat none) and an *ahead* file keeps its current reset behavior. An `updated` row carries a past-tense detail ("was missing" / "was outdated (v…)"). **Scope-aware (`LETS_RULES_SCOPE`):** `scope=user` + missing project copy + present global = `delegated`; `scope=user` + nothing anywhere = `not-initialized`; a project copy under `scope=user` gets a report-only duplication hint. Any scope other than `user` is project semantics (fail-safe). |
| `~/.claude/rules/lets-rules.md` (`user-rules`) | `drift.Check` against plugin source — **row omitted entirely when the file is absent** (user-scope install not in use; update never bootstraps it — that's `lets init --user`) | Same sync + same order-aware `deferred` gate as project rules EXCEPT `ahead`: a newer/customized global copy gets `status=ahead` ("not overwritten") and is left alone. Deliberately EXCLUDED from the `consistent` check (an ahead copy is a customization, not a partial upgrade). `annotateInSyncBehind` applies (upstream = plugin). |
| `.claude/rules/tracker-<name>.md` (`tracker-rules`) | `drift.Check` against the plugin's `tracker-<name>.md` for the resolved `LETS_TRACKER` — **row appears only when the value names a shipped adapter** (unset/typo/pre-platform project → no row, artifact set unchanged) | Same sync + same order-aware `deferred` gate as project rules (only `StateOutdated` defers; missing still installs). Always project-local (no user scope). EXCLUDED from `consistent` (plugin-version-locked like `user-rules`). **Switch semantics:** the documented switch path is edit-`.env`-then-`/lets:update`, so the row also applies init Step 8b's switch actions — removes the deactivated shipped adapter (never two loaded at once) and scaffolds the create-once board profile; both reported on the row's `detail`. |

**`next_action` (the self-driving loop).** After computing the artifacts, `Run` sets a single top-level `next_action` (`kind` ∈ `init | binary | plugin | reload | done`, plus `message`, `command` for binary, `version` for done) derived purely from the artifact statuses — the one ordered step the user should take this run (init → binary → plugin → reload → done). Re-running advances one step until `kind=done` (`✓ Everything on vX.Y.Z`). `next_action.command` is **execution-bound**: it is only ever the `installScriptCmd` const, never interpolated with dynamic data (a byte-equal test pins it). `deferred` rows land in the `Unknown` summary bucket (the actionable step is counted once, on the plugin row); `next_action` is the single source of "do this".

Latest-release lookup hits `https://api.github.com/repos/restarter/lets-workflow/releases/latest` (5s timeout), cached 1h at `<project>/.lets/cache/update-check.json`; on a network failure it falls back to a stale cache entry if one exists, else reports `unknown`. The result also carries `consistent` (binary == plugin == installed-rules frontmatter version, ignoring `dev`) to flag a partial upgrade.

Two reference frames (the lets-kaw72 fix, `schema_version=2` — unchanged; `next_action` + `deferred` are additive): `.env`/`rules` report `in-sync` relative to their *local* source (the binary and the plugin respectively), distinct from `up-to-date` (== latest release) used for `binary`/`plugin`. So two `in-sync` rows at different versions is expected, not a contradiction; `annotateInSyncBehind` appends "itself behind latest v…" to an in-sync row whose upstream is itself `outdated`. `Summary.UpToDate` (JSON `up_to_date`) is the combined in-sync bucket (`in-sync` + `up-to-date`).

Refuses: from a worktree (`--git-dir != --git-common-dir`) — `.claude/` isn't shared into worktrees.

Same `--json` contract semantics as `lets init` (single JSON object, valid even on error, `SilenceUsage`/`SilenceErrors`, `TestResult_SchemaContract` guards `schema_version` bumps).

## `lets worktree`

Internal subcommand. Invoked by the `/lets:worktree` slash command (`commands/worktree.md`) as a thin dispatcher — markdown captures user intent via `AskUserQuestion`, shells out with `--json`, renders the result. All filesystem + git operations live here (`internal/cli/worktree.go` cobra factory + `internal/worktreecmd/` package, build constraint `//go:build unix`; Windows ships a no-op stub at `internal/cli/worktree_stub.go`).

```bash
lets worktree create <name> [--attach | --new-branch] [--branch <ref>] [--switch-main-if-needed] [--no-symlink-lets] [--no-symlink-beads] [--print-cd] [--json]
lets worktree remove <name> [--force] [--delete-branch [--force-branch]] [--branch-only --branch <name>] [--json]
lets worktree list [--json]
lets worktree info [--dir <path>] [--json]
```

Refuses: from inside a worktree (`create` only — the other three work in either repo), or when name validation fails (positive allowlist + `git check-ref-format`).

### `lets worktree --json` contract

Same shape conventions as `lets init` (single JSON object, valid even on error, `SilenceUsage` + `SilenceErrors`), with the additional structural notes below:

- **Envelope core.** Every subcommand result embeds `Envelope` (`internal/worktreecmd/result.go`): `schema_version`, `ok`, `subcommand`, `project_root`, `steps[]`, optional `error`. Per-subcommand result wrappers (`CreateResult`, `RemoveResult`, `ListResult`, `InfoResult`) add their own payload keys (`worktree`, `next_steps`, `removed`, `worktrees`, `main`, `in_worktree`, `main_root`, `rollback`). `TestResult_SchemaContract` pins these keys for all 4 wrappers + the bare Envelope.
- **Typed exit codes (10..21).** Defined in `internal/worktreecmd/exit.go`; scripts branch on `$?` without parsing prose. 22..29 reserved for a future `lets worktree adopt` subcommand. `main.go`'s `exitCoder` interface routes a typed `*worktreecmd.Error` (even through `fmt.Errorf("...%w", err)` wrapping) to the matching exit code; untyped errors fall through to `ExitGeneric` (1). `TestExitCoder_AsMatchesWorktreeError` (`cmd/lets/main_test.go`) pins that contract.
- **`error.kind` taxonomy.** When `ok=false`, the typed `error` object carries a snake_case `kind` for programmatic branching (`dirty_worktree`, `unpushed_commits`, `worktree_path_exists`, `branch_unmerged`, `branch_checked_out_in_main`, `branch_in_use_other_worktree`, `not_in_repo`, `inside_worktree`, `post_create_failed`, `rollback_refused_path_escape`, …). One exit code can carry multiple kinds (`ExitBranchConflict=13` covers both attach-time conflict variants); parse `error.kind` for specifics.
- **Rollback contract.** On `create` failure after `git worktree add` has run, `rollback` is populated with `{attempted, succeeded, residual: [...]}`. Residual entries name what couldn't be cleaned up (path, `branch:<name>`, `main_repo_on_branch:<actual> (expected <prev>)`) so the caller surfaces concrete cleanup instructions instead of hand-waving.
- **Stream split for shell composition.** `--print-cd` writes the absolute worktree path to **stdout** (one line, no newline-padding) while keeping `--json` envelope on **stderr** — gh-style. Lets shell wrappers compose `cd "$(lets worktree create foo --print-cd)" && claude` without parsing JSON. Without `--print-cd`, `--json` envelope goes to stdout as usual.
- **`next_steps.absolute_path`.** Load-bearing field that `commands/worktree.md` reads to tell the user where to `cd`. Renaming it without a `SchemaVersion` bump silently breaks the markdown skill — pinned by `TestResult_SchemaContract.create_success`.
- **Worktree-effective ignores via `info/exclude` (lets-x5ucf).** `EnsureGitignore` (Step 6) writes the main repo's *working* `.gitignore`, but a fresh worktree checks out its branch's *committed* `.gitignore` — which may lack the `.lets` entry, or carry a directory-only `/.lets/` that can't match the `.lets` symlink — so `.lets` / `.beads/.env` would surface as untracked inside the worktree (the child-repo report). Step 9.5 (`ensureWorktreeExcludes`) therefore appends the narrow entries `.lets` and `.beads/.env` to the shared `info/exclude` (resolved via `git rev-parse --git-path info/exclude` → the common git dir, so it is effective in main + every worktree, untracked, never pushed). Idempotent; only the actually-symlinked paths are added; narrow patterns leave other untracked `.beads/` content visible. Best-effort — a failure is a `StepWarn`, never a create failure. The old dir-only `.lets/` "not gitignored" warn now probes `.lets` (no slash) so it only fires when the symlink is genuinely unignored.
- **Dir NAME vs branch REF decoupling (`--branch`, lets-x5ucf).** The worktree directory name (positional `<name>`, validated by the slash-forbidding `nameRE` allowlist) and the attached/created branch ref are independent. `--branch <ref>` overrides the name-derived branch: the ref is attached/created **verbatim** (no `worktree-` prefix) and is validated by `validateBranchRef` (rejects leading `-`, then `git check-ref-format` — so `/` is allowed for git-flow refs like `feature/x`). The envelope already expresses the split via `worktree.name` (dir) vs `worktree.branch` (ref); `--branch` adds **no new field and no schema bump**. New error kinds on a bad ref: `empty_branch` / `invalid_branch` (both `ExitUsage=2`).

## lets cmux

Optional, macOS-only worktree launcher. Internal subcommand wired by `commands/worktree.md` (Step C3.5): after `lets worktree create`, when `LETS_LAUNCHER=cmux` (or `--cmux`), the skill shells `lets cmux open` to open the worktree in a cmux workspace (manaflow-ai/cmux) running `claude '/lets:start <id>'`. Cobra factory `internal/cli/cmux.go` (`//go:build unix`) + `internal/cmuxcmd/` package; Windows ships a stub at `internal/cli/cmux_stub.go` returning a clear "macOS-only" error.

```bash
lets cmux open <path> [--name <slug>] [--description <text>] [--command <cmd>] [--force] [--json] [--quiet]
lets cmux rename --title <new> [--ref <ref> | --cwd <path>] [--json] [--quiet]
lets cmux notify --title <text> [--subtitle <text>] [--body <text>] [--ref <ref> | --cwd <path>] [--json] [--quiet]
```

- **Canonical cmux form.** Runs `cmux workspace create --cwd <path> [--name] [--description] [--command]` with `CMUX_QUIET=1` (silences cmux's deprecation/notice output). The legacy `new-workspace` alias is avoided. No `--focus`: `cmux workspace create` does not accept it (verified via `--help`).
- **Description stamp (`--description`).** `cmux workspace create` accepts `--description` (cmux: "create … same flags as new-workspace"); the workspace stores it and exposes it in `cmux workspace list --json` under `description` (verified live). `commands/worktree.md` Step C3.5 uses it to stamp `<task-id> · <task-title>` so each running session self-identifies which beads task it belongs to (the `--name` slug is the short tab label; the description carries the canonical id + full title). Taskless worktrees omit it.
- **External cmux schema (pinned).** `open`'s guard and `rename`'s resolution parse `cmux workspace list --json` for fields `ref` / `title` / `selected` / `current_directory` — an external contract (manaflow-ai/cmux), verified against **cmux 0.64.x**. If cmux renames a field, `json.Unmarshal` leaves it zero-valued and the guard/resolution degrade SILENTLY (look like "no matching workspace"). Re-verify on cmux upgrades; a live smoke test is the only true guard given the build-tag blocks importable unit tests.
- **Duplicate-session guard (`open`).** Before creating, `open` lists workspaces (`cmux workspace list --json`) and refuses to spawn a second one whose `current_directory` already matches `<path>` — returns `ok=true`, `launch.launched=false`, `reason=already_open`, plus `existing_ref`/`existing_title`. `--force` overrides. This enforces "one live session per worktree" at the launcher level (the in-scope slice of the spawner-concurrency class; the deeper git-index/session-id mutex lives in the external session spawner, not here). A list failure is non-fatal — it falls through and creates.
- **`rename`.** Relabels a cmux workspace tab (`cmux workspace rename <ref> --title`). Resolution: explicit `--ref`, else `--cwd` match against `current_directory`, else the active (`selected`) workspace. Side-effect-free (no git/files). Never hard-fails on cmux absence (`reason`: `not_macos` | `cmux_not_found` | `workspace_not_found` | `cmux_error`); only a missing `--title` is a hard error (`ExitUsage`). **Invoked on-demand** (by an agent or by hand — e.g. a session relabels its own tab after claiming a task) so it can stamp identity into the tab and disambiguate concurrent sessions; it is **not** auto-wired into a `/lets:*` command yet.
- **Strictly optional, never hard-fails.** Detects cmux via `exec.LookPath("cmux")` + `runtime.GOOS=="darwin"`. On non-macOS, cmux-not-found, or a cmux exec error, returns `ok=true` with `launch.launched=false`, a `reason` (`not_macos` | `cmux_not_found` | `cmux_error`), and a `fallback_command` (`cd <path> && claude`). The only hard error is a missing/invalid `--path` (`ExitPathInvalid=10`).
- **JSON envelope.** `internal/cmuxcmd/result.go` — `Envelope` (`schema_version`, `ok`, `subcommand`, `steps[]`, optional `error`) + a `launch` block (`launched`, `workspace_name`, `description`, `path`, `command`, `reason`, `fallback_command`). `TestResult_SchemaContract` pins `SchemaVersion`. Exit codes in `internal/cmuxcmd/exit.go`; `main.go`'s generic `exitCoder` interface routes `*cmuxcmd.Error` to its code.
- **Autonomous launch (`--auto`).** `--auto` lives at the `/lets:worktree create` level (a `--command` string change, no `cmux`/Go change): when set, the launched command becomes `claude --permission-mode auto '/lets:start <id>'` across both launchers (terminal + cmux). Maps ONLY to `--permission-mode auto`, never `bypassPermissions` — autonomous implementation that still gates push / PR / `bd close` / external via the LETS AUTO MODE rules. See `commands/worktree.md` Step C3.5.
- **`notify` (gate-notification sink, lets-m8ecy).** Wraps `cmux notify --workspace <ref> --title [--subtitle] [--body]` (verified flags). Resolution mirrors `rename`: explicit `--ref`, else `--cwd` match, else the active workspace. Same never-hard-fail contract (`reason`: `not_macos` | `cmux_not_found` | `workspace_not_found` | `cmux_error`; only a missing `--title` is `ExitUsage`). **`Notified=true` means cmux ENQUEUED the notification, NOT that a human saw it** — callers keep an in-band signal (the gate halts visibly too). Exec is injection-safe: args go straight to `execve`, no shell, so a task title with `;`/backticks/`$()` is a single literal argv element. **Non-unix stub divergence (intentional):** unlike `open`/`rename` (whose Windows stubs return a hard error), the `notify` stub emits the SAME graceful `ok=true,notified=false,reason=not_macos` envelope and exits 0 — because `notify` is fired from `--json` gate snippets that parse the output, so a bare non-zero exit would break them cross-platform. Driven at LETS human-gate points by the autonomous pipeline (`commands/plan-workflow.md` GATE 1/2, `commands/execute.md` execute-blocked), marker-gated on `.lets/cache/pipeline-state-<id>` so only autonomous runs notify.

## lets statusline

Internal subcommand. Renders the Claude Code statusline; the project's `.claude/settings.json` invokes `lets statusline` directly (no flag) on every render. `lets init` points `statusLine.command` at it via value-match against `"lets statusline"`, leaving foreign user-customized commands alone. The legacy bash shim (`plugins/lets/scripts/lets/statusline.sh`, and the per-project `.lets/statusline.sh`) was retired in `lets-8ilsl`; `MigrateStatuslineSh` deletes a byte-equal legacy shim (matched against the frozen `internal/initcmd/embedded_statusline_shim.sh` snapshot) and calls `SetStatusLine`.

**Rich box is the default.** `internal/statusline/rich.go` renders a closed box (`┌─┐ │ ├─┤ └─┘`) wrapping identity / budget / task / rotating-tip lines. Flags:

- `--compact` — fall back to the legacy 2-line `renderLines` (for terminals where the box misbehaves).
- `--light` — light palette (default is dark).
- `--no-tip` (or env `LETS_STATUSLINE_TIP=off`/`0`/`false`) — hide the bottom tip line.
- `--no-dir` (or env `LETS_STATUSLINE_DIR=off`/`0`/`false`) — hide the Full-tier location pill.
- `--no-task` (or env `LETS_STATUSLINE_TASK=off`/`0`/`false`) — hide the task line AND skip its background `bd` refresh.
- `--rich` — hidden accepted no-op (rich is already the default).

**Width.** Two `COLUMNS`-driven tiers: **Full** (≥ `bpWide`=72) and **Compact** (< 72; fails open to Full when `COLUMNS` is absent). Below `bpFill`=90 the box fills the window (more tip room); at/above it hugs the widest line. Always capped at `fullMaxLine`=120 with a `boxRightMargin`=4 right gutter (CC's render area is a few cells narrower than the `COLUMNS` it passes, plus ambiguous-width glyphs). Sized **cell-accurately** — `cellWidth`/`fitCell` size each glyph; `wideRunes` maps any 2-cell glyph but is **currently empty**, since every emitted glyph is 1-cell text. **Two things drift the border, neither fixable by `wideRunes` alone: (1) a 2-cell glyph not registered in `wideRunes`; (2) font substitution — a glyph absent from the monospace face falls back to a font that draws it 2 cells while `cellWidth` counts 1** (this is why `glyphFolder` was changed from `☰` U+2630 to `»` U+00BB in `lets-6md86`: cmux/Ghostty substituted `☰` to 2 cells). Prefer glyphs present in standard monospace fonts; to check a glyph's real on-terminal width, print it between rulers (`printf '%s\n' 'ref |X|' 'glyph |<G>|'` — the closing `|` drifts right if 2-cell). Rows are `richRow{plain | prefix+mid+suffix}`: the task title / tip live in a flex `mid` that clips first, keeping the id + notes/hint suffix in frame. The **Full-tier location pill** shows the project/worktree root folder name (git top-level basename via `detectProjectRoot`, stable across `cd` into subdirs like `cli/`), or the literal word `worktree` inside a worktree (the dir name already equals the branch). **Compact** (< 72) drops the pill, PR, and the model's `(… context)` paren and shortens `window`→`w`, but keeps model name + effort (high-signal); a meaningless `.`/empty folder or branch is suppressed rather than rendered as a stray `⎇ .`. Universal 1-cell text glyphs (no Nerd Font, no emoji), a static `⚘` brand mark (the 🌱→🪴→🌿→🌳→🌴 growth ladder is implemented but parked behind it), color-graded effort, no progress bars (token counts + paren reset deltas).

**Task line (off the render hot path).** Reads `.lets/cache/task-status`, self-refreshed by a detached `lets statusline --fetch-task-only` subprocess (`bd show`, 90s TTL, id-only placeholder debounces the spawn) — no bd/network on the render path.

**Payload robustness.** `flexISO` decodes a numeric `resets_at`; `workspace.git_worktree` is intentionally not decoded (CC sends a string) — either would otherwise blank the bar. Escape-injection defense: `stripControl` folds C0/ESC/DEL/C1 control bytes to spaces in **every** externally-sourced field — folder/branch/worktree name, `model.display_name`, `pr.review_state`, `effort` — in `Render` before the renderer adds its own ANSI; `sanitizeField` is `stripControl` plus the `|`-fold/trim for the pipe-delimited bd-title cache line.

**Interactive guard.** Run by hand, `lets statusline` would block forever on the stdin read (it expects CC's JSON). `RunE` type-asserts `cmd.InOrStdin()` to `*os.File` and checks `os.ModeCharDevice` (after the `--fetch-*` branches, which don't touch stdin): a character-device stdin prints a one-shot wiring hint to stderr and exits 0, so CC never sees a crashed bar. Type-asserting (not reading `os.Stdin` directly) keeps test readers from tripping the guard (`lets-7frjs`).

## lets statusline config

`lets statusline config` persists the appearance render flags so a choice survives across sessions without hand-editing settings (`lets-vpwvs`). Logic in `internal/statuslinecmd/` (own `SchemaVersion` + JSON envelope + typed exit codes, mirroring `worktreecmd`); reuses `initcmd.AtomicWriteBytes`.

- **Target = `.claude/settings.local.json`** (personal, gitignored) — NOT the tracked `settings.json`, so persisting `--light` never forces it on collaborators (`settings.local.json` overrides `settings.json` for the `statusLine` key). Only the `statusLine` key is rewritten; all other local keys are preserved; atomic write; malformed JSON is refused rather than mutated.
- **Absolute state.** The persisted appearance is exactly the flags passed (`--light --compact --no-tip --no-dir --no-task`); `command` becomes `lets statusline [flags]`. Zero flags with neither `--reset` nor `--show` is rejected (`usage`, exit 2) to avoid an accidental reset; `--reset` deliberately persists the bare default.
- **`--show`** reads the current persisted appearance (parses the trailing flags back), no write. **`--force`** overwrites a foreign (non-`lets statusline`) command, otherwise refused (`foreign_statusline`, exit 30). **`--json`** emits the envelope.
- **Exit codes:** 0 ok · 2 usage · 10 not-in-repo · 30 foreign · 31 malformed-settings · 32 filesystem.
- A changed `statusLine` command is re-read by Claude Code only on session start — restart to apply. `/lets:statusline` (`commands/statusline.md`) is the interactive front door (AskUserQuestion → `lets statusline config … --json`).

## lets hook

Two cobra subcommands wired to Claude Code hooks: `lets hook session-start` (SessionStart) and `lets hook precompact` (PreCompact). Both take `--rules=${CLAUDE_PLUGIN_ROOT}/rules/lets-rules.md` and currently share output via `sessionstart.Run()` — the subcommands stay distinct for future divergence (e.g. context snapshotting before compaction). Project root is detected via `git rev-parse --show-toplevel` with an `os.Getwd()` fallback.

Output (≈2KB, well under Claude Code's 10K hook cap — `lets-q9bx7`):

- Optional `## LETS Notice` — a SCOPE-AWARE drift check (`sessionstart.go::driftCheck`). Decision table: project rules present (any state) → single-scope behavior, `drift.Message` wording (`/lets:update` for `outdated`/`unknown`/`ahead`, `/lets:init` for `missing`); project rules missing + global `~/.claude/rules/lets-rules.md` present and current → **no notice** (the user-scope install covers the project — the lets-wug9k nag fix); project missing + global drifted → `drift.MessageUser` wording (names the global path; `/lets:update` or `lets init --user`); both missing + `LETS_RULES_SCOPE=user` (from the merged env) → a precise "global copy missing, run `lets init --user`" notice (the project opted into the global copy which is now gone — the generic `/lets:init` nag would install a project copy against that choice); both missing otherwise → the classic `/lets:init` nag. A one-line "surface this to the user" instruction is appended (hook-only, NOT part of `drift.Message`; `commands/start.md` carries the same rule so a large slash command can't crowd the notice out).
- `## LETS Config` — the whitelisted `LETS_*` values from the MERGED env (`~/.lets/.env` overlaid by project `.lets/.env`; project wins per key, only non-empty values mask) + an `### About these values` explainer embedded from `internal/hook/sessionstart/local_config_explainer.md`. `LETS_MERGE_BRANCH` falls back to the repo's origin default branch (`gitutil.DefaultBranch`, 1s timeout, value capped at `envfile.MaxValueLen`), else literal `main`, when neither file supplies it — initialized projects always carry the key, so the extra git spawn only fires in uninitialized repos (per hook fire: SessionStart AND PreCompact).

The workflow rules themselves do NOT travel through the hook — they live in `.claude/rules/lets-rules.md` (the uncapped project-instructions channel; user-scope installs add the global `~/.claude/rules/lets-rules.md` floor), copied there by `lets init` / `lets init --user` and frontmatter-version-tracked. **Dual-hook rationale:** SessionStart on a `compact` source re-injects rules into the post-compaction context; PreCompact ensures rules are in the pre-compaction context the auto-summary is generated from — together they prevent workflow drift after compaction in long sessions.

## JSON envelope conventions

Every `lets <sub> --json` emits a single JSON object on stdout, valid even on `ok=false` (partial-completion contract — `steps[]` carries work done before the error; `error` carries `kind`/`message`/`remediation`). The cobra layer sets `SilenceUsage` + `SilenceErrors`; the human-readable error duplicates `result.Error` to stderr.

**`SchemaVersion` is per-package, not shared.** `initcmd`, `updatecmd`, and `worktreecmd` each declare their own `const SchemaVersion` (`initcmd` + `worktreecmd` = 1, `updatecmd` = 2), so a breaking change in one doesn't force a coordinated bump in the others. Field additions are minor (consumers ignore unknown fields); each package's `TestResult_SchemaContract` test fails on key drift, forcing a conscious bump decision.

**New `--json` subcommand packages should copy `worktreecmd`'s pattern** — a shared `Envelope` core + per-subcommand result wrappers — rather than inventing a new shape. Per-subcommand contracts: the `### ... --json contract` subsections above (`lets init`, `lets update`, `lets worktree`).
