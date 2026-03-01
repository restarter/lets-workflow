#!/bin/bash
# hooks/worktree-cleanup.sh
# WorktreeRemove hook - fire-and-forget cleanup
#
# Contract:
#   INPUT (stdin): JSON with "worktree_path" field
#   OUTPUT: fire-and-forget (failures logged, not fatal)

set -euo pipefail

# Debug log
MAIN_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")
DEBUG_LOG="${MAIN_ROOT}/.lets/cache/worktree-hook.log"
mkdir -p "$(dirname "$DEBUG_LOG")"
echo "$(date '+%H:%M:%S') REMOVE HOOK FIRED" >> "$DEBUG_LOG"

INPUT=$(cat)
echo "$(date '+%H:%M:%S') remove stdin: $INPUT" >> "$DEBUG_LOG"
WORKTREE_PATH=$(echo "$INPUT" | jq -r '.worktree_path // empty')

if [ -z "$WORKTREE_PATH" ]; then
  echo "[worktree-cleanup] WARNING: no worktree_path in input" >&2
  exit 0
fi

echo "[worktree-cleanup] path=$WORKTREE_PATH" >&2

if [ ! -d "$WORKTREE_PATH" ]; then
  echo "[worktree-cleanup] worktree already removed" >&2
  exit 0
fi

# Resolve main repo root from worktree
MAIN_ROOT=$(cd "$WORKTREE_PATH" && git rev-parse --git-common-dir 2>/dev/null | xargs dirname)

# Remove via bd (handles git worktree remove + beads redirect cleanup)
if command -v bd &>/dev/null && [ -d "${MAIN_ROOT}/.beads" ]; then
  echo "[worktree-cleanup] using bd worktree remove" >&2
  cd "$MAIN_ROOT"
  bd worktree remove "$WORKTREE_PATH" 2>&1 >&2 || {
    echo "[worktree-cleanup] bd failed, trying git" >&2
    git worktree remove "$WORKTREE_PATH" 2>&1 >&2 || true
  }
else
  echo "[worktree-cleanup] using git worktree remove" >&2
  git worktree remove "$WORKTREE_PATH" 2>&1 >&2 || true
fi

git worktree prune 2>/dev/null || true
echo "[worktree-cleanup] done" >&2
