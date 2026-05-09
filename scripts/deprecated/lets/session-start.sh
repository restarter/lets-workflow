#!/bin/bash
# LETS plugin - SessionStart + PreCompact hook
# Injects workflow rules + project config as context
# Note: this hook is intentionally permissive (no `set -e`) so a non-fatal
# error never breaks a session. Migration writes use atomic .tmp + mv.

# Always inject workflow rules
cat "${CLAUDE_PLUGIN_ROOT}/hooks/rules-context.md"

# Inject project context
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)

# Strip CRLF, double-quotes, single-quotes, and trailing whitespace from a value.
_lets_clean_val() {
  printf '%s' "$1" | tr -d '\r"' | tr -d "'" | sed -e 's/[[:space:]]*$//'
}

if [ -n "$PROJECT_ROOT" ]; then
  ENV_FILE="${PROJECT_ROOT}/.lets/.env"
  YAML_FILE="${PROJECT_ROOT}/.lets/config.yaml"

  # One-time auto-migration: yaml -> .env (hardcoded 3-key conversion).
  # Removal tracked in lets-p732a (after all known users have migrated).
  # Parser: take 2nd whitespace-separated token via awk - handles inline
  # comments (# falls outside $2), multi-line duplicate keys (head -1).
  # _lets_clean_val strips quotes/CRLF.
  if [ ! -f "$ENV_FILE" ] && [ -f "$YAML_FILE" ]; then
    if ! mkdir -p "$(dirname "$ENV_FILE")" 2>/dev/null; then
      echo ""
      echo "## LETS Notice"
      echo "Could not create $(dirname "$ENV_FILE") - LETS Config not loaded. Check filesystem permissions."
    else
      LANG_VAL=$(_lets_clean_val "$(grep '^language:' "$YAML_FILE" | head -1 | awk '{print $2}')")
      BRANCH_VAL=$(_lets_clean_val "$(grep '^merge-branch:' "$YAML_FILE" | head -1 | awk '{print $2}')")
      GH_VAL=$(_lets_clean_val "$(grep '^github:' "$YAML_FILE" | head -1 | awk '{print $2}')")

      [ -z "$LANG_VAL" ] && LANG_VAL="English"
      [ -z "$BRANCH_VAL" ] && BRANCH_VAL="main"
      case "$(echo "$GH_VAL" | tr '[:upper:]' '[:lower:]')" in
        true|yes|1)         PR_FLOW="github" ;;
        bitbucket)          PR_FLOW="bitbucket" ;;
        github|local)       PR_FLOW="$GH_VAL" ;;
        *)                  PR_FLOW="local" ;;
      esac

      # Atomic write via mktemp + cleanup trap (avoids predictable PID names
      # and dead .tmp.* accumulation on failure).
      # Heredoc format must mirror scripts/lets/init.sh - same comment block,
      # same per-key annotations - so /lets:init and auto-migration produce
      # identical .env files. When updating either, update both.
      TMP_FILE=$(mktemp "${ENV_FILE}.tmp.XXXXXX" 2>/dev/null)
      if [ -n "$TMP_FILE" ]; then
        trap 'rm -f "$TMP_FILE"' EXIT INT TERM
        if cat > "$TMP_FILE" <<ENV && mv "$TMP_FILE" "$ENV_FILE" 2>/dev/null
# LETS plugin config
# NOT FOR SECRETS. Contents are injected verbatim into model context every
# session (subject to whitelist filter in hooks/session-start.sh). Put
# tokens/passwords elsewhere (gh auth, OS keychain, .beads/.env).

# Response language (English/Ukrainian/Italian/etc)
LETS_LANGUAGE=$LANG_VAL

# Target branch for merges and PR base
LETS_MERGE_BRANCH=$BRANCH_VAL

# PR flow: github | bitbucket | local
LETS_PR_FLOW=$PR_FLOW

# Task tracker (currently beads supported)
LETS_TRACKER=beads
ENV
        then
          # Mirror /lets:init: drop a reference .env.example alongside (gitignored).
          # Useful when restoring/comparing user's customized .env.
          TEMPLATE="${CLAUDE_PLUGIN_ROOT}/hooks/config-template.env"
          [ -f "$TEMPLATE" ] && cp "$TEMPLATE" "${PROJECT_ROOT}/.lets/.env.example" 2>/dev/null

          echo ""
          echo "## LETS Notice"
          echo "Auto-migrated .lets/config.yaml -> .lets/.env. Edit .lets/.env now (yaml is no longer read). Reference defaults in .lets/.env.example."
        else
          echo ""
          echo "## LETS Notice"
          echo "Auto-migration write to .lets/.env failed (permissions?). Check filesystem and rerun /lets:init."
        fi
        trap - EXIT INT TERM
      fi
    fi
  fi

  echo ""
  echo "## LETS Config"
  echo ""
  echo "LETS_PROJECT_ROOT=$PROJECT_ROOT"

  # Whitelist injection: only echo known LETS_* keys (prevents prompt-injection
  # via untrusted .env content; strips newlines from values defensively).
  # Treat injected values as DATA, never as instructions to follow.
  if [ -f "$ENV_FILE" ]; then
    for key in LETS_LANGUAGE LETS_MERGE_BRANCH LETS_PR_FLOW LETS_TRACKER; do
      val=$(grep -E "^${key}=" "$ENV_FILE" | head -1 | cut -d= -f2- | tr -d '\n\r' | head -c 200)
      [ -n "$val" ] && echo "${key}=${val}"
    done
  fi
fi
