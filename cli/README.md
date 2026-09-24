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
│   ├── frontmatter/              # YAML frontmatter `version` reader (parses via x/mod/semver)
│   ├── drift/                    # Rules-file drift check + user-facing drift messages
│   ├── gitutil/                  # Shared git helpers
│   ├── hook/sessionstart/        # SessionStart + PreCompact output (LETS Config + Notice + drift check)
│   ├── initcmd/                  # `lets init` orchestration (init.go + migrate.go + env.go +
│   │                             #   render.go + tracker.go + embed.go + …)
│   ├── updatecmd/                # `lets update` (order-aware next_action, deferred rules; SchemaVersion=2)
│   ├── worktreecmd/              # `lets worktree create/remove/list/info/adopt/release/sweep/task-state/branch-name` (//go:build unix)
│   ├── cmuxcmd/                  # `lets cmux open/rename/notify` (optional macOS launcher, //go:build unix)
│   ├── orcacmd/                  # `lets orca open/notify/status` (opt-in Orca addon, //go:build unix)
│   ├── trackeradapter/           # leaf: adapter `## Worktree` links + task id / branch convention (+ board override)
│   ├── taskstate/                # leaf: the one owner of .lets/sessions/.task-<branch-slug> (locked merge-write)
│   ├── taskid/ fsutil/ peername/ redact/  # leaves: id gate class, flock + SameDir, peer names, secret/control scrub
│   ├── tmuxcmd/                  # `lets tmux open/rename/notify` (portable launcher, Linux+macOS, //go:build unix)
│   ├── notifycmd/                # `lets notify` (launcher-neutral gate sink, dispatches on LETS_LAUNCHER, //go:build unix)
│   ├── handoffcmd/               # `lets handoff targets/send/codex/await` (hand-off delivery, //go:build unix)
│   ├── agentrun/                 # leaf: external agent providers (codex exec + rollout, report-file contract)
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
- `--language`, `--merge-branch`, `--pr-flow`, `--tracker`, `--launcher` — preference flags. Empty value (no flag passed) signals "use existing or fail if creating fresh". Non-empty triggers regen with new value. `--tracker` (`beads` | `none`) additionally installs the matching `.claude/rules/tracker-<name>.md` adapter on a fresh init (an existing `.env` value wins on re-init, with a warning).
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

