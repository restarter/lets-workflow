# Releasing LETS

How to cut a new release of `lets` (CLI binary + Claude Code plugin).

## Versioning

Single semver across **4 source-tree concerns**:

| Concern | File | Edited by |
|---|---|---|
| Plugin manifest | `plugins/lets/.claude-plugin/plugin.json::version` | `bump-version.sh` |
| Marketplace listing | `.claude-plugin/marketplace.json::plugins[name=lets].version` | `bump-version.sh` |
| Workflow rules | `plugins/lets/rules/lets-rules.md` frontmatter `version:` | `bump-version.sh` |
| CLI binary | `cli/internal/version/version.Version` | `make build` (ldflags from git tag) |

Drift between any of the first three fails CI (`verify-versions.yml`). The CLI binary version derives from `git describe --tags --exact-match` at build time — automatic.

Pre-release tags use semver suffix (`v0.6.0-rc.1`, `v0.6.0-beta.1`). goreleaser auto-flags these as prerelease on GitHub Releases.

## Flow at a glance

```
Phase 1: bump        →  Phase 2: review     →  Phase 3: tag           →  Phase 4: distribute
release/X.Y.Z branch    PR to main             main + manual tag push     GHA runs goreleaser
make bump VERSION=X     verify-versions.yml    make release-tag           release.yml: verify + build + upload
PR with bump commit     reviewer approval                                 5 archives + checksums on GH Releases
```

## Phase 1 — Bump

On a clean main, branch off and run the bump:

```bash
git checkout main && git pull
git checkout -b release/0.6.0
make bump VERSION=0.6.0
```

What happens (stable release):
- `scripts/release/bump-version.sh` edits the version-source files — `plugin.json`, `marketplace.json`, and the frontmatter-versioned rules files (`lets-rules.md` + the three shipped tracker adapters `tracker-{beads,planfix-mcp,none}.md`) — plus `CHANGELOG.md` (renames `[Unreleased]` → `[0.6.0] - DATE` with all accumulated entries; inserts fresh empty `[Unreleased]` above; updates bottom-of-file links)
- Runs gates: `make test`, `make build`, `verify-versions`
- Commits "chore: release v0.6.0"

The script commits only after all gates pass. On a gate failure the version + CHANGELOG edits stay in the working tree, so a plain re-run dies at the clean-tree pre-check — `git restore` the edited files (`plugin.json`, `marketplace.json`, `lets-rules.md` + the tracker adapters, `CHANGELOG.md`) first, then fix and re-run.

For **prereleases** (`X.Y.Z-rc.N`, `-beta.N`, `-alpha.N`) the flow is identical EXCEPT:
- bump-version.sh edits the version-source files (`plugin.json`, `marketplace.json`, `lets-rules.md` + the 3 tracker adapters) but leaves **CHANGELOG intact** — full content stays under `[Unreleased]`
- release.yml falls back to `[Unreleased]` content for the GH Release page notes
- See "Validating before the real release" below for the canonical rc → final ceremony.

For a dry-run that stages edits without committing:

```bash
make bump VERSION=0.6.0 DRY_RUN=1
```

## Phase 2 — Review

```bash
git push -u origin release/0.6.0
gh pr create --title 'chore: release v0.6.0'
```

The PR triggers `verify-versions.yml`. After approval, merge.

## Phase 3 — Tag

After merge:

```bash
git checkout main && git pull
make release-tag VERSION=0.6.0
```

This tags the merge commit and pushes the tag. The push triggers `release.yml`.

## Phase 4 — Distribute (automated)

`release.yml` runs:
1. **guard** — `verify-versions.sh --against-tag`
2. **release** — extracts `[0.6.0]` section from `CHANGELOG.md`, runs goreleaser

goreleaser builds 5 archives (darwin amd64/arm64, linux amd64/arm64, windows amd64), uploads them with `lets_0.6.0_checksums.txt` to a GitHub Release page populated with the CHANGELOG section.

Watch progress:

```bash
gh run watch
```

## Validating before the real release

To exercise the full pipeline without burning the final tag, cut a prerelease (semver suffix `-rc.N`, `-beta.N`, etc.). Prereleases are deliberately lightweight:

