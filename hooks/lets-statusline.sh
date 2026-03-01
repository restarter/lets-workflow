#!/bin/sh
# LETS-branded statusline
# Receives JSON from Claude Code via stdin, outputs formatted status lines.
#
# Layout:
#   Line 1: [LETS] feature/branch-name
#   Line 2: Opus 4.6 │ ctx 38% (76k/200k) │ 5h 0% · 7d 22%

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
input=$(cat)

# --- model ---
model=$(echo "$input" | jq -r '.model.display_name // ""')

# --- folder ---
dir=$(echo "$input" | jq -r '.workspace.current_dir // .cwd // ""')
dir_name=$(basename "$dir")

# --- git branch ---
branch=""
if [ -d "${dir}/.git" ] || git -C "$dir" rev-parse --git-dir > /dev/null 2>&1; then
  branch=$(git -C "$dir" symbolic-ref --short HEAD 2>/dev/null || git -C "$dir" rev-parse --short HEAD 2>/dev/null)
fi

# --- usage stats (5h / 7d) from cache ---
CACHE_FILE="/tmp/.claude_usage_cache"
five_h=""
seven_d=""
five_h_reset=""
seven_d_reset=""

if [ -f "$CACHE_FILE" ]; then
  five_h=$(sed -n '1p' "$CACHE_FILE")
  seven_d=$(sed -n '2p' "$CACHE_FILE")
  five_h_reset=$(sed -n '3p' "$CACHE_FILE")
  seven_d_reset=$(sed -n '4p' "$CACHE_FILE")
else
  bash "$SCRIPT_DIR/fetch-usage.sh" > /dev/null 2>&1 &
fi

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

# line 1: LETS › branch or folder
printf "\033[1;38;2;255;215;0mLETS Workflow\033[0m"
printf "%b" "$SEP"
if [ -n "$branch" ]; then
  printf "\033[38;2;232;160;144m%s\033[0m" "$branch"
else
  printf "\033[38;2;232;160;144m%s\033[0m" "$dir_name"
fi

# line 2: model › ctx › usage
printf "\n"
printf "\033[1;38;2;255;175;50m%s\033[0m" "$model"

# Context
if [ -n "$ctx_str" ]; then
  printf "%b" "$SEP"
  printf "\033[38;2;190;176;140mwindow %s\033[0m" "$ctx_str"
  [ -n "$ctx_tokens_str" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$ctx_tokens_str"
fi

# Usage stats
if [ -n "$five_h" ]; then
  printf "\033[90m \xc2\xb7 \033[0m"
  printf "\033[38;2;190;176;140m5h %s%%\033[0m" "$five_h"
  if [ -n "$five_h_reset" ]; then
    delta=$(compute_delta "$five_h_reset")
    [ -n "$delta" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$delta"
  fi
fi
if [ -n "$seven_d" ]; then
  printf "\033[90m \xc2\xb7 \033[0m"
  printf "\033[38;2;190;176;140m7d %s%%\033[0m" "$seven_d"
  if [ -n "$seven_d_reset" ]; then
    delta=$(compute_delta "$seven_d_reset")
    [ -n "$delta" ] && printf " \033[2;38;2;190;176;140m(%s)\033[0m" "$delta"
  fi
fi
