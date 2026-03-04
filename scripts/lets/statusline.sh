#!/bin/sh
# LETS-branded statusline for Claude Code
# Copied to .lets/statusline.sh by /lets:install
# Receives JSON from Claude Code via stdin, outputs formatted status lines.
#
# Layout:
#   Line 1: [LETS] feature/branch-name
#   Line 2: Opus 4.6 » ctx 38% (76k/200k) » 5h 0% · 7d 22%

input=$(cat)

# --- per-project cache in .lets/cache/ ---
dir=$(echo "$input" | jq -r '.workspace.current_dir // .cwd // ""')
PROJECT_ROOT=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)
if [ -z "$PROJECT_ROOT" ]; then
  PROJECT_ROOT="$dir"
fi
LETS_CACHE="${PROJECT_ROOT}/.lets/cache"
mkdir -p "$LETS_CACHE" 2>/dev/null

# --- background fetch function (inlined from fetch-usage.sh) ---
# Fetches Claude API usage stats and writes to .lets/cache/usage (per-project).
# Cache format (4 lines): 5h%, 7d%, 5h_resets_at, 7d_resets_at
_fetch_usage() {
  _fu_cache_file="${LETS_CACHE}/usage"
  _fu_creds_file="$HOME/.claude/.credentials.json"

  # Read token fresh each time (no caching - it's a credential)
  _fu_token=""

  # macOS: try Keychain first (newer Claude Code stores creds here)
  if [ "$(uname)" = "Darwin" ]; then
    _fu_keychain=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null)
    if [ -n "$_fu_keychain" ]; then
      _fu_token=$(printf '%s' "$_fu_keychain" | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null)
    fi
  fi

  # All platforms: fall back to credentials.json file
  if [ -z "$_fu_token" ] && [ -f "$_fu_creds_file" ]; then
    _fu_token=$(jq -r '.claudeAiOauth.accessToken // empty' "$_fu_creds_file" 2>/dev/null)
  fi

  if [ -z "$_fu_token" ]; then
    return
  fi

  _fu_json=$(curl -s -m 3 \
    -H "accept: application/json" \
    -H "anthropic-beta: oauth-2025-04-20" \
    -H "authorization: Bearer $_fu_token" \
    -H "user-agent: claude-code/2.1.11" \
    "https://api.anthropic.com/oauth/usage" 2>/dev/null)

  if [ -z "$_fu_json" ]; then
    return
  fi

  _fu_5h_raw=$(printf '%s' "$_fu_json" | jq -r '.five_hour.utilization // empty' 2>/dev/null)
  _fu_7d_raw=$(printf '%s' "$_fu_json" | jq -r '.seven_day.utilization // empty' 2>/dev/null)
  _fu_5h_reset=$(printf '%s' "$_fu_json" | jq -r '.five_hour.resets_at // ""' 2>/dev/null)
  _fu_7d_reset=$(printf '%s' "$_fu_json" | jq -r '.seven_day.resets_at // ""' 2>/dev/null)

  if [ -n "$_fu_5h_raw" ] && [ -n "$_fu_7d_raw" ]; then
    _fu_5h=$(printf "%.0f" "$_fu_5h_raw")
    _fu_7d=$(printf "%.0f" "$_fu_7d_raw")
    printf '%s\n%s\n%s\n%s\n' "$_fu_5h" "$_fu_7d" "$_fu_5h_reset" "$_fu_7d_reset" > "$_fu_cache_file"
  fi
}

# --- model ---
model=$(echo "$input" | jq -r '.model.display_name // ""')

# --- folder (reuse $dir from cache section above) ---
dir_name=$(basename "$dir")

# --- git branch ---
branch=""
if [ -d "${dir}/.git" ] || git -C "$dir" rev-parse --git-dir > /dev/null 2>&1; then
  branch=$(git -C "$dir" symbolic-ref --short HEAD 2>/dev/null || git -C "$dir" rev-parse --short HEAD 2>/dev/null)
fi

# --- usage stats (5h / 7d) from cache ---
CACHE_FILE="${LETS_CACHE}/usage"
five_h=""
seven_d=""
five_h_reset=""
seven_d_reset=""

CACHE_TTL=300  # refresh if older than 5 minutes
cache_fresh=0

if [ -f "$CACHE_FILE" ]; then
  if [ "$(uname)" = "Darwin" ]; then
    cache_age=$(( $(date -u +%s) - $(stat -f %m "$CACHE_FILE" 2>/dev/null || echo 0) ))
  else
    cache_age=$(( $(date -u +%s) - $(stat -c %Y "$CACHE_FILE" 2>/dev/null || echo 0) ))
  fi
  if [ "$cache_age" -lt "$CACHE_TTL" ]; then
    cache_fresh=1
  fi
  five_h=$(sed -n '1p' "$CACHE_FILE")
  seven_d=$(sed -n '2p' "$CACHE_FILE")
  five_h_reset=$(sed -n '3p' "$CACHE_FILE")
  seven_d_reset=$(sed -n '4p' "$CACHE_FILE")
  # Validate reset timestamps look like ISO dates
  case "$five_h_reset" in [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T*) ;; *) five_h_reset="" ;; esac
  case "$seven_d_reset" in [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T*) ;; *) seven_d_reset="" ;; esac
