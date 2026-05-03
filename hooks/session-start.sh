#!/bin/bash
# LETS plugin - SessionStart + PreCompact hook
# Injects workflow rules + project config as context
# Note: this hook is intentionally permissive (no `set -e`) so a non-fatal
# error never breaks a session. Migration writes use atomic .tmp + mv.

# Always inject workflow rules
cat "${CLAUDE_PLUGIN_ROOT}/hooks/rules-context.md"

# Inject project context
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)

if [ -n "$PROJECT_ROOT" ]; then
  ENV_FILE="${PROJECT_ROOT}/.lets/.env"
  YAML_FILE="${PROJECT_ROOT}/.lets/config.yaml"

  # One-time auto-migration: yaml -> .env (hardcoded 3-key conversion)
  # Strips inline yaml comments, handles CRLF, case-insensitive boolean.
  # Atomic write (.tmp + mv) prevents partial-write corruption.
  if [ ! -f "$ENV_FILE" ] && [ -f "$YAML_FILE" ]; then
    mkdir -p "$(dirname "$ENV_FILE")"
    LANG_VAL=$(grep '^language:' "$YAML_FILE" \
      | sed -E 's/^language:[[:space:]]*//; s/[[:space:]]*#.*$//' \
      | tr -d '\r')
    BRANCH_VAL=$(grep '^merge-branch:' "$YAML_FILE" \
      | sed -E 's/^merge-branch:[[:space:]]*//; s/[[:space:]]*#.*$//' \
      | tr -d '\r')
    GH_VAL=$(grep '^github:' "$YAML_FILE" \
      | sed -E 's/^github:[[:space:]]*//; s/[[:space:]]*#.*$//' \
      | tr -d '\r')

    [ -z "$LANG_VAL" ] && LANG_VAL="English"
    [ -z "$BRANCH_VAL" ] && BRANCH_VAL="main"
    case "$(echo "$GH_VAL" | tr '[:upper:]' '[:lower:]')" in
      true|yes|1) PR_FLOW="github" ;;
      *)          PR_FLOW="local" ;;
    esac

    TMP_FILE="${ENV_FILE}.tmp.$$"
    {
      echo "# LETS plugin config (auto-migrated from config.yaml on $(date +%Y-%m-%d))"
      echo ""
      echo "LETS_LANGUAGE=$LANG_VAL"
      echo "LETS_MERGE_BRANCH=$BRANCH_VAL"
      echo "LETS_PR_FLOW=$PR_FLOW"
      echo "LETS_TRACKER=beads"
    } > "$TMP_FILE" && mv "$TMP_FILE" "$ENV_FILE"

    # Surface migration to model context (one-time)
    echo ""
    echo "## LETS Notice"
    echo "Auto-migrated .lets/config.yaml -> .lets/.env. Edit .lets/.env now (yaml is no longer read)."
  fi

  echo ""
  echo "## LETS Config"
  echo ""
  echo "LETS_PROJECT_ROOT=$PROJECT_ROOT"

  if [ -f "$ENV_FILE" ]; then
    grep -Ev '^(#|[[:space:]]*$)' "$ENV_FILE"
  fi
fi