- bump-version.sh **does NOT touch CHANGELOG.md** for prereleases — only source-tree version sources (plugin.json, marketplace.json, lets-rules.md + the 3 tracker adapters' frontmatter)
- release.yml falls back to `[Unreleased]` content for the GH Release page notes
- goreleaser auto-flags `-rc.N` as prerelease via `release.prerelease: auto`

```bash
git checkout main && git pull
git checkout -b release/0.6.0-rc.1
make bump VERSION=0.6.0-rc.1
# → source-tree version files changed (plugin.json, marketplace.json, lets-rules.md + 3 tracker adapters; CHANGELOG untouched)
git push -u origin release/0.6.0-rc.1
gh pr create
# After merge:
git checkout main && git pull
make release-tag VERSION=0.6.0-rc.1
# release.yml runs; GH Releases gets a prerelease entry whose notes are pulled from [Unreleased].
```

When ready for the final cut:

```bash
git checkout main && git pull   # source-tree is now at 0.6.0-rc.1
git checkout -b release/0.6.0
make bump VERSION=0.6.0
# → 4 files changed: source-tree files bumped to 0.6.0 + CHANGELOG promotes
#   [Unreleased] → [0.6.0] - DATE with full accumulated content + bottom links updated.
#   Bottom-link "previous" tag is the last STABLE tag (-rc.N is filtered out),
#   so [0.6.0]: compare/v0.5.0...v0.6.0 (NOT compare from rc.1).
```

Multiple rc cuts are supported — each `make bump VERSION=0.6.0-rc.N` only touches source-tree, and the GH Release page for each rc shows the same `[Unreleased]` snapshot.

Before the **first ever** tag is pushed (after merging the PR that adds this release infrastructure), run a local goreleaser snapshot to verify the build matrix end-to-end:

```bash
brew install goreleaser/tap/goreleaser   # or equivalent for your OS
goreleaser check
goreleaser release --snapshot --clean --skip=publish,sign
ls dist/lets_*                            # 5 archives + checksums.txt
rm -rf dist/                              # cleanup
```

If `goreleaser check` reports schema errors, fix `.goreleaser.yml` before tagging anything. Catching syntax issues here is cheap; catching them at tag-push burns a tag.

## Recovery

> **Release immutability + tag protection are ON.** Once a release is published its assets and tag can't be modified, and `v*` tags are protected by the `Protect release tags` ruleset (deleting/moving one needs the Repository-admin bypass). When in doubt, **roll forward** — bump to the next patch (or `-rc.N+1` when validating) rather than re-use a burnt tag; a partial release may already have been consumed.

**Bump committed but not yet merged**: discard branch, redo. No harm done.

**Tag pushed but `release.yml` fails before goreleaser publishes** (nothing on the Releases page yet):
- Inspect failure, fix the underlying issue (likely goreleaser config or CHANGELOG extraction).
- As a Repository admin, delete the failed tag — `git push --delete origin v0.6.0 && git tag -d v0.6.0` — then re-run `make release-tag VERSION=0.6.0`. (The tag ruleset's admin bypass allows the delete.)
- Don't want to touch the tag? Bump to `v0.6.1` (or `v0.6.0-rc.2`) and tag that instead.

**`release.yml` published but the assets are wrong** (tag is now immutable):
- You can't overwrite it — immutability blocks modifying a published release. Bump to the next patch (`v0.6.1`), fix, ship that. Optionally `gh release delete v0.6.0 --yes` to hide the bad one; the immutable tag stays put.

**`verify-versions` finds drift on main** (shouldn't happen if PR-time check passed): treat as a bug — investigate which file is out of sync, fix on a hotfix branch.

## Rationale

**Why two phases** — bump is reviewable (can revert), tag is reproducible (same tag → same binaries). Mixing them means tag commits to changes that haven't been reviewed.

**Why bash for orchestration** — bump-version.sh + verify-versions.sh are file-edits + gates, natural for bash + jq + awk. No Go binary involvement; CI doesn't need `setup-go` for verify.

**Why goreleaser** — single declarative config builds 5 platforms in parallel, handles archives + checksums + GH Release creation + prerelease detection (semver suffix `-rc.1` etc.). Battle-tested in beads and similar Go CLIs.

**Prereleases skip CHANGELOG mutation** — rc/beta/alpha tags exist as validation snapshots; the full release entry is reserved for the stable tag. release.yml synthesizes prerelease notes from `[Unreleased]`. PREV_TAG (used to compute compare-link bottom of CHANGELOG) filters prereleases so stable releases compare against the previous **stable** tag.

## Out of scope (future)

- Mac code signing / notarization (Gatekeeper warning workaround: `xattr -cr /usr/local/bin/lets`)
- Windows Authenticode signing
- Homebrew tap auto-update (`lets-odg13`)
- Scoop / winget manifests (`lets-hdrdr.1`)
