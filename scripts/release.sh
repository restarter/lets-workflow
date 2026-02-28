#!/bin/bash
set -e

# Simple release script for LETS plugin
# Usage: ./scripts/release.sh 0.3.0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.3.0"
    exit 1
fi

NEW_VERSION=$1

# Strip 'v' prefix if present
NEW_VERSION="${NEW_VERSION#v}"

# Validate semver
if ! [[ $NEW_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Invalid version format '$NEW_VERSION'${NC}"
    echo "Expected: MAJOR.MINOR.PATCH (e.g., 0.3.0)"
    exit 1
fi

# Must run from repo root
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

# Must be on main
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ]; then
    echo -e "${RED}Error: Must be on main branch (currently on $BRANCH)${NC}"
    exit 1
fi

# Must be clean
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${RED}Error: Working tree is not clean${NC}"
    git status --short
    exit 1
fi

# Get current version from plugin.json
CURRENT_VERSION=$(python3 -c "import json; print(json.load(open('.claude-plugin/plugin.json'))['version'])")
echo -e "${YELLOW}Bumping: $CURRENT_VERSION -> $NEW_VERSION${NC}"

# Check tag doesn't exist
if git rev-parse "v$NEW_VERSION" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag v$NEW_VERSION already exists${NC}"
    exit 1
fi

# Check CHANGELOG has the version
if ! grep -q "\[$NEW_VERSION\]" CHANGELOG.md 2>/dev/null; then
    echo -e "${RED}Error: CHANGELOG.md missing entry for [$NEW_VERSION]${NC}"
    echo "Add a ## [$NEW_VERSION] section to CHANGELOG.md first"
    exit 1
fi

# Update plugin.json
echo "  Updating .claude-plugin/plugin.json"
python3 -c "
import json
with open('.claude-plugin/plugin.json', 'r') as f:
    data = json.load(f)
data['version'] = '$NEW_VERSION'
with open('.claude-plugin/plugin.json', 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
"

# Commit, tag, push
echo "  Committing..."
git add .claude-plugin/plugin.json
git commit -m "chore: Bump version to $NEW_VERSION"

echo "  Tagging v$NEW_VERSION..."
git tag "v$NEW_VERSION"

echo "  Pushing..."
git push origin main
git push origin "v$NEW_VERSION"

# Create GitHub release from CHANGELOG
echo "  Creating GitHub release..."
# Extract changelog section for this version
NOTES=$(sed -n "/^## \[$NEW_VERSION\]/,/^## \[/p" CHANGELOG.md | sed '$d')
gh release create "v$NEW_VERSION" --title "v$NEW_VERSION" --notes "$NOTES"

echo ""
echo -e "${GREEN}Released v$NEW_VERSION${NC}"
