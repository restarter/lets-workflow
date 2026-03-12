#!/bin/bash
# Per-project LETS initialization
# Usage: init.sh [--language LANG] [--merge-branch BRANCH] [--github] [--skip-beads] [--quiet] [--help]
set -e

# Defaults
LANGUAGE="English"
MERGE_BRANCH="main"
GITHUB="false"
SKIP_BEADS=""
QUIET=""

# Parse arguments
while [ $# -gt 0 ]; do
  case "$1" in
    --language)    LANGUAGE="$2"; shift 2 ;;
    --merge-branch) MERGE_BRANCH="$2"; shift 2 ;;
    --github)      GITHUB="true"; shift ;;
    --skip-beads)  SKIP_BEADS="true"; shift ;;
    --quiet)       QUIET="true"; shift ;;
    --help)
      echo "Usage: init.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --language LANG        Response language (default: English)"
      echo "  --merge-branch BRANCH  Target branch for merges (default: main)"
      echo "  --github               Enable GitHub PR workflow (default: false)"
      echo "  --skip-beads           Skip beads initialization"
      echo "  --quiet                Suppress informational output"
      echo "  --help                 Show this help"
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

info() { [ -z "$QUIET" ] && echo "$1" || true; }
warn() { echo "$1" >&2; }

PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$PROJECT_ROOT" ]; then
  warn "Error: not in a git repository."
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  warn "Error: jq is required but not found."
  warn "Install: brew install jq (macOS) or apt install jq (Linux)"
  exit 1
fi

LETS_DIR="${PROJECT_ROOT}/.lets"
LETS_CACHE="${LETS_DIR}/cache"
SETTINGS="${PROJECT_ROOT}/.claude/settings.json"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PARTIAL=0

info "[LETS] Initializing project in ${PROJECT_ROOT}..."

# Create .lets/ structure
mkdir -p "$LETS_DIR/sessions" "$LETS_DIR/reviews" "$LETS_DIR/plans" "$LETS_DIR/execution" "$LETS_CACHE"
chmod 700 "$LETS_CACHE"
info "[ok]   .lets/ structure"

# Ensure file ends with newline before appending
if [ -f "${PROJECT_ROOT}/.gitignore" ] && [ -n "$(tail -c 1 "${PROJECT_ROOT}/.gitignore" 2>/dev/null)" ]; then
  echo '' >> "${PROJECT_ROOT}/.gitignore"
fi

GITIGNORE_CHANGED=""
for entry in '.lets/' '.beads/' '.worktrees/'; do
  if ! grep -q "^${entry}$" "${PROJECT_ROOT}/.gitignore" 2>/dev/null; then
    echo "$entry" >> "${PROJECT_ROOT}/.gitignore"
    GITIGNORE_CHANGED="true"
  fi
done

if [ -n "$GITIGNORE_CHANGED" ]; then
  info "[ok]   .gitignore entries"
else
  info "[skip] .gitignore (already configured)"
fi

# Copy statusline
if [ -f "${SCRIPT_DIR}/statusline.sh" ]; then
  if cmp -s "${SCRIPT_DIR}/statusline.sh" "${LETS_DIR}/statusline.sh" 2>/dev/null; then
    info "[skip] statusline (unchanged)"
  else
    cp "${SCRIPT_DIR}/statusline.sh" "${LETS_DIR}/statusline.sh"
    chmod +x "${LETS_DIR}/statusline.sh"
    info "[ok]   statusline"
  fi
else
  warn "[warn] statusline.sh not found at ${SCRIPT_DIR}/statusline.sh"
fi

# Configure .claude/settings.json
mkdir -p "${PROJECT_ROOT}/.claude"
STATUSLINE_CMD='bash -c '\''cat | bash $(git rev-parse --show-toplevel 2>/dev/null)/.lets/statusline.sh 2>/dev/null'\'''

