#!/bin/bash
set -e

LETS_DIR="${HOME}/.lets"
LETS_CACHE="${LETS_DIR}/cache"
REPO_RAW="https://raw.githubusercontent.com/nickolay-umbo/lets-plugin-claude/main/scripts/lets"
SETTINGS_FILE="${HOME}/.claude/settings.json"

echo "Installing LETS statusline..."

# Check jq (hard dependency - statusline needs it)
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required but not found." >&2
  echo "Install: brew install jq (macOS) or apt install jq (Linux)" >&2
  exit 1
fi

# Create directories
mkdir -p "$LETS_DIR" "$LETS_CACHE"
chmod 700 "$LETS_CACHE"

# Download statusline
echo "Downloading statusline.sh..."
curl -sfL "${REPO_RAW}/statusline.sh" -o "${LETS_DIR}/statusline.sh" \
  || { echo "Failed to download statusline.sh" >&2; exit 1; }
chmod +x "${LETS_DIR}/statusline.sh"

echo "Installed: ${LETS_DIR}/statusline.sh"

# Configure GLOBAL settings.json
mkdir -p "${HOME}/.claude"

if [ ! -f "$SETTINGS_FILE" ]; then
  # Create new settings.json
  cat > "$SETTINGS_FILE" <<'SETTINGS'
{
  "statusLine": {
    "type": "command",
    "command": "bash -c 'cat | bash ~/.lets/statusline.sh'"
  }
}
SETTINGS
  echo "Created: ${SETTINGS_FILE}"
elif grep -q '\.lets/statusline' "$SETTINGS_FILE" 2>/dev/null; then
  echo "Already configured: ${SETTINGS_FILE}"
else
  # Atomic merge with jq: write to tmp, validate, then move
  jq '.statusLine = {"type": "command", "command": "bash -c '\''cat | bash ~/.lets/statusline.sh'\''"}' \
    "$SETTINGS_FILE" > "${SETTINGS_FILE}.tmp" \
    && jq empty "${SETTINGS_FILE}.tmp" \
    && mv "${SETTINGS_FILE}.tmp" "$SETTINGS_FILE"
  echo "Updated: ${SETTINGS_FILE}"
fi

# Warn about old project-level statusLine configs
_check_old_config() {
  project_root=$(git rev-parse --show-toplevel 2>/dev/null || true)
  if [ -n "$project_root" ] && [ -f "${project_root}/.claude/settings.json" ]; then
    if grep -q 'rev-parse.*statusline\|hooks/lets-statusline' "${project_root}/.claude/settings.json" 2>/dev/null; then
      echo ""
      echo "Warning: old statusLine config found in ${project_root}/.claude/settings.json"
      echo "Remove the statusLine block from that file - global config will be used instead."
    fi
  fi
}
_check_old_config

echo ""
echo "Done! Restart Claude Code to see the new statusline."
