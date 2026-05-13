# Installing lets

`lets` is the companion CLI for the `lets-workflow` Claude Code plugin. It powers
SessionStart/PreCompact hooks, the `lets statusline` renderer, and the
`lets init` / `lets update` per-project setup that the plugin's slash commands invoke under the hood.

This guide covers installing the **binary** (`lets` on `$PATH`), the **plugin**
(in Claude Code), and **per-project initialization**.

## TL;DR

```bash
# 1. Install the lets binary (macOS / Linux)
curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash

# 2. Install the Claude Code plugin (one-time, inside Claude Code)
/plugin marketplace add restarter/lets-workflow
/plugin install lets

# 3. Initialize a project
cd your-project && claude
/lets:init
```

---

## 1. Install the `lets` binary

### Quick install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash
```

### What the installer does

1. **Detects your platform** (`darwin`/`linux` × `amd64`/`arm64`).
2. **Fetches the latest release** from GitHub Releases.
3. **Downloads** the platform-specific archive **and** the SHA256 checksums file.
4. **Verifies the archive against `checksums.txt`** — refuses to install on mismatch.
5. **Extracts** and installs to `/usr/local/bin/lets` (if writable) or `~/.local/bin/lets` (fallback). Does **not** auto-elevate via `sudo` — to install globally on a system where `/usr/local/bin` needs root, pipe it through sudo: `curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | sudo bash`.
6. **Warns** if the install dir isn't on `$PATH`, and prints the line to add.
7. **Warns** if multiple `lets` binaries are reachable on `$PATH`, with a per-binary version comparison and concrete reorder/remove instructions.
8. **Verifies** by running `lets version`.

The installer always uses the **latest stable release**. To install an older version, use the manual download steps below.

### Environment overrides

| Variable | Purpose |
|---|---|
| `GITHUB_TOKEN` | Authenticated requests to api.github.com. Useful for: private/Enterprise repos, CI runners hitting the 60/hr unauth rate limit. When set, the installer pulls assets via `api.github.com/repos/.../assets/<id>` (works through Bearer auth) instead of public `github.com/.../releases/download/...` URLs. |

---

## 2. Set up the Claude Code plugin

Once `lets` is on `$PATH`, install the plugin from Claude Code's marketplace. **Run these commands inside Claude Code, not in your shell:**

```
/plugin marketplace add restarter/lets-workflow
/plugin install lets
```

This is a one-time setup per machine. When Claude Code asks who to install for, pick **"Install for all collaborators on this repository"** (project scope — committed to `.claude/settings.json`, so teammates inherit it) or **"Install for you, in this repo only"** (local scope); avoid the **everywhere** / user-scope option, where the SessionStart/PreCompact hooks fire in *every* project you open. Tip: in `/plugin` → **Marketplaces** → `lets-workflow`, **Enable auto-update** so the plugin stays current on its own.

> Verify the plugin loaded: `/lets:` commands should now autocomplete in Claude Code. (The `🌱 LETS Workflow vX.Y.Z » <branch>` statusline appears once you've run `/lets:init` in a project — see step 3.)

---

## 3. Initialize a project

In each project where you want to use LETS:

```bash
cd your-project
claude
```

Then inside the Claude Code session:

```
/lets:init
```

This creates `.lets/` (gitignored), populates `.lets/.env` with sensible defaults, copies the workflow rules to `.claude/rules/lets-rules.md`, wires up the statusline, and runs `bd init` if beads is installed. Re-run anytime to self-heal drift or change config.

You're done — start working with `/lets:start`. Later, when a new release ships, run `/lets:update` to re-sync `.lets/.env` and the rules file (and see version status for the binary and the plugin).

---

## Manual download

If you prefer not to pipe `curl` to `bash`, or you want to pin a specific version:

```bash
VERSION=X.Y.Z                       # the version you want — see https://github.com/restarter/lets-workflow/releases
OS=darwin                           # or linux
ARCH=arm64                          # or amd64

# Archive + checksums
curl -fsSLO "https://github.com/restarter/lets-workflow/releases/download/v${VERSION}/lets_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSLO "https://github.com/restarter/lets-workflow/releases/download/v${VERSION}/lets_${VERSION}_checksums.txt"

# Verify
shasum -a 256 -c "lets_${VERSION}_checksums.txt" --ignore-missing

