#!/bin/bash
# LETS plugin - SessionStart hook
# Injects workflow rules + project config as context

# Always inject workflow rules
cat "${CLAUDE_PLUGIN_ROOT}/hooks/rules-context.md"

# Inject project context
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)

if [ -n "$PROJECT_ROOT" ]; then
  CONFIG_FILE="${PROJECT_ROOT}/.lets/config.yaml"

  echo ""
  echo "## LETS Config"
  echo ""
  echo "project-root: $PROJECT_ROOT"

  if [ -f "$CONFIG_FILE" ]; then
    cat "$CONFIG_FILE"
  fi
fi