fi

# Background refresh if cache missing or stale
[ "$cache_fresh" -eq 0 ] && _fetch_usage > /dev/null 2>&1 &

# --- compute_delta: given a raw ISO timestamp, returns human-readable time until reset ---
compute_delta() {
  clean=$(echo "$1" | sed 's/\.[0-9]*//' | sed 's/[+-][0-9][0-9]:[0-9][0-9]$//' | sed 's/Z$//')
  if [ "$(uname)" = "Darwin" ]; then
    reset_epoch=$(TZ=UTC date -j -f "%Y-%m-%dT%H:%M:%S" "$clean" "+%s" 2>/dev/null)
  else
    reset_epoch=$(TZ=UTC date -d "$clean" "+%s" 2>/dev/null)
  fi
  if [ -z "$reset_epoch" ]; then return; fi
  now_epoch=$(date -u "+%s")
  diff=$(( reset_epoch - now_epoch ))
  if [ "$diff" -le 0 ]; then echo "now"; return; fi
  days=$(( diff / 86400 ))
  hours=$(( (diff % 86400) / 3600 ))
  minutes=$(( (diff % 3600) / 60 ))
  if [ "$days" -gt 0 ]; then
    echo "${days}d ${hours}h"
  elif [ "$hours" -gt 0 ]; then
    echo "${hours}h ${minutes}m"
  else
    echo "${minutes}m"
  fi
}

# --- context window ---
used=$(echo "$input" | jq -r '.context_window.used_percentage // empty')
ctx_str=""
ctx_tokens_str=""
if [ -n "$used" ]; then
  used_int=$(printf "%.0f" "$used")
  ctx_str="${used_int}%"
  ctx_used=$(echo "$input" | jq -r '(.context_window.current_usage.cache_read_input_tokens + .context_window.current_usage.cache_creation_input_tokens + .context_window.current_usage.input_tokens + .context_window.current_usage.output_tokens) // empty' 2>/dev/null)
  ctx_total=$(echo "$input" | jq -r '.context_window.context_window_size // empty' 2>/dev/null)
  if [ -n "$ctx_used" ] && [ -n "$ctx_total" ]; then
    ctx_used_k=$(( ctx_used / 1000 ))
    ctx_total_k=$(( ctx_total / 1000 ))
    ctx_tokens_str="${ctx_used_k}k/${ctx_total_k}k"
  fi
fi

# --- assemble output ---
SEP="\033[38;2;153;122;0m \xc2\xbb \033[0m"

# line 1: LETS » branch or folder
printf "\xf0\x9f\x8c\xb1 \033[1;38;2;255;215;0mLETS Workflow\033[0m"
printf "%b" "$SEP"
if [ -n "$branch" ]; then
  printf "\033[38;2;232;160;144m%s\033[0m" "$branch"
else
  printf "\033[38;2;232;160;144m%s\033[0m" "$dir_name"
fi

# line 2: model » ctx · usage
printf "\n"
printf "\033[1;38;2;255;175;50m%s\033[0m" "$model"
printf "%b" "$SEP"

# Context (hidden when unavailable)
if [ -n "$ctx_str" ]; then
  printf "\033[38;2;190;176;140mwindow %s\033[0m" "$ctx_str"
  [ -n "$ctx_tokens_str" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$ctx_tokens_str"
  printf "\033[90m \xc2\xb7 \033[0m"
fi

# Usage stats (color-coded: green <50%, yellow 50-80%, red >80%)
usage_color() {
  val="$1"
  case "$val" in
    [0-9]*) ;;
    *) printf "\033[38;2;130;200;130m"; return ;;
  esac
  if [ "$val" -ge 80 ]; then
    printf "\033[38;2;255;100;100m"
  elif [ "$val" -ge 50 ]; then
    printf "\033[38;2;255;200;80m"
  else
    printf "\033[38;2;130;200;130m"
  fi
}

if [ -n "$five_h" ]; then
  printf "%b5h %s%%\033[0m" "$(usage_color "$five_h")" "$five_h"
  if [ -n "$five_h_reset" ]; then
    delta=$(compute_delta "$five_h_reset")
    [ -n "$delta" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$delta"
  fi
  [ -n "$seven_d" ] && printf "\033[90m \xc2\xb7 \033[0m"
fi
if [ -n "$seven_d" ]; then
  printf "%b7d %s%%\033[0m" "$(usage_color "$seven_d")" "$seven_d"
  if [ -n "$seven_d_reset" ]; then
    delta=$(compute_delta "$seven_d_reset")
    [ -n "$delta" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$delta"
  fi
fi