# Install
tar -xzf "lets_${VERSION}_${OS}_${ARCH}.tar.gz"
sudo install lets /usr/local/bin/lets
lets version
```

Pick a release from <https://github.com/restarter/lets-workflow/releases>.

---

## Windows

Native Windows install (`winget`, `scoop`, `install.ps1`) is tracked in [lets-hdrdr.1](https://github.com/restarter/lets-workflow/issues). Until then:

- **WSL** — run the [Quick install](#quick-install-macos--linux) above inside WSL. The bash installer detects WSL and prints a heads-up that the binary is only visible inside WSL (not native Windows).
- **Git Bash** — the bash installer detects MINGW/MSYS/CYGWIN and exits with a pointer here. Download `lets_<VERSION>_windows_amd64.zip` manually from [Releases](https://github.com/restarter/lets-workflow/releases), unzip, and place `lets.exe` somewhere on `%PATH%`.

---

## From source

Requires Go 1.22+ and a checkout of the repo. Useful for plugin contributors:

```bash
git clone https://github.com/restarter/lets-workflow
cd lets-workflow
make install      # installs to /usr/local/bin or ~/.local/bin (smart fallback)
lets version
```

`make install` bypasses GitHub Releases entirely and builds from the local tree. This is the canonical install method for maintainers.

---

## Troubleshooting

### "command not found: lets" after install

Either `/usr/local/bin` is missing from your `$PATH`, or the installer fell back to `~/.local/bin` and that's not on `$PATH` either. Add this to your shell profile (`~/.bashrc`, `~/.zshrc`, `~/.profile`):

```bash
export PATH="$PATH:$HOME/.local/bin"
```

Then start a new shell, or `source ~/.zshrc` (etc.).

### macOS Gatekeeper: "lets cannot be opened"

Rare — `curl + tar + mv` doesn't trigger Gatekeeper for binaries that didn't come through a browser/Finder. If it does happen:

```bash
xattr -cr /usr/local/bin/lets
```

This clears the `com.apple.quarantine` extended attribute. Binaries are not yet code-signed; notarized macOS builds are a future concern.

### Checksum mismatch

The installer refuses to install if the downloaded archive doesn't match the SHA256 entry in `checksums.txt`. This means either the download was corrupted (rare with HTTPS) or the release was tampered with (extremely unlikely on GitHub Releases). Re-run the installer; if it fails again, open an issue.

### "Multiple 'lets' executables on your PATH"

The installer warns when more than one `lets` binary is reachable via `$PATH` (e.g. an old `~/go/bin/lets` from a previous `go install`, plus the freshly installed `/usr/local/bin/lets`). The warning lists each binary with its version and tells you whether the **installed** copy is first in PATH:

```
⚠ Multiple 'lets' executables on your PATH — an older copy may be executed instead of the one we installed.
    Found (entries earlier in PATH take precedence):
      1. /Users/you/go/bin/lets  →  lets v0.4.0
      2. /usr/local/bin/lets     →  lets v0.5.0

    We installed to: /usr/local/bin/lets
⚠ The 'lets' first in your PATH is different from the one we installed.
    To make the newly installed 'lets' the one you get when running 'lets', either:
      - Remove or rename the older /Users/you/go/bin/lets from your PATH, or
      - Reorder your PATH so /usr/local/bin appears before /Users/you/go/bin
```

If the warning ends with `✓ The installed 'lets' is first in your PATH.` — you're fine, no action needed. Otherwise, follow the suggested fix and reload your shell.

### API rate limit

Unauthenticated requests to `api.github.com` are limited to 60/hour per IP. If you hit this (typically only on shared CI runners), set `GITHUB_TOKEN`:

```bash
GITHUB_TOKEN=$(gh auth token) bash <(curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh)
```

The token is only used for the Authorization header during the install run; it's never written to disk.

---

## What gets installed

A single static binary `lets` (`CGO_ENABLED=0`, no dynamic library deps).
No config files written to system locations, no daemon, no `$PATH` mutations.

To uninstall:

```bash
rm /usr/local/bin/lets       # or ~/.local/bin/lets, depending on where it landed
```

The plugin (in Claude Code) and per-project `.lets/` directories are independent — uninstall them separately if needed.

---

## Dependencies

`lets` and `bd` (beads) both need to be on `$PATH` for the plugin to work end-to-end. Install beads via its own one-liner:

```bash
curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash
```

See [beads docs](https://github.com/steveyegge/beads) for details.
