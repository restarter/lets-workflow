#!/bin/bash
# DEPRECATED: For Direct SQL mode (recommended), edit .beads/metadata.json directly.
# See scripts/dolt/README.md for instructions.
# This script sets up GitHub remote push/pull mode - kept for reference.
#
# Beads Remote Setup - connects local beads to shared GitHub remote.
#
# Run after cloning the project (legacy push/pull mode):
#   scripts/deprecated/beads/setup-beads-remote.sh
#
# What it does:
#   1. Inits dolt database, starts sql-server, runs bd init
#   2. Detects git protocol (SSH vs HTTPS)
#   3. Adds GitHub repo as dolt remote
#   4. Fetches remote data and resets local to match
#
# Prerequisites:
#   - bd (beads CLI) installed
#   - dolt installed (brew install dolt)
#   - git credentials configured (gh auth or SSH keys)

set -e

# ============================================
# Config
# ============================================
GITHUB_REPO="restarter/lets-workflow-beads"
PREFIX="lets"
DOLT_PORT=3307

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PROJECT_DIR="$(pwd)"

# ============================================
# Helpers
# ============================================

# Detect git protocol and build remote URL accordingly.
# Priority: gh auth protocol > SSH test > fallback to HTTPS.
detect_remote_url() {
  local repo="$1"

  # 1. Check gh auth protocol setting
  if command -v gh &>/dev/null; then
    local proto
    proto=$(gh config get git_protocol 2>/dev/null || true)
    if [ "$proto" = "ssh" ]; then
      echo "git@github.com:${repo}.git"
      return
    elif [ "$proto" = "https" ]; then
      echo "https://github.com/${repo}.git"
      return
    fi
  fi

  # 2. Test SSH connectivity
  if ssh -o BatchMode=yes -o ConnectTimeout=5 -T git@github.com 2>&1 | grep -q "successfully authenticated"; then
    echo "git@github.com:${repo}.git"
    return
  fi

  # 3. Fallback to HTTPS
  echo "https://github.com/${repo}.git"
}

# Start dolt sql-server if not already running on DOLT_PORT.
ensure_dolt_server() {
  if lsof -iTCP:${DOLT_PORT} -sTCP:LISTEN &>/dev/null; then
    echo -e "  ${YELLOW}Warning: port ${DOLT_PORT} already in use, assuming dolt server${NC}"
    return 0
  fi

  echo "  Starting dolt sql-server on port ${DOLT_PORT}..."
  cd "$DB_DIR"
  dolt sql-server --port "$DOLT_PORT" --host 127.0.0.1 &>/dev/null &
  DOLT_SERVER_PID=$!
  cd "$PROJECT_DIR"
  sleep 2

  if ! kill -0 "$DOLT_SERVER_PID" 2>/dev/null; then
    echo -e "${RED}Error: dolt sql-server failed to start${NC}"
    exit 1
  fi
}

# Stop dolt sql-server if we started it.
stop_dolt_server() {
  if [ -n "$DOLT_SERVER_PID" ]; then
    kill "$DOLT_SERVER_PID" 2>/dev/null || true
    wait "$DOLT_SERVER_PID" 2>/dev/null || true
    DOLT_SERVER_PID=""
  fi
}

trap 'stop_dolt_server' EXIT

# ============================================
# Checks
# ============================================
if ! command -v bd &>/dev/null; then
  echo -e "${RED}Error: bd not found. Install beads first.${NC}"
  exit 1
fi

if ! command -v dolt &>/dev/null; then
  echo -e "${RED}Error: dolt not found. Install: brew install dolt${NC}"
  exit 1
fi

if [ ! -d ".beads" ]; then
  echo -e "${RED}Error: .beads/ not found. Run from project root.${NC}"
  exit 1
fi

# Check if already set up (remote exists and data is synced)
DB_DIR=".beads/dolt/$PREFIX"
if [ -d "$DB_DIR/.dolt" ]; then
  cd "$DB_DIR"
  if dolt remote -v 2>/dev/null | grep -q "origin"; then
    echo -e "${YELLOW}Already set up. Remote:${NC}"
    dolt remote -v
    echo ""
    echo "To sync: bd dolt pull / bd dolt push"
    cd "$PROJECT_DIR"
    exit 0
  fi
  cd "$PROJECT_DIR"
fi

echo -e "${GREEN}=== Beads Remote Setup ===${NC}"
echo ""

# ============================================
# 1. Init beads (fresh local database)
# ============================================
echo -e "${YELLOW}[1/4] Initializing beads...${NC}"
stop_dolt_server
rm -f .beads/metadata.json
rm -rf .beads/dolt

# dolt init creates the database directory that bd init expects
mkdir -p "$DB_DIR"
cd "$DB_DIR"
dolt init --name "beads" --email "beads@local" >/dev/null
cd "$PROJECT_DIR"

# bd init needs a running dolt sql-server
ensure_dolt_server
bd init --force --prefix "$PREFIX" 2>&1 | tail -5
stop_dolt_server

# ============================================
# 2. Add remote + fetch + reset (via dolt directly)
# ============================================
echo -e "${YELLOW}[2/4] Detecting git protocol...${NC}"

if [ ! -d "$DB_DIR/.dolt" ]; then
  echo -e "${RED}Error: Database not found at $DB_DIR after init${NC}"
  exit 1
fi

REMOTE_URL=$(detect_remote_url "$GITHUB_REPO")
echo "  Using: $REMOTE_URL"

echo -e "${YELLOW}[3/4] Connecting to remote...${NC}"
cd "$DB_DIR"
dolt remote add origin "$REMOTE_URL"

echo -e "${YELLOW}[4/4] Syncing data from remote...${NC}"
dolt fetch origin 2>&1 | tail -1
dolt reset --hard origin/main
cd "$PROJECT_DIR"

# ============================================
# Verify
# ============================================
echo ""
ensure_dolt_server
ISSUES=$(bd list --status=all 2>&1 | grep -c "${PREFIX}-" || true)
stop_dolt_server

echo -e "${GREEN}=== Setup Complete ===${NC}"
echo ""
echo "  Remote:  $REMOTE_URL"
echo "  Issues synced: $ISSUES"
echo ""
echo "  Verify:  bd list"
echo "  Pull:    bd dolt pull"
echo "  Push:    bd dolt push"
echo ""