if [ ! -f "$SETTINGS" ]; then
  printf '{\n  "statusLine": {\n    "type": "command",\n    "command": "%s"\n  }\n}\n' "$STATUSLINE_CMD" > "$SETTINGS"
  info "[ok]   .claude/settings.json (created)"
elif grep -q 'statusLine' "$SETTINGS" 2>/dev/null; then
  info "[skip] .claude/settings.json (statusLine exists)"
else
  jq --arg cmd "$STATUSLINE_CMD" '.statusLine = {"type": "command", "command": $cmd}' \
    "$SETTINGS" > "${SETTINGS}.tmp" \
    && jq empty "${SETTINGS}.tmp" \
    && mv "${SETTINGS}.tmp" "$SETTINGS" \
    || { rm -f "${SETTINGS}.tmp"; warn "[err]  Failed to update ${SETTINGS}"; exit 1; }
  info "[ok]   .claude/settings.json (updated)"
fi

# Create config.yaml
CONFIG="${LETS_DIR}/config.yaml"
if [ -f "$CONFIG" ]; then
  info "[skip] config.yaml (exists)"
else
  # Read template from hooks/ (relative to plugin root)
  TEMPLATE="${SCRIPT_DIR}/../../hooks/config-template.yaml"
  if [ -f "$TEMPLATE" ]; then
    # Substitute values in template
    sed -e "s|^language:.*|language: ${LANGUAGE}|" \
        -e "s|^merge-branch:.*|merge-branch: ${MERGE_BRANCH}|" \
        -e "s|^github:.*|github: ${GITHUB}|" \
        "$TEMPLATE" > "$CONFIG"
  else
    # Fallback: inline template (manual invocation without plugin context)
    cat > "$CONFIG" <<YAML
# LETS plugin config

language: ${LANGUAGE}      # Response language (English/Ukrainian/Italian/etc)
merge-branch: ${MERGE_BRANCH}    # Target branch for merges and PR base
github: ${GITHUB}         # GitHub PR workflow: PR on done instead of local merge (requires gh CLI)
YAML
  fi
  info "[ok]   config.yaml (language: ${LANGUAGE}, merge-branch: ${MERGE_BRANCH}, github: ${GITHUB})"
fi

# Initialize beads
if [ -n "$SKIP_BEADS" ]; then
  info "[skip] beads (--skip-beads)"
elif ! command -v bd >/dev/null 2>&1; then
  warn "[warn] bd (beads) not found. Install beads plugin, then re-run."
  PARTIAL=1
else
  if [ -d "${PROJECT_ROOT}/.beads/dolt" ] && [ -n "$(ls -A "${PROJECT_ROOT}/.beads/dolt" 2>/dev/null)" ]; then
    info "[skip] beads (already initialized)"
  else
    # Clean up empty .beads/dolt/ if present (known blocker for bd init)
    if [ -d "${PROJECT_ROOT}/.beads/dolt" ] && [ -z "$(ls -A "${PROJECT_ROOT}/.beads/dolt" 2>/dev/null)" ]; then
      rm -rf "${PROJECT_ROOT}/.beads"
    fi
    if bd init; then
      bd config set hash_length 5 2>/dev/null || true
      info "[ok]   beads initialized"
    else
      warn "[warn] bd init failed. Beads may need manual setup."
      PARTIAL=1
    fi
  fi

  # Gitignore .beads/hooks/ if not present
  if [ -d "${PROJECT_ROOT}/.beads/hooks" ]; then
    if ! grep -q "^\.beads/hooks/$" "${PROJECT_ROOT}/.gitignore" 2>/dev/null; then
      echo '.beads/hooks/' >> "${PROJECT_ROOT}/.gitignore"
    fi
  fi
fi

echo ""
if [ "$PARTIAL" -eq 1 ]; then
  info "[LETS] Done (partial - beads needs attention)."
  exit 2
else
  info "[LETS] Done. Restart Claude Code to see the statusline."
  exit 0
fi
