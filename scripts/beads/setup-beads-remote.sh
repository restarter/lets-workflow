#!/bin/bash
# Beads Remote Setup - connects local beads to shared GitHub remote.
#
# Run after cloning the project:
#   scripts/beads/setup-beads-remote.sh
#
# What it does:
#   1. bd init (creates local dolt database with schema)
#   2. Adds GitHub repo as dolt remote
#   3. Fetches remote data and resets local to match
#
# Prerequisites:
#   - bd (beads CLI) installed
#   - dolt installed (brew install dolt)
#   - git credentials configured (gh auth or SSH keys)

set -e

# ============================================
# Config
# ============================================
REMOTE_URL="https://github.com/restarter/lets-workflow-beads.git"
PREFIX="lets"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PROJECT_DIR="$(pwd)"

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
echo -e "${YELLOW}[1/3] Initializing beads...${NC}"
bd dolt stop 2>/dev/null || true
rm -f .beads/metadata.json
rm -rf .beads/dolt
bd init --force --prefix "$PREFIX" 2>&1 | tail -5
bd dolt stop 2>/dev/null || true

# ============================================
# 2. Add remote + fetch + reset (via dolt directly)
# ============================================
echo -e "${YELLOW}[2/3] Connecting to remote...${NC}"

if [ ! -d "$DB_DIR/.dolt" ]; then
  echo -e "${RED}Error: Database not found at $DB_DIR after init${NC}"
  exit 1
fi

cd "$DB_DIR"
dolt remote add origin "$REMOTE_URL"
echo "  Remote added: $REMOTE_URL"

echo -e "${YELLOW}[3/3] Syncing data from remote...${NC}"
dolt fetch origin 2>&1 | tail -1
dolt reset --hard origin/main
cd "$PROJECT_DIR"

# ============================================
# Verify
# ============================================
echo ""
ISSUES=$(bd list --status=all 2>&1 | grep -c "${PREFIX}-" || true)
echo -e "${GREEN}=== Setup Complete ===${NC}"
echo ""
echo "  Issues synced: $ISSUES"
echo ""
echo "  Verify:  bd list"
echo "  Pull:    bd dolt pull"
echo "  Push:    bd dolt push"
echo ""
