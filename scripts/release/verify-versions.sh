#!/bin/bash
# scripts/release/verify-versions.sh — Verify source-tree version coherence.
#
# Reads version from 3 source-tree files and asserts they agree.
# With --against-tag, also requires HEAD to be tagged and tag to match source-tree.
#
# Used in:
#   • scripts/release/bump-version.sh   (post-edit gate, Phase 1)
#   • .github/workflows/verify-versions.yml  (PR + tag-push guard)
#   • .github/workflows/release.yml          (pre-goreleaser guard)
#
# Exit codes:
#   0  All sources agree (optional: also tag matches)
#   1  Drift detected
#   2  Error (file missing, not in repo, --against-tag without tagged HEAD)

set -e

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "ERROR: not in a git repository" >&2
  exit 2
}
cd "$ROOT"

# Frontmatter-versioned rules files: lets-rules.md + the shipped tracker
# adapters (drift-tracked alike; bumped together by bump-version.sh).
# tracker-TEMPLATE.md and *.board.md pin 0.0.0 by design - excluded.
RULES_FILES="plugins/lets/rules/lets-rules.md
plugins/lets/rules/tracker-beads.md
plugins/lets/rules/tracker-planfix-mcp.md
plugins/lets/rules/tracker-none.md"

# Required files exist
for f in plugins/lets/.claude-plugin/plugin.json \
         .claude-plugin/marketplace.json \
         $RULES_FILES; do
  [ -f "$f" ] || { echo "ERROR: file missing: $f" >&2; exit 2; }
done

# Read source-tree versions
PLUGIN_JSON=$(jq -r .version plugins/lets/.claude-plugin/plugin.json)
MARKET_JSON=$(jq -r '.plugins[] | select(.name=="lets") | .version' .claude-plugin/marketplace.json)

read_frontmatter_version() {
  awk '
    /^---$/ { c++; next }
    c==1 && /^version:/ { sub(/^version:[ \t]*/, ""); sub(/[ \t]*$/, ""); print; exit }
  ' "$1"
}
RULES_MD=$(read_frontmatter_version plugins/lets/rules/lets-rules.md)

# Sanity: all must be non-empty
for v in "$PLUGIN_JSON" "$MARKET_JSON" "$RULES_MD"; do
  if [ -z "$v" ] || [ "$v" = "null" ]; then
    echo "ERROR: empty or missing version field" >&2
    printf "  %-30s '%s'\n" "plugin.json:"               "$PLUGIN_JSON" >&2
    printf "  %-30s '%s'\n" "marketplace.json:"          "$MARKET_JSON" >&2
    printf "  %-30s '%s'\n" "lets-rules.md frontmatter:" "$RULES_MD"    >&2
    exit 2
  fi
done

# Compare source-tree versions (plugin.json is the reference)
ok=true
[ "$PLUGIN_JSON" = "$MARKET_JSON" ] || ok=false
[ "$MARKET_JSON" = "$RULES_MD" ]    || ok=false
for f in $RULES_FILES; do
  fv=$(read_frontmatter_version "$f")
  if [ "$fv" != "$PLUGIN_JSON" ]; then
    ok=false
    printf "  %-45s %s (want %s)\n" "$f frontmatter:" "${fv:-<missing>}" "$PLUGIN_JSON"
  fi
done

if ! $ok; then
  echo "Version drift detected:"
  printf "  %-30s %s\n" "plugin.json:"               "$PLUGIN_JSON"
  printf "  %-30s %s\n" "marketplace.json:"          "$MARKET_JSON"
  printf "  %-30s %s\n" "lets-rules.md frontmatter:" "$RULES_MD"
  exit 1
fi

# Optional tag check (used in tag-push CI + release.yml)
if [ "$1" = "--against-tag" ]; then
  TAG=$(git describe --tags --exact-match 2>/dev/null) || {
    echo "ERROR: HEAD is not tagged (--against-tag requires tagged commit)" >&2
    exit 2
  }
  TAG_VER="${TAG#v}"
  if [ "$TAG_VER" != "$PLUGIN_JSON" ]; then
    echo "Drift between tag and source-tree:"
    printf "  %-30s %s\n" "git tag:"     "$TAG_VER"
    printf "  %-30s %s\n" "source-tree:" "$PLUGIN_JSON"
    exit 1
  fi
  echo "✓ Tag and source-tree match: v$PLUGIN_JSON"
  exit 0
fi

echo "✓ Source-tree versions match: $PLUGIN_JSON"
