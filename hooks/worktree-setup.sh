#!/bin/bash
# hooks/worktree-setup.sh
# WorktreeCreate hook - owns the full worktree lifecycle
#
# Contract:
#   INPUT (stdin):  JSON with "name" field
#   OUTPUT (stdout): absolute path to created worktree
#   If registered, Claude Code skips built-in git worktree add.
#
# Worktrees are stored in .lets/worktrees/ (already gitignored via .lets/).
# Only .beads/ redirect is set up - agents don't need .lets/ access.

set -euo pipefail

# Resolve main repo root
MAIN_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$MAIN_ROOT" ]; then
  echo "ERROR: not in a git repo" >&2
  exit 1
fi

# Debug log
DEBUG_LOG="${MAIN_ROOT}/.lets/cache/worktree-hook.log"
mkdir -p "$(dirname "$DEBUG_LOG")"
echo "$(date '+%H:%M:%S') HOOK FIRED" >> "$DEBUG_LOG"

INPUT=$(cat)
echo "$(date '+%H:%M:%S') stdin: $INPUT" >> "$DEBUG_LOG"
NAME=$(echo "$INPUT" | jq -r '.name // empty')

if [ -z "$NAME" ]; then
  echo "ERROR: no worktree name in hook input" >&2
  exit 1
fi

echo "[worktree-setup] name=$NAME" >&2

WORKTREE_DIR="${MAIN_ROOT}/.lets/worktrees"
WORKTREE_PATH="${WORKTREE_DIR}/${NAME}"
BRANCH_NAME="worktree-${NAME}"

echo "[worktree-setup] main_root=$MAIN_ROOT" >&2
echo "[worktree-setup] worktree_path=$WORKTREE_PATH" >&2

mkdir -p "$WORKTREE_DIR"

# Create worktree via bd (handles git worktree add + beads redirect)
cd "$MAIN_ROOT"
if command -v bd &>/dev/null && [ -d "${MAIN_ROOT}/.beads" ]; then
  echo "[worktree-setup] using bd worktree create" >&2
  bd worktree create "$WORKTREE_PATH" --branch "$BRANCH_NAME" 2>&1 >&2
else
  echo "[worktree-setup] bd not available, using git worktree add" >&2
  git worktree add -b "$BRANCH_NAME" "$WORKTREE_PATH" 2>&1 >&2
fi

echo "[worktree-setup] done" >&2

# MUST print the worktree path
echo "$WORKTREE_PATH"
