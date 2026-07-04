#!/bin/bash
# scripts/release/bump-version.sh — Phase 1 of release flow.
#
# Edits source-tree version + (stable only) CHANGELOG, runs gates, commits.
# Does NOT push, does NOT tag — those are explicit maintainer steps.
#
# Usage:
#   scripts/release/bump-version.sh 0.5.0
#   scripts/release/bump-version.sh 0.5.0 --dry-run
#   scripts/release/bump-version.sh 0.5.0-rc.1   # prerelease (goreleaser auto-detects)
#
# Pre-conditions:
#   • Run on a non-main branch (e.g. release/0.5.0)
#   • Working tree is clean
#   • Tag v<VERSION> does not yet exist
#   • For STABLE versions: CHANGELOG has [Unreleased] section with content under it
#   • For PRERELEASES (X.Y.Z-rc.N etc.): CHANGELOG is NOT touched; release.yml falls
#     back to [Unreleased] content for GH Release notes

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

if [ $# -lt 1 ]; then
  echo "Usage: $0 <version> [--dry-run]" >&2
  echo "Example: $0 0.5.0" >&2
  exit 1
fi

NEW_VERSION="${1#v}"   # strip optional 'v' prefix
DRY_RUN=false
[ "$2" = "--dry-run" ] && DRY_RUN=true

# Validate semver (with optional prerelease suffix)
if ! [[ $NEW_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
  echo -e "${RED}Error: invalid version '$NEW_VERSION'${NC}" >&2
  echo "Expected: MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-PRERELEASE" >&2
  exit 1
fi

# Detect prerelease (anything with '-' suffix per semver)
IS_PRERELEASE=false
if [[ "$NEW_VERSION" == *-* ]]; then
  IS_PRERELEASE=true
fi

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

# Pre-checks ---------------------------------------------------------------

BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "main" ]; then
  echo -e "${RED}Error: run on a release branch, not main.${NC}" >&2
  echo "  git checkout -b release/$NEW_VERSION" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo -e "${RED}Error: working tree is not clean${NC}" >&2
  git status --short >&2
  exit 1
fi

if git rev-parse "v$NEW_VERSION" >/dev/null 2>&1; then
  echo -e "${RED}Error: tag v$NEW_VERSION already exists${NC}" >&2
  exit 1
fi

# Validate CHANGELOG has [Unreleased] section (always required as anchor)
if ! grep -q '^## \[Unreleased\]' CHANGELOG.md; then
  echo -e "${RED}Error: CHANGELOG.md missing [Unreleased] section${NC}" >&2
  exit 1
fi

# For STABLE versions, [Unreleased] must have content (will be promoted to [X.Y.Z]).
# For PRERELEASES we don't touch CHANGELOG, so empty [Unreleased] is fine.
if ! $IS_PRERELEASE; then
  UNRELEASED_CONTENT=$(awk '/^## \[Unreleased\]/{flag=1; next} /^## \[/{flag=0} flag' CHANGELOG.md | tr -d '[:space:]')
  if [ -z "$UNRELEASED_CONTENT" ]; then
    echo -e "${RED}Error: CHANGELOG [Unreleased] is empty — nothing to release as stable${NC}" >&2
    exit 1
  fi
fi

CURRENT=$(jq -r .version plugins/lets/.claude-plugin/plugin.json)
echo -e "${YELLOW}Bumping: $CURRENT → $NEW_VERSION${NC}"
[ "$DRY_RUN" = true ] && echo "  (dry-run mode: no commits will be made)"

# Edits --------------------------------------------------------------------

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

# 1. plugin.json
echo "  • plugins/lets/.claude-plugin/plugin.json"
jq --indent 2 --arg v "$NEW_VERSION" '.version = $v' \
   plugins/lets/.claude-plugin/plugin.json > "$TMP"
mv "$TMP" plugins/lets/.claude-plugin/plugin.json
TMP=$(mktemp)

# 2. marketplace.json
echo "  • .claude-plugin/marketplace.json"
jq --indent 2 --arg v "$NEW_VERSION" \
   '(.plugins[] | select(.name=="lets") | .version) |= $v' \
   .claude-plugin/marketplace.json > "$TMP"
mv "$TMP" .claude-plugin/marketplace.json
TMP=$(mktemp)

# 3. lets-rules.md + shipped tracker adapter frontmatter.
#    Shipped adapters (tracker-beads/planfix-mcp/none) are drift-tracked like
#    lets-rules.md, so their version: bumps at the same ceremony. tracker-TEMPLATE.md
#    and *.board.md deliberately pin 0.0.0 (author-facing / user-owned) - excluded.
for rules_file in plugins/lets/rules/lets-rules.md \
                  plugins/lets/rules/tracker-beads.md \
                  plugins/lets/rules/tracker-planfix-mcp.md \
                  plugins/lets/rules/tracker-none.md; do
  echo "  • $rules_file (frontmatter)"
  awk -v new="$NEW_VERSION" '
    BEGIN { c=0 }
    /^---$/ { c++; print; next }
    c==1 && /^version:/ { print "version: " new; next }
    { print }
  ' "$rules_file" > "$TMP"
  mv "$TMP" "$rules_file"
  TMP=$(mktemp)
done

# 4. CHANGELOG: STABLE only — rename [Unreleased] → [X.Y.Z] - DATE + bottom links.
#    For prereleases, CHANGELOG is left intact; release.yml uses [Unreleased] as notes.
if $IS_PRERELEASE; then
  echo "  • CHANGELOG.md (skipped — prerelease uses [Unreleased] as notes via release.yml fallback)"
else
  echo "  • CHANGELOG.md (heading + bottom links)"
  DATE=$(date +%Y-%m-%d)
  # Last STABLE tag (filter prereleases — '-' suffix). Fallback v0.1.0 (initial release).
  PREV_TAG=$(git tag --list --sort=-v:refname | grep -v -- '-' | head -1 || true)
  [ -z "$PREV_TAG" ] && PREV_TAG="v0.1.0"
  REPO_URL=$(jq -r .repository plugins/lets/.claude-plugin/plugin.json)

  # Heading: insert empty [Unreleased] above existing one (which becomes [X.Y.Z] - DATE)
  awk -v new="$NEW_VERSION" -v date="$DATE" '
    /^## \[Unreleased\]$/ && !done {
      print "## [Unreleased]"
      print ""
      print "## [" new "] - " date
      done=1
      next
    }
    { print }
  ' CHANGELOG.md > "$TMP"
  mv "$TMP" CHANGELOG.md
  TMP=$(mktemp)

  # Bottom links: update [Unreleased] link, insert [X.Y.Z] link above
  awk -v new="$NEW_VERSION" -v prev="$PREV_TAG" -v repo="$REPO_URL" '
    /^\[Unreleased\]:/ {
      print "[Unreleased]: " repo "/compare/v" new "...HEAD"
      print "[" new "]: " repo "/compare/" prev "...v" new
      next
    }
    { print }
  ' CHANGELOG.md > "$TMP"
  mv "$TMP" CHANGELOG.md
fi

# Gates --------------------------------------------------------------------

echo ""
echo -e "${YELLOW}Running gates...${NC}"

echo "  • make test"
make test || { echo -e "${RED}FAILED: tests${NC}" >&2; exit 1; }

echo "  • make build"
make build || { echo -e "${RED}FAILED: build${NC}" >&2; exit 1; }

echo "  • bash scripts/release/verify-versions.sh"
bash scripts/release/verify-versions.sh || {
  echo -e "${RED}FAILED: verify-versions reports drift after edit (script bug)${NC}" >&2
  exit 1
}

# Commit -------------------------------------------------------------------

if [ "$DRY_RUN" = true ]; then
  echo ""
  echo -e "${YELLOW}--dry-run: changes are in working tree, NOT committed.${NC}"
  echo "  Review:    git diff"
  echo "  Discard:   git restore plugins/lets/.claude-plugin/plugin.json \\"
  echo "                          .claude-plugin/marketplace.json \\"
  echo "                          plugins/lets/rules/lets-rules.md \\"
  echo "                          CHANGELOG.md"
  exit 0
fi

if $IS_PRERELEASE; then
  git add plugins/lets/.claude-plugin/plugin.json \
          .claude-plugin/marketplace.json \
          plugins/lets/rules/lets-rules.md
else
  git add plugins/lets/.claude-plugin/plugin.json \
          .claude-plugin/marketplace.json \
          plugins/lets/rules/lets-rules.md \
          CHANGELOG.md
fi
git commit -m "chore: release v$NEW_VERSION"

echo ""
echo -e "${GREEN}✓ Bumped to v$NEW_VERSION on branch $BRANCH${NC}"
echo ""
echo "Next steps:"
echo "  1. Push branch:    git push -u origin $BRANCH"
echo "  2. Open PR:        gh pr create --title 'chore: release v$NEW_VERSION'"
echo "  3. Merge PR        (CI runs verify-versions.yml on it)"
echo "  4. Tag and push:   git checkout main && git pull && make release-tag VERSION=$NEW_VERSION"
echo "  5. Watch:          gh run watch    (release.yml triggered by tag push)"
