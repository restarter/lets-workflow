# Onboarding Test Report

Task: **Onboarding test: first external developer setup** (`6m8.1`)
Date: 2026-02-25 / 2026-02-26
Tester: Artem Shybko
PR: [#3](https://github.com/restarter/lets-workflow/pull/3)

## Environment

- OS: Linux Mint 20 (Ubuntu 20.04, GLIBC 2.31)
- Node.js: v25.6.1 (nvm)
- Claude Code: 2.1.58
- bd: 0.56.1 (Go build from source)
- gh: 2.87.3
- Dolt: installed via install.sh

## Full Step-by-Step Log

### 1. gh CLI - smooth

- `apt` install with official repo, no issues
- Version: 2.87.3

### 2. bd CLI - MAJOR BLOCKER

- `npm install -g @beads/bd` FAILED (GLIBC 2.32+ required, have 2.31)
- `curl install.sh | bash` - same GLIBC error
- Fix: install Go, `go install github.com/steveyegge/beads/cmd/bd@latest`
- Copy binary to `~/.local/bin/bd` (overwrite install.sh binary)
- Friction: 3 failed attempts before Go workaround. Undocumented.

### 3. Dolt - smooth with caveats

- `sudo bash install.sh` worked fine (no GLIBC issue)
- Created `~/.dolt-databases/beads_lets-plugin-claude`, ran `dolt init`
- Started server: `cd ~/.dolt-databases && dolt sql-server --port 3307 &`
- Caveat: server must be running before bd works, no autostart configured

### 4. bd init - BLOCKER

- `bd init --prefix lets-plugin-claude` failed - empty `.beads/dolt/` dir blocked it
- Fix: `rm -rf .beads/dolt` then reinit
- `bd init --from-jsonl` did NOT import issues (known bug in bd 0.56.1)
- Workaround: custom Python SQL script to INSERT 74 issues + 70 dependencies
- Friction: no working import path from jsonl to fresh dolt. Required custom scripting.

### 5. LETS plugin - smooth

- Auto-discovered via `.claude-plugin/plugin.json` in repo root
- No manual install needed

### 6. Beads plugin - smooth

- Installed from marketplace via `/plugins` command
- Required Claude Code restart to load

### 7. SSH / GitHub - minor blocker

- `git push` failed - SSH key not configured for GitHub
- Error: Host key verification failed
- Fix: configure SSH key or switch remote to HTTPS + `gh auth`

### 8. .lets/config.yaml - smooth

- Created manually from `hooks/config-template.yaml`
- Set language, merge-branch, github flag

### 9. autoCompact - smooth

- `claude config set --global autoCompact false` (run outside session)

### 10. AGENTS.md cleanup - manual

- `bd onboard` generated section with emojis, broken `docs/QUICKSTART.md` ref, duplicated beads plugin rules
- Removed entirely - beads plugin injects its own rules via SessionStart hook

### 11. Repo fingerprint - manual

- `bd doctor` showed fingerprint mismatch (different clone)
- Fix: `bd migrate --update-repo-id`

### 12. .beads/hooks/ - manual

- `bd init` creates git hooks in `.beads/hooks/`
- These are per-developer, should NOT be in git
- Added `.beads/hooks/` to `.gitignore`

## Blockers Summary

| # | Blocker | Severity | Automation potential |
|---|---------|----------|---------------------|
| 1 | bd binary needs GLIBC 2.32+ | HIGH | Document Go workaround in install guide |
| 2 | bd init --from-jsonl doesn't import | HIGH | Report bug to beads, document SQL workaround |
| 3 | Dolt server not autostarted | MEDIUM | Provide systemd unit file in install guide |
| 4 | Empty .beads/dolt blocks reinit | MEDIUM | Document rm -rf workaround |
| 5 | SSH key not configured | LOW | Detect and suggest in install script |
| 6 | bd onboard generates bad AGENTS.md | LOW | Don't run bd onboard, or clean up after |
| 7 | Repo fingerprint mismatch on clone | LOW | Document bd migrate --update-repo-id |

## What Should /lets:install Automate

1. Check prerequisites: gh, bd, dolt, claude code version
2. Create .lets/config.yaml from template (ask language, merge-branch, github flag)
3. Run `claude config set --global autoCompact false`
4. Verify LETS plugin auto-discovered
5. Guide beads plugin installation from marketplace
6. Run `bd init` with proper error handling
7. Run `bd migrate --update-repo-id` if fingerprint mismatch
8. Add .beads/hooks/ to .gitignore if missing
9. Verify `bd doctor` passes

## What Worked Well

- LETS plugin auto-discovery (zero config)
- Beads marketplace install
- config.yaml template
- /lets:start, /lets:commit, /lets:done, /lets:pr workflow
- Session summaries and context recovery

## Session Log

### Session 1 (2026-02-25, ~3 hours)

Installed gh, bd (via Go build due to GLIBC), Dolt. Imported 74 issues + 70 dependencies into fresh Dolt backend. Documented all friction points and blockers. Created PR #3.

Commits:
- c94b2fa chore: Add beads Dolt integration and agent instructions

### Session 2 (2026-02-26, ~2 hours)

PR #3 code review (4 agents: compliance, docs, devops, security). Found 5 docs issues. Removed .beads/hooks/ from git tracking. Confirmed LETS plugin auto-discovered. Installed beads plugin from marketplace.

Commits:
- 447817c chore: Remove beads hooks from git tracking, update gitignore

### Session 3 (2026-02-26, ~30 min)

Created .lets/config.yaml. Disabled autoCompact. Fixed AGENTS.md (removed duplicate beads section). Fixed repo fingerprint. Pushed all commits. Posted PR review. Wrote final documentation.

Commits:
- 3eb1736 docs: Remove duplicate beads section from AGENTS.md

## Total Time

- ~5.5 hours (expected without blockers: 1-2 hours)
- Main time sink: bd/dolt installation and data import issues
