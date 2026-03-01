#!/bin/bash
# hooks/validate-readonly.sh
# PreToolUse hook for read-only agents
# Blocks destructive commands while allowing read-only operations
#
# Exit codes:
#   0 - allowed
#   2 - blocked (Claude Code shows error to agent)

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

if [ -z "$COMMAND" ]; then
  exit 0
fi

# Block destructive commands
# bd: create/update/close/delete/worktree/init modify beads state
# git: commit/push/checkout/reset/merge/rebase/branch -d/clean modify repo state
# dolt: init/sql modify database
# file: rm/mv/cp/mkdir/chmod/chown modify filesystem
# editors: sed -i/tee/truncate modify files
# pkg: pip/npm/yarn/brew install modify system state
# net: curl|bash/wget execute remote code
if echo "$COMMAND" | grep -qiE '\b(bd (create|update|close|delete|worktree|init|reopen)|git (commit|push|checkout|reset|merge|rebase|branch -[dD]|stash (drop|clear)|clean)|dolt (init|sql)|rm (-rf?|--recursive)|rmdir|mv |cp |mkdir|chmod|chown|sed -i|tee |truncate|pip install|npm install|yarn add|brew install)\b'; then
  echo "BLOCKED: command not allowed for read-only agent" >&2
  exit 2
fi

# Block piped execution and wget (remote code execution)
if echo "$COMMAND" | grep -qiE 'curl.*\|.*bash|curl.*\|.*sh|wget'; then
  echo "BLOCKED: remote code execution not allowed for read-only agent" >&2
  exit 2
fi

# Block output redirection (file writes via shell)
if echo "$COMMAND" | grep -qE '>[^&]|>>'; then
  echo "BLOCKED: output redirection not allowed for read-only agent" >&2
  exit 2
fi

exit 0