1. Writes `~/.claude/rules/lets-rules.md` through `rulescache.Sync` — the same writer the SessionStart hook uses to keep that file a cache of the running plugin (see [lets hook](#lets-hook)); `--user` is the explicit bootstrap, so it may create the file. `created` / `written` → an ok step with the cache Notice; `noop` → skip ("matches the running plugin"); `kept-newer` / `skipped` / `failed` → a warn step naming the reason, with `drift.detected=true`. Only an installed plugin writes: a `--plugin-root` outside `~/.claude/plugins/cache/<marketplace>/lets/<version>` is refused ("not an installed plugin"). A copy that is not byte-identical to some installed LETS release is COPIED to `lets-rules.md.bak` (`.bak-2`, … — never reused) before it is replaced; a newer cache is never downgraded by an older plugin. After this first install the hook keeps the file current — `lets init --user` is not the refresh path.
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

Internal subcommand. Invoked by the `/lets:update` slash command (`commands/update.md`) with `--plugin-root=${CLAUDE_PLUGIN_ROOT}`. Syncs the drift-able LETS artifacts (four core + the optional `tracker-rules` adapter row) and reports the optional user-scope global rules (`user-rules`, read-only - the session hook maintains them); never prompts, never touches `settings.json` or beads — that's `lets init`'s job (init = setup; update = sync). Lives in `internal/cli/update.go` (cobra factory) + `internal/updatecmd/` (orchestration, GitHub latest-release lookup, plugin-version reader).

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
| Claude Code plugin | `plugin.json::version` of the **installed** plugin vs latest release — `ResolveInstalledRoot` picks the newest `installPath` of `lets@<marketplace>` in `~/.claude/plugins/installed_plugins.json` (version read from each path's `plugin.json`), because `${CLAUDE_PLUGIN_ROOT}` is frozen per session and can be older than what is installed; a missing, malformed or ambiguous index falls back to the handed root, unverified, with a note on the row | Report only — `outdated` → drives `next_action.kind=plugin`; `/lets:update` can refresh it on disk in-session (`claude plugin marketplace update lets-workflow && claude plugin update lets@lets-workflow`, approval-gated) or the user runs `/plugin marketplace update lets-workflow` + `/reload-plugins`. When the installed plugin is newer than the loaded one the row says so and `loaded_plugin_version` carries the loaded version; the project `rules` / `tracker-rules` rows compare against the installed plugin. |
| `.claude/rules/lets-rules.md` | `drift.Check` against plugin source | Re-copy from the plugin (atomic write) on detected drift; `unknown` if the plugin's own frontmatter is unparseable; `in-sync` (tracks the plugin) otherwise. **Order-aware deferral (lets-rlue4):** when the installed rules are `outdated` AND the plugin itself is behind (outdated vs latest, or locally < the binary), the row is `deferred` and the file is **NOT written** — syncing now would advance to a stale lower version (the half-step). Only `StateOutdated` defers: a *missing* file still installs (behind rules beat none) and an *ahead* file keeps its current reset behavior. An `updated` row carries a past-tense detail ("was missing" / "was outdated (v…)"). **Scope-aware (`LETS_RULES_SCOPE`):** `scope=user` + missing project copy + present global = `delegated`; `scope=user` + nothing anywhere = `not-initialized`; a project copy under `scope=user` gets a report-only duplication hint. Any scope other than `user` is project semantics (fail-safe). |
| `~/.claude/rules/lets-rules.md` (`user-rules`) | Read-only: the cache key (`~/.lets/cache/rules-cache.json`), the file and the installed plugin's rules — **row omitted entirely when the file is absent** (user-scope install not in use; update never bootstraps it — that's `lets init --user`, the first install) | **Never written** — the SessionStart hook owns the file (see [lets hook](#lets-hook)). `delegated` (healthy) only when the plugin root is verified, and the key, the file and that plugin's rules share one sha256 while the key names that version; the detail names the cached version + a 12-char hash. Otherwise `unknown` with the reason: not recorded yet, edited since the last sync, the installed plugin carries different rules, or not verified against the installed plugin. EXCLUDED from `consistent` and from `annotateInSyncBehind` (informational). |
| `.claude/rules/tracker-<name>.md` (`tracker-rules`) | `drift.Check` against the plugin's `tracker-<name>.md` for the resolved `LETS_TRACKER` — **row appears when the value names a shipped adapter, or a user-authored/unshipped one with an installed `.claude/rules/tracker-<name>.md` copy (reported `delegated`, "left as-is")** (only a name with neither a shipped source nor an installed copy — unset/typo/pre-platform project → no row, artifact set unchanged) | Same sync + same order-aware `deferred` gate as project rules (only `StateOutdated` defers; missing still installs). Always project-local (no user scope). EXCLUDED from `consistent` (plugin-version-locked like `user-rules`). **Switch semantics:** the documented switch path is edit-`.env`-then-`/lets:update`, so the row also applies init Step 8b's switch actions — removes the deactivated shipped adapter (never two loaded at once) and scaffolds the create-once board profile; both reported on the row's `detail`. |

**`next_action` (the self-driving loop).** After computing the artifacts, `Run` sets a single top-level `next_action` (`kind` ∈ `init | binary | plugin | reload | main-checkout | new-session | user-rules | done`, plus `message`, `command` for binary, `version` for done) derived purely from the artifact statuses — the one ordered step the user should take this run (init → binary → plugin, or `reload` when a newer plugin is already installed but this session still runs the older one → `reload` after a rules sync → `main-checkout` for a worktree run → `new-session` / `user-rules` for the global rules row → done). Re-running advances one step until `kind=done` (`✓ Everything on vX.Y.Z`) — except where a re-run cannot change the answer, and the message says so: `reload` for an installed-but-not-loaded plugin, `new-session` (the hook fixes the global rules at the next session start) and `user-rules` (a diagnostic, e.g. an unverified plugin root). `next_action.command` is **execution-bound**: it is only ever the `installScriptCmd` const, never interpolated with dynamic data (a byte-equal test pins it). `deferred` rows land in the `Unknown` summary bucket (the actionable step is counted once, on the plugin row); `next_action` is the single source of "do this".

Latest-release lookup hits `https://api.github.com/repos/restarter/lets-workflow/releases/latest` (5s timeout), cached 1h at `<project>/.lets/cache/update-check.json`; on a network failure it falls back to a stale cache entry if one exists, else reports `unknown`. The result also carries `consistent` (binary == plugin == installed-rules frontmatter version, ignoring `dev`) to flag a partial upgrade.

Two reference frames (the lets-kaw72 fix, `schema_version=2` — unchanged; `next_action` + `deferred` are additive): `.env`/`rules` report `in-sync` relative to their *local* source (the binary and the plugin respectively), distinct from `up-to-date` (== latest release) used for `binary`/`plugin`. So two `in-sync` rows at different versions is expected, not a contradiction; `annotateInSyncBehind` appends "itself behind latest v…" to an in-sync row whose upstream is itself `outdated`. `Summary.UpToDate` (JSON `up_to_date`) is the combined in-sync bucket (`in-sync` + `up-to-date`).

From a worktree (`--git-dir != --git-common-dir`) the run succeeds: `.env`, `rules` and `tracker-rules` come back `skipped` (counted in `summary.skipped`, never as up to date) with a detail naming the main checkout, `main_checkout` carries its path, and `next_action.kind=main-checkout` points there — `.claude/` isn't shared into worktrees. The binary, plugin and global-rules rows are checked as usual. `skipped`, `summary.skipped`, `main_checkout` and `loaded_plugin_version` are additive (`schema_version` stays 2).

Same `--json` contract semantics as `lets init` (single JSON object, valid even on error, `SilenceUsage`/`SilenceErrors`, `TestResult_SchemaContract` guards `schema_version` bumps).

## `lets worktree`

Internal subcommand. Invoked by the `/lets:worktree` slash command (`commands/worktree.md`) as a thin dispatcher — markdown captures user intent via `AskUserQuestion`, shells out with `--json`, renders the result. All filesystem + git operations live here (`internal/cli/worktree.go` cobra factory + `internal/worktreecmd/` package, build constraint `//go:build unix`; Windows ships a no-op stub at `internal/cli/worktree_stub.go`).

```bash
lets worktree create <name> [--attach | --new-branch] [--branch <ref>] [--switch-main-if-needed] [--no-symlink-lets] [--no-store-links] [--plugin-root <dir>] [--print-cd] [--json]
lets worktree remove <name> [--force] [--delete-branch [--force-branch]] [--branch-only --branch <name>] [--json]
lets worktree list [--json]
lets worktree info [--dir <path>] [--task-candidate [--ref-file <file>] [--plugin-root <dir>]] [--json]
lets worktree adopt [--dir <path>] [--task <id>] [--links-only] [--replace-task] [--plugin-root <dir>] [--json]
lets worktree release [--dir <path>] [--json]
lets worktree record --task <id>... [--ref <ref>] [--json]
lets worktree sweep [--apply] [--json]
lets worktree task-state show|set [--task <id> --start <sha>] [--session-sha <sha> --session-id <sid>] [--orc <name>] [--clear-task|--clear-origin|--clear-orc] [--create] [--wait <dur>] [--json]
lets worktree branch-name --task <id> --title-file <file> [--worktree] [--plugin-root <dir>] [--json]
```

Refuses: from inside a worktree (`create` only), or when name validation fails (positive allowlist + `git check-ref-format`). `--no-symlink-beads` survives as a hidden alias of `--no-store-links`.

- **Declared store links.** `create` and `adopt` link `.lets` plus whatever the active tracker adapter's `## Worktree` `links:` declares (beads: `.beads/.env` 0600, parents 0700); an installed adapter that predates the section falls back to the plugin copy (`--plugin-root`, default `$CLAUDE_PLUGIN_ROOT`). Every write goes through `os.OpenRoot`, and a committed symlink is never followed.
- **`adopt`** makes a worktree LETS did not create (Orca, a teammate, `git worktree add`) a LETS worktree: locked on `<main>/.lets/locks/adopt.lock`, idempotent, never calls a tracker or Orca and never deletes a directory. A cache-only real `.lets` moves to `.lets.pre-adopt[-N]`; any other content stops with exit 22. The task step records the id in `.task-<branch-slug>` - from `--task`, the existing file, or the convention's `accept:` shape / directory name as an unconfirmed `origin: branch|dir` candidate. Runs from Orca's `orca.yaml` setup hook and from the SessionStart self-heal.
- **`release`** runs before a worktree goes away (Orca's archive hook). A worktree removed outside Orca - gh >= 2.99 `gh pr merge --delete-branch` from another checkout removes the head's linked worktree - is never released; the rules say to merge a worker's PR without `--delete-branch`, and `lets worktree record` / `/lets:start --main` surface what slipped through. It writes `.lets/cache/released-<task-id>` (`<id>|<branch>|<iso>|dirty=<bool>|unpushed=<bool>|snapshot=<present|stale|missing>`) FIRST; a task-state file that names a valid task is removed only once that marker is on disk, and only if - under its lock - it still names that task (a writer that got in between keeps its revision). A file that names no task is removed without a marker: that is the normal state after `/lets:done` closed the task. An unreadable file, an invalid id, a failed marker or a changed file is kept, and `released.task_state_kept` says which. A record that is not `present`, a marker that could not be written, or a kept file prints one `lets: ...` line on stderr (even with `--quiet`) and fires `lets notify`. Once a linked worktree is resolved it never refuses - not for dirty or unpushed work; it does refuse the main checkout and a directory outside a git repository.
- **`record`** answers per task: `record.state` (`present` - the tip is an ancestor of a snapshot's `- head:` line; `stale` - snapshots exist but none covers the tip, `detail=unanchored` for ones written before the line existed; `missing`), the local traces (`task_state` files, `refs/heads/` branches in a created or accepted shape), the live `worktrees` holding them, `marker`, and `orphan` (a trace, no worktree, no marker). An inventory that fails half-way (an unreadable task-state file or marker, a failed `git for-each-ref`) fails the call instead of answering `orphan=false`. Read-only; never calls a tracker. Task-state temps are named `.tasktmp-<slug>.<digits>`, outside the `.task-` namespace, so every `.task-*` entry is a state file.
- **`sweep`** lists local task branches (convention prefixes) merged into `origin/<LETS_MERGE_BRANCH>` (else the local merge-branch); `--apply` deletes them. Checked-out and never-diverged branches are skipped; a squash merge is not detectable and stays in `unmerged`.
- **`remove` is origin-aware**: a branch that is an ancestor of `origin/<merge>` is deleted with `-D` even when the local merge-branch lags; a worktree already gone still finishes the branch step (`removed.already_gone`); a worktree outside `.worktrees/` is refused (`worktree_external` - archive it in Orca, which runs `release`).
- **`task-state`** is the CLI of the `taskstate` package, the one writer of `.task-<branch-slug>`: `set` merge-writes only the given keys under a lock and keeps unknown lines; `--orc` on the merge-branch writes nothing (`orc_on_merge_branch`), and a `--task` that differs from the file's without `--start` writes nothing (`task_mismatch`).
- **`branch-name` / `info --task-candidate`** are how markdown uses the convention: Go derives the slug from an untrusted title file and renders `branch:` / `worktree-branch:`, and reads a created-shape id off HEAD or a `--ref-file`. `branch-name` returns `reasons[]` from `LoadConvention`, so a `source` of `default` is never unexplained: a convention with no `id:` declared anywhere is dropped whole - board templates included - and says so with `convention_undeclared` + `convention_keys_ignored_no_id`.

### `lets worktree --json` contract

Same shape conventions as `lets init` (single JSON object, valid even on error, `SilenceUsage` + `SilenceErrors`), with the additional structural notes below:

- **Envelope core.** Every subcommand result embeds `Envelope` (`internal/worktreecmd/result.go`): `schema_version`, `ok`, `subcommand`, `project_root`, `steps[]`, optional `error`. Per-subcommand result wrappers (`CreateResult`, `RemoveResult`, `ListResult`, `InfoResult`, plus the adopt / release / sweep / task-state / branch-name results) add their own payload keys (`worktree`, `next_steps`, `removed`, `worktrees`, `main`, `in_worktree`, `main_root`, `task_candidate`, `rollback`, `store_links`, `moved_aside`, `task`, `released`, `merged` / `unmerged` / `deleted`, `task_state`). `TestResult_SchemaContract` pins these keys for all 4 wrappers + the bare Envelope.
- **Typed exit codes (10..26).** Defined in `internal/worktreecmd/exit.go`; scripts branch on `$?` without parsing prose. 22 `lets_dir_conflict` (a real `.lets` with non-cache content), 23 not a linked worktree, 24 a declared store link could not be made, 25 `.task-<slug>` already names a different task, 26 the task-state lock was still held at the deadline; 27..29 are free. `main.go`'s `exitCoder` interface routes a typed `*worktreecmd.Error` (even through `fmt.Errorf("...%w", err)` wrapping) to the matching exit code; untyped errors fall through to `ExitGeneric` (1). `TestExitCoder_AsMatchesWorktreeError` (`cmd/lets/main_test.go`) pins that contract.
- **`error.kind` taxonomy.** When `ok=false`, the typed `error` object carries a snake_case `kind` for programmatic branching (`dirty_worktree`, `unpushed_commits`, `worktree_path_exists`, `branch_unmerged`, `branch_checked_out_in_main`, `branch_in_use_other_worktree`, `not_in_repo`, `inside_worktree`, `post_create_failed`, `rollback_refused_path_escape`, …). One exit code can carry multiple kinds (`ExitBranchConflict=13` covers both attach-time conflict variants); parse `error.kind` for specifics.
- **Rollback contract.** On `create` failure after `git worktree add` has run, `rollback` is populated with `{attempted, succeeded, residual: [...]}`. Residual entries name what couldn't be cleaned up (path, `branch:<name>`, `main_repo_on_branch:<actual> (expected <prev>)`) so the caller surfaces concrete cleanup instructions instead of hand-waving.
- **Stream split for shell composition.** `--print-cd` writes the absolute worktree path to **stdout** (one line, no newline-padding) while keeping `--json` envelope on **stderr** — gh-style. Lets shell wrappers compose `cd "$(lets worktree create foo --print-cd)" && claude` without parsing JSON. Without `--print-cd`, `--json` envelope goes to stdout as usual.
- **`next_steps.absolute_path`.** Load-bearing field that `commands/worktree.md` reads to tell the user where to `cd`. Renaming it without a `SchemaVersion` bump silently breaks the markdown skill — pinned by `TestResult_SchemaContract.create_success`.
- **Worktree-effective ignores via `info/exclude` (lets-x5ucf).** `EnsureGitignore` (Step 6) writes the main repo's *working* `.gitignore`, but a fresh worktree checks out its branch's *committed* `.gitignore` — which may lack the `.lets` entry, or carry a directory-only `/.lets/` that can't match the `.lets` symlink — so `.lets` and the store links would surface as untracked inside the worktree (the child-repo report). Step 9.5 (`ensureWorktreeExcludes`, shared with `adopt`) therefore appends the narrow entries `.lets`, `.lets.pre-adopt*` and each linked store path (beads: `.beads/.env`) to the shared `info/exclude` (resolved via `git rev-parse --git-path info/exclude` → the common git dir, so it is effective in main + every worktree, untracked, never pushed). Idempotent; only the actually-symlinked paths are added; narrow patterns leave other untracked `.beads/` content visible. Best-effort — a failure is a `StepWarn`, never a create failure. The old dir-only `.lets/` "not gitignored" warn now probes `.lets` (no slash) so it only fires when the symlink is genuinely unignored.
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

## lets tmux

The portable third `LETS_LAUNCHER` (Linux + macOS), a file-for-file sibling of `lets cmux` driving the real `tmux` CLI. Cobra factory `internal/cli/tmux.go` (`//go:build unix`) + `internal/tmuxcmd/` package; Windows ships a stub at `internal/cli/tmux_stub.go`. Wired by `commands/worktree.md` Step C3.5 (`LETS_LAUNCHER=tmux` or `--tmux`). Shares the cmux contract — read "## lets cmux" for the envelope shape, the never-hard-fail rule, and the duplicate guard; only the tmux-specific deltas are listed here.

```bash
lets tmux open <path> [--name <slug>] [--description <text>] [--command <cmd>] [--force] [--json] [--quiet]
lets tmux rename --title <new> [--ref <ref> | --cwd <path>] [--json] [--quiet]
lets tmux notify --title <text> [--subtitle <text>] [--body <text>] [--ref <ref> | --cwd <path>] [--json] [--quiet]
```

- **Inside vs outside `$TMUX`.** `open` checks `$TMUX`. Inside a session → `tmux new-window -c <path>` (a new window in the current session; `in_existing_session=true`, no attach needed). Outside → `tmux new-session -d` (a **detached** session) and surfaces `attach_command` (`tmux attach -t <name>`). **Never auto-attaches in the `--json`/command path** — attaching would seize the terminal of whatever invoked it (a Claude Code bash subprocess). `-P -F '#{session_name}:#{window_index}'` prints the created target, so the window index is never guessed. Command delivery is `send-keys` (the pane keeps its shell after the command exits).
- **`--description` → `@lets_task`, NOT the window name.** Unlike cmux (a tooltip with no width budget), the tmux **window name IS the status line**. So the window name stays the short `--name` slug and the full `<task-id> · <title>` is stamped into a window-level user option (`set-option -w @lets_task`), readable via `list-panes -F '#{@lets_task}'`. Best-effort — a stamp failure does not sink a launch that already created the window.
- **Duplicate guard by `pane_current_path`.** `open` scans `tmux list-panes -a -F …` and refuses a second pane at the same path (`reason=already_open`, `existing_target`/`existing_title`; `--force` overrides). Paths compared via `cleanPath` (`filepath.Abs` + `EvalSymlinks`) — **load-bearing on macOS**, where tmux reports `pane_current_path` symlink-resolved (`/tmp` → `/private/tmp`); without it the guard fails open.
- **`notify` broadcasts to attached clients; `no_client` is not a phantom success.** `display-message -t <session>` into a session with ZERO attached clients exits 0 while displaying to nobody (verified, tmux 3.6b). Since `open` creates the worktree session detached, a target-scoped notify would report `Notified=true` on the autonomous pipeline's own default path. So `notify` enumerates `tmux list-clients` (server-wide, no `-t`) and `display-message -c <client> -d 0 <msg>` to each; zero clients → `notified=false, reason=no_client`. `Clients` counts how many the message reached. `-d 0` holds the message until a keypress (man tmux: "a delay of zero waits for a key press") — the tmux default `display-time` (750ms) would vanish before an away-from-keyboard operator returns. Message carries the task identity because it may land on a client attached to another session.
- **`reason` vocabulary** (no `not_macos` — tmux is cross-platform on unix): `tmux_not_found` | `tmux_error` | `already_open` (open) | `pane_not_found` (rename) | `no_client` (notify). **Windows stub asymmetry** mirrors cmux: `open`/`rename` hard-error (`errTmuxUnsupported`); `notify` emits a graceful `ok=true,notified=false,reason=not_supported` envelope (exit 0) so `--json` gate snippets parse cross-platform.
- **Test seam.** Every tmux invocation goes through an overridable package `var` (`lookTmux`, `insideTmux`, `runTmux`, `listPanesRaw`, `listClientsRaw`, `activeTargetRaw`), so the table tests run with no tmux server and no host tmux. Live smoke is still the only true guard for the real CLI contract (the build tag blocks importable unit tests, same as cmux).
- **Not `make dev-tmux`.** `make dev-tmux` is a **dev harness** (build a dev binary, spawn one Claude pane per existing `.worktrees/*`, for testing the plugin); `LETS_LAUNCHER=tmux` is the **worktree launcher** (open ONE new worktree session during `/lets:worktree create`). No shared code.

## lets orca

Opt-in Orca addon (stablyai/orca desktop app): `LETS_LAUNCHER=orca` is the one switch for every Orca integration - without it nothing looks up an Orca binary. Cobra factory `internal/cli/orca.go` (`//go:build unix`) + `internal/orcacmd/`; Windows ships a stub.

```bash
lets orca open --repo <main-checkout> --name <slash-free-name> [--prompt '/lets:start <id>'] [--force] [--json] [--quiet]
lets orca notify --title <text> [--body <text>] [--cwd <path>] [--json] [--quiet]
lets orca status [--json] [--quiet]
lets orca card --phase start|pr|closed|end|blocked|gate [--comment <text>] [--json] [--quiet]
lets orca repos [--json]
lets orca wake (--repo <main> | --repo-index <n>) --session <sid> [--pid <n>] --title <name> [--json]
```

- **Binary resolution.** The app bundle first (`/Applications/Orca.app/Contents/Resources/bin/orca`, then `~/Applications/...`), PATH last and only when `status --json` decodes into Orca's envelope: a broken root-owned `/usr/local/bin/orca` symlink and the GNOME screen reader named `orca` both exist in the wild.
- **No pinned capability table.** The Orca CLI is versioned with the app, so `orcacmd` calls the verb and classifies the failure (`orca_not_found`, `orca_app_not_running`, `orca_capability_missing` for Orca's `invalid_argument` on an unknown flag, `orca_output_unrecognized`, `orca_error`, ...). Status reads `result.app.running` and the version from `result.runtime.appVersion` (Orca 1.4.203).
- **`open`** runs `worktree create --repo path:<repo> --name <name> --no-parent --agent claude [--prompt]` (argv, no shell). Orca turns `/` in a name into `-` and has no branch option, so callers pass `<task-id>-<slug>` (an `accept:` shape adopt recognizes). Dup guard on `worktree ps` (`already_open`, `--force` overrides). Never hard-fails except an invalid `--repo` (exit 10): any Orca failure is `launched=false` + `reason` + `fallback_command` (`/lets:worktree create <name> --no-orca`, which chains to cmux, then terminal). Orca launches Claude with the user's own agent command, so `--permission-mode auto` cannot be passed (`/lets:worktree` renders `orca_auto_unsupported`).
- **`notify`** writes `worktree set --worktree id:<id> --comment "<title>: <body>"` on this worktree's card: the target comes from `ORCA_WORKTREE_ID` when it names this checkout, else a `worktree ps` row whose path is the same directory as `--cwd`.
- **`card`** mirrors a LETS phase onto this worktree's Orca card: `start` -> `in-progress`, `pr` -> `in-review`, `closed` -> `completed`, and `end` / `blocked` / `gate` write only the comment (redacted, 280 runes). It reads `LETS_LAUNCHER` itself and without `orca` returns `orca_not_enabled` with no exec, even inside an Orca terminal; without `ORCA_WORKTREE_ID` it returns `orca_env_absent`; an env id that is not this checkout gives `orca_env_mismatch`. Go never reads the tracker - the phase snippets in start / done / end / execute / plan-workflow pass the title, and each snippet carries the `[ "{LETS_LAUNCHER}" = "orca" ] &&` guard (`TestOrcaCardSnippetsGated`). `card` and `notify` both write the card comment, so the last writer wins; Orca ignores an empty `--comment`, so a comment cannot be cleared from the CLI.
- **`repos`** decodes `repo list --json` in Go and keeps only directories git reports as main checkouts (others are dropped by display name), indexed in Orca's order, so the hub never types an Orca-supplied path into a shell.
- **`wake`** resumes a stopped orchestrator: refuses `main_alive` / `liveness_unknown` from the Claude registry (orcacmd reads the `ccregistry` leaf, never `peerscmd`), then `terminal create --worktree path:<repo> --title <name> --command "claude -r <sid>"` (a `/rename` name would open the resume picker, so the session id is used) and a startup-only `terminal wait --for tui-idle` with one stale-handle retry.
- **`status`** reports `status.bin`: the resolved binary a multi-step Orca flow (the `/lets:team --backend orca` run) uses for every later call, as Orca's orchestration guide requires.
- **`orca.yaml`.** `lets init` writes it (marker-owned; a foreign file is left alone with a warning) only when the project `.lets/.env` has `LETS_LAUNCHER=orca`: `scripts.setup` runs `lets worktree adopt --quiet` and `scripts.archive` runs `lets worktree release --quiet`, both unable to fail the hook and with a GUI-safe PATH. Orca reads it from the main checkout and asks the user once to trust it.

## lets peers

The Go side of peer messaging between LETS sessions (`internal/peerscmd/`, `internal/ccregistry/`, `//go:build unix`; the Windows stub answers who / tail / orchestrator with a parseable `not_supported` envelope and hard-errors the rest). Used by the `/lets:orc` skill and the orient Peers block - the skill composes; Go reads, frames, addresses and sends.

```bash
lets peers who [--role R] [--orc NAME] [--session SID] [--exclude-session SID] [--prune] [--probe-orca] [--timeout-ms 2500] [--json]
lets peers tail (<name> | --to-session SID | --to-terminal HANDLE) [--last N] [--since-message ID --sent-at ISO] [--addressed-to-session SID [--count-only]] [--repo DIR | --repo-index N] --json
lets peers frame --to-session SID --kind ask|ping|tell|ask-ro [--session SID] --json
lets peers tell --to-session SID --msgid ID [--repo DIR | --repo-index N] --json
lets peers wait --to-session SID --since-message ID --sent-at ISO [--timeout-ms N] --json
lets peers role set orchestrator|worker|peer [--task ID] [--scope TEXT] [--takeover] [--session SID] [--cwd DIR] --json
lets peers role clear [--session SID]
lets peers orchestrator [--session SID] [--cwd DIR] --json
lets peers who (--repo DIR | --orca-repos) [--role orchestrator] --json      # the hub: read-only, adds last_orchestrators
lets peers ask-ro (--repo DIR | --repo-index N) --session SID [--pid N] --msgid ID --json
```

- **Sources.** The Claude Code session registry (`<claude config dir>/sessions/<pid>.json`, `$CLAUDE_CONFIG_DIR` honoured) is always read; each entry is judged on its own: the filename pid's liveness first (a stale file never degrades anything), then `peerProtocol: 1` and a valid session id. A live entry that cannot be read is counted in `registry_protocol_unknown` while the readable rows still come back. `<pid>.<hash>.key` peer-token files are never opened. Transcripts are read backward in 64 KiB chunks under an 8 MiB cap. Orca terminals are consulted only with `LETS_LAUNCHER=orca` or `--probe-orca` - `ORCA_WORKTREE_ID` alone never selects the source. For another project (`--repo`, `--repo-index`, `--orca-repos`) the calling session's switch decides, never that project's own `LETS_LAUNCHER`.
- **The join rule.** A session and an Orca terminal are one peer only when the terminal handle equals the `orca_terminal:` the session reported about itself in its role file (from `ORCA_TERMINAL_HANDLE`) and the terminal's worktree is the session's worktree root; the pair must be unique. Titles are never identity. A joined row sends over Orca (`send=orca`); a Claude-only row whose name is unique across the whole registry sends over `SendMessage` (`send=claude`); an Orca-only non-Claude agent is read-only in v1 (`non_claude_send_unsupported_v1`).
- **Liveness per holder.** Role files record the holder's pid. The session id found under any live registry pid is alive (`claude -r` resumes the same id under a new pid); otherwise a dead pid, or a pid now owned by another session, is dead (pruned); a pid the registry cannot read, or a pid-less holder it does not show, is `unknown` - never treated as dead. A holder whose pid now runs under ANOTHER session id, in a process that started no later than the role was written (the registry's `startedAt`), is the same session with a re-minted id (`/clear`, an in-session `/resume`): every load moves its role file to the new id in memory, and `role set`, `who --prune` and the caller's own heal write the move under the lock instead of pruning it; a reused pid (started later) is still pruned.
- **Roles.** `.lets/sessions/peers/<session>.role` (dir 0700, file 0600, atomic, under `.lets/locks/peers.lock`). Several orchestrators may share a repo, each unique by its LIVE registry name, with an optional one-line `scope`; `name_held` unless `--takeover`, which demotes only that holder. A worker branch's `orc:` line in `.task-<slug>` binds it to one orchestrator; `orchestrator` resolves self | bound (never re-routed when dead) | single | ambiguous | none. A session with no role file gets its own back on its next `frame` / `orchestrator` / `who --session` call and at SessionStart (startup, resume, clear): a worker from its branch's `task:` (never on the merge-branch), an orchestrator from a user-set name (`nameSource=user`) with no live holder and exactly one dead role file or last-seen record under it - several are refused as `reclaim_ambiguous` in `degraded[]`. `role` and `orchestrator` envelopes carry `remediation` with every `reason`.
- **Addressability.** `orchestrator` returns a `target` only when THIS repo's own `peers()` computation (the same one `who` uses) can reach it - a bound or unbound candidate that is cross-repo, dead, or present but unsendable comes back in `refused[{name, scope, session6, reason, detail, hint}]` instead, with `reason` one of `target_in_other_repo` | `target_not_alive` | `target_unsendable` | `bound_ambiguous`, never a target the send path cannot reach. `Peer.send` (`who`, `orchestrator`) is emitted only where it was actually computed - an absent `send` is "not computed", never "unreachable". `peers()` is memoized per resolved repo context, so a resolution that tests several candidates pays its cost (a git subprocess and a transcript stat per registry row) once, bounded by the same ~2500 ms budget `who` uses.
- **Resumed sessions.** One session id held by two live pids (a `claude -r` beside the still-running original, counted over the whole machine's registry) is one peer with `send=none`, `reason=session_duplicated`. A bound orchestrator is the live holder of the bound name, else - when nobody holds it live - the one role file registered under it (a resumed session can come back under another registry name); several registered candidates refuse with `bound_ambiguous`, never a pick. `tell` to a session that is no peer of this repo reports `state=not_a_live_peer_of_this_repo`. `tail` takes the peer's exact live name positionally (exactly one of `<name>`, `--to-session`, `--to-terminal`; `peer_not_found` / `peer_ambiguous`).
- **Non-completion is not a refusal.** A `refused[].reason` says a SPECIFIC target cannot be addressed; `source=none` with `reason=branch_unreadable` (the caller's branch could not be read at all) or `reason=budget_exhausted` (the branch WAS read, but the ~2500 ms budget ran out before resolution could test its candidates) says nothing was established either way - no `target`, no `refused[]` entry, just a `Degraded{git, branch_unreadable}` or `Degraded{context, deadline_exceeded}`; neither means "no orchestrator is alive". A `git worktree list` that could not be read degrades the same way rather than silently narrowing the peer set to the main checkout: `Degraded{git, worktrees_unreadable}`, with every OTHER worktree's session excluded from that one resolution.
- **Framing.** `frame` issues a 16-hex msgid from `crypto/rand` and the header `[lets-peer id=… kind=… from_sid=… to_sid=… from="<role>/<name>" to="<name>"]`; addressing compares session ids exactly, names are display only. A sender with no registry name (or none the registry shows) frames as `from="<role>/<session6>"` - only the target must resolve. `frame` refuses a session addressing itself (`self_send`, no header issued) and prunes handoff files older than 24h before issuing a new one. The skill writes the whole message with the Write tool to `handoff_path` (`.lets/cache/peer-msg/<msgid>.txt`; `peer-msg/` must be a real 0700 directory). `tell` opens it `O_NOFOLLOW` and requires a regular file of at most 8 KiB that starts with the issued header, but no longer always deletes it - see Sending below.
- **Reading.** A turn's text is capped at 2 KiB, or 16 KiB when it is a reply (`--since-message`) or a message addressed to the reader (`--addressed-to-session`); a whole call is capped at 64 KiB, dropping the oldest turns first (the newest turn is never dropped). `tail` returns the last 5 turns, or with `--since-message` and no `--last` the whole reply (up to 100 turns). `omitted` counts turns left out entirely, by `--last` or by the call cap. `truncated_bytes` counts SOURCE bytes lost to either cut: bytes removed inside a turn that was kept, plus the full source size of a turn the call cap dropped - its kept text AND whatever the per-turn cap had already cut from that same turn, so a heavily-capped dropped turn is not under-reported. So a whole-turn drop moves both numbers, and `omitted: 0` next to a non-zero `truncated_bytes` means every turn is present but at least one was shortened. A screen is redacted (`Creds` then `Text`, so a URL's `user:password@host` and a bare token are both caught) as one text before it is capped at 40 lines, so a private key spanning lines - whole, or cut by the screen edge - never reaches `screen[]`.
- **Sending.** Go sends only over Orca and only when the transcript ends a turn with no open tool call, the screen shows the idle `❯` prompt with no approval / trust dialog, and Orca reports the agent `done`, all under a machine-wide `~/.lets/locks/peer-send-<session>.lock`; a stale handle re-checks before one retry; `observed` means the target transcript shows the msgid, and nothing is resent. A Claude-routed peer gets `delivered=false reason=claude_transport_model_send` plus the framed text for the skill's `SendMessage`. On any other non-delivery, `tell` exits 11 while `ok` stays true - the command ran, the peer was not reachable. `note`'s presence tells the two shapes apart: when nothing was ever typed the handoff is KEPT and `note` names the retry, which reuses the SAME msgid; when a send was attempted but delivery could not be proven (e.g. a stale-handle retry that also failed) the handoff is CONSUMED - `note` is absent, and it is never retried, since a resend after an unproven attempt could duplicate a message that did land. A delivered send and the claude route also consume the handoff. `wait` is satisfied only by an end-of-turn record after the message.
- **Hub reads.** `who --repo` (a validated main checkout) and `--orca-repos` (every repo `lets orca repos` validated, rows tagged `repo_index`) never prune or write in the other project. They add `last_orchestrators[{name, scope, session, session6, pid, source}]` from `.lets/sessions/peers/last/<hex(name)>.last` (written when a dead orchestrator is pruned, one file per name) and from dead orchestrator role files not yet pruned (`source: role_file`); a malformed session or pid is omitted with a note. To message a LIVE orchestrator of another project, `tell` and `tail` take `--repo` / `--repo-index`: the handoff stays in this checkout and the target is looked up by session id in its own repo (a name matches only inside the repo it belongs to).
- **`ask-ro`** answers a read-only question from a STOPPED orchestrator by forking its session headlessly in its checkout: `claude -p --resume <sid> --fork-session --permission-mode plan --tools Read,Grep,Glob --disallowedTools Bash,Write,Edit,NotebookEdit,WebFetch,WebSearch,mcp__* --strict-mcp-config --mcp-config .lets/cache/ask-ro-mcp-empty.json --output-format json`, pinned by test (a `--dangerously*` or `--allowed*` flag panics). Spike 6.0: the disallow list alone still leaves Task, Workflow, SendMessage and CronCreate loaded; `--tools` leaves exactly Read, Grep and Glob, and the forked session leaves the original transcript untouched. The environment is an allowlist (no session id, no `ORCA_*` / `LETS_*`), `claude --help` must list every narrowing flag (`headless_readonly_unenforceable` otherwise), the prompt comes from a `frame --kind ask-ro` handoff consumed like `tell`'s, the run times out after 5 minutes, and the answer is redacted and capped at 8 KiB. `main_alive` and `liveness_unknown` refuse: never a second process on a live session.
- **Exits.** 0 (degraded sources included), 1 generic, 2 usage, 10 not in a repo, 11 `tell` ran but did not deliver (`ok=true`; the envelope is authoritative).
- **Residual.** A same-user process that can write `.lets` could forge `orca_terminal:`; such a process can already type into terminals, so v1 accepts it.

## lets notify

Launcher-neutral gate-notification sink (`internal/notifycmd/`, `internal/cli/notify.go`). Resolves `LETS_LAUNCHER` (project `.lets/.env` over `~/.lets/.env` via `letsconfig.MergedEnv` — the ONE precedence rule, shared with the SessionStart hook) and dispatches to `cmuxcmd.Notify` / `tmuxcmd.Notify` / `orcacmd.Notify`. Exists so the autonomous-pipeline gate snippets (`commands/plan-workflow.md`, `commands/execute.md`) never hardcode a launcher — a `lets cmux notify` snippet is silent for a tmux user, and interpolating the launcher name would need a `terminal`-guard replicated across command files.

```bash
lets notify --title <text> [--subtitle <text>] [--body <text>] [--ref <ref> | --cwd <path>] [--json] [--quiet]
```

- **Dispatch + reasons.** `cmux`/`tmux` delegate to the launcher package and pass its `reason` through verbatim (the two launchers do NOT share a reason vocabulary — `notifycmd` never interprets it, the only contract is "non-empty reason = degraded"). `terminal` → `notified=false, reason=launcher_terminal` (no channel). An unknown hand-edited value → `notified=false, reason=launcher_unknown` (echoed in `launcher`). A launcher returning no result → `launcher_error` (the mandatory nil-guard, so a future second hard-error path in a launcher can never panic a gate). Only a missing `--title` is a hard error (`ExitUsage`).
- **`--cwd` does double duty.** It resolves the launcher target AND locates the project `.lets/.env` (`gitutil.ProjectRoot(cwd)`). This is how a worktree works: `.lets/` is a symlink to the main repo's, so the resolved `LETS_LAUNCHER` matches what the hook injected. The gate snippets call it as `lets notify --cwd "$LETS_PROJECT_ROOT" …`.
- **Non-unix stub.** Emits a graceful `ok=true,notified=false,reason=not_supported` envelope (exit 0), same rationale as the tmux/cmux notify stubs.
- **Validation is elsewhere but paired.** `letsconfig.ShippedLaunchers` + `ValidLauncher` gate `lets init --launcher=<x>` (rejected with the whitelist in the message); `internal/cli/launcher_test.go::TestShippedLaunchers_MatchSubcommands` pins the whitelist to the registered subcommands in both directions — the launcher analogue of `initcmd`'s `TestShippedTrackers_MatchOnDisk`.

## lets handoff

Delivery of a hand-off brief (`internal/handoffcmd/`, `internal/agentrun/`, `internal/cli/handoff.go`, `//go:build unix`; the Windows stub answers every subcommand with a parseable `not_supported` envelope, exit 0). Used by `/lets:handoff --codex | --send` - the command composes and saves the brief (artifact-path kind `handoff`), Go runs the agent, types into Orca and reads the report back.

```bash
lets handoff targets [--match <term_handle | agent name | title fragment>] [--json]
lets handoff send --brief <abs path> (--terminal <handle> | --new codex) [--json]
lets handoff codex --brief <abs path> [--timeout 30m] [--json]
lets handoff await --brief <abs path> --agent <send.agent> --since <send.sent_at> [--fingerprint <send.fingerprint>] [--timeout 30m] [--json]
```

- **Envelope.** `schema_version` (1), `ok`, `subcommand`, `steps[]`, `error` on `ok=false`, then `targets{available, terminals[{handle, title, agent, state, last_output_at}], reason}`, `send{delivery, reason, handle, title, agent, input_line, created, sent_at, fingerprint, screen_tail}` or `run{provider, ran, complete, reason, exit_code, session_id, report_path, events_path, stderr_path, rollout_path, stderr_tail, duration_ms, workspace_changed, warnings}`. There is no result file: a long run is started in the background and its own output is the result.
- **Exits.** 0 (Orca or Codex absent is a named reason, not an error), 1 generic, 2 usage, 10 `brief_invalid` (the brief must be a regular `[A-Za-z0-9._-]+.md` file directly in `<root>/.lets/handoffs/`, and the checkout path must match `^/[A-Za-z0-9._/ -]+$` so the pointer line stays inert words).
- **Targets.** Orca terminals of this checkout that hold an agent (`agentIdentity`, or the `worktree ps` agent row joined by `tabId:leafId`), writable and connected, never `$ORCA_TERMINAL_HANDLE` (this session's own tab), newest output first. A plain shell is never a target (`not_an_agent`).
- **Send.** One line - `Read the hand-off brief at <path> and follow it exactly.` - plus Enter, once, under a per-pane lock `~/.lets/locks/handoff-send-pane-<tabId>_<leafId>.lock` (a handle when Orca gives no pane key), so a stale-handle re-join and a sender aimed at the new handle share it. Refused only on evidence, nothing typed: Orca's agent state `working` / `waiting` (`target_busy`); a non-empty Orca `draft` for the input line, any agent (`input_line_not_clear` - in a Claude pane typed text never reaches the screen and Enter would submit it with the pointer); a dialog in any agent's tab - a permission or trust question, an update prompt - that Enter would answer (`target_busy`); the Codex / Claude screen recognizers seeing a busy agent (`target_busy`) or typed text (`input_line_not_clear`). Screen evidence counts only near the live input, so words from history cannot refuse an idle tab: a working line within 4 lines above the prompt line; for Codex / Claude a dialog only when the lowest prompt line is not a ready composer (a dialog's options take the prompt glyph); for other agents a dialog only in the bottom 12 lines; case-insensitive. `--new codex` sends only once the new tab's own record names an agent (re-read up to 5 times) - a Codex that failed to start leaves a shell, which is `not_an_agent`. An agent without a recognizer (Antigravity: its prompt echoes its mode, `/plan `) is typed into with `input_line=unknown`. There is no interrupt. `--new codex` opens `codex --sandbox read-only` in this checkout and waits for startup (`startup_not_idle` when an update or trust prompt holds it). A receipt with `turn_started` is `proven`; input accepted without it is `unproven` + `delivery_unconfirmed` + `screen_tail` (redacted with `Creds` + `Text` over the whole frame, control bytes replaced, last 40 lines). A `terminal_handle_stale` rejection re-lists and sends once to the terminal holding the same pane; any other failure is never retried.
- **Codex headless.** `codex exec --sandbox read-only --cd <root> --json --color never --output-last-message <base>-report.md -` with the brief on stdin, in its own process group; on return the whole group is SIGKILLed (os/exec `WaitDelay` kills only the leader). The report is the `-o` file (the root turn's last agent message); when empty, the rollout's `task_complete.last_agent_message`. `thread.started` gives `session_id` (`codex resume <id>`); `turn.failed` is `turn_failed`, `error` events are warnings.
- **Await, Codex.** Scans every `~/.codex/sessions/*/*/*/rollout-*.jsonl` modified since the send (a rollout lives under the date its session STARTED). The delivered prompt is a `response_item` user message carrying the brief path, not older than the send, in a root session (`session_meta.thread_source` user); its turn is the last `task_started` before it, and the `task_complete` / `turn_aborted` with that `turn_id` ends the wait. A marker in a tool output, an echo, an older turn or a subagent session never matches; two root rollouts carrying it are `marker_ambiguous`.
- **Await, any other agent.** The report-file contract: the `--send` brief asks the agent to write `<base>-agent-report.md`, then create `<base>-agent-report.done`. The done file (regular, not older than the send) ends the wait; the report is opened `O_NOFOLLOW`, read up to 256 KiB, redacted and capped.
- **Outputs.** Siblings of the brief, 0600, never overwritten (`report_exists`): `-report.md`, `-events.jsonl`, `-stderr.txt` (Go), `-agent-report.md`, `-agent-report.done` (the agent). Every report is redacted (`Creds` + `Text`) and capped at 128 KiB. A content fingerprint (status, `git diff HEAD`, untracked names / sizes / mtimes) taken before and after sets `workspace_changed`.
- **Reasons.** `orca_*`, `not_an_agent`, `terminal_is_self`, `terminal_other_checkout`, `terminal_not_writable`, `terminal_not_found`, `target_busy`, `input_line_not_clear`, `startup_not_idle`, `another_send_in_progress`, `send_lock_failed`, `delivery_unconfirmed`, `input_not_accepted`, `codex_not_found`, `headless_unsupported`, `await_unsupported_agent`, `timeout`, `canceled`, `exit_nonzero`, `turn_failed`, `turn_aborted`, `report_empty`, `report_exists`, `report_unreadable`, `marker_not_found`, `marker_ambiguous`, `io_error`.
- **Lane boundary.** The handoff lane is not the peer lane: `handoffcmd` never imports `peerscmd` (`TestDeniedImports`), and the handoff and peer sends lock different files (`handoff-send-pane-<pane>` vs `peer-send-<session>`) - a shared transport lock belongs to lets-cbmg7.

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

- Optional `## LETS Notice` — the project rules drift check (`sessionstart.go::driftCheck`) plus the global rules cache outcome (below). Drift decision table: project rules present (any state) → `drift.Message` wording (`/lets:update` for `outdated`/`unknown`/`ahead`, `/lets:init` for `missing`); project rules missing + global `~/.claude/rules/lets-rules.md` present, or `LETS_RULES_SCOPE=user` (from the merged env) → **no drift notice** (the user scope covers the project — the lets-wug9k nag fix; the global file is not drift-checked, the cache sync speaks for it); both missing otherwise → the classic `/lets:init` nag. A one-line "surface this to the user" instruction is appended (hook-only, NOT part of `drift.Message`; `commands/start.md` carries the same rule so a large slash command can't crowd the notice out). Outside a git project the hook prints only a non-empty Notice (the cache outcome) and nothing else; a no-op prints nothing.
- `## LETS Config` — the whitelisted `LETS_*` values from the MERGED env (`~/.lets/.env` overlaid by project `.lets/.env`; project wins per key, only non-empty values mask) + an `### About these values` explainer embedded from `internal/hook/sessionstart/local_config_explainer.md`. `LETS_MERGE_BRANCH` falls back to the repo's origin default branch (`gitutil.DefaultBranch`, 1s timeout, value capped at `envfile.MaxValueLen`), else literal `main`, when neither file supplies it — initialized projects always carry the key, so the extra git spawn only fires in uninitialized repos (per hook fire: SessionStart AND PreCompact).

The workflow rules themselves do NOT travel through the hook — they live in `.claude/rules/lets-rules.md` (the uncapped project-instructions channel; copied by `lets init`, frontmatter-version-tracked) and, for user-scope installs, the global `~/.claude/rules/lets-rules.md` floor, which the hook keeps as a cache (below).

**Global rules cache** (`internal/rulescache`, lets-tg008). `session-start` (every source: startup, resume, clear, compact — after the worktree self-heal, so a fresh worktree's linked `.lets/.env` is seen) calls `rulescache.Sync` with the plugin root this session actually loaded (`--rules` minus `rules/lets-rules.md`). `precompact` does not. The contract:

- **Key = content hash**, stored in `~/.lets/cache/rules-cache.json` (sha256, version, source path) — never the frontmatter version (one version string has shipped with two contents). Every sync hashes the plugin rules and the installed file; there is no size/mtime shortcut. An unparseable key, or one whose hash is not 64 hex chars, means "no key" and re-hashes.
- **One transaction under one lock** (`~/.lets/cache/rules-cache.lock`, 2 s): read, compare, back up and write re-read everything inside it. A busy lock skips with a named reason; it never waits longer.
- **Forward only:** a plugin older than the cached copy never replaces it (Notice: "kept vX … Not an error").
- **Only an installed plugin writes:** the root must be `~/.claude/plugins/cache/<marketplace>/lets/<v>[-suffix]` with `plugin.json` name `lets`, version `<v>`, matching the rules frontmatter. A `--plugin-dir` checkout or anything uncertain skips (Notice only when that withheld a change; an equal copy stays silent and records no key).
- **Hand edits are never lost:** a copy that is neither the cached hash nor byte-identical to an installed LETS release is COPIED to `lets-rules.md.bak`, `.bak-2`, … (`O_EXCL`, never reused) before the active file is replaced atomically; a failed replace leaves it as it was. Claude Code loads only `*.md`, so backups never load — keep your own rules in a separate `.md` file.
- **Create only when asked:** a missing file is created only inside an initialized LETS project whose merged `LETS_RULES_SCOPE=user` (or by `lets init --user`); elsewhere the hook only maintains an existing file.
- **Visibility:** a Notice line only when something happened (installed, refreshed, backed up, kept newer, skipped a change, failed). A failure never blocks startup.

**Headless one-session lag (stated contract).** Interactive sessions get the running plugin's rules in the same session. A headless `claude -p` run (including `lets peers ask-ro` and a headless hand-off) loads memory before the hook runs, so the first such run after a plugin update reads the previous rules once; the next start of any kind is current. **Dual-hook rationale:** SessionStart on a `compact` source re-injects rules into the post-compaction context; PreCompact ensures rules are in the pre-compaction context the auto-summary is generated from — together they prevent workflow drift after compaction in long sessions.

## JSON envelope conventions

Every `lets <sub> --json` emits a single JSON object on stdout, valid even on `ok=false` (partial-completion contract — `steps[]` carries work done before the error; `error` carries `kind`/`message`/`remediation`). The cobra layer sets `SilenceUsage` + `SilenceErrors`; the human-readable error duplicates `result.Error` to stderr.

**`SchemaVersion` is per-package, not shared.** `initcmd`, `updatecmd`, `worktreecmd`, `cmuxcmd`, `tmuxcmd`, `orcacmd`, `notifycmd`, and `statuslinecmd` each declare their own `const SchemaVersion` (`updatecmd` = 2; the rest = 1), so a breaking change in one doesn't force a coordinated bump in the others. Field additions are minor (consumers ignore unknown fields); each package's `TestResult_SchemaContract` test fails on key drift, forcing a conscious bump decision.

**New `--json` subcommand packages should copy `worktreecmd`'s pattern** — a shared `Envelope` core + per-subcommand result wrappers — rather than inventing a new shape. Per-subcommand contracts: the `### ... --json contract` subsections above (`lets init`, `lets update`, `lets worktree`).
