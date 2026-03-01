#!/bin/sh
# Fetches Claude API usage stats and writes to .lets/cache/usage.
# Usage: fetch-usage.sh <cache-dir>
# Line 1: five_hour.utilization (integer %)
# Line 2: seven_day.utilization (integer %)
# Line 3: five_hour.resets_at (raw ISO string)
# Line 4: seven_day.resets_at (raw ISO string)
# All output is suppressed; meant to be run in background.

CACHE_DIR="${1:?Usage: fetch-usage.sh <cache-dir>}"
mkdir -p "$CACHE_DIR" 2>/dev/null
CACHE_FILE="${CACHE_DIR}/usage"
TOKEN_CACHE="${CACHE_DIR}/token"
CREDS_FILE="$HOME/.claude/.credentials.json"
TOKEN_TTL=900  # 15 minutes

# --- get token (with 15-min cache) ---
# Try: 1) token cache, 2) macOS Keychain, 3) credentials.json file
token=""
if [ -f "$TOKEN_CACHE" ]; then
  if [ "$(uname)" = "Darwin" ]; then
    file_mtime=$(stat -f %m "$TOKEN_CACHE" 2>/dev/null || echo 0)
  else
    file_mtime=$(stat -c %Y "$TOKEN_CACHE" 2>/dev/null || echo 0)
  fi
  cache_age=$(( $(date -u +%s) - file_mtime ))
  if [ "$cache_age" -lt "$TOKEN_TTL" ]; then
    token=$(cat "$TOKEN_CACHE" 2>/dev/null)
  fi
fi

if [ -z "$token" ]; then
  # macOS: try Keychain first (newer Claude Code stores creds here)
  if [ "$(uname)" = "Darwin" ]; then
    keychain_json=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null)
    if [ -n "$keychain_json" ]; then
      token=$(printf '%s' "$keychain_json" | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null)
    fi
  fi

  # All platforms: fall back to credentials.json file
  if [ -z "$token" ] && [ -f "$CREDS_FILE" ]; then
    token=$(jq -r '.claudeAiOauth.accessToken // empty' "$CREDS_FILE" 2>/dev/null)
  fi

  if [ -z "$token" ]; then
    exit 0
  fi
  (umask 077; printf '%s' "$token" > "$TOKEN_CACHE")
fi

usage_json=$(curl -s -m 3 \
  -H "accept: application/json" \
  -H "anthropic-beta: oauth-2025-04-20" \
  -H "authorization: Bearer $token" \
  -H "user-agent: claude-code/2.1.11" \
  "https://api.anthropic.com/oauth/usage" 2>/dev/null)

if [ -z "$usage_json" ]; then
  exit 0
fi

five_h_raw=$(printf '%s' "$usage_json" | jq -r '.five_hour.utilization // empty' 2>/dev/null)
seven_d_raw=$(printf '%s' "$usage_json" | jq -r '.seven_day.utilization // empty' 2>/dev/null)
five_h_reset=$(printf '%s' "$usage_json" | jq -r '.five_hour.resets_at // ""' 2>/dev/null)
seven_d_reset=$(printf '%s' "$usage_json" | jq -r '.seven_day.resets_at // ""' 2>/dev/null)

if [ -n "$five_h_raw" ] && [ -n "$seven_d_raw" ]; then
  five_h=$(printf "%.0f" "$five_h_raw")
  seven_d=$(printf "%.0f" "$seven_d_raw")
  printf '%s\n%s\n%s\n%s\n' "$five_h" "$seven_d" "$five_h_reset" "$seven_d_reset" > "$CACHE_FILE"
fi
