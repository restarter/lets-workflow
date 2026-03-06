#!/bin/bash
# Per-project LETS setup script
# Run from project root: bash path/to/install.sh
# Or via plugin: /lets:install
set -e

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$PROJECT_ROOT" ]; then
  echo "Error: not in a git repository." >&2
  exit 1
fi

LETS_DIR="${PROJECT_ROOT}/.lets"
LETS_CACHE="${LETS_DIR}/cache"
SETTINGS="${PROJECT_ROOT}/.claude/settings.json"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Setting up LETS in ${PROJECT_ROOT}..."

# Check jq (hard dependency - statusline needs it)
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required but not found." >&2
  echo "Install: brew install jq (macOS) or apt install jq (Linux)" >&2
  exit 1
fi

# Create .lets/ structure
mkdir -p "$LETS_DIR/sessions" "$LETS_DIR/reviews" "$LETS_DIR/plans" "$LETS_DIR/execution" "$LETS_CACHE"
chmod 700 "$LETS_CACHE"

# Add .lets/ to .gitignore
if ! grep -q '^\.lets/' "${PROJECT_ROOT}/.gitignore" 2>/dev/null; then
  echo '.lets/' >> "${PROJECT_ROOT}/.gitignore"
  echo "Added .lets/ to .gitignore"
fi

# Copy statusline
if [ -f "${SCRIPT_DIR}/statusline.sh" ]; then
  cp "${SCRIPT_DIR}/statusline.sh" "${LETS_DIR}/statusline.sh"
  chmod +x "${LETS_DIR}/statusline.sh"
  echo "Installed: ${LETS_DIR}/statusline.sh"
else
  echo "Warning: statusline.sh not found at ${SCRIPT_DIR}/statusline.sh" >&2
fi

# Configure .claude/settings.json
mkdir -p "${PROJECT_ROOT}/.claude"
STATUSLINE_CMD='bash -c '\''cat | bash "$(git rev-parse --show-toplevel 2>/dev/null)/.lets/statusline.sh" 2>/dev/null'\'''

if [ ! -f "$SETTINGS" ]; then
  printf '{\n  "statusLine": {\n    "type": "command",\n    "command": "%s"\n  }\n}\n' "$STATUSLINE_CMD" > "$SETTINGS"
  echo "Created: ${SETTINGS}"
elif grep -q 'statusLine' "$SETTINGS" 2>/dev/null; then
  echo "Already configured: ${SETTINGS}"
else
  jq --arg cmd "$STATUSLINE_CMD" '.statusLine = {"type": "command", "command": $cmd}' \
    "$SETTINGS" > "${SETTINGS}.tmp" \
    && jq empty "${SETTINGS}.tmp" \
    && mv "${SETTINGS}.tmp" "$SETTINGS" \
    || { rm -f "${SETTINGS}.tmp"; echo "Error: failed to update ${SETTINGS} - is it valid JSON?" >&2; exit 1; }
  echo "Updated: ${SETTINGS}"
fi

echo ""
echo "Done! Restart Claude Code to see the statusline."
